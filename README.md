# Oiviak3s Operator

A production-grade Kubernetes operator for managing geo-distributed K3s workloads with intelligent placement, health monitoring, and automated failover.

## Overview

The Oiviak3s Operator provides cloud-native orchestration for edge computing scenarios where workloads need to be intelligently placed across geographically distributed nodes with varying resource availability, network connectivity, and reliability tiers.

### Key Features

- **Intelligent Workload Placement**: Multi-strategy placement engine considering geography, resources, and node tiers
- **Health Monitoring**: Continuous health checks via kubelet, resource availability, and network connectivity
- **Automated Failover**: Policy-driven failover with immediate, graceful, and manual strategies
- **Stable Shared Endpoint IP**: Optional kube-vip backed LoadBalancer endpoint that keeps one IP across failover/failback
- **Notification Integration**: Real-time alerts via Telegram and Discord
- **ZeroTier Integration**: Secure mesh networking for distributed clusters
- **SOLID Architecture**: Clean, extensible codebase following best practices

## Architecture

The operator implements three custom resources:

### NodeHealthStatus
Tracks the health status of each node using multiple health checkers:
- **Kubelet Checker**: Verifies node Ready condition and heartbeat freshness
- **Resource Checker**: Monitors CPU, memory, and storage availability
- **Network Checker**: Tests ZeroTier connectivity and latency

### RegionalWorkload
Manages workload placement with intelligent scheduling:
- **Geographic Strategy**: Prefers nodes in specified regions
- **Resource-Aware Strategy**: Considers available resources vs requirements
- **Tier-Based Strategy**: Respects node tier and power stability labels

### FailoverPolicy
Defines failover behavior when nodes or workloads become unhealthy:
- **Triggers**: Node unhealthy duration, workload unhealthy duration, regional outage
- **Strategies**: Immediate, graceful (with drain), or manual failover
- **Notifications**: Configurable alerts for different failover events

## Prerequisites

- Kubernetes cluster (K3s recommended) version 1.25+
- Go 1.22+ (for building from source)
- kubectl configured with cluster access
- kustomize v4+ (for deployment)
- ZeroTier network for distributed nodes
- Node labels configured:
  - `oiviak3s.io/region`: Region identifier (e.g., "hanoi", "cloud")
  - `oiviak3s.io/tier`: Node tier (primary, secondary, tertiary)
  - `oiviak3s.io/power-stability`: Power reliability (high, medium, low)

## Installation

### 1. Clone the Repository

```bash
git clone https://github.com/oiviadesu/oiviak3s-operator.git
cd oiviak3s-operator
```

### 2. Configure Secrets

Create a secret with your configuration:

```bash
cp config/manager/secret.yaml.example config/manager/secret.yaml
# Edit secret.yaml with your values
kubectl apply -f config/manager/secret.yaml
```

### 3. Install CRDs

```bash
make manifests
make install
```

### 4. Deploy the Operator

```bash
make deploy
```

Or build and deploy a custom image:

```bash
make docker-build docker-push IMG=<your-registry>/oiviak3s-operator:tag
make deploy IMG=<your-registry>/oiviak3s-operator:tag
```

## Configuration

The operator is configured via environment variables (see `config/manager/manager.yaml`):

### Cluster Configuration
- `CLUSTER_REGION_HANOI`: Label value used for the Hanoi region (default: "hanoi")
- `CLUSTER_REGION_MELBOURNE`: Label value used for the Melbourne region (default: "melbourne")

### ZeroTier Configuration
- `ZEROTIER_NETWORK_ID`: Your ZeroTier network ID (required)
- `ZEROTIER_INTERFACE`: ZeroTier interface name (default: "zt0")

### Health Check Configuration
- `HEALTH_CHECK_INTERVAL`: Interval between health checks (default: "30s")
- `HEALTH_CHECK_TIMEOUT`: Timeout for each health check (default: "10s")
- `FAILOVER_THRESHOLD`: Consecutive failures before marking unhealthy (default: "3")

### Placement Configuration
- `PLACEMENT_STRATEGY`: Primary placement strategy (default: "geographic")
- `DEFAULT_REGION_PREFERENCE`: Preferred region used when a workload does not specify its own region order (default: "hanoi")

### Shared Endpoint Configuration
- `SHARED_ENDPOINT_ENABLED`: Enable shared endpoint by default for new workloads (default: "true")
- `SHARED_ENDPOINT_MODE`: Shared endpoint mode (default: "kube-vip")
- `SHARED_ENDPOINT_IP`: Shared endpoint IP address (default: "192.168.86.8")
- `SHARED_ENDPOINT_AUTO_FAILBACK`: Auto-failback to preferred nodes after recovery (default: "true")

### Notification Configuration (Optional)
- `TELEGRAM_BOT_TOKEN`: Telegram bot token for notifications
- `TELEGRAM_CHAT_ID`: Telegram chat ID for notifications
- `DISCORD_WEBHOOK_URL`: Discord webhook URL for notifications

### Observability Configuration
- `METRICS_BIND_ADDRESS`: Prometheus metrics endpoint (default: ":8080")
- `HEALTH_PROBE_BIND_ADDRESS`: Health/readiness probe endpoint (default: ":8081")
- `LOG_LEVEL`: Logging level (default: "info")
- `LOG_FORMAT`: Log format - "json" or "console" (default: "json")

## Usage

### Example: Deploy OpenWebUI to Hanoi Region

```yaml
apiVersion: geo.oiviak3s.io/v1alpha1
kind: RegionalWorkload
metadata:
  name: openwebui
  namespace: openwebui
spec:
  workloadRef:
    apiVersion: apps/v1
    kind: Deployment
    name: openwebui
    namespace: openwebui
  placementConstraints:
    regionPreference:
    - hanoi
    - melbourne
    requireLabels:
      oiviak3s.io/power-stability: high
    avoidNodes: []
    antiAffinity:
    - openwebui
    tierPreference:
    - primary
    - secondary
  failoverConfig:
    enabled: true
    maxFailoverTime: 5m
    minHealthyReplicas: 1
    healthCheckGracePeriod: 1m
  sharedEndpoint:
    enabled: true
    mode: kube-vip
    ip: 192.168.86.8
    autoFailback: true
  notificationEnabled: true
```

See `deployments/examples/` for more examples.

### Autoscaling (auto scale up/down)

The repository now includes an autoscaling pack under `deployments/examples/autoscaling/`:

- `hpa-immich-server.yaml`
- `hpa-worldlinkd.yaml`
- `hpa-kubernetes-dashboard-gateway.yaml`
- `keda-scaledobject-qps-template.yaml`
- `keda-scaledobject-queue-template.yaml`
- `keda-scaledobject-node-disk-template.yaml`
- `apply-autoscaling.sh`

Apply autoscaling baseline (CPU/RAM, auto up/down):

```bash
bash deployments/examples/autoscaling/apply-autoscaling.sh
```

Optional KEDA trigger templates (QPS/queue/disk):

```bash
# Enable template apply only after you have real metric backends configured
ENABLE_KEDA_TEMPLATES=true \
PROM_NAMESPACE=monitoring \
PROM_SERVICE=prometheus-server \
QUEUE_KEY=your_real_queue_key \
bash deployments/examples/autoscaling/apply-autoscaling.sh
```

Safety guard:

- The helper detects existing native HPAs on the same deployment target and skips KEDA ScaledObject apply for that target to avoid multi-HPA selector conflicts.

Important scope note:

- Pod instance autoscaling is fully automated with HPA/KEDA (scale up and down).
- This cluster is currently bare-metal k3s. Automatic scaling of **physical server count** needs an external infrastructure API (for example MAAS/Cluster API/Talos automation). Without that API, Kubernetes can autoscale pods, but cannot magically provision new physical nodes by itself.

### MariaDB HA on k3s (Galera) + Full Migration from external host

The repository now includes a starter set for MariaDB HA deployment and full-fidelity migration from an external MariaDB source (current source host: `192.168.86.40`).

Artifacts:

- `deployments/examples/mariadb-ha-secret.yaml.example`
- `deployments/examples/mariadb-ha-values.yaml`
- `deployments/examples/mariadb-ha-deploy.sh`
- `deployments/examples/mariadb-migrate-from-oiviapi.sh`
- `deployments/migration-data/mariadb-from-oiviapi/README.md`

Deploy HA cluster:

```bash
cp deployments/examples/mariadb-ha-secret.yaml.example deployments/examples/mariadb-ha-secret.yaml
# edit secret values
bash deployments/examples/mariadb-ha-deploy.sh
```

Source-host preparation (requires `sudo` on `192.168.86.40`):

```bash
# Arch
sudo pacman -Sy --noconfirm mariadb-backup

# Ubuntu/Debian
sudo apt-get update && sudo apt-get install -y mariadb-backup

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

Run migration helper:

```bash
SRC_DB_ROOT_PASSWORD='<root_password>' \
bash deployments/examples/mariadb-migrate-from-oiviapi.sh
```

The helper performs source/target preflight, creates a physical backup artifact with checksum, and prints the next restore/cutover commands.

### Kubernetes Dashboard Integration

The repository now includes a repo-managed Kubernetes Dashboard stack under `deployments/dashboard/` and an Oiviak3s public entrypoint example under `deployments/examples/kubernetes-dashboard-k3s-migration.yaml`.

Architecture summary:

- `deployments/dashboard/namespace.yaml` creates a dedicated `kubernetes-dashboard` namespace.
- `deployments/dashboard/kubernetes-dashboard-v7.14.0.yaml` vendors a Helm-rendered Dashboard 7.14.0 snapshot with internal `ClusterIP` services only.
- `deployments/examples/kubernetes-dashboard-k3s-migration.yaml` adds a thin HAProxy TCP passthrough gateway on port `9443`.
- `RegionalWorkload` manages only the public gateway deployment, so Oiviak3s continues to own a single public workload and shared endpoint for Dashboard traffic.
- TLS stays encrypted all the way from the client to the Dashboard's internal Kong proxy. The public HAProxy workload does TCP passthrough only and does not terminate TLS.
- The vendored snapshot does not install `metrics-server`; Dashboard metrics views require an existing cluster-level metrics server.

Mirrored ingress behavior on this k3s cluster:

- On this cluster, the practical way to expose the same workload through both `192.168.86.8` and `192.168.86.40` is **one** `LoadBalancer` Service per workload, with k3s ServiceLB restricted to those two ingress nodes.
- The service then mirrors the same selector and port set across both ingress nodes automatically; you do not need two separate `LoadBalancer` Services for the same workload.
- The API still supports `sharedEndpoint.endpoints[]` for VIP-aware implementations, but the checked-in examples use the k3s-compatible single-service shape.

Required k3s ServiceLB node selection:

```bash
kubectl label node oiviax509fj-master svccontroller.k3s.cattle.io/enablelb=true --overwrite
kubectl label node oiviapi-worker svccontroller.k3s.cattle.io/enablelb=true --overwrite
kubectl label node oiviamacmini-worker svccontroller.k3s.cattle.io/enablelb-
kubectl label node server67 svccontroller.k3s.cattle.io/enablelb-
```

Important runtime limitation on this cluster:

k3s ServiceLB uses `hostPort` per Service port. Two separate `LoadBalancer` Services that both want the same public port on the same ingress nodes will conflict, so this cluster should publish mirrored traffic with one shared-endpoint Service per workload.

Deployment order:

```bash
kubectl apply -f deployments/dashboard/namespace.yaml
kubectl -n kubernetes-dashboard create secret generic kubernetes-dashboard-csrf \
  --from-literal=private.key="$(openssl rand -base64 256 | tr -d '\n')"
kubectl apply -f deployments/dashboard/kubernetes-dashboard-v7.14.0.yaml
kubectl apply -f deployments/examples/kubernetes-dashboard-k3s-migration.yaml
```

Access and verification:

```bash
kubectl get pods -n kubernetes-dashboard
kubectl get regionalworkload -n kubernetes-dashboard kubernetes-dashboard-gateway
kubectl get svc -n kubernetes-dashboard | grep kubernetes-dashboard-gateway-shared-endpoint
curl -kI https://192.168.86.8:9443/
curl -kI https://192.168.86.40:9443/
```

If the example is unchanged, both `https://192.168.86.8:9443/` and `https://192.168.86.40:9443/` should reach the same Dashboard gateway behavior.

By default this vendored path uses Kong's built-in certificate, so browsers and API clients will treat it as untrusted unless you replace that certificate path in a follow-up hardening step.

Always verify the effective public address with:

```bash
kubectl get svc -n kubernetes-dashboard | grep kubernetes-dashboard-gateway-shared-endpoint
```

If your cluster is using a different LoadBalancer implementation than kube-vip, or if the control-plane node does not advertise an `ExternalIP`, the observed `EXTERNAL-IP` may not list every reachable ingress node. Treat the ServiceLB node selection and the service status together as the source of truth for actual published reachability.

For the currently documented routed node set:

- `x509fj=192.168.86.8`
- `macmini=192.168.86.41`
- `pi=192.168.86.40`
- `server67=192.168.86.15`

Do not document or deploy a Dashboard address outside that routed set unless you have explicitly provisioned an additional VIP on your network.

Runtime user and token flow:

- The repo does not create an admin `ClusterRoleBinding`, bootstrap token, or long-lived credential.
- Create a runtime-only user yourself after rollout. For a temporary cluster-admin session, for example:

```bash
kubectl -n kubernetes-dashboard create serviceaccount admin-user
kubectl create clusterrolebinding kubernetes-dashboard-admin-user \
  --clusterrole=cluster-admin \
  --serviceaccount=kubernetes-dashboard:admin-user
kubectl -n kubernetes-dashboard create token admin-user
```

- Delete the temporary binding and service account when you are done, or replace the `cluster-admin` binding with a narrower role for day-to-day use.

Manual failover check:

```bash
kubectl get endpoints -n kubernetes-dashboard kubernetes-dashboard-kong-proxy
kubectl get svc -n kubernetes-dashboard kubernetes-dashboard-gateway-shared-endpoint -o wide
# then simulate node failure/drain according to your runbook and confirm HTTPS recovers on :9443
```

### Node Health Monitoring

The operator automatically creates NodeHealthStatus resources for each node:

```bash
kubectl get nodehealthstatus
kubectl describe nodehealthstatus <node-name>
```

### Failover Policy

Configure automated failover behavior:

```yaml
apiVersion: geo.oiviak3s.io/v1alpha1
kind: FailoverPolicy
metadata:
  name: production-failover
spec:
  enabled: true
  trigger:
    nodeUnhealthyDuration: 5m
    workloadUnhealthyDuration: 3m
    regionalOutage: true
  strategy:
    type: graceful
    drainTimeout: 5m
    gracePeriod: 30s
    targetRegionPreference:
    - melbourne
  notificationRule:
    enabled: true
    onFailoverStart: true
    onFailoverComplete: true
    onFailoverFailed: true
    onNodeHealthChange: false
    minSeverity: warning
  targetWorkloads:
  - openwebui
```

## Development

### Build

```bash
make build
```

### Run Locally

```bash
make run
```

### Generate CRDs and Code

```bash
make manifests generate
```

### Run Tests

```bash
make test
```

### Format and Vet

```bash
make fmt
make vet
```

## Monitoring

The operator exposes Prometheus metrics at `:8080/metrics`:

- `nodehealthstatus_status`: Node health status by node and region
- `regionalworkload_placement_score`: Placement decision scores
- `failoverpolicy_events_total`: Total failover events by policy

## Troubleshooting

### Check Operator Logs

```bash
kubectl logs -n oiviak3s-system deployment/oiviak3s-controller-manager -f
```

### Verify CRD Installation

```bash
kubectl get crds | grep geo.oiviak3s.io
```

### Check Node Labels

```bash
kubectl get nodes --show-labels
```

### Test ZeroTier Connectivity

```bash
# From each node
ping <zerotier-ip-of-other-node>
```

### Common Issues

**Issue**: Workload not placed
- **Solution**: Check node labels match `requireLabels` in RegionalWorkload
- **Solution**: Verify nodes are healthy via NodeHealthStatus

**Issue**: Health checks failing
- **Solution**: Verify ZeroTier interface name matches `ZEROTIER_INTERFACE`
- **Solution**: Check kubelet port 10250 is accessible over ZeroTier

**Issue**: Notifications not working
- **Solution**: Verify Telegram/Discord secrets are configured correctly
- **Solution**: Check operator logs for notification errors

**Issue**: External host cannot reach `192.168.86.8`
- **Solution**: Verify the external host has a route to the LAN/VIP subnet; ZeroTier membership alone is not enough
- **Solution**: Check the shared endpoint service status and kube-vip ownership with `kubectl get svc -n <ns> <workload>-shared-endpoint -o yaml`

**Issue**: Dashboard port `9443` is unreachable on the ingress nodes
- **Solution**: Check `kubectl get svc -n kubernetes-dashboard kubernetes-dashboard-gateway-shared-endpoint -o yaml` and confirm the operator exposed port `9443` from the gateway deployment
- **Solution**: Verify nothing else is already using the same public port on the ServiceLB ingress nodes; k3s ServiceLB reserves host ports per service port

**Issue**: Dashboard login page loads but API calls fail
- **Solution**: Confirm the internal `kubernetes-dashboard-kong-proxy`, `kubernetes-dashboard-api`, `kubernetes-dashboard-auth`, and `kubernetes-dashboard-web` services all have ready endpoints
- **Solution**: Recreate the runtime `kubernetes-dashboard-csrf` secret and restart the Dashboard deployments if the secret was missing during initial rollout

**Issue**: Browser cannot complete token login
- **Solution**: Make sure you are using HTTPS to the Dashboard VIP and port; token login is expected to fail over plain HTTP
- **Solution**: Verify the runtime service account or binding was created outside the default example, since the repo intentionally ships no admin bootstrap

## Contributing

Contributions are welcome! Please:
1. Fork the repository
2. Create a feature branch
3. Commit your changes with descriptive messages
4. Add tests for new functionality
5. Submit a pull request

## License

Copyright 2026 oiviadesu.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.

## Acknowledgments

Built with:
- [Kubebuilder](https://book.kubebuilder.io/) - Kubernetes operator framework
- [controller-runtime](https://github.com/kubernetes-sigs/controller-runtime) - Kubernetes controller libraries
- [ZeroTier](https://www.zerotier.com/) - Software-defined networking

## Contact

For questions or support, please open an issue on GitHub.
