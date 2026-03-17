#!/usr/bin/env bash
set -euo pipefail

# Autoscaling deployment helper for Oiviak3s.
# SOLID-ish shell structure:
# - Single responsibility per function
# - Open/closed via env flags
# - Explicit preflight + typed exit paths

ROOT_DIR="${ROOT_DIR:-/home/oiviadesu/git/Oiviak3s/deployments/examples/autoscaling}"
ENABLE_KEDA_INSTALL="${ENABLE_KEDA_INSTALL:-true}"
ENABLE_KEDA_TEMPLATES="${ENABLE_KEDA_TEMPLATES:-false}"
ENABLE_PVC_AUTOGROW="${ENABLE_PVC_AUTOGROW:-true}"
PROM_NAMESPACE="${PROM_NAMESPACE:-monitoring}"
PROM_SERVICE="${PROM_SERVICE:-prometheus-server}"
QUEUE_KEY="${QUEUE_KEY:-CHANGE_ME_QUEUE_KEY}"
PROM_SERVER_ADDRESS="${PROM_SERVER_ADDRESS:-}"

log() { echo "[autoscaling] $*"; }
warn() { echo "[autoscaling][WARN] $*" >&2; }
fail() { echo "[autoscaling][ERROR] $*" >&2; exit 1; }

require_command() {
  command -v "$1" >/dev/null 2>&1 || fail "Missing required command: $1"
}

check_cluster_access() {
  kubectl version --request-timeout=10s >/dev/null 2>&1 || fail "kubectl cannot reach cluster"
}

check_metrics_server() {
  kubectl -n kube-system get deploy metrics-server >/dev/null 2>&1 || fail "metrics-server not found; HPA CPU/RAM metrics unavailable"
}

install_keda() {
  log "Installing/upgrading KEDA"
  helm repo add kedacore https://kedacore.github.io/charts >/dev/null 2>&1 || true
  helm repo update >/dev/null
  kubectl create namespace keda --dry-run=client -o yaml | kubectl apply -f - >/dev/null
  helm upgrade --install keda kedacore/keda \
    --namespace keda \
    --wait --timeout 15m >/dev/null
}

keda_api_available() {
  kubectl get crd scaledobjects.keda.sh >/dev/null 2>&1
}

prometheus_available() {
  kubectl -n "${PROM_NAMESPACE}" get svc "${PROM_SERVICE}" >/dev/null 2>&1
}

resolve_prometheus_server_address() {
  if [[ -n "${PROM_SERVER_ADDRESS}" ]]; then
    echo "${PROM_SERVER_ADDRESS}"
    return
  fi

  local ip
  ip="$(kubectl -n "${PROM_NAMESPACE}" get svc "${PROM_SERVICE}" -o jsonpath='{.spec.clusterIP}' 2>/dev/null || true)"
  [[ -n "${ip}" ]] || fail "Unable to resolve Prometheus service ClusterIP"
  echo "http://${ip}"
}

apply_hpa_resources() {
  log "Applying CPU/RAM HPA resources"
  kubectl apply -f "${ROOT_DIR}/hpa-immich-server.yaml"
  kubectl apply -f "${ROOT_DIR}/hpa-worldlinkd.yaml"
  kubectl apply -f "${ROOT_DIR}/hpa-kubernetes-dashboard-gateway.yaml"
}

apply_keda_templates() {
  [[ "${ENABLE_KEDA_TEMPLATES}" == "true" ]] || {
    log "Skipping KEDA template apply (ENABLE_KEDA_TEMPLATES=false)"
    return
  }

  keda_api_available || fail "KEDA API (ScaledObject) is not available"

  if prometheus_available; then
    if kubectl -n worldlinkd get hpa worldlinkd >/dev/null 2>&1; then
      warn "Detected native HPA worldlinkd/worldlinkd; skipping worldlinkd-qps ScaledObject to avoid HPA selector conflicts"
    else
      local prom_addr
      prom_addr="$(resolve_prometheus_server_address)"
      log "Using Prometheus address: ${prom_addr}"
      log "Applying KEDA QPS template (Prometheus detected)"
      sed "s#http://prometheus-server.monitoring.svc.cluster.local#${prom_addr}#g" \
        "${ROOT_DIR}/keda-scaledobject-qps-template.yaml" | kubectl apply -f -
    fi

    if kubectl -n immich get hpa immich-server >/dev/null 2>&1; then
      warn "Detected native HPA immich/immich-server; skipping immich-node-disk-pressure ScaledObject to avoid HPA selector conflicts"
    else
      local prom_addr_disk
      prom_addr_disk="$(resolve_prometheus_server_address)"
      log "Using Prometheus address: ${prom_addr_disk}"
      log "Applying KEDA node-disk template (Prometheus detected)"
      sed "s#http://prometheus-server.monitoring.svc.cluster.local#${prom_addr_disk}#g" \
        "${ROOT_DIR}/keda-scaledobject-node-disk-template.yaml" | kubectl apply -f -
    fi
  else
    warn "Prometheus service ${PROM_NAMESPACE}/${PROM_SERVICE} not found; skipping QPS/node-disk templates"
  fi

  if [[ "${QUEUE_KEY}" == "CHANGE_ME_QUEUE_KEY" ]]; then
    warn "QUEUE_KEY not configured; skipping queue template"
  else
    log "Applying queue-based KEDA template"
    sed "s/CHANGE_ME_QUEUE_KEY/${QUEUE_KEY}/g" "${ROOT_DIR}/keda-scaledobject-queue-template.yaml" | kubectl apply -f -
  fi
}

apply_storage_classes() {
  if [[ -f "${ROOT_DIR}/longhorn-autogrow-storageclass.yaml" ]]; then
    log "Applying longhorn-autogrow StorageClass"
    kubectl apply -f "${ROOT_DIR}/longhorn-autogrow-storageclass.yaml"
  else
    warn "longhorn-autogrow StorageClass manifest not found at ${ROOT_DIR}/longhorn-autogrow-storageclass.yaml"
  fi
}

apply_pvc_autogrow() {
  [[ "${ENABLE_PVC_AUTOGROW}" == "true" ]] || {
    log "Skipping PVC autogrow install (ENABLE_PVC_AUTOGROW=false)"
    return
  }

  prometheus_available || {
    warn "Prometheus service ${PROM_NAMESPACE}/${PROM_SERVICE} not found; skipping PVC autogrow"
    return
  }

  local prom_addr
  prom_addr="$(resolve_prometheus_server_address)"
  log "Applying PVC autogrow CronJob (Prometheus: ${prom_addr})"
  sed "s#http://prometheus-server.monitoring.svc.cluster.local#${prom_addr}#g" \
    "${ROOT_DIR}/pvc-autogrow-cronjob.yaml" | kubectl apply -f -
}

print_status() {
  log "Current autoscaling resources"
  kubectl get hpa -A || true
  if keda_api_available; then
    kubectl get scaledobject -A || true
  fi
  kubectl -n oiviak3s-system get cronjob pvc-autogrow >/dev/null 2>&1 && \
    kubectl -n oiviak3s-system get cronjob pvc-autogrow || true

  cat <<'EOF'

NOTE on node/server autoscaling:
- This cluster is bare-metal k3s (no cloud node group API discovered).
- Pod autoscaling (instances) is fully automatic with HPA/KEDA.
- Physical server count autoscaling requires external infrastructure automation (e.g. MAAS/Cluster-API/Talos APIs).

NOTE on PVC autoscaling:
- The PVC autogrow CronJob expands only PVCs explicitly annotated for opt-in.
- It requires Prometheus volume metrics and a StorageClass that supports expansion.
- The current default StorageClass `local-path` is intentionally skipped because Rancher local-path ignores volume capacity limits and is not a true quota-backed auto-resize target.
EOF
}

main() {
  require_command kubectl
  require_command helm

  check_cluster_access
  check_metrics_server

  apply_hpa_resources

  if [[ "${ENABLE_KEDA_INSTALL}" == "true" ]]; then
    install_keda
  else
    log "Skipping KEDA install (ENABLE_KEDA_INSTALL=false)"
  fi

  apply_storage_classes
  apply_keda_templates
  apply_pvc_autogrow
  print_status
}

main "$@"
