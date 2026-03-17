#!/usr/bin/env bash
set -euo pipefail

MANIFEST="${MANIFEST:-/home/oiviadesu/git/Oiviak3s/deployments/examples/longhorn/engine-binaries-bind.yaml}"

if [[ ! -f "${MANIFEST}" ]]; then
  echo "Manifest not found: ${MANIFEST}" >&2
  exit 1
fi

kubectl apply -f "${MANIFEST}"
kubectl -n longhorn-system rollout status ds/longhorn-engine-binaries-bind --timeout=5m
