# OpenSandbox view — design

**Date:** 2026-08-17
**Status:** approved, pending implementation plan

## Problem

The dashboard lists `Sandbox` CRs, and in clusters driven by OpenSandbox those rows
are bare UUIDs with a Ready badge. Two things are missing:

1. **Who created a sandbox, and for what.** Owner, team, and experiment are invisible,
   so a list of 93 UUIDs cannot be reasoned about.
2. **OpenSandbox's own view of state.** OSB runs its own lifecycle state machine. A
   sandbox can be `Running` to Kubernetes and `Pending` to OSB — the disagreement
   between the two control planes is itself the signal an operator needs, and today
   the dashboard only ever shows one side.

The dashboard should stay useful for sandboxes created by *any* creator, not just OSB.

## What was measured

Against `gke_algo-studio-main_us-central1_main` on 2026-08-17, with the OSB server
reached through `kubectl port-forward svc/opensandbox-server 18080:80`:

| Observation | Value |
|---|---|
| `Sandbox` CRs in cluster | 93 |
| CRs carrying `opensandbox.io/id` | 92 |
| Records from `GET /v1/sandboxes` | 92 (`pagination.totalItems`) |
| Join on `opensandbox.io/id` | 92/92 matched, 0 orphans either side |
| Join on CR name | 29/92 — **unusable** |
| State pairs observed | `Running ↔ Ready`, 92/92 |

Two findings drive the design:

**The join key is the label, not the name.** 29 CRs are named `<uuid>` and 63 are named
`sandbox-<uuid>`, and both conventions appear within the same five-hour window — this is
not a version cutover, so CR name cannot be used to correlate. The `opensandbox.io/id`
label matched every record in both directions and is the contract this design relies on.

**Absence of divergence today does not argue against showing it.** All 92 sandboxes agree
right now because the fleet is at rest. Disagreement lives in transitions — creation,
pause, expiry, teardown — and is only ever observable if both views are already being
recorded. OSB additionally has states the Ready condition cannot structurally express:
`Pending, Pausing, Paused, Resuming, Stopping, Terminated, Failed` all collapse into
`NotReady`.

The one non-OSB CR (`instance-element-hq-element-web-…`, labels `app`,
`swe-instance-id`) confirms multiple creators are already live in this cluster.

Metadata completeness varies and the UI must tolerate it: 62 CRs carry the full set
(`experiment`, `owner`, `project`, `session_id`, `team`), 30 carry only `session_id`.

## Approach

**Server-side join.** The dashboard backend calls OSB, joins on the label, and serves one
enriched list.

*Alternative considered and rejected:* having the browser call OSB directly and merge
client-side. It would place a shared API key in the browser and require CORS on the OSB
server. Server-side keeps the key in-cluster and gives one place to cache.

### Data split

Owner, team, and experiment exist identically in the CR labels and the OSB response.
They are read from **labels**, so that an OSB outage costs one column rather than the
whole table.

| Field | Source | Survives OSB down |
|---|---|---|
| `creator` | label `opensandbox.io/id` present → `opensandbox`, else `unknown` | yes |
| `owner`, `team`, `experiment` | CR labels | yes |
| `phase` | `.status.conditions[Ready]` | yes |
| `osb.state`, reason, message, expiry | OSB API | no — renders `—` |

## `internal/osb`

Mirrors `internal/prom`: constructor plus `Option` values, and a narrow interface declared
in `server` so tests can substitute it.

```go
func NewClient(baseURL, apiKey string, opts ...Option) (*Client, error)
func (c *Client) ListSandboxes(ctx context.Context) (map[string]Sandbox, error) // keyed by id
func (c *Client) Diagnostics(ctx context.Context, id string) (*Diagnostics, error)
```

- **Pagination.** `pageSize=200` (the documented maximum) looping while
  `pagination.hasNextPage`. 92 items is a single request today. The loop carries a page cap
  so a server-side `hasNextPage` bug cannot spin forever; the cap is marked with a
  `ponytail:` comment naming its ceiling.
- **Read-only.** The OSB API exposes `POST /v1/sandboxes/{id}/pause`,
  `DELETE /v1/sandboxes/{id}` and similar. This dashboard is read-only by design, so the
  client exposes no method capable of reaching a non-GET route. A test asserts this.
- **Caching.** A TTL cache (default 5s, env-tunable) guarded by a mutex, because the UI
  already refetches every list at 5s. A mutex rather than `singleflight`: concurrent
  requests blocking on one upstream fetch is the same outcome with less machinery.

## Divergence

An explicit agreement table, evaluated server-side so the UI only renders the marker:

| OSB state | Agrees with |
|---|---|
| `Running` | `Ready` |
| `Pending`, `Pausing`, `Paused`, `Resuming`, `Stopping`, `Terminated`, `Failed` | `NotReady`, `Unknown` |
| anything unrecognised | never flags divergence; logged once |

`diverged` is true when the pair is not in the table. OSB `Pending` against CR `Ready`
flags. Treating unrecognised states as "no opinion" means a future OSB state ships as a
blank cell rather than a fleet-wide false alarm.

## API changes

`ResourceSummary` is shared by all four resource kinds, so OSB data hangs off an optional
pointer that is only ever populated for sandboxes:

```go
type ResourceSummary struct {
    // ...existing fields...
    Creator    string   `json:"creator,omitempty"`
    Owner      string   `json:"owner,omitempty"`
    Team       string   `json:"team,omitempty"`
    Experiment string   `json:"experiment,omitempty"`
    Osb        *OsbView `json:"osb,omitempty"`
}

type OsbView struct {
    State     string     `json:"state"`
    Reason    string     `json:"reason,omitempty"`
    Message   string     `json:"message,omitempty"`
    ExpiresAt *time.Time `json:"expiresAt,omitempty"`
    Diverged  bool       `json:"diverged"`
}
```

The list response gains a sibling to `items` carrying the three OSB states the UI must
distinguish. **The list never fails because OSB is down:**

| `osb.status` | Meaning | HTTP |
|---|---|---|
| absent | `OPENSANDBOX_URL` unset; dashboard behaves exactly as today | 200 |
| `unreachable` | configured, fetch failed; CR data still served, UI shows a banner | 200 |
| `ok` | joined, with `fetchedAt` | 200 |

New filters follow the existing `?namespace=`/`?phase=` pattern: `?creator=` and
`?osbState=`.

New route for the detail drawer: `GET /api/v1/sandboxes/{namespace}/{name}/osb`, which
reads the CR to obtain its `opensandbox.io/id`, then returns OSB's
`diagnostics/summary` and `diagnostics/events`. 404 when the sandbox has no OSB label,
503 when OSB is unconfigured.

## UI

The sandbox list gains **two always-present state columns** plus identity columns:

```
NAME              CREATOR      PHASE      OSB STATE     OWNER
sandbox-04c671…   opensandbox  Ready      Running       odeda
sandbox-13d28e…   opensandbox  NotReady   Pending    ⚠  odeda
sandbox-2eda72…   opensandbox  Ready      Pausing    ⚠  odeda
instance-elem…    unknown      Ready      —             —
```

`⚠` marks a `diverged` row. The OSB columns are driven off `ResourceConfig` in
`resources/config.ts`, the same mechanism `showPhase` already uses to keep the shared
table generic across the four kinds.

The detail drawer gains an **OpenSandbox** section — state, reason, message, expiry, and
OSB's own event list beside the Kubernetes events already rendered.

## Configuration

Follows the Prometheus precedent in `main.go`, including its typed-nil guard when
assigning to an interface-typed `Deps` field.

| Setting | Flag | Env |
|---|---|---|
| base URL | `--opensandbox-url` | `OPENSANDBOX_URL` |
| API key | — | `OPENSANDBOX_API_KEY` |
| cache TTL | — | `AGENT_SANDBOX_DASHBOARD_OSB_TTL` |

The TTL keeps the `AGENT_SANDBOX_DASHBOARD_` prefix of the existing
`AGENT_SANDBOX_DASHBOARD_METRICS_TIMEOUT` rather than introducing a second convention.
The prefix is stale after the repo rename, but renaming a released env var is a breaking
change and belongs in its own PR.

The API key comes from a `secretKeyRef` in the Deployment, so no Secret-read RBAC is
added to the dashboard's ClusterRole. It travels in the `OPEN-SANDBOX-API-KEY` header —
never a URL, never a log line, never a problem+json body.

## Testing

- `internal/osb`: pagination across multiple pages against `httptest`; auth header
  present; non-200 and malformed JSON produce errors; context cancellation; page cap
  honoured; TTL cache serves within the window and refetches after it; one upstream fetch
  under concurrent callers; no non-GET method reachable.
- `internal/server`: join by label; unmatched label yields no `osb` field; OSB unreachable
  returns 200 with `osb.status: unreachable`; OSB unconfigured omits the field entirely;
  divergence true and false cases; `creator` derived for both OSB and non-OSB CRs;
  `?creator=` and `?osbState=` filters.

Divergence cannot be reproduced against the live cluster — all 92 sandboxes agree, and
manufacturing a disagreement would mean pausing someone's running sandbox. It is covered
by a fake OSB server in unit tests only.

## Verification

Local rancher-desktop has no OSB server, so the end-to-end check runs the dashboard
read-only against algo-studio: `--kubeconfig` on that context plus
`--opensandbox-url=http://127.0.0.1:18080` through a port-forward to
`svc/opensandbox-server`. 93 CRs against 92 OSB records exercises the matched path and the
one unmatched-creator row.

## Out of scope

- **`diagnostics/logs`** — a streaming and payload-size problem, not a column.
- **Per-creator counts on the Overview page** — cheap to add later; not what this change
  is for.
- **`/v1/pools` and `/v1/snapshots`** — both return empty in algo-studio and no
  `SandboxWarmPool` CRs exist, so there is nothing to render.
- **Any write action.** Pause, resume, terminate, and renew-expiration stay out; the
  dashboard is read-only.

## PR boundaries

Roughly 500 lines total, past the 400-line guideline, so it ships as three PRs:

1. **`internal/osb`** — client, pagination, cache, tests. No wiring; nothing user-visible. ~250 lines.
2. **Server join** — DTO fields, divergence, filters, drawer route, `main.go` wiring, deployment env. ~200 lines.
3. **UI** — the two state columns and `⚠`, identity columns, drawer diagnostics section. ~150 lines.

## Risks

- **The label is the contract.** If OSB stops stamping `opensandbox.io/id`, the join
  silently degrades to every row reading `creator: unknown` with no OSB state. Worth an
  explicit "OSB reported N sandboxes, matched M" log line so the gap is visible.
- **Creator detection is label-inference, not ownership.** There are no
  `ownerReferences` on any of the 93 CRs, so labels are the only available signal. A
  creator that stamps nothing recognisable reads as `unknown` — correct, but not
  informative.
- **The agreement table encodes a judgment** about which OSB states imply not-ready. It is
  in one function with one test per pair, so it is cheap to correct once a real
  divergence is observed in the wild.
