# sandbox-dashboard

Lightweight read-only operational dashboard for [kubernetes-sigs/agent-sandbox](https://github.com/kubernetes-sigs/agent-sandbox).
Aggregates `Sandbox`, `SandboxClaim`, `SandboxTemplate`, and `SandboxWarmPool` status across a cluster, plus Prometheus-backed charts for the controller's existing latency / rate metrics.

## What it does

- **Overview** of all four CRDs with per-phase counts.
- **List + detail drawer** per resource kind, with status conditions, spec (YAML), and recent events.
- **Metrics page** with four charts (sandbox creation latency p50/p95, claim startup latency p50/p95, claim controller startup latency p50/p95, claim creation rate). Backed by a whitelisted Prometheus proxy — the SPA never sends raw PromQL.
- Read-only RBAC. No write actions, no in-app auth (delegated to whatever ingress is in front).

## Install

The shipped manifests deploy into the `default` namespace. Apply the kustomize base:

```bash
kubectl apply -k deploy/kustomize/
```

To use a different namespace, override it in an overlay (`namespace: your-ns`) — the RBAC is a ClusterRole, so nothing else needs changing.

Then port-forward to see it:

```bash
kubectl port-forward -n default svc/sandbox-dashboard 8080:80
open http://localhost:8080
```

The dashboard exposes:
- `/healthz`, `/readyz` — for kubelet probes (already wired into the Deployment).
- `/api/v1/overview`, `/api/v1/{sandboxes,claims,templates,warmpools}` — read-only JSON.
- `/api/v1/metrics/{name}` — whitelisted Prometheus proxy (503 when Prometheus is unconfigured).
- `/` — the embedded SPA.

### Configuring Prometheus (optional)

The shipped Deployment leaves `PROMETHEUS_URL` unset, so `/metrics` returns 503 and the SPA renders a "Prometheus is not configured" placeholder. To enable charts, point the dashboard at a Prometheus reachable from inside the cluster.

Create your own overlay:

```bash
mkdir -p deploy/kustomize/overlays/my-cluster
cat > deploy/kustomize/overlays/my-cluster/kustomization.yaml <<'EOF'
apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization

resources:
  - ../../   # the base

patches:
  - target:
      kind: Deployment
      name: sandbox-dashboard
    patch: |
      - op: add
        path: /spec/template/spec/containers/0/env
        value:
          - name: PROMETHEUS_URL
            value: "http://prometheus.monitoring.svc:9090"
EOF
kubectl apply -k deploy/kustomize/overlays/my-cluster
```

If Prometheus is unreachable from a configured URL, the dashboard returns 502 problem+json and the SPA renders an inline error — the rest of the UI keeps working.

### Configuring OpenSandbox (optional)

When sandboxes are created by [OpenSandbox](https://github.com/open-sandbox), the dashboard
can show OpenSandbox's own lifecycle state next to the Kubernetes Ready condition. The two
can disagree — a sandbox whose pod is `Ready` may still be `Pending` to OpenSandbox — and
that disagreement is worth seeing.

```bash
OPENSANDBOX_URL=http://opensandbox-server.default.svc \
OPENSANDBOX_API_KEY=$(kubectl -n default get secret opensandbox-server-api-key \
  -o jsonpath='{.data.api-key}' | base64 -d) \
./dashboard --kubeconfig=$HOME/.kube/config
```

Sandboxes are matched to OpenSandbox records on the `opensandbox.io/id` label, not the
resource name. The list gains three things:

- a **Creator** column, so sandboxes from other creators stay legible
- an **OSB State** column, marked `⚠` when it disagrees with the Ready condition
- a **stale** marker (`⏱`) when OpenSandbox has not advanced a transient state within
  `AGENT_SANDBOX_DASHBOARD_OSB_STALE_AFTER` (default `60s`). Filter with `?stale=true`.

Both are optional and degrade quietly: with `OPENSANDBOX_URL` unset the columns disappear,
and if OpenSandbox is unreachable the list still serves Kubernetes data with a warning.

| Env | Default | Purpose |
|---|---|---|
| `OPENSANDBOX_URL` | unset | Base URL; unset disables the integration |
| `OPENSANDBOX_API_KEY` | unset | Sent as the `OPEN-SANDBOX-API-KEY` header |
| `AGENT_SANDBOX_DASHBOARD_OSB_TTL` | `5s` | How long the inventory is cached |
| `AGENT_SANDBOX_DASHBOARD_OSB_STALE_AFTER` | `60s` | Staleness threshold |

### Exposing the dashboard

The Service is `ClusterIP`. Wrap it in your existing ingress / IAP / oauth2-proxy stack. The dashboard ships with no built-in auth on purpose — that's an operator responsibility. Do not expose it directly via a public LoadBalancer.

### Private image

While the GHCR package is private (default while the repo is private), pulls require an `imagePullSecret`. Create one and uncomment the `imagePullSecrets` block in `deploy/kustomize/deployment.yaml`:

```bash
kubectl create secret docker-registry ghcr-pull \
  --namespace=default \
  --docker-server=ghcr.io \
  --docker-username=<your-gh-username> \
  --docker-password=<a-classic-PAT-with-read:packages>
```

Once the image package is flipped to public, leave the `imagePullSecrets` line commented out.

## Development

```bash
# unit tests
make test

# Go vet + gofmt over cmd/ and internal/
make lint

# envtest integration test (needs setup-envtest on PATH)
make test-integration

# build the binary with embedded SPA
make build
./dashboard --kubeconfig=$HOME/.kube/config
```

For UI hot-reload during development:

```bash
# terminal 1 — backend with API + watch
./dashboard --kubeconfig=$HOME/.kube/config

# terminal 2 — Vite dev server, proxies /api → :8080
cd ui && npm run dev
```

Open http://localhost:5173.

Local container build:

```bash
make docker
docker run --rm -p 8080:8080 -v ~/.kube/config:/kubeconfig \
  sandbox-dashboard:local --kubeconfig=/kubeconfig
```

## Architecture

Single Go binary. Embeds a controller-runtime read-only Manager (`get`/`list`/`watch` informers for the four CRDs + Pods + Events) and an HTTP server. The SPA (React + Vite + Tailwind) is built and embedded via `embed.FS`; one container image, one Deployment. All API reads go through the informer cache so the kube-apiserver only sees the long-running watches.

The Prometheus integration is a *soft dependency*: the dashboard runs without it; the metrics page surfaces the unconfigured state per-chart.

## Status

`v0.1.0` — first general-availability release.

## License

Apache-2.0
