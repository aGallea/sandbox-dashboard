# sandbox-dashboard chart

Read-only operational dashboard for [kubernetes-sigs/agent-sandbox](https://github.com/kubernetes-sigs/agent-sandbox).

```bash
helm install sandbox-dashboard oci://ghcr.io/agallea/charts/sandbox-dashboard \
  --namespace default
```

Nothing is required: with no values set you get the dashboard reading Sandboxes, claims,
templates, warm pools, pods and events cluster-wide, with both optional integrations off.

## What it installs

A Deployment, a Service, a ServiceAccount, and a role plus binding — five resources, all
read-only. Optionally an Ingress, and a Secret when you pass the OpenSandbox key inline.

The role is a **ClusterRole** by default, because the dashboard aggregates across namespaces.
Its names carry the release name, so two installs in one cluster do not fight over the same
cluster-scoped object. Setting `watchNamespaces` swaps it for a Role and RoleBinding in each
listed namespace — see [Narrowing the scope](#narrowing-the-scope).

## What each integration adds

Both are off by default, and the UI never shows a control it cannot answer. Nothing is hidden
behind a flag you have to know about: install it bare, and each page tells you what a URL would
add.

| | Off (default) | On |
|---|---|---|
| **Prometheus** | The metrics page says so once, with the command to enable it; the overview's reserved-against-used panel points at `PROMETHEUS_URL`; footprint bars show reservations only. | Ten charts across fleet / controller / claims; live usage inset into the footprint bars and the overview meters. |
| **OpenSandbox** | No OSB State column, no "stale only" filter, no OpenSandbox section in the drawer. `Creator` and `Session` still work — they come from labels, not from the API. | OSB State column with divergence and staleness markers, the stale filter, OpenSandbox diagnostics in the drawer, and OSB state as a grouping dimension once the fleet holds more than one. |

Neither one failing takes the dashboard with it: Prometheus unreachable costs those panels, and
OpenSandbox unreachable leaves the rows intact with `osb.status: "unreachable"` reported.

## Turning the integrations on

Both are soft dependencies — the dashboard runs without either and says so in the UI.

```bash
helm upgrade --install sandbox-dashboard oci://ghcr.io/agallea/charts/sandbox-dashboard \
  --namespace default \
  --set prometheus.url=http://prometheus.monitoring.svc:9090 \
  --set openSandbox.url=http://opensandbox-server.default.svc \
  --set openSandbox.existingSecret=opensandbox-server-api-key
```

[`values-example.yaml`](values-example.yaml) is a fully wired install — both integrations plus an
authenticated ingress — kept honest by CI, which renders it on every pull request:

```bash
helm install sandbox-dashboard oci://ghcr.io/agallea/charts/sandbox-dashboard \
  -n default \
  -f deploy/helm/sandbox-dashboard/values-example.yaml
```

`openSandbox.existingSecret` is the recommended path: the key reaches the pod through a
`secretKeyRef`, never as an argument, so it stays out of `kubectl describe`. `openSandbox.apiKey`
exists for a quick trial and makes the chart create the Secret — but anything in values is
stored in the release, so prefer the Secret you already have.

Setting both is rejected at render time, along with a few other combinations that would install
cleanly and then serve nothing. `values.schema.json` catches typos and bad values the same way,
including a `prometheus.controllerJob` that is not a usable PromQL label value.

## Exposing it

The dashboard has **no authentication of its own** — by design, since it is read-only and every
cluster already has an opinion about ingress auth. `ingress.enabled=true` with no annotations
means anyone who can reach the host can read it, and `NOTES.txt` says so on install.
`values.yaml` carries commented annotation sets for oauth2-proxy, nginx basic auth, and GCP IAP.


## Narrowing the scope

By default the dashboard watches every namespace, which needs a ClusterRole — and
that grants `get`/`list`/`watch` on **pods and events cluster-wide**. On a shared
cluster that means it can read every pod spec in every namespace.

If sandboxes live in known namespaces, list them:

```bash
helm upgrade --install sandbox-dashboard oci://ghcr.io/agallea/charts/sandbox-dashboard \
  --namespace default \
  --set 'watchNamespaces={default}'
```

That replaces the ClusterRole with one Role and RoleBinding per namespace, so the
install can only read what it will actually show.

One value drives both the RBAC and the informers on purpose. They have to agree:
a Role with cluster-wide informers fails closed but late — the list calls 403,
the cache never syncs, and `/readyz` stays 503 with the cause buried in the
manager's logs. Two separate settings could be set inconsistently; one cannot.

Two things to know:

- **The namespaces must already exist.** Helm will not create them, and a Role
  targeting a missing namespace fails the install.
- **A narrowed install shows a partial fleet with nothing in the UI saying so.**
  The startup log and the post-install notes name the scope; the pages do not.

## Values

| Key | Default | Purpose |
|---|---|---|
| `replicaCount` | `1` | Replicas. The dashboard is stateless; each replica keeps its own informer cache. |
| `image.repository` | `ghcr.io/agallea/sandbox-dashboard` | Image. |
| `image.tag` | `""` | Defaults to the chart's `appVersion`. |
| `image.pullPolicy` | `IfNotPresent` | |
| `imagePullSecrets` | `[]` | Needed only while the GHCR package is private. |
| `nameOverride` / `fullnameOverride` | `""` | Standard naming overrides. |
| `serviceAccount.create` | `true` | Set `false` only with `serviceAccount.name`. |
| `serviceAccount.name` | `""` | Defaults to the release's full name. |
| `serviceAccount.annotations` | `{}` | For IRSA / Workload Identity. |
| `watchNamespaces` | `[]` | Namespaces to watch. Empty means every namespace, via a ClusterRole. Listing namespaces narrows the informers *and* the RBAC to a Role per namespace — see [Narrowing the scope](#narrowing-the-scope). |
| `rbac.create` | `true` | The role and binding. A ClusterRole when `watchNamespaces` is empty, otherwise a Role per namespace. Without an equivalent role the pod serves no resource. |
| `service.type` | `ClusterIP` | |
| `service.port` | `80` | Service port; the container port is `listenPort`. |
| `service.annotations` | `{}` | |
| `listenPort` | `8080` | Container port. One value drives the listener, the container port and the probes. |
| `ingress.enabled` | `false` | |
| `ingress.className` | `""` | |
| `ingress.annotations` | `{}` | Where authentication goes. See the examples in `values.yaml`. |
| `ingress.hosts` | one placeholder host | |
| `ingress.tls` | `[]` | |
| `prometheus.url` | `""` | Empty disables the metrics page and the usage panels (they report it). |
| `prometheus.controllerJob` | `agent-sandbox-controller` | Prometheus `job` label of the agent-sandbox controller. `controller_runtime_*` is exported by every controller-runtime binary in the cluster, so this is what isolates its numbers. |
| `prometheus.timeout` | `10s` | Bounds one range query. |
| `openSandbox.url` | `""` | Empty disables the integration; rows carry no OpenSandbox state. |
| `openSandbox.existingSecret` | `""` | Secret holding the API key. Recommended. |
| `openSandbox.existingSecretKey` | `api-key` | Key within that Secret. |
| `openSandbox.apiKey` | `""` | Inline alternative; the chart creates the Secret. |
| `openSandbox.ttl` | `5s` | How long the inventory is cached. |
| `openSandbox.staleAfter` | `60s` | How long a transient state may sit before it is reported stale. |
| `openSandbox.timeout` | `5s` | Bounds one inventory fetch. |
| `extraEnv` | `[]` | Anything the chart does not model, appended last. |
| `resources` | 50m/64Mi → 500m/256Mi | Sized for the informer cache; raise memory for very large fleets. |
| `podSecurityContext`, `securityContext` | nonroot, read-only rootfs, all caps dropped | Satisfies a `restricted` Pod Security Standard as shipped. |
| `podAnnotations`, `podLabels` | `{}` | |
| `nodeSelector`, `tolerations`, `affinity`, `topologySpreadConstraints`, `priorityClassName` | empty | Standard scheduling controls. |

## What this chart deliberately does not include

- **A ServiceMonitor.** The dashboard exposes no Prometheus endpoint of its own — it reads
  Prometheus rather than feeding it.
- **A PodDisruptionBudget or HPA.** It is a read-only viewer; one replica going away costs a
  page refresh, and load is a handful of cached reads.
- **CRDs.** They belong to agent-sandbox. Install that first.

## Upgrading

```bash
helm upgrade sandbox-dashboard oci://ghcr.io/agallea/charts/sandbox-dashboard --reuse-values
```

`helm uninstall` removes everything, including the role and binding — cluster-scoped by
default, or the per-namespace ones when `watchNamespaces` is set.
