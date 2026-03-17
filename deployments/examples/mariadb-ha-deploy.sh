#!/usr/bin/env bash
set -euo pipefail

# Deploy MariaDB Galera HA in k3s.
# Prereqs:
#   - helm installed
#   - namespace + secret prepared
#   - storage class 'longhorn-ha' exists (or edit values file)

NAMESPACE="${NAMESPACE:-mariadb}"
VALUES_FILE="${VALUES_FILE:-/home/oiviadesu/git/Oiviak3s/deployments/examples/mariadb-ha-values.yaml}"
SECRET_FILE="${SECRET_FILE:-/home/oiviadesu/git/Oiviak3s/deployments/examples/mariadb-ha-secret.yaml}"
RELEASE_NAME="${RELEASE_NAME:-mariadb-ha}"
VERBOSE="${VERBOSE:-false}"
HELM_TIMEOUT="${HELM_TIMEOUT:-6m}"
ENGINE_BINARIES_BIND_SCRIPT="${ENGINE_BINARIES_BIND_SCRIPT:-/home/oiviadesu/git/Oiviak3s/deployments/examples/longhorn/apply-engine-binaries-bind.sh}"

is_verbose=false
case "${VERBOSE}" in
  1|true|TRUE|yes|YES|on|ON)
    is_verbose=true
    ;;
esac

if [[ "${is_verbose}" == "true" ]]; then
  set -x
fi

dump_diagnostics() {
  set +e
  echo
  echo "===== Diagnostics (${NAMESPACE}) ====="
  kubectl -n "${NAMESPACE}" get pods,svc,pvc,sts -o wide
  echo
  kubectl -n "${NAMESPACE}" get events --sort-by=.metadata.creationTimestamp | tail -n 120
  echo
  kubectl -n "${NAMESPACE}" describe sts "${RELEASE_NAME}"
  echo
  kubectl -n "${NAMESPACE}" describe pod "${RELEASE_NAME}-0"
  echo
  kubectl -n "${NAMESPACE}" logs "${RELEASE_NAME}-0" -c mariadb-galera --tail=200
}

if [[ ! -f "${VALUES_FILE}" ]]; then
  echo "Values file not found: ${VALUES_FILE}" >&2
  exit 1
fi

if [[ ! -f "${SECRET_FILE}" ]]; then
  cat <<EOF >&2
Secret file not found: ${SECRET_FILE}
Copy template and edit secrets first:
  cp /home/oiviadesu/git/Oiviak3s/deployments/examples/mariadb-ha-secret.yaml.example ${SECRET_FILE}
EOF
  exit 1
fi

if [[ -x "${ENGINE_BINARIES_BIND_SCRIPT}" ]]; then
  echo "[0/5] Applying Longhorn engine binaries bind mount"
  "${ENGINE_BINARIES_BIND_SCRIPT}"
fi

echo "[1/5] Creating namespace ${NAMESPACE}"
kubectl create namespace "${NAMESPACE}" --dry-run=client -o yaml | kubectl apply -f -

echo "[2/5] Applying MariaDB HA secret"
kubectl apply -f "${SECRET_FILE}"

echo "[3/5] Adding/refreshing Bitnami chart repo"
helm repo add bitnami https://charts.bitnami.com/bitnami >/dev/null 2>&1 || true
helm repo update >/dev/null

echo "[4/5] Installing/Upgrading MariaDB Galera release: ${RELEASE_NAME}"
echo "    Helm timeout: ${HELM_TIMEOUT}"
helm_args=(
  upgrade
  --install
  "${RELEASE_NAME}"
  bitnami/mariadb-galera
  --namespace
  "${NAMESPACE}"
  -f
  "${VALUES_FILE}"
  --wait
  --timeout
  "${HELM_TIMEOUT}"
)

if [[ "${is_verbose}" == "true" ]]; then
  helm_args+=(--debug)
fi

if ! helm "${helm_args[@]}"; then
  echo "Helm install/upgrade failed. Collecting diagnostics..." >&2
  dump_diagnostics
  exit 1
fi

echo "[5/5] Current status"
kubectl -n "${NAMESPACE}" get pods,svc,pvc

echo
cat <<'EOF'
Done.

Writer endpoint (inside cluster):
  mariadb-ha.mariadb.svc.cluster.local:3306

Check Galera health:
  kubectl -n mariadb exec -it sts/mariadb-ha -- mysql -uroot -p -e "SHOW STATUS LIKE 'wsrep_cluster_size';"
EOF
