# sandbox-dashboard

Lightweight read-only operational dashboard for [kubernetes-sigs/agent-sandbox](https://github.com/kubernetes-sigs/agent-sandbox).
Aggregates `Sandbox`, `SandboxClaim`, `SandboxTemplate`, and `SandboxWarmPool` status across a cluster, plus Prometheus-backed charts for the controller's existing latency / rate metrics.

## What it does

- **Overview** of the whole fleet: per-CRD counters, a triage strip, one cell per sandbox ordered by age, reserved-against-used meters, share and runtime breakdowns, and a per-group resource footprint.
- **List + detail drawer** per resource kind, with status conditions, spec (YAML), recent events, and — for sandboxes, when configured — OpenSandbox's own lifecycle state alongside the Kubernetes Ready condition.
- **Metrics page** with ten charts in three sections — fleet size and reserved-against-used over time, controller reconcile latency / throughput / queue wait, and the claim-path latencies. Backed by a whitelisted Prometheus proxy — the SPA never sends raw PromQL.
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
- `/api/v1/usage` — live CPU/memory use per sandbox pod, keyed `namespace/pod` (503 when Prometheus is unconfigured, 502 when it is unreachable).
- `/api/v1/sandboxes/{namespace}/{name}/osb` — OpenSandbox's own diagnostics for one sandbox (404 when the sandbox was not created by OpenSandbox, 503 when OpenSandbox is unconfigured).
- `/` — the embedded SPA.

### The overview page

The counters come from `/api/v1/overview`; everything else is rolled up in the browser from
the same `/api/v1/sandboxes` response the list page fetches, so slicing the fleet a new way
costs a function rather than an endpoint. Past a few thousand sandboxes, move the rollups
server-side.

The **Group by** control is not a fixed list. Every label key the fleet carries becomes a
candidate dimension alongside image, node, namespace, request size, readiness, OSB state and
creator, and each is kept only if it actually divides the fleet: at least two distinct values,
and no more than one value per two sandboxes. That single rule means a cluster stamping `team=`
gets a Team dimension with no configuration, while a key that is unique per sandbox
(`session_id`, `opensandbox.io/id`) or has one value for everything is dropped instead of
drawing a chart with 168 slices of one. Dimensions that split the fleet into six parts or fewer
render as a ring; longer tails render as a ranked list of the largest five with the rest folded
into Other. The selection lives in the URL (`?by=node`), so a view can be shared.

**Reserved against used** and the used half of the footprint bars need Prometheus. Without it
those panels say so and the rest of the page is unaffected.

### The metrics page

Three sections, served from the registry in `internal/prom/registry.go` via
`GET /api/v1/metrics` so the SPA holds no second copy of the list:

| Section | Charts | Source |
|---|---|---|
| Fleet | sandboxes (ready / not ready), expired sandboxes, CPU and memory reserved-against-used | the controller's `agent_sandboxes` gauge; cAdvisor and kube-state-metrics for the pods |
| Controller | reconcile latency p50/p95, reconcile throughput and errors, work queue wait p95 | `controller_runtime_*` and `workqueue_*` |
| Claims | claim startup latency, sandbox creation latency, claim creation rate | `agent_sandbox_*` histograms |

Two things are worth knowing before reading an empty chart:

- **The claim charts only fill in for claim-based launches.** The controller records those
  histograms from its SandboxClaim reconciler alone, and they are `HistogramVec`s — with no
  observation there is no series at all. A fleet that creates `Sandbox` objects directly (as
  OpenSandbox does) leaves all three empty, and the page says so rather than drawing empty axes.
- **The fleet resource charts need a kube-state-metrics that exposes pod labels.** They scope
  cAdvisor to sandbox pods by joining `kube_pod_labels` on the controller's
  `agents.x-k8s.io/sandbox-name-hash` label, which is exact where namespace matching would not
  be. Without those label series the charts report no samples instead of a wrong number.

The controller charts are scoped by scrape job, because `controller_runtime_*` is exported by
every controller-runtime binary in the cluster. Override the job name if yours differs:

| Env | Default | Purpose |
|---|---|---|
| `AGENT_SANDBOX_DASHBOARD_CONTROLLER_JOB` | `agent-sandbox-controller` | Prometheus `job` label of the agent-sandbox controller |

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
resource name.

`creator` is derived from that label alone, so `GET /api/v1/sandboxes` reports it —
`opensandbox` when the label is present, `unknown` otherwise — whether or not OpenSandbox is
configured. The identity fields `owner`, `team`, `experiment` and `sessionId` are likewise read
from labels and need no OpenSandbox configuration, but unlike `creator` they have no fallback
value: each key is omitted when its label is absent. In practice `session_id` is the one most
reliably present.

Configuring OpenSandbox adds a per-row `osb` object:

- `osb.state` plus OpenSandbox's own reason, message and expiry
- `osb.diverged` — OpenSandbox's state disagrees with the Kubernetes Ready condition
- `osb.stale` — a transient OpenSandbox state has not advanced within
  `AGENT_SANDBOX_DASHBOARD_OSB_STALE_AFTER` (default `60s`)

plus a sibling `osb` object on the response reporting `status`, `reported` and `matched`.
Filter with `?creator=`, `?osbState=` and `?stale=true`. The sandbox list shows these as
**Creator**, **Session**, **Owner** and **OSB State** columns, with `⚠` when OpenSandbox
disagrees with the Ready condition and `⏱` plus the state's age when a transient OpenSandbox
state has stopped advancing. Filter with the creator, OSB-state and stale-only controls above
the table.

The `osb` fields are the only part that depends on configuration: with `OPENSANDBOX_URL`
unset they are absent entirely and the rest of the row is unaffected, and if OpenSandbox is
unreachable the list still returns 200 with Kubernetes data plus `osb.status: "unreachable"`.
Note that `?stale=true` and `?osbState=` return an empty list in that state — check
`osb.status` before reading an empty result as "nothing is stale".

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

The Prometheus integration is a *soft dependency*: the dashboard runs without it; the metrics page surfaces the unconfigured state per-chart, and the overview's usage panels do the same. `/api/v1/usage` deliberately sits outside the sandbox list: joining live usage into the list would put Prometheus in the request path of the main table, so one outage would cost the whole page instead of one panel. Its queries are scoped to the namespaces sandboxes actually live in — on a 1500-pod cluster holding 169 sandboxes in one namespace, that is 267 series in 0.7s rather than 1527 in 1.6s. OpenSandbox is a soft dependency the same way: the dashboard runs without it, and degrades to Kubernetes-only state rather than failing requests when it is unreachable.

## Status

`v0.1.0` — first general-availability release.

## License

Apache-2.0
