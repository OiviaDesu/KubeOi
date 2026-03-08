## Plan: K3s Geo-Distributed HA Operator with Intelligent Failover

Build production-grade operator to manage geo-distributed K3s cluster (Hanoi-Melbourne) with zero-downtime failover, automatic control plane migration on power loss, and intelligent workload placement.

**Architecture**: HA K3s cluster (3 server nodes) + Go operator (Kubebuilder) with SOLID principles, interface-based design, dependency injection, and plugin system for extensibility.

**SOLID Compliance:**
- **SRP**: Each controller has separate responsibility, pkg/health, pkg/placement, pkg/notification are separated
- **OCP**: Plugin interfaces for health checkers, notifiers - extend without modifying core
- **LSP**: All implementations satisfy interface contracts, swappable
- **ISP**: Specific interfaces (HealthChecker, Notifier, MetricsCollector) instead of monolithic
- **DIP**: Controllers depend on abstractions, not concrete implementations

### Steps

**Phase 1: Infrastructure Foundation (Setup HA K3s)**

1. **Convert K3s to HA mode with embedded etcd** *(depends on current K3s state)*
   - Backup current K3s datastore on x509fj
   - Reinstall x509fj as first server node with `--cluster-init` flag
   - Join Mac Mini M4 as second server node
   - Join Pi 400 as third server node (quorum for etcd)
   - Verify etcd cluster health and leader election

2. **Configure node labels and taints for priority tiers** *(parallel with step 3)*
   - Label nodes: `region=hanoi/melbourne`, `tier=primary/secondary/tertiary`, `power-stability=low/medium/high`
   - Taint x509fj: `power-unstable=true:PreferNoSchedule` (prefer other nodes for critical workloads)
   - Add resource labels: CPU/memory capacity for scheduler decisions

3. **Setup distributed storage with Longhorn** *(parallel with step 2)*
   - Deploy Longhorn to cluster (3-replica distributed block storage)
   - Configure storage classes: `longhorn-hanoi` (2 replicas in Hanoi), `longhorn-ha` (3 replicas)
   - Create PVs for stateful apps: Minecraft world data, AdGuard config, SearXNG indices
   - Test failover: kill node, verify PV remounts on surviving nodes

4. **Network and connectivity validation**
   - Document VPN topology (which VPN? Tailscale/WireGuard/ZeroTier?)
   - Measure latency between Hanoi and Melbourne (important for etcd quorum)
   - Configure K3s `--node-external-ip` if needed for cross-site communication
   - Setup MetalLB or K3s ServiceLB for LoadBalancer services

**Phase 2: Operator Development** *(depends on Phase 1 completion)*

5. **Initialize Kubebuilder project with SOLID package structure**
   - `kubebuilder init --domain oiviak3s.io --repo github.com/onegate/oiviak3s-operator`
   - Create API scaffolds for 3 CRDs
   - Setup package structure (see detailed structure below)
   - Configure RBAC, Makefile targets

6. **Define core interfaces (SOLID ISP + DIP)** *(parallel with step 7)*
   - `pkg/health/interfaces.go`: `HealthChecker`, `HealthProvider` interfaces
   - `pkg/placement/interfaces.go`: `WorkloadPlacer`, `NodeSelector` interfaces
   - `pkg/notification/interfaces.go`: `Notifier`, `AlertFormatter` interfaces
   - `pkg/metrics/interfaces.go`: `MetricsCollector` interface (abstraction over Prometheus)

7. **Implement CRDs** *(parallel with step 6)*
   - `api/v1alpha1/nodehealthstatus_types.go`: NodeHealthStatus (cluster-scoped)
   - `api/v1alpha1/regionalworkload_types.go`: RegionalWorkload (namespaced)
   - `api/v1alpha1/failoverpolicy_types.go`: FailoverPolicy (cluster-scoped)
   - Generate deepcopy, clientset via `make generate`

8. **Implement health check plugins (SOLID OCP)** *(depends on step 6)*
   - `pkg/health/kubelet/checker.go`: KubeletHealthChecker (checks node conditions)
   - `pkg/health/resource/checker.go`: ResourcePressureChecker (memory/disk)
   - `pkg/health/network/checker.go`: NetworkLatencyChecker (ping test)
   - `pkg/health/registry.go`: Plugin registry for dynamic health checker registration
   - Each checker implements `HealthChecker` interface, composable

9. **Implement notification plugins (SOLID OCP + LSP)** *(parallel with step 8)*
   - `pkg/notification/telegram/notifier.go`: TelegramNotifier (Bot API client)
   - `pkg/notification/discord/notifier.go`: DiscordNotifier (Webhook client)
   - `pkg/notification/formatter.go`: Alert message formatter (reusable)
   - `pkg/notification/factory.go`: Factory pattern for notifier instantiation from .env config

10. **Implement workload placement engine (SOLID SRP)** *(depends on steps 6-8)*
    - `pkg/placement/strategy/geographic.go`: GeographicStrategy (region preference)
    - `pkg/placement/strategy/resource.go`: ResourceBasedStrategy (CPU/memory aware)
    - `pkg/placement/strategy/tier.go`: TierStrategy (primary > secondary > tertiary)
    - `pkg/placement/engine.go`: Composition of strategies, calculates optimal pod distribution
    - `pkg/placement/affinity.go`: Pod affinity/anti-affinity rule generator

11. **Build controllers with dependency injection** *(depends on steps 7-10)*
    - `controllers/nodehealthstatus_controller.go`: NodeMonitorController
      - Constructor accepts `[]HealthChecker` (DIP), reconcile every 10s
      - Aggregates results from all health checkers
      - Updates NodeHealthStatus CRD status
    - `controllers/regionalworkload_controller.go`: WorkloadPlacementController
      - Constructor accepts `WorkloadPlacer` interface
      - Watches NodeHealthStatus changes, triggers rebalancing
      - Injects affinity rules into target Deployments
    - `controllers/failoverpolicy_controller.go`: FailoverController
      - Constructor accepts `Notifier` interface
      - Detects unhealthy nodes > threshold, triggers drain/cordon
      - Sends alerts via notifier implementations

12. **Wire dependencies with configuration** *(depends on step 11)*
    - `cmd/manager/main.go`: Main entry point with dependency injection
    - Load .env via `godotenv` library
    - Instantiate health checkers, notifiers, placement engine from config
    - Pass to controller constructors (no global state, pure DI)
    - Setup Prometheus metrics endpoint (standard Kubebuilder)

**Phase 3: Deployment & Observability** *(depends on Phase 2)*

13. **Create configuration management**
    - `.env` file template with all config (see .env specification below)
    - `pkg/config/loader.go`: Config struct + validation logic (SOLID SRP)
    - Load at startup, fail fast if invalid config

14. **Build operator container image**
    - Multi-stage Dockerfile: `golang:1.22-alpine` builder + `gcr.io/distroless/static` runtime
    - Push to registry (Docker Hub / GHCR)
    - Create Kustomize manifests for deployment

15. **Deploy operator to K3s cluster**
    - `make install` (apply CRDs)
    - `make deploy IMG=<registry>/oiviak3s-operator:latest`
    - Verify operator pod running, check logs for initialization

16. **Setup monitoring** *(parallel with step 17)*
    - Prometheus ServiceMonitor for operator metrics
    - Custom metrics: `node_health_score`, `failover_events_total`, `workload_migrations_total`
    - Log aggregation for E2E testing

17. **E2E testing on real cluster** *(parallel with step 16)*
    - Test suite: `test/e2e/` directory
    - Test scenarios: failover, placement, notification delivery
    - Collect logs via `kubectl logs` for validation

**Phase 4: Application Migration & Testing** *(depends on Phase 3)*

18. **Create RegionalWorkload resources for existing apps**
    - OpenWebUI: `region-preference: hanoi`, `min-replicas-per-region: 1`
    - SearXNG: `region-preference: hanoi`, fallback to Melbourne
    - Minecraft: `region-preference: hanoi`, use Longhorn PVC, avoid Pi 400
    - AdGuard Home: `region-preference: any`, lightweight workload

19. **End-to-end failover testing**
    - Scenario 1: Shutdown x509fj → verify control plane still works (etcd quorum)
    - Scenario 2: Shutdown x509fj → verify workload pods migrate within 60s
    - Scenario 3: Shutdown x509fj + Mac Mini → verify Pi 400 handles critical workloads
    - Scenario 4: x509fj comes back online → verify workloads gradually migrate back
    - Collect logs for each scenario: operator logs, pod events, notification delivery

20. **Load testing & latency validation**
    - Simulate user traffic to OpenWebUI from Hanoi → verify low latency
    - Test Minecraft server: ensure no lag on x509fj, acceptable on Mac Mini
    - Verify Pi 400 handles DNS queries (AdGuard) with acceptable latency

---

### Relevant Files & SOLID Package Structure

**Project root:**
```
oiviak3s-operator/
├── .env                          # Configuration (not committed)
├── .env.example                  # Template with documentation
├── README.md                     # Comprehensive setup guide, architecture docs
├── Dockerfile                    # Multi-stage build
├── Makefile                      # Kubebuilder-generated + custom targets
├── PROJECT                       # Kubebuilder metadata
├── go.mod / go.sum              # Dependencies
├── cmd/
│   └── manager/
│       └── main.go              # Entry point, dependency injection wiring
├── api/
│   └── v1alpha1/
│       ├── nodehealthstatus_types.go
│       ├── regionalworkload_types.go
│       ├── failoverpolicy_types.go
│       ├── groupversion_info.go
│       └── zz_generated.deepcopy.go
├── controllers/
│   ├── nodehealthstatus_controller.go    # Health monitoring reconciler
│   ├── regionalworkload_controller.go    # Workload placement reconciler
│   └── failoverpolicy_controller.go      # Failover orchestration reconciler
├── pkg/
│   ├── config/
│   │   ├── config.go                     # Config struct
│   │   ├── loader.go                     # .env loader with validation
│   │   └── validator.go                  # Config validation logic (SRP)
│   ├── health/
│   │   ├── interfaces.go                 # HealthChecker, HealthProvider interfaces (DIP)
│   │   ├── registry.go                   # Plugin registry (OCP)
│   │   ├── aggregator.go                 # Combines multiple checker results (SRP)
│   │   ├── kubelet/
│   │   │   └── checker.go                # KubeletHealthChecker implementation
│   │   ├── resource/
│   │   │   └── checker.go                # ResourcePressureChecker implementation
│   │   └── network/
│   │       └── checker.go                # NetworkLatencyChecker implementation
│   ├── placement/
│   │   ├── interfaces.go                 # WorkloadPlacer, NodeSelector interfaces (DIP)
│   │   ├── engine.go                     # Placement engine orchestrator (SRP)
│   │   ├── affinity.go                   # Pod affinity rule generator (SRP)
│   │   └── strategy/
│   │       ├── geographic.go             # Geographic strategy (OCP)
│   │       ├── resource.go               # Resource-based strategy (OCP)
│   │       └── tier.go                   # Tier priority strategy (OCP)
│   ├── notification/
│   │   ├── interfaces.go                 # Notifier, AlertFormatter interfaces (ISP)
│   │   ├── factory.go                    # Factory pattern for instantiation (OCP)
│   │   ├── formatter.go                  # Alert message formatter (SRP)
│   │   ├── telegram/
│   │   │   └── notifier.go               # TelegramNotifier implementation (LSP)
│   │   └── discord/
│   │       └── notifier.go               # DiscordNotifier implementation (LSP)
│   ├── metrics/
│   │   ├── interfaces.go                 # MetricsCollector interface (DIP)
│   │   └── prometheus.go                 # Prometheus implementation
│   └── k8s/
│       ├── client.go                     # Kubernetes client wrapper (SRP)
│       └── drain.go                      # Node drain operations (SRP)
├── config/
│   ├── crd/                              # CRD YAML manifests
│   ├── rbac/                             # Role, RoleBinding, ServiceAccount
│   ├── manager/                          # Deployment for operator
│   ├── prometheus/                       # ServiceMonitor
│   └── samples/                          # Sample CR instances (OpenWebUI, Minecraft, etc.)
├── test/
│   └── e2e/
│       ├── failover_test.go              # E2E failover scenarios
│       ├── placement_test.go             # E2E workload placement tests
│       └── notification_test.go          # E2E notification delivery tests
└── deployments/
    ├── k3s/
    │   ├── ha-conversion.sh              # Script to convert single-node to HA
    │   ├── node-labels.sh                # Script to label nodes with region/tier
    │   └── longhorn/
    │       └── longhorn-values.yaml      # Helm values for Longhorn
    └── examples/
        ├── openwebui-regionalworkload.yaml
        ├── minecraft-regionalworkload.yaml
        ├── searxng-regionalworkload.yaml
        └── adguard-regionalworkload.yaml
```

**Key SOLID patterns applied:**

- **SRP**: Each package has single responsibility (health checking, placement logic, notifications)
- **OCP**: Strategy pattern for placement, plugin registry for health checkers - add new without modifying existing
- **LSP**: All notifier implementations satisfy Notifier interface, swappable
- **ISP**: Narrow interfaces (HealthChecker separate from HealthProvider, Notifier separate from AlertFormatter)
- **DIP**: Controllers depend on interfaces (`HealthChecker`, `Notifier`, `WorkloadPlacer`), not concrete types

**Dependency Injection in `cmd/manager/main.go`:**
```go
// Load config from .env
cfg := config.LoadConfig()

// Instantiate health checkers (DIP - plugins)
healthRegistry := health.NewRegistry()
healthRegistry.Register(kubelet.NewChecker(k8sClient))
healthRegistry.Register(resource.NewChecker(k8sClient))
healthRegistry.Register(network.NewChecker(cfg.NetworkTimeout))

// Instantiate notifiers (factory pattern + DIP)
notifier := notification.NewMultiNotifier(
    telegram.NewNotifier(cfg.TelegramToken, cfg.TelegramChatID),
    discord.NewNotifier(cfg.DiscordWebhook),
)

// Instantiate placement engine (strategy composition)
placer := placement.NewEngine(
    strategy.NewGeographicStrategy(cfg.RegionPriority),
    strategy.NewTierStrategy(),
    strategy.NewResourceStrategy(k8sClient),
)

// Wire controllers with dependencies
nodeHealthController := controllers.NewNodeHealthStatusController(k8sClient, healthRegistry)
workloadController := controllers.NewRegionalWorkloadController(k8sClient, placer)
failoverController := controllers.NewFailoverPolicyController(k8sClient, notifier, k8sDrainer)
```

---

### .env File Specification

```bash
# Kubernetes connection
KUBECONFIG=/home/user/.kube/config

# Operator behavior
OPERATOR_NAMESPACE=oiviak3s-system
RECONCILE_INTERVAL_SECONDS=10
HEALTH_CHECK_TIMEOUT_SECONDS=5
FAILOVER_THRESHOLD_SECONDS=30

# Health check plugins (comma-separated)
# Options: kubelet,resource,network
ENABLED_HEALTH_CHECKERS=kubelet,resource,network
NETWORK_LATENCY_CHECK_ENABLED=true
NETWORK_PING_TARGETS=8.8.8.8,1.1.1.1

# Notification settings
NOTIFICATION_ENABLED=true
TELEGRAM_ENABLED=true
TELEGRAM_BOT_TOKEN=123456:ABC-DEF1234ghIkl-zyx57W2v1u123ew11
TELEGRAM_CHAT_ID=-1001234567890

DISCORD_ENABLED=false
DISCORD_WEBHOOK_URL=https://discord.com/api/webhooks/...

# Placement engine tuning
REGION_PRIORITY=hanoi,melbourne
TIER_PRIORITY=primary,secondary,tertiary
PREFER_LOW_RESOURCE_PRESSURE=true
RESOURCE_PRESSURE_THRESHOLD=80  # percentage

# Metrics & observability
METRICS_PORT=8080
METRICS_PATH=/metrics
LOG_LEVEL=info  # Options: debug,info,warn,error

# Feature flags
AUTO_DRAIN_UNHEALTHY_NODES=true
AUTO_RESTART_CRASHLOOP_PODS=true
ENABLE_CROSS_REGION_SCHEDULING=true
```

---

### README.md Content Outline

**README.md sections:**

1. **Project Overview**
   - What is Oiviak3s Operator
   - Use case: geo-distributed K3s cluster with power instability
   - Key features: zero-downtime failover, intelligent placement, health monitoring

2. **Architecture**
   - Cluster topology diagram (ASCII art or Mermaid)
   - CRD design: NodeHealthStatus, RegionalWorkload, FailoverPolicy
   - SOLID principles applied
   - Controller architecture with plugin system

3. **Prerequisites**
   - K3s HA cluster (3 server nodes)
   - Longhorn installed
   - Node labels configured
   - VPN connectivity Hanoi-Melbourne

4. **Installation**
   - Operator deployment steps
   - .env configuration guide
   - RBAC setup
   - Verification commands

5. **Usage**
   - Creating RegionalWorkload resources
   - Configuring FailoverPolicy
   - Monitoring NodeHealthStatus
   - Viewing metrics and logs

6. **Examples**
   - OpenWebUI, Minecraft, SearXNG, AdGuard deployments

7. **Development**
   - Building from source
   - Running E2E tests
   - Project structure

8. **Troubleshooting**
   - Common issues and solutions
   - Debugging commands

9. **References**
   - Kubebuilder, K3s, Longhorn documentation links

*(No emoji, all comments in English only)*

---

### Reference Implementations

- **Kubebuilder Book**: https://book.kubebuilder.io
- **K3s HA**: https://docs.k3s.io/datastore/ha-embedded
- **Longhorn**: https://longhorn.io
- **K8s Scheduler**: https://kubernetes.io/docs/concepts/scheduling-eviction/
- **Go Clean Architecture**: https://github.com/bxcodec/go-clean-arch
- **Telegram Bot API**: https://core.telegram.org/bots/api
- **Discord Webhooks**: https://discord.com/developers/docs/resources/webhook

**K3s nodes modification:**
- x509fj: `--cluster-init --disable traefik --disable servicelb`
- Mac Mini M4: Join as server with `--server` flag
- Pi 400: Join as server with `--server` flag

---

### Verification Steps

**After Phase 1 (Infrastructure):**
1. Run `kubectl get nodes` → verify all 3 nodes show `Ready`, role `control-plane,master`
2. Run `kubectl -n kube-system get pods | grep etcd` → verify 3 etcd pods running
3. Run `kubectl get sc` → verify Longhorn storage classes exist
4. Create test PVC, deploy pod, kill node, verify pod restarts on another node with PVC intact

**After Phase 2 (Operator Development):**
1. Run `kubectl get crd` → verify `nodehealthstatuses`, `regionalworkloads`, `failoverpolicies` exist
2. Run `kubectl get nodehealthstatus` → verify all 3 nodes have status resources
3. Create sample RegionalWorkload → verify operator updates deployment's affinity rules
4. Check operator logs: `kubectl logs -n oiviak3s-system deployment/oiviak3s-operator-controller-manager`

**After Phase 3 (Deployment):**
1. Run `kubectl top nodes` → verify metrics-server installed
2. Curl `http://<operator-pod-ip>:8080/metrics` → verify Prometheus metrics exposed
3. Simulate node failure: `kubectl drain x509fj --ignore-daemonsets --delete-emptydir-data`
4. Verify Telegram alert received within 30s

**After Phase 4 (Apps):**
1. Test Minecraft connectivity: join server, play for 5min, shutdown x509fj mid-game
2. Verify minimal disruption (< 30s disconnect), server resumes on Mac Mini
3. Access OpenWebUI from Hanoi with latency < 50ms, access from Melbourne with acceptable latency
4. Check AdGuard Home query logs → DNS working from all regions

---

### Decisions & Assumptions

**Storage decision**: Longhorn recommended over NFS because:
- Distributed replication (3 replicas across nodes)
- Automatic failover when node dies
- No single point of failure (NFS server would be SPOF)
- Good performance for block storage needs (Minecraft world files)

**Why HA cluster instead of single-server promotion**:
- K3s cannot promote agent → server at runtime (requires reinstall)
- HA mode provides zero-downtime failover via etcd leader election
- Control plane automatically fails over without operator intervention
- Operator focuses on workload placement, not control plane management

**Geographic awareness approach**:
- Node labels + custom scheduler logic (not full custom scheduler, just pod affinity injection)
- Avoid scheduling cross-region dependencies (e.g., OpenWebUI frontend in Hanoi, backend in Melbourne)
- Accept that etcd replication will have latency between Hanoi and Melbourne (~100-300ms), but etcd handles this

**Power failure handling**:
- No automatic power-on mechanism (would require hardware IPMI/WoL integration)
- Operator assumes node is "down" after 30s missed heartbeats
- When x509fj comes back online, operator gradually migrates workloads back (prefer primary tier)
- User can manually trigger immediate migration via FailoverPolicy CRD updates

**Minecraft server failover**:
- Use Longhorn PVC for world data (automatically mounts on new node)
- Accept 30-60s downtime during failover (pod reschedule + PV mount time)
- Not true hot-standby (e.g., BungeeCord multi-server setup) - that's Phase 5 if needed

**Resource constraints on Pi 400**:
- 4GB RAM limit, avoid heavy workloads
- Mark Pi 400 with `tier=tertiary` taint
- Only schedule lightweight workloads or during emergency (both Hanoi nodes down)
- Acceptable workloads: AdGuard Home (DNS), SearXNG (search), maybe downscaled OpenWebUI

**SOLID architecture decision**:
- **Interface-based design with dependency injection** (production-grade approach)

### Delta Updates (March 8, 2026)

**Platform compatibility strategy (k3s now, kubeadm later):**
- Operator must use only upstream Kubernetes APIs (`apps/v1`, `core/v1`, `policy/v1`) and avoid k3s-only APIs.
- Add `pkg/platform` abstraction for distro differences:
  - `Detector` interface: detects runtime (`k3s`, `kubeadm`) from node labels/version strings.
  - `Capability` interface: feature flags for ingress, serviceLB behavior, default CNI assumptions.
- Keep CRDs distro-agnostic; no k3s-specific fields in schema.
- Migration path requires no CRD/spec changes, only platform adapter switches.

**Network constraints confirmed:**
- All inter-node traffic goes through ZeroTier interfaces only.
- Fixed ZeroTier IP mapping is treated as source of truth:
  - x509fj: `192.168.86.8`
  - mac mini m4: `192.168.86.4`
  - pi 400: `192.168.86.40`
- Add preflight validation in operator startup for ZeroTier reachability matrix.

**Public exposure constraints confirmed:**
- No NAT from ISP; public services exposed via Cloudflare Tunnel.
- Recommended default: single shared tunnel with hostname-based routing for easier operations.
- Fallback mode when Cloudflare is down: VPN-only access.
- Operator scope: monitor tunnel health and alert; tunnel lifecycle automation is optional phase-2.

**Control-plane latency risk update:**
- User does not accept high etcd-latency impact.
- New recommended topology options (must choose one before implementation):
  1. Keep all voting control-plane members in Hanoi; Melbourne runs as worker-only.
  2. Use external datastore/witness near Hanoi and keep Melbourne non-voting for control plane.
- Current plan item "2 Hanoi + 1 Melbourne voting member" is now marked high-risk and not preferred.

**Operations reality constraints:**
- x509fj and mac mini m4 are not guaranteed always-on.
- This creates a correlated power-domain risk in Hanoi; zero-downtime objective may be violated during dual outage.
- Plan must include degraded mode policy for Pi 400 emergency-only workloads.

**Documentation constraints reinforced:**
- Only `README.md` is allowed as markdown documentation file.
- All code comments and developer-facing messages must be in English.
- No emoji in comments, logs, alerts, docs.

- Controllers receive dependencies via constructor (no global state, testable)
- Plugin system for health checkers (register at startup, extensible without code modification)
- Factory pattern for notifiers (enable/disable via config)
- Strategy pattern for placement logic (compose multiple strategies)

### Final Decisions Captured (March 8, 2026)

- **Topology chosen**: Keep `2 Hanoi + 1 Melbourne` as voting control-plane members, accepting latency risk.
- **Cloudflare Tunnel layout**: `cloudflared` runs on host (outside cluster), not in-cluster.
- **Storage strategy**: Longhorn with 3 replicas.
- **Security baseline (phase 1)**:
  - Secrets not hardcoded; injected via env/Secret.
  - Basic audit logging enabled.
- **Upgrade policy**: Rolling upgrades with canary workloads.
- **Certificate/token policy**: Fully automatic rotation.

### End-to-End Blind Spot Register

1. **Power-domain correlation in Hanoi**
   - x509fj and mac mini can both lose power; this can collapse quorum and write availability.
   - Mitigation: documented degraded mode, startup order, and emergency recovery runbook.

2. **Cross-region etcd latency and election instability**
   - Voting member in Melbourne can increase leader election churn and API tail latency.
   - Mitigation: tune etcd/election timeouts, monitor raft metrics, define fail-open/fail-safe thresholds.

3. **ZeroTier single-overlay dependency**
   - If ZeroTier control plane or local agent fails, all node-to-node traffic breaks.
   - Mitigation: health probes for ZeroTier interface, automated alerting, manual fallback SOP.

4. **Cloudflare Tunnel external dependency**
   - Public ingress depends on Cloudflare account/tunnel availability.
   - Mitigation: explicit VPN-only fallback mode, clear DNS failover expectations.

5. **Host-level cloudflared drift**
   - Running cloudflared outside cluster risks config drift and unknown version skew.
   - Mitigation: systemd-managed version pinning, config checksum checks, periodic conformance checks.

6. **Longhorn under intermittent nodes**

### Additional Blind Spot Answers (March 8, 2026)

- **Time sync status**: Unknown.
  - Action: enforce NTP baseline before HA tuning.
- **ZeroTier MTU testing**: Not tested.
  - Action: add MTU/MSS validation and throughput test in preflight checklist.
- **Offsite backup target**: Not decided.
  - Action: propose S3-compatible target as default and wire snapshot export hooks.
- **Restore drill cadence**: currently not desired.
  - Risk: unverified recovery path during real incidents.
- **Cloudflare DNS strategy**: subdomain-per-app.
  - Action: define one hostname per service with clear ownership and TTL policy.
- **Break-glass admin access**: Yes, via ZeroTier + SSH bastion policy.
  - Action: document bastion hardening and emergency runbook in README.

   - Replica rebuild storms when unstable nodes flap; IO pressure can impact workloads.
   - Mitigation: anti-affinity, rebuild rate limits, disk reservation thresholds.

7. **Stateful consistency expectations**
   - "Best-effort" RPO/RTO may still surprise for Minecraft world integrity.
   - Mitigation: periodic snapshots, restore drills, integrity checks after failover.

8. **Secret lifecycle and rotation blast radius**
   - Auto-rotation can break integrations if rollout orchestration is incomplete.
   - Mitigation: staged rotation, dual-token overlap window, rollback token path.

9. **Upgrade path drift (k3s -> kubeadm)**
   - Hidden k3s assumptions in manifests/charts can block migration later.
   - Mitigation: platform-agnostic API usage gate in CI and compatibility tests.

10. **Observability gap**
    - "kubectl logs/events only" delays root-cause analysis during cross-site failures.
    - Mitigation: minimum event retention and structured log correlation IDs in operator.

11. **Failure testing frequency too low**
    - "Test when needed" can leave unvalidated recovery paths.
    - Mitigation: lightweight quarterly disaster rehearsal at minimum.

12. **Resource asymmetry (Pi 400)**
    - Emergency placement to Pi can silently degrade app behavior.
    - Mitigation: strict placement constraints and emergency-only profile definitions.

- Clear separation: api/, controllers/, pkg/ (domain logic), cmd/ (wiring)
- Testability: mock interfaces for unit tests, real implementations for E2E

**Code style requirements**:
- **All comments in English** (no Vietnamese, no other languages)
- **No emoji anywhere** (not in comments, logs, errors, notifications)
- **SOLID principles strictly enforced** (interfaces, single responsibility, dependency injection)
- Go standard formatting: `gofmt`, `golint`, `go vet`
- Error handling: explicit errors, no panics in business logic
- Logging: structured logging with levels (use logr from controller-runtime)

**Documentation constraints**:
- **Only README.md allowed** (no other markdown files in repo)
- .env configuration separate from code
- All setup guides, architecture docs consolidated in README.md
- Example YAMLs in deployments/examples/ directory
- Inline code comments for complex logic only

---

### Further Considerations

1. **Network latency monitoring**: Should operator measure real-time latency Hanoi ↔ Melbourne and adjust geographic preferences dynamically? Or static labels sufficient?
   - **Recommendation**: Start with static labels (simpler), add dynamic latency measurement in Phase 5 if needed.

2. **Backup strategy for stateful data**: Longhorn provides snapshots, but should operator automate daily backups to external storage (e.g., S3, Backblaze B2)?
   - **Recommendation**: Phase 5 feature - add CRD `BackupSchedule` to trigger Longhorn snapshots + offsite upload.

3. **Cost optimization**: Pi 400 in Melbourne likely has higher power cost (or colocation fee). Should operator have "power budget" awareness to minimize Pi usage?
   - **Recommendation**: Not needed for home lab, but add `power-cost` label if you want operator to prefer cheaper nodes.



### Resolved Defaults From User (March 8, 2026)

1. **Backup destination (resolved)**
- No external backup target for now.
- Primary backup target: `x509fj:/home/oiviadesu` due to largest storage capacity.
- Add a warning in README: this is not offsite backup and does not protect against site-wide Hanoi outage.

2. **NTP/time sync (default selected by assistant)**
- Standardize all nodes to `chrony`.
- Use `time.cloudflare.com` + `pool.ntp.org` as upstream sources.
- Add preflight check: clock skew must be <= 200 ms between nodes.

3. **ZeroTier MTU/MSS baseline (default selected by assistant)**
- Start with interface MTU `2800` (ZeroTier common baseline), validate path MTU.
- Add MSS clamping on host firewall for TCP overlays.
- Acceptance criteria:
  - Packet loss < 1% over 10-minute iperf test
  - Stable throughput for backup stream
  - No recurrent disconnect for Minecraft session test (15 minutes)

4. **Auto rotation policy (default selected by assistant)**
- Certificates: use `cert-manager` with automatic renewal at 2/3 lifetime.
- Tokens/secrets: rotate every 90 days with overlap window 7 days.
- Rollout strategy: canary secret update, then full rollout.

5. **Dual-Hanoi outage degraded mode (default selected by assistant)**
- If both x509fj and mac mini are down:
  - Pi 400 enters emergency profile.
  - Only critical lightweight services run (AdGuard, status page, minimal API).
  - Heavy workloads (Minecraft, OpenWebUI full profile) are paused.
- Recovery order when Hanoi returns:
  1) restore control-plane quorum
  2) restore storage health
  3) resume heavy workloads

### Blind Spot Closures

- Backup blind spot is partially closed with local target on x509fj, but remains **single-site risk**.
- Time sync, MTU/MSS, and rotation policies now have concrete defaults and measurable acceptance gates.
- Degraded-mode behavior is now explicit for worst-case Hanoi power loss.

### Open Risk (must accept explicitly)

- Because backup stays in Hanoi (`x509fj`), catastrophic Hanoi outage can still cause unrecoverable data loss.
- Recommendation for Phase 2: replicate snapshots from `x509fj` to offsite object storage.
