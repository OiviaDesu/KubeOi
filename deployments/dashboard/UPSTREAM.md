# Kubernetes Dashboard upstream snapshot

This directory vendors a repo-managed snapshot of the upstream Kubernetes Dashboard Helm chart.

- Dashboard chart: `kubernetes-dashboard` `7.14.0`
- Dashboard images:
  - `docker.io/kubernetesui/dashboard-api:1.14.0`
  - `docker.io/kubernetesui/dashboard-auth:1.4.0`
  - `docker.io/kubernetesui/dashboard-web:1.7.0`
  - `docker.io/kubernetesui/dashboard-metrics-scraper:1.2.2`
- Gateway dependency rendered from upstream chart:
  - Kong chart `2.52.0`
  - `kong:3.9`

How the snapshot was produced:

```bash
helm dependency build /tmp/kubernetes-dashboard-upstream/charts/kubernetes-dashboard
helm template kubernetes-dashboard /tmp/kubernetes-dashboard-upstream/charts/kubernetes-dashboard \
  --namespace kubernetes-dashboard \
  --set nginx.enabled=false \
  --set cert-manager.enabled=false \
  --set metrics-server.enabled=false \
  --set app.ingress.enabled=false
```

Local repo adjustments:

- `Namespace` is managed separately in `namespace.yaml`.
- The runtime CSRF secret is not committed; operators must create `kubernetes-dashboard-csrf` themselves before rollout.
- No sample admin `ServiceAccount`, token, or `ClusterRoleBinding` is committed.
- The public Oiviak3s entrypoint is modeled separately under `deployments/examples/` as a TCP passthrough gateway so the operator can keep managing a single public workload.
