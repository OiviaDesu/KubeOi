# MariaDB HA Migration Runbook (oiviapi -> k3s)

Source:
- External MariaDB host: `192.168.86.40`

Target:
- k3s MariaDB Galera HA release in namespace `mariadb`

## 1) Deploy HA MariaDB in k3s

1. Copy and edit secret:
   - `cp deployments/examples/mariadb-ha-secret.yaml.example deployments/examples/mariadb-ha-secret.yaml`
2. Deploy:
   - `bash deployments/examples/mariadb-ha-deploy.sh`

## 2) Prepare source host (requires sudo)

Run on source host (`192.168.86.40`):

```bash
# Install backup tool
# Arch:
sudo pacman -Sy --noconfirm mariadb-backup
# Ubuntu/Debian:
sudo apt-get update && sudo apt-get install -y mariadb-backup

# Replication-safe config
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
```

## 3) Run migration helper

```bash
SRC_DB_ROOT_PASSWORD='<root_password>' \
bash deployments/examples/mariadb-migrate-from-oiviapi.sh
```

This helper performs:
- source/target preflight checks
- backup artifact creation from source via `mariabackup`
- checksum generation
- outputs exact restore and cleanup commands

## 4) Cutover checklist (no-loss focus)

- [ ] Source binlog enabled and healthy
- [ ] Full backup artifact + checksum verified
- [ ] Restore complete on target seed node
- [ ] Replication catch-up complete (`lag ~ 0`)
- [ ] Maintenance mode on app (short write freeze)
- [ ] Final sync and endpoint switch to k3s writer service
- [ ] Object inventory + row-count/checksum validation passed

## 5) Validate HA

- Galera size should remain 3 in healthy state.
- Simulate single pod failure and verify writes still work.
- Keep source read-only for short rollback window.

## Notes

- For production-grade HA, use a replicated storage class (e.g. Longhorn HA), not `local-path`.
- Pi-tier nodes should remain tertiary/emergency-only for stateful primary traffic.
