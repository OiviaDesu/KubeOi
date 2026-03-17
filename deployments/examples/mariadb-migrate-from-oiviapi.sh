#!/usr/bin/env bash
set -euo pipefail

# Full-fidelity migration helper from external source MariaDB (oiviapi 192.168.86.40)
# to MariaDB HA running in k3s.
#
# This script is orchestration-focused and prints critical commands (including sudo)
# for host-level operations that must run on source host.

SRC_HOST="${SRC_HOST:-192.168.86.40}"
SRC_SSH_USER="${SRC_SSH_USER:-oiviadesu}"
SRC_DB_ROOT_USER="${SRC_DB_ROOT_USER:-root}"
SRC_DB_ROOT_PASSWORD="${SRC_DB_ROOT_PASSWORD:-}"
SRC_DB_PORT="${SRC_DB_PORT:-3306}"
NAMESPACE="${NAMESPACE:-mariadb}"
TARGET_RELEASE="${TARGET_RELEASE:-mariadb-ha}"
WORKDIR="${WORKDIR:-/home/oiviadesu/git/Oiviak3s/deployments/migration-data/mariadb-from-oiviapi}"
TS="$(date +%Y%m%d-%H%M%S)"
RUN_DIR="${WORKDIR}/run-${TS}"

mkdir -p "${RUN_DIR}"

if [[ -z "${SRC_DB_ROOT_PASSWORD}" ]]; then
  echo "Missing required env: SRC_DB_ROOT_PASSWORD" >&2
  exit 1
fi

cat <<EOF
=== MariaDB HA Migration Helper ===
Source : ${SRC_SSH_USER}@${SRC_HOST}:${SRC_DB_PORT}
Target : ${TARGET_RELEASE}.${NAMESPACE}.svc.cluster.local:3306
Run dir: ${RUN_DIR}
EOF

echo "[1/7] Preflight: verifying k8s target is reachable"
kubectl -n "${NAMESPACE}" get pods -l app.kubernetes.io/name=mariadb-galera > "${RUN_DIR}/k8s-pods.txt"
kubectl -n "${NAMESPACE}" get svc > "${RUN_DIR}/k8s-services.txt"

echo "[2/7] Preflight: source MariaDB quick checks"
ssh "${SRC_SSH_USER}@${SRC_HOST}" "mysql -u${SRC_DB_ROOT_USER} -p'${SRC_DB_ROOT_PASSWORD}' -e \"SHOW VARIABLES WHERE Variable_name IN ('version','log_bin','binlog_format','gtid_strict_mode','server_id'); SHOW MASTER STATUS;\"" \
  | tee "${RUN_DIR}/source-baseline.txt"

echo "[3/7] Required sudo commands on source host (${SRC_HOST})"
cat <<'EOF'
# Run these on source host (192.168.86.40) with sudo:

# A) Install mariadb-backup (choose distro-specific command)
# Arch:
sudo pacman -Sy --noconfirm mariadb-backup
# Ubuntu/Debian:
sudo apt-get update && sudo apt-get install -y mariadb-backup

# B) Ensure replication-safe config exists
sudo mkdir -p /etc/my.cnf.d
sudo tee /etc/my.cnf.d/99-replication.cnf >/dev/null <<'CNF'
[mysqld]
server_id=40
log_bin=mysql-bin
binlog_format=ROW
binlog_row_image=FULL
sync_binlog=1
expire_logs_days=7
innodb_flush_log_at_trx_commit=1
gtid_strict_mode=ON
log_slave_updates=ON
CNF
sudo systemctl restart mariadb
sudo systemctl --no-pager --full status mariadb | sed -n '1,30p'
EOF

echo "[4/7] Triggering source-side physical backup via SSH"
ssh "${SRC_SSH_USER}@${SRC_HOST}" "mkdir -p /tmp/mariadb-export-${TS} && mariabackup --backup --target-dir=/tmp/mariadb-export-${TS}/base_backup --user=${SRC_DB_ROOT_USER} --password='${SRC_DB_ROOT_PASSWORD}' && mariabackup --prepare --target-dir=/tmp/mariadb-export-${TS}/base_backup && tar -C /tmp/mariadb-export-${TS} -czf /tmp/mariadb-export-${TS}.tar.gz base_backup"

echo "[5/7] Pulling backup artifact locally"
scp "${SRC_SSH_USER}@${SRC_HOST}:/tmp/mariadb-export-${TS}.tar.gz" "${RUN_DIR}/"
sha256sum "${RUN_DIR}/mariadb-export-${TS}.tar.gz" | tee "${RUN_DIR}/backup.sha256"

echo "[6/7] Next commands to restore into k3s target"
cat <<EOF
# Copy artifact into first galera pod and restore manually (review before run):
POD=\$(kubectl -n ${NAMESPACE} get pod -l app.kubernetes.io/name=mariadb-galera -o jsonpath='{.items[0].metadata.name}')
kubectl -n ${NAMESPACE} cp ${RUN_DIR}/mariadb-export-${TS}.tar.gz \${POD}:/tmp/mariadb-export.tar.gz
kubectl -n ${NAMESPACE} exec -it \${POD} -- bash
# inside pod:
#   cd /tmp && tar -xzf mariadb-export.tar.gz
#   # follow your vetted galera restore/seed runbook
EOF

echo "[7/7] Cleanup command for source temporary files (run after verify)"
cat <<EOF
ssh "${SRC_SSH_USER}@${SRC_HOST}" "rm -rf /tmp/mariadb-export-${TS} /tmp/mariadb-export-${TS}.tar.gz"
EOF

echo "Migration helper finished. Artifact at ${RUN_DIR}"
