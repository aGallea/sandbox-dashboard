<h1 align="center">sandbox-dashboard</h1>

<p align="center">
  <strong>Read-only operational dashboard for <a href="https://github.com/kubernetes-sigs/agent-sandbox">kubernetes-sigs/agent-sandbox</a></strong>
</p>

<p align="center">
  <a href="#readme">
    <img src="https://img.shields.io/badge/README-blue?style=for-the-badge" alt="README" />
  </a>
  <a href="deploy/helm/sandbox-dashboard/README.md">
    <img src="https://img.shields.io/badge/HELM_CHART-555?style=for-the-badge" alt="Helm chart" />
  </a>
  <a href="CONTRIBUTING.md">
    <img src="https://img.shields.io/badge/CONTRIBUTING-555?style=for-the-badge" alt="Contributing" />
  </a>
  <a href="CHANGELOG.md">
    <img src="https://img.shields.io/badge/CHANGELOG-555?style=for-the-badge" alt="Changelog" />
  </a>
</p>

<div align="center">
  <a href="https://github.com/aGallea/sandbox-dashboard/actions/workflows/ci.yml">
    <img src="https://github.com/aGallea/sandbox-dashboard/actions/workflows/ci.yml/badge.svg?branch=main" alt="CI" />
  </a>
  <a href="LICENSE">
    <img src="https://img.shields.io/badge/License-Apache_2.0-blue.svg" alt="License: Apache-2.0" />
  </a>
  <a href="https://github.com/aGallea/sandbox-dashboard/releases/latest">
    <img src="https://img.shields.io/github/v/release/aGallea/sandbox-dashboard" alt="Latest Release" />
  </a>
  <a href="https://go.dev/">
    <img src="https://img.shields.io/badge/Go-1.26%2B-00ADD8.svg" alt="Go 1.26+" />
  </a>
  <img src="https://img.shields.io/badge/Kubernetes-1.31%2B-326CE5" alt="Kubernetes 1.31+" />
</div>

---

It answers the two questions a cluster owner actually has about a sandbox fleet: **is anything
wrong**, and **is the fleet worth what it reserves**. One Go binary with the SPA embedded, no
database, no write permissions anywhere.

## Install

Requires an existing [agent-sandbox](https://github.com/kubernetes-sigs/agent-sandbox) install —
this reads its CRDs, it does not create them.

**Helm** (recommended):

```bash
helm install sandbox-dashboard oci://ghcr.io/agallea/charts/sandbox-dashboard \
  --namespace default
```

**Plain YAML**, no Helm — generated from the same chart, same namespace:

```bash
kubectl apply -f https://raw.githubusercontent.com/aGallea/sandbox-dashboard/main/deploy/install.yaml
```

**Locally against your kubeconfig**, no cluster install at all:

```bash
docker run --rm -p 8080:8080 -v ~/.kube/config:/kubeconfig:ro \
  ghcr.io/agallea/sandbox-dashboard:latest --kubeconfig=/kubeconfig
```

Then:

```bash
kubectl port-forward -n default svc/sandbox-dashboard 8080:80
open http://localhost:8080
```

Both optional integrations are off by default; the dashboard runs without either and each page
says which one is missing rather than hiding a control or failing. To wire both up, start from the
worked example — CI renders it on every pull request, so it cannot drift from the chart:

```bash
helm install sandbox-dashboard oci://ghcr.io/agallea/charts/sandbox-dashboard \
  -n default \
  -f deploy/helm/sandbox-dashboard/values-example.yaml   # edit the URLs first
```

| | Off (default) | On |
|---|---|---|
| **Prometheus** | The metrics page says so once, with the command to enable it; the overview's reserved-against-used panel points at `PROMETHEUS_URL`. | Ten charts, plus live usage in the overview's meters and footprint bars. |
| **OpenSandbox** | No OSB State column, no "stale only" filter, no OpenSandbox section in the drawer. `Creator` and `Session` still work — they come from labels. | OSB State with divergence and staleness markers, the stale filter, and OpenSandbox diagnostics in the drawer. |

Every value the chart accepts is documented in
[`deploy/helm/sandbox-dashboard/README.md`](deploy/helm/sandbox-dashboard/README.md). Users of
kustomize can treat `deploy/install.yaml` as a base and patch it:

```yaml
resources:
  - https://raw.githubusercontent.com/aGallea/sandbox-dashboard/main/deploy/install.yaml
patches:
  - ...
```

## What it shows

- **Overview** — per-CRD counters, a triage strip of what needs acting on, one cell per sandbox
  ordered by age, reserved-against-used meters, share and runtime breakdowns, and a per-group
  resource footprint.
- **List + detail drawer** per resource kind, with status conditions, spec (YAML), recent events,
  and — for sandboxes, when configured — OpenSandbox's own lifecycle state alongside the
  Kubernetes Ready condition.
- **Metrics** — ten charts in three sections: fleet size and reserved-against-used over time,
  controller reconcile latency / throughput / queue wait, and the claim-path latencies.

HTTP surface:

| Path | |
|---|---|
| `/` | the embedded SPA |
| `/healthz`, `/readyz` | probes; `/readyz` stays 503 until every informer has synced |
| `/api/v1/overview` | per-CRD counts |
| `/api/v1/{sandboxes,claims,templates,warmpools}` | read-only JSON, one row per resource |
| `/api/v1/sandboxes/{namespace}/{name}` | detail, conditions, spec, events |
| `/api/v1/sandboxes/{namespace}/{name}/osb` | OpenSandbox diagnostics for one sandbox |
| `/api/v1/sandboxes/{namespace}/{name}/logs` | the pod's log lines as plain text; `?tail=N`, `?head=N`, `?container=` |
| `/api/v1/metrics` | the chart catalog |
| `/api/v1/metrics/{name}` | whitelisted Prometheus proxy — the SPA never sends PromQL |
| `/api/v1/usage` | live CPU/memory per sandbox pod, keyed `namespace/pod` |

## Configuration

Every setting is an environment variable, so the chart, a plain Deployment patch and a local
binary all configure it the same way. Four also have flags, which win over the environment.

| Env | Flag | Default | Purpose |
|---|---|---|---|
| `AGENT_SANDBOX_DASHBOARD_LISTEN_ADDR` | `--listen-addr` | `:8080` | Bind address. A bare port is accepted. |
| `AGENT_SANDBOX_DASHBOARD_WATCH_NAMESPACES` | `--watch-namespaces` | every namespace | Comma-separated namespaces to watch. See [Scope](#scope). |
| `AGENT_SANDBOX_DASHBOARD_LOG_LEVEL` | `--log-level` | `info` | `debug`, `info`, `warn` or `error`. Probe requests (`/healthz`, `/readyz`) are logged at `debug`. |
| `PROMETHEUS_URL` | `--prometheus-url` | unset | Base URL. Unset disables the metrics page and the usage panels. |
| `OPENSANDBOX_URL` | `--opensandbox-url` | unset | Base URL. Unset disables the OpenSandbox fields. |
| `OPENSANDBOX_API_KEY` | — | unset | Sent as the `OPEN-SANDBOX-API-KEY` header. Env-only, so it can come from a Secret. |
| `AGENT_SANDBOX_DASHBOARD_CONTROLLER_JOB` | — | `agent-sandbox-controller` | Prometheus `job` label of the agent-sandbox controller. |
| `AGENT_SANDBOX_DASHBOARD_METRICS_TIMEOUT` | — | `10s` | Bounds one Prometheus range query. |
| `AGENT_SANDBOX_DASHBOARD_OSB_TTL` | — | `5s` | How long the OpenSandbox inventory is cached. |
| `AGENT_SANDBOX_DASHBOARD_OSB_STALE_AFTER` | — | `60s` | How long a transient OpenSandbox state may sit before it is reported stale. |
| `AGENT_SANDBOX_DASHBOARD_OSB_TIMEOUT` | — | `5s` | Bounds one OpenSandbox inventory fetch. |
| — | `--kubeconfig` | in-cluster | Registered by controller-runtime. Omit inside a cluster. |

### Scope

By default the dashboard watches every namespace, which needs a ClusterRole — and that grants
`get`/`list`/`watch` on **pods and events cluster-wide**, plus `get` on `pods/log` for the log viewer. On a shared cluster it can read every
pod spec in every namespace.

If sandboxes live in known namespaces, say so and it reads only those:

```bash
helm upgrade --install sandbox-dashboard oci://ghcr.io/agallea/charts/sandbox-dashboard \
  --namespace default \
  --set 'watchNamespaces={default}'
```

The chart derives the RBAC from the same value, swapping the ClusterRole for a Role and
RoleBinding per namespace. They have to agree: a Role with cluster-wide informers fails closed
but late — the list calls 403, the cache never syncs, and `/readyz` stays 503 with the cause
only in the logs. One value cannot be set inconsistently; two could.

The namespaces must already exist. A narrowed install shows a partial fleet with nothing in the
UI saying so — the startup log and the install notes name the scope.

## Prometheus (optional)

```bash
helm upgrade --install sandbox-dashboard oci://ghcr.io/agallea/charts/sandbox-dashboard \
  --set prometheus.url=http://prometheus.monitoring.svc:9090
```

Without it, `/api/v1/metrics/*` and `/api/v1/usage` return 503 problem+json and the UI renders
the unconfigured state per panel. If Prometheus is configured but unreachable the dashboard
returns 502 and the rest of the UI keeps working.

Two things decide how much of the metrics page fills in:

- **The claim-path charts only have data for claim-based launches.** The controller records those
  histograms from its SandboxClaim reconciler alone, and they are `HistogramVec`s — with no
  observation there is no series at all. A fleet that creates `Sandbox` objects directly (as
  OpenSandbox does) leaves all three empty, and the page says "No samples in this range" rather
  than drawing empty axes.
- **The fleet resource charts need a kube-state-metrics that exposes pod labels.** They scope
  cAdvisor to sandbox pods by joining `kube_pod_labels` on the controller's
  `agents.x-k8s.io/sandbox-name-hash` label, which is exact where matching by namespace would
  not be. Without those label series the charts report no samples instead of a wrong number.

The controller charts are scoped by scrape job because `controller_runtime_*` is exported by
every controller-runtime binary in the cluster. Override with
`--set prometheus.controllerJob=<your job label>` if yours differs.

## OpenSandbox (optional)

When sandboxes are created by [OpenSandbox](https://github.com/open-sandbox), the dashboard can
show OpenSandbox's own lifecycle state next to the Kubernetes Ready condition. The two can
disagree — a sandbox whose pod is `Ready` may still be `Pending` to OpenSandbox — and that
disagreement is worth seeing.

```bash
helm upgrade --install sandbox-dashboard oci://ghcr.io/agallea/charts/sandbox-dashboard \
  --set openSandbox.url=http://opensandbox-server.default.svc \
  --set openSandbox.existingSecret=opensandbox-server-api-key
```

Or against a local binary:

```bash
OPENSANDBOX_URL=http://localhost:8090 \
OPENSANDBOX_API_KEY=$(kubectl -n default get secret opensandbox-server-api-key \
  -o jsonpath='{.data.api-key}' | base64 -d) \
./dashboard --kubeconfig=$HOME/.kube/config
```

Sandboxes are matched to OpenSandbox records on the `opensandbox.io/id` label, not the resource
name. Measured against one production fleet the label matched 102/102 records while the CR name
matched only 29/92, because OpenSandbox writes both `<uuid>` and `sandbox-<uuid>`.

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
  `AGENT_SANDBOX_DASHBOARD_OSB_STALE_AFTER`

plus a sibling `osb` object on the response reporting `status`, `reported` and `matched`.
Filter with `?creator=`, `?osbState=` and `?stale=true`. The sandbox list shows these as
**Creator**, **Session**, **Owner** and **OSB State** columns, with `⚠` when OpenSandbox
disagrees with the Ready condition and `⏱` plus the state's age when a transient OpenSandbox
state has stopped advancing.

With `OPENSANDBOX_URL` unset the `osb` fields are absent entirely and the rest of the row is
unaffected; if OpenSandbox is unreachable the list still returns 200 with Kubernetes data plus
`osb.status: "unreachable"`. Note that `?stale=true` and `?osbState=` return an empty list in
that state — check `osb.status` before reading an empty result as "nothing is stale".

## Exposing it

The Service is `ClusterIP` and the dashboard ships **no authentication of its own**, on purpose:
it is read-only, and every cluster already has an opinion about ingress auth. Wrap it in your
existing ingress / IAP / oauth2-proxy stack — `values.yaml` carries commented annotation sets for
each. Do not put it behind a public LoadBalancer without one.

## Development

```bash
make test              # Go unit tests, race detector on
make lint              # go vet + gofmt over cmd/ and internal/
make ui-test           # UI unit tests (vitest) + eslint
make test-integration  # envtest (needs setup-envtest on PATH)
make helm-lint         # helm lint + render the chart under several value sets
make manifests         # regenerate deploy/install.yaml from the chart
make build             # binary with the SPA embedded
```

For UI hot reload, run the backend and the Vite dev server side by side:

```bash
# terminal 1 — API on :8080
./dashboard --kubeconfig=$HOME/.kube/config

# terminal 2 — Vite on :5173, proxying /api to :8080
cd ui && npm run dev
```

`deploy/install.yaml` is generated. Change the chart and run `make manifests`; CI fails if the
two disagree.

## Architecture

Single Go binary. It embeds a controller-runtime read-only Manager (`get`/`list`/`watch`
informers for the four CRDs plus Pods and Events) and an HTTP server. The SPA (React + Vite +
Tailwind) is built and embedded via `embed.FS` — one image, one Deployment. Every API read goes
through the informer cache, so the kube-apiserver only ever sees the long-running watches.

Both integrations are *soft dependencies*, and the failure boundaries are deliberate:

- `/api/v1/usage` sits outside the sandbox list. Joining live usage into the list would put
  Prometheus in the request path of the main table, so one outage would cost the whole page
  instead of one panel. Its queries are scoped to the namespaces sandboxes actually live in — on
  a 1500-pod cluster holding 169 sandboxes in one namespace, that is 267 series in 0.7s rather
  than 1527 in 1.6s.
- OpenSandbox failures degrade rows, never requests: the Kubernetes data is still worth serving,
  so the list returns 200 with `osb.status: "unreachable"`.
- Non-finite Prometheus samples are dropped rather than charted. A `histogram_quantile` over a
  window with no observations is NaN, which is not a reading — and JSON cannot encode it.

## Status

`v0.4.0`. The chart version and `appVersion` track the git tag, so `v0.4.0` publishes chart 0.4.0 pointing at image 0.4.0.

## Contributing

Contributions are welcome. See [CONTRIBUTING.md](CONTRIBUTING.md) for the dev setup, the test
and lint commands, and how to work on the Helm chart. Please read the
[Code of Conduct](CODE_OF_CONDUCT.md) before opening an issue or a PR.

Security issues go to the address in [SECURITY.md](SECURITY.md), not to a public issue.

## License

Apache-2.0 — see [LICENSE](LICENSE).
