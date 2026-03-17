#!/usr/bin/env bash
set -euo pipefail

# Export SearXNG data from Pi before migration.
# Source of truth: /home/oiviadesu/docker
#
# Usage:
#   PI_HOST=192.168.86.40 PI_USER=oiviadesu \
#   OUT_DIR=/home/oiviadesu/git/Oiviak3s/deployments/migration-data/searxng-from-pi \
#   bash deployments/migration-data/searxng-from-pi/export-searxng-from-pi.sh

PI_HOST="${PI_HOST:-192.168.86.40}"
PI_USER="${PI_USER:-oiviadesu}"
PI_BASE="${PI_BASE:-/home/oiviadesu/docker}"
OUT_DIR="${OUT_DIR:-/home/oiviadesu/git/Oiviak3s/deployments/migration-data/searxng-from-pi}"
TS="$(date +%Y%m%d-%H%M%S)"
STAGE_DIR="${OUT_DIR}/snapshot-${TS}"
ARCHIVE_PATH="${OUT_DIR}/searxng-from-pi-${TS}.tar.gz"
CHECKSUM_PATH="${OUT_DIR}/searxng-from-pi-${TS}.sha256"

mkdir -p "${STAGE_DIR}"

echo "[1/6] Finding SearXNG container on Pi (${PI_USER}@${PI_HOST})"
SEARX_CONTAINER="$(ssh "${PI_USER}@${PI_HOST}" "docker ps --format '{{.Names}} {{.Image}}' | awk '/searxng\/searxng/ {print \$1; exit}' || true")"

if [[ -n "${SEARX_CONTAINER}" ]]; then
  echo "Found running SearXNG container: ${SEARX_CONTAINER}"
  echo "[2/6] Stopping SearXNG container for consistent copy"
  # shellcheck disable=SC2029
  ssh "${PI_USER}@${PI_HOST}" "docker stop '${SEARX_CONTAINER}'"
  CONTAINER_STOPPED=1
else
  echo "No running SearXNG container detected. Continuing with filesystem copy."
  CONTAINER_STOPPED=0
fi

cleanup() {
  if [[ "${CONTAINER_STOPPED}" -eq 1 ]]; then
    echo "[6/6] Restarting SearXNG container on Pi"
    # shellcheck disable=SC2029
    ssh "${PI_USER}@${PI_HOST}" "docker start '${SEARX_CONTAINER}'" || true
  fi
}
trap cleanup EXIT

echo "[3/6] Capturing SearXNG-related files from ${PI_BASE}"
rsync -a --delete \
  --prune-empty-dirs \
  --include='*/' \
  --include='*searxng*' \
  --include='docker-compose*.yml' \
  --include='docker-compose*.yaml' \
  --exclude='*' \
  "${PI_USER}@${PI_HOST}:${PI_BASE}/" \
  "${STAGE_DIR}/"

echo "[4/6] Writing source metadata"
{
  echo "pi_host=${PI_HOST}"
  echo "pi_user=${PI_USER}"
  echo "pi_base=${PI_BASE}"
  echo "snapshot_time=${TS}"
  echo "container=${SEARX_CONTAINER:-none}"
} > "${STAGE_DIR}/EXPORT_METADATA.txt"

echo "[5/6] Creating archive and checksum"
tar -C "${STAGE_DIR}" -czf "${ARCHIVE_PATH}" .
sha256sum "${ARCHIVE_PATH}" > "${CHECKSUM_PATH}"

echo "Done"
echo "Archive : ${ARCHIVE_PATH}"
echo "SHA256  : ${CHECKSUM_PATH}"
echo "Snapshot: ${STAGE_DIR}"
