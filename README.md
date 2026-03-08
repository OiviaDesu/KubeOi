# Oiviak3s Operator

A production-grade Kubernetes operator for managing geo-distributed K3s workloads with intelligent placement, health monitoring, and automated failover.

## Overview

The Oiviak3s Operator provides cloud-native orchestration for edge computing scenarios where workloads need to be intelligently placed across geographically distributed nodes with varying resource availability, network connectivity, and reliability tiers.

### Key Features

- **Intelligent Workload Placement**: Multi-strategy placement engine considering geography, resources, and node tiers
- **Health Monitoring**: Continuous health checks via kubelet, resource availability, and network connectivity
- **Automated Failover**: Policy-driven failover with immediate, graceful, and manual strategies
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
- `CLUSTER_REGIONS`: Comma-separated list of regions (default: "hanoi,cloud")
- `DEFAULT_REGION`: Default region for placement (default: "hanoi")

### ZeroTier Configuration
- `ZEROTIER_NETWORK_ID`: Your ZeroTier network ID (required)
- `ZEROTIER_INTERFACE`: ZeroTier interface name (default: "ztmjfaywil")

### Health Check Configuration
- `HEALTH_CHECK_INTERVAL`: Interval between health checks (default: "30s")
- `HEALTH_CHECK_TIMEOUT`: Timeout for each health check (default: "10s")
- `HEALTH_FAILURE_THRESHOLD`: Consecutive failures before marking unhealthy (default: "3")

### Placement Configuration
- `PLACEMENT_STRATEGY`: Primary placement strategy (default: "geographic")

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
    kind: Deployment
    name: openwebui
  placementConstraints:
    regionPreference:
    - hanoi
    tierPreference:
    - primary
    - secondary
    requireLabels:
      oiviak3s.io/power-stability: high
  resourceRequirements:
    cpu: "2000m"
    memory: "4Gi"
```

See `deployments/examples/` for more examples.

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
  triggers:
    nodeUnhealthyDuration: 5m
    workloadUnhealthyDuration: 3m
    regionalOutage:
      enabled: true
      minUnhealthyNodes: 50
  strategy:
    type: Graceful
    drainTimeout: 5m
    targetRegionPreference:
    - cloud
  notificationRules:
    onFailoverTriggered: true
    onFailoverCompleted: true
    onFailoverFailed: true
    minSeverity: warning
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

### Linting

```bash
make lint
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
