#!/usr/bin/env bash
set -euo pipefail

# Control-plane failover/failback helper for single-primary k3s setups.
#
# What this script does:
#   - failover: promote preferred standby to k3s server (mac mini first, then Pi)
#   - failback: when x509 comes back, re-promote x509 and demote standby to agent
#
# IMPORTANT:
# - k3s does not support instant in-place "agent -> server" promotion without reinstall steps.
# - This script automates those steps over SSH.
# - Run from a host that can SSH to all nodes and has sudo rights on remote nodes.
#
# Usage:
#   # Promote standby if primary (x509) is down
#   bash deployments/examples/control-plane-failover.sh failover
#
#   # Return control-plane role to x509 when it is healthy again
#   bash deployments/examples/control-plane-failover.sh failback
#
# Optional env overrides:
#   PRIMARY_HOST=192.168.86.8
#   MAC_HOST=192.168.86.41
#   PI_HOST=192.168.86.40
#   SSH_USER=oiviadesu
#   K3S_TOKEN='...'
#   PREFER_HOSTS='macmini pi'
#
# Simulation mode (for dry testing without touching hosts):
#   SIMULATE=1 PRIMARY_UP=0 MAC_UP=1 PI_UP=1 \
#   MAC_SERVER_ACTIVE=0 PI_SERVER_ACTIVE=0 \
#   bash deployments/examples/control-plane-failover.sh failover

MODE="${1:-}"
if [[ -z "${MODE}" || ("${MODE}" != "failover" && "${MODE}" != "failback") ]]; then
  echo "Usage: $0 <failover|failback>"
  exit 1
fi

# Auto-load project .env if present
SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd -- "${SCRIPT_DIR}/../.." && pwd)"
ENV_FILE="${ENV_FILE:-${ROOT_DIR}/.env}"
if [[ -f "${ENV_FILE}" ]]; then
  # shellcheck disable=SC1090
  source "${ENV_FILE}"
fi

SSH_USER="${SSH_USER:-oiviadesu}"
PRIMARY_HOST="${PRIMARY_HOST:-${x509fj:-oiviax509fj-master}}"
MAC_HOST="${MAC_HOST:-${macmini:-${mac:-oiviamacmini-worker}}}"
PI_HOST="${PI_HOST:-${pi:-oiviapi-worker}}"

# Preference order for standby promotion
PREFER_HOSTS="${PREFER_HOSTS:-macmini pi}"

# Optional static token (if empty, script reads from active server over SSH)
K3S_TOKEN="${K3S_TOKEN:-}"

SIMULATE="${SIMULATE:-0}"
PRIMARY_UP="${PRIMARY_UP:-1}"
MAC_UP="${MAC_UP:-1}"
PI_UP="${PI_UP:-1}"
MAC_SERVER_ACTIVE="${MAC_SERVER_ACTIVE:-0}"
PI_SERVER_ACTIVE="${PI_SERVER_ACTIVE:-0}"
SIM_TOKEN="${SIM_TOKEN:-SIMULATED-K3S-TOKEN}"

log() { echo "[$(date +%H:%M:%S)] $*"; }

ssh_run() {
  local host="$1"
  shift
  if [[ "$SIMULATE" == "1" ]]; then
    log "[SIM] ssh ${SSH_USER}@${host} $*"
    return 0
  fi
  ssh -o BatchMode=yes -o ConnectTimeout=5 "${SSH_USER}@${host}" "$@"
}

host_up() {
  local host="$1"
  if [[ "$SIMULATE" == "1" ]]; then
    case "$host" in
      "$PRIMARY_HOST") [[ "$PRIMARY_UP" == "1" ]] ; return ;;
      "$MAC_HOST") [[ "$MAC_UP" == "1" ]] ; return ;;
      "$PI_HOST") [[ "$PI_UP" == "1" ]] ; return ;;
      *) return 1 ;;
    esac
  fi
  ssh_run "$host" "echo ok" >/dev/null 2>&1
}

service_active() {
  local host="$1"
  local svc="$2"
  if [[ "$SIMULATE" == "1" ]]; then
    if [[ "$svc" == "k3s" ]]; then
      case "$host" in
        "$PRIMARY_HOST") [[ "$PRIMARY_UP" == "1" ]] ; return ;;
        "$MAC_HOST") [[ "$MAC_SERVER_ACTIVE" == "1" ]] ; return ;;
        "$PI_HOST") [[ "$PI_SERVER_ACTIVE" == "1" ]] ; return ;;
      esac
    fi
    return 1
  fi
  ssh_run "$host" "systemctl is-active ${svc}" 2>/dev/null | grep -q '^active$'
}

get_ip() {
  local host="$1"
  if [[ "$SIMULATE" == "1" ]]; then
    case "$host" in
      "$PRIMARY_HOST") echo "192.168.86.8" ;;
      "$MAC_HOST") echo "192.168.86.41" ;;
      "$PI_HOST") echo "192.168.86.40" ;;
      *) echo "127.0.0.1" ;;
    esac
    return 0
  fi
  ssh_run "$host" "hostname -I | awk '{print \$1}'"
}

read_token_from_server() {
  local host="$1"
  if [[ "$SIMULATE" == "1" ]]; then
    echo "$SIM_TOKEN"
    return 0
  fi
  ssh_run "$host" "sudo cat /var/lib/rancher/k3s/server/token"
}

promote_host_to_server() {
  local host="$1"
  local primary_url="$2"
  local token="$3"

  log "Promoting ${host} to k3s server"
  ssh_run "$host" "sudo systemctl stop k3s-agent || true"
  ssh_run "$host" "sudo systemctl disable k3s-agent || true"

  # If primary_url is empty, bootstrap standalone server (last resort).
  if [[ -z "$primary_url" ]]; then
    ssh_run "$host" "curl -sfL https://get.k3s.io | sudo INSTALL_K3S_EXEC='server --cluster-init' sh -"
  else
    ssh_run "$host" "curl -sfL https://get.k3s.io | sudo K3S_URL='${primary_url}' K3S_TOKEN='${token}' INSTALL_K3S_EXEC='server --server ${primary_url}' sh -"
  fi

  ssh_run "$host" "sudo systemctl enable k3s && sudo systemctl restart k3s"
}

demote_host_to_agent() {
  local host="$1"
  local server_url="$2"
  local token="$3"

  log "Demoting ${host} to k3s agent"
  ssh_run "$host" "sudo systemctl stop k3s || true"
  ssh_run "$host" "sudo systemctl disable k3s || true"
  ssh_run "$host" "sudo /usr/local/bin/k3s-uninstall.sh || true"

  ssh_run "$host" "curl -sfL https://get.k3s.io | sudo K3S_URL='${server_url}' K3S_TOKEN='${token}' sh -"
  ssh_run "$host" "sudo systemctl enable k3s-agent && sudo systemctl restart k3s-agent"
}

select_standby_host() {
  for id in ${PREFER_HOSTS}; do
    case "$id" in
      mac|macmini)
        if host_up "$MAC_HOST"; then
          echo "$MAC_HOST"
          return 0
        fi
        ;;
      pi)
        if host_up "$PI_HOST"; then
          echo "$PI_HOST"
          return 0
        fi
        ;;
    esac
  done
  return 1
}

if [[ "$MODE" == "failover" ]]; then
  if host_up "$PRIMARY_HOST" && service_active "$PRIMARY_HOST" k3s; then
    log "Primary ${PRIMARY_HOST} is still healthy; no failover needed"
    exit 0
  fi

  standby="$(select_standby_host || true)"
  if [[ -z "$standby" ]]; then
    log "No standby host available (mac/pi both unreachable)"
    exit 2
  fi

  # If primary is down, join existing control-plane is impossible; bootstrap standby.
  if [[ -z "$K3S_TOKEN" ]] && host_up "$PRIMARY_HOST"; then
    K3S_TOKEN="$(read_token_from_server "$PRIMARY_HOST")"
  fi

  if [[ -n "$K3S_TOKEN" ]] && host_up "$PRIMARY_HOST"; then
    primary_ip="$(get_ip "$PRIMARY_HOST")"
    promote_host_to_server "$standby" "https://${primary_ip}:6443" "$K3S_TOKEN"
  else
    log "Primary unavailable for join; bootstrapping standby as new cluster-init server"
    promote_host_to_server "$standby" "" ""
  fi

  log "Failover complete. New server candidate: ${standby}"
  exit 0
fi

# failback mode
if ! host_up "$PRIMARY_HOST"; then
  log "Primary ${PRIMARY_HOST} is not reachable yet; cannot fail back"
  exit 3
fi

# Identify current active standby server (mac preferred, otherwise pi)
active_server=""
if service_active "$MAC_HOST" k3s; then
  active_server="$MAC_HOST"
elif service_active "$PI_HOST" k3s; then
  active_server="$PI_HOST"
fi

if [[ -z "$active_server" ]]; then
  log "No active standby server detected; nothing to fail back"
  exit 0
fi

if [[ -z "$K3S_TOKEN" ]]; then
  K3S_TOKEN="$(read_token_from_server "$active_server")"
fi
active_ip="$(get_ip "$active_server")"

# Rejoin x509 as server from active server, then demote standby back to agent.
promote_host_to_server "$PRIMARY_HOST" "https://${active_ip}:6443" "$K3S_TOKEN"
primary_ip="$(get_ip "$PRIMARY_HOST")"
demote_host_to_agent "$active_server" "https://${primary_ip}:6443" "$K3S_TOKEN"

log "Failback complete. Primary restored: ${PRIMARY_HOST}"
