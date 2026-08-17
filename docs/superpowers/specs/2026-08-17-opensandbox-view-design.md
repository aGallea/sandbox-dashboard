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

Against `gke_algo-studio-main_us-central1_main` on 2026-08-17 at ~12:35, with the OSB server
reached through `kubectl port-forward svc/opensandbox-server 18080:80`:

| Observation | Value |
|---|---|
| `Sandbox` CRs in cluster | 93 |
| CRs carrying `opensandbox.io/id` | 92 |
| Records from `GET /v1/sandboxes` | 92 (`pagination.totalItems`) |
| Join on `opensandbox.io/id` | 92/92 matched, 0 orphans either side |
| Join on CR name | 29/92 — **unusable** |
| State pairs observed | `Running ↔ Ready`, 92/92 |

A second measurement at 14:10 caught the cluster mid-incident and is recorded separately
below; the join numbers there were 102 OSB records against 110 CRs, still matching 102/102
on the label.

Two findings drive the design:

**The join key is the label, not the name.** 29 CRs are named `<uuid>` and 63 are named
`sandbox-<uuid>`, and both conventions appear within the same five-hour window — this is
not a version cutover, so CR name cannot be used to correlate. The `opensandbox.io/id`
label matched every record in both directions and is the contract this design relies on.

**Divergence is real, and it is expensive.** The first measurement above caught a fleet at
rest, where the two views could not disagree. Ninety minutes later the same cluster
produced a live incident that is the strongest argument for this feature — see
[Observed incident](#observed-incident-2026-08-17). OSB additionally has states the Ready
condition cannot structurally express: `Pending, Pausing, Paused, Resuming, Stopping,
Terminated, Failed` all collapse into `NotReady`.

The one non-OSB CR (`instance-element-hq-element-web-…`, labels `app`,
`swe-instance-id`) confirms multiple creators are already live in this cluster.

Metadata completeness varies and the UI must tolerate it: 62 CRs carry the full set
(`experiment`, `owner`, `project`, `session_id`, `team`), 30 carry only `session_id`.

## Observed incident (2026-08-17)

At 14:10, `opensandbox-server:v0.2.2` in algo-studio reported 10 of 102 sandboxes as
`Pending / SANDBOX_PENDING / "Sandbox is pending scheduling"` while every one of their pods
was `1/1 Running` and every CR read `Ready`, most within two seconds of creation.

OSB's pod watch had died silently somewhere between 09:32:44 — its last
`state: Running - Pod is Ready` log line — and 12:40. It logged no exception, no
`ProtocolError`, no reconnect and no retry in 24 hours of output. It blocked on a dead
stream for the full 600s per sandbox, then reported `Last state: Pending`: the last event it
ever received. Meanwhile `diagnostics/inspect`, which performs an on-demand read rather
than consuming the watch, returned correct live pod state throughout — so credentials,
RBAC, networking and the API server were all healthy. The single replica had 23h uptime and
zero restarts, so nothing ever forced a re-watch.

The failure is not cosmetic. On timeout OSB **deletes the healthy sandbox**:

```
Timeout waiting for sandbox … Elapsed: 600.5s, Last state: Pending
Creation failed, cleaning up sandbox …
```

**70 healthy sandboxes were destroyed in 80 minutes** (10 in the 12:00 hour, 40 in 13:00,
20 in 14:00), beginning at 12:50:02. A `kubectl rollout restart` at 14:17 restored the
watch, cleared all 10 stuck rows to `Running`, and rescued those 10 about a minute before
their scheduled cleanup. The upstream defect — a watch with no timeout and no reconnect
from the last `resourceVersion` — is unfixed, so this recurs on every watch drop.

Three design consequences:

1. **Divergence is worth a column.** This is precisely `osb.state: Pending` against
   `phase: Ready`, sustained for minutes, on 10 rows at once.
2. **A state diff alone is not enough.** The sharper signal was `lastTransitionAt` frozen at
   `createdAt` while the CR moved on. Staleness is what distinguishes a dead watch from an
   ordinary in-flight transition, and it is what should raise an alarm.
3. **The threshold can be tight.** Pods reach Ready in roughly two seconds here, so any OSB
   `Pending` older than about a minute is already anomalous. There is no need to tolerate a
   wide normal band.

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

## Divergence and staleness

Two independent signals. Divergence catches the two control planes disagreeing; staleness
catches OSB not moving at all. The incident above needed both, and staleness was the
sharper of the two.

### Divergence

An explicit agreement table, evaluated server-side so the UI only renders the marker:

| OSB state | Agrees with |
|---|---|
| `Running` | `Ready` |
| `Pending`, `Pausing`, `Paused`, `Resuming`, `Stopping`, `Terminated`, `Failed` | `NotReady`, `Unknown` |
| anything unrecognised | never flags divergence; logged once |

`diverged` is true when the pair is not in the table. OSB `Pending` against CR `Ready`
flags. Treating unrecognised states as "no opinion" means a future OSB state ships as a
blank cell rather than a fleet-wide false alarm.

### Staleness

`stateAgeSeconds` is `now - osb.status.lastTransitionAt`, and `stale` is true when a
**non-terminal** OSB state has sat there longer than a threshold (default 60s, env-tunable).

Only non-terminal states are eligible. `Running`, `Terminated` and `Failed` are resting
places — a sandbox legitimately runs for hours — so age against them means nothing.
`Pending`, `Pausing`, `Resuming` and `Stopping` are transitions that should resolve in
seconds; age against those is the signal. `Paused` is deliberately excluded: it is a state a
caller chooses to hold.

Why this earns its place next to `diverged`: during the incident every stuck sandbox had
`lastTransitionAt == createdAt` exactly, because OSB never received a second event. A frozen
timestamp identifies a dead watch, where a bare state mismatch could just as well be a
sandbox mid-creation. It is also the one signal that still works when the CR side is
unremarkable — no divergence to spot, just OSB standing still.

The default 60s comes from measurement, not taste: pods here reached Ready about two seconds
after creation, so a minute is already far outside normal.

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
    State            string     `json:"state"`
    Reason           string     `json:"reason,omitempty"`
    Message          string     `json:"message,omitempty"`
    ExpiresAt        *time.Time `json:"expiresAt,omitempty"`
    LastTransitionAt *time.Time `json:"lastTransitionAt,omitempty"`
    StateAgeSeconds  int64      `json:"stateAgeSeconds"`
    Diverged         bool       `json:"diverged"`
    Stale            bool       `json:"stale"`
}
```

The list response gains a sibling to `items` carrying the three OSB states the UI must
distinguish. **The list never fails because OSB is down:**

| `osb.status` | Meaning | HTTP |
|---|---|---|
| absent | `OPENSANDBOX_URL` unset; dashboard behaves exactly as today | 200 |
| `unreachable` | configured, fetch failed; CR data still served, UI shows a banner | 200 |
| `ok` | joined, with `fetchedAt` | 200 |

New filters follow the existing `?namespace=`/`?phase=` pattern: `?creator=`, `?osbState=`,
and `?stale=true`. `?stale=true` is the "show me the incident" query — during the event above
it would have returned exactly the 10 affected rows out of 102.

New route for the detail drawer: `GET /api/v1/sandboxes/{namespace}/{name}/osb`, which
reads the CR to obtain its `opensandbox.io/id`, then returns OSB's
`diagnostics/summary` and `diagnostics/events`. 404 when the sandbox has no OSB label,
503 when OSB is unconfigured.

## UI

The sandbox list gains **two always-present state columns** plus identity columns:

```
NAME              CREATOR      PHASE      OSB STATE          OWNER
sandbox-04c671…   opensandbox  Ready      Running            odeda
sandbox-13d28e…   opensandbox  Ready      Pending ⚠ 9m ⏱     odeda
sandbox-2eda72…   opensandbox  Ready      Pausing ⚠          odeda
sandbox-9ff407…   opensandbox  NotReady   Pending            pazshepsels
instance-elem…    unknown      Ready      —                  —
```

`⚠` marks `diverged`; `⏱` with the state age marks `stale`. They are independent — row 2 is
the incident signature (both), row 3 disagrees but is young, and row 4 is an ordinary
in-flight creation that should raise nothing at all. That third case is why staleness is a
separate flag rather than a stronger divergence rule.

The OSB columns are driven off `ResourceConfig` in `resources/config.ts`, the same mechanism
`showPhase` already uses to keep the shared table generic across the four kinds.

The detail drawer gains an **OpenSandbox** section — state, reason, message, expiry, state
age, and OSB's own event list beside the Kubernetes events already rendered. The incident
showed why the event list matters: OSB's `diagnostics/events` is served by an on-demand read
and stayed correct while the watch-fed state was frozen, so the two side by side localise the
fault to the watch.

## Configuration

Follows the Prometheus precedent in `main.go`, including its typed-nil guard when
assigning to an interface-typed `Deps` field.

| Setting | Flag | Env |
|---|---|---|
| base URL | `--opensandbox-url` | `OPENSANDBOX_URL` |
| API key | — | `OPENSANDBOX_API_KEY` |
| cache TTL | — | `AGENT_SANDBOX_DASHBOARD_OSB_TTL` |
| staleness threshold | — | `AGENT_SANDBOX_DASHBOARD_OSB_STALE_AFTER` (default `60s`) |

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
  `?creator=`, `?osbState=` and `?stale=true` filters.
- **Staleness**, with a clock injected so the tests stay deterministic: a young non-terminal
  state is not stale; the same state past the threshold is; `Running`, `Terminated` and
  `Failed` are never stale regardless of age; `Paused` is never stale; and the incident's
  exact shape — `lastTransitionAt == createdAt`, OSB `Pending`, CR `Ready` — flags both
  `diverged` and `stale`.

The incident of 2026-08-17 supplies the fixture values for these cases. Reproducing it
against a live cluster is still not possible on demand — it requires OSB's watch to break —
so the assertions run against a fake OSB server seeded with the recorded timestamps.

## Verification

Local rancher-desktop has no OSB server, so the end-to-end check runs the dashboard
read-only against algo-studio: `--kubeconfig` on that context plus
`--opensandbox-url=http://127.0.0.1:18080` through a port-forward to
`svc/opensandbox-server`. 110 CRs against 102 OSB records exercises the matched path and the
unmatched-creator rows.

Because the fleet is normally healthy, the divergence and staleness columns will read clean
in a live check. That is expected and is not evidence they work — those paths are proven by
the unit tests seeded from the incident, and the live run only confirms the join, the
identity columns, and graceful degradation when OSB is stopped.

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
2. **Server join** — DTO fields, divergence, staleness, filters, drawer route, `main.go` wiring, deployment env. ~230 lines.
3. **UI** — the two state columns with `⚠`/`⏱`, identity columns, drawer diagnostics section. ~160 lines.

## Risks

- **The label is the contract.** If OSB stops stamping `opensandbox.io/id`, the join
  silently degrades to every row reading `creator: unknown` with no OSB state. Worth an
  explicit "OSB reported N sandboxes, matched M" log line so the gap is visible.
- **Creator detection is label-inference, not ownership.** There are no
  `ownerReferences` on any of the CRs, so labels are the only available signal. A
  creator that stamps nothing recognisable reads as `unknown` — correct, but not
  informative.
- **The agreement table encodes a judgment** about which OSB states imply not-ready. It is
  in one function with one test per pair, so it is cheap to correct as real divergences are
  observed in the wild.
- **`lastTransitionAt` is OSB's own timestamp, and staleness trusts it.** The incident showed
  it frozen rather than wrong, which is the case this detects. A clock skew between OSB and
  the dashboard, or a state OSB rewrites without advancing the timestamp, would show up as
  false staleness. The threshold is env-tunable so a noisy install can be widened without a
  redeploy.
- **This dashboard cannot fix what it surfaces.** It is read-only by design, so the remedy
  for a dead watch is still a `rollout restart` performed elsewhere. The value here is
  cutting detection from "someone notices their eval results are short" to one glance at a
  `⏱` column.
