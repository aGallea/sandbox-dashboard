# Contributing to sandbox-dashboard

Thank you for your interest in contributing! This document explains how to get the project running locally, coding standards, and how to submit changes.

## Table of Contents

- [Dev Environment Setup](#dev-environment-setup)
- [Running It](#running-it)
- [Running Tests](#running-tests)
- [Code Style](#code-style)
- [Working on the Chart](#working-on-the-chart)
- [PR Process](#pr-process)

---

## Dev Environment Setup

### Prerequisites

| Tool                                                                  | Version | Install                                                          |
| --------------------------------------------------------------------- | ------- | ---------------------------------------------------------------- |
| Go                                                                    | 1.26+   | [go.dev/dl](https://go.dev/dl/) or `brew install go`             |
| Node.js                                                               | 20+     | [nodejs.org](https://nodejs.org) or `brew install node`          |
| Helm                                                                  | 3.16+   | `brew install helm`                                              |
| A Kubernetes cluster with [agent-sandbox][as] installed               | —       | kind, minikube, or a real one                                     |
| [`setup-envtest`](https://book.kubebuilder.io/reference/envtest) | —       | `go install sigs.k8s.io/controller-runtime/tools/setup-envtest@latest` (integration tests only) |

[as]: https://github.com/kubernetes-sigs/agent-sandbox

The dashboard reads agent-sandbox CRDs; it does not create them. Without an agent-sandbox
install the API serves empty lists, which is enough to work on the UI but not to see real data.

### Clone and install

```sh
git clone https://github.com/aGallea/sandbox-dashboard.git
cd sandbox-dashboard

# Go modules
go mod download

# Frontend dependencies
make ui-install
```

---

## Running It

### Against a cluster

```sh
# Builds the SPA, embeds it, and serves everything on :8080
make run
```

`make run` passes `--kubeconfig=$KUBECONFIG` (falling back to `~/.kube/config`). It needs
read access to the agent-sandbox CRDs, Pods, and Events.

### With UI hot reload

The embedded SPA is rebuilt only by `make build`, so for frontend work run the API and the
Vite dev server side by side:

```sh
# Terminal 1 — API on :8080
make build && ./dashboard --kubeconfig=$HOME/.kube/config

# Terminal 2 — Vite on :5173, proxying /api to :8080
cd ui && npm run dev
```

Open <http://localhost:5173>.

### The optional integrations

Both Prometheus and OpenSandbox are soft dependencies — the dashboard hides any control it
cannot answer, so a missing integration is a smaller UI, not an error. To develop against
them, port-forward the services and point the flags at localhost:

```sh
./dashboard \
  --kubeconfig=$HOME/.kube/config \
  --prometheus-url=http://localhost:9090 \
  --opensandbox-url=http://localhost:8081
```

If you add a feature that depends on an integration, it must degrade: when the integration is
unconfigured or unreachable, the control that needs it should not render at all.

---

## Running Tests

### Go

```sh
# Unit tests, race detector on
make test

# Single package
go test -race ./internal/server/...

# With output
go test -v -race ./internal/server/...

# Integration tests against a real API server (needs setup-envtest on PATH)
make test-integration
```

### TypeScript (frontend)

```sh
# Unit tests (vitest) + eslint
make ui-test

# Or directly
cd ui && npm test
cd ui && npx vitest        # watch mode
cd ui && npx tsc --noEmit  # type check only
```

### Chart

```sh
# helm lint plus a render under several value sets
make helm-lint
```

---

## Code Style

### Go

```sh
# Vet + gofmt over cmd/ and internal/
make lint

# Format in place
gofmt -w cmd internal
```

CI enforces `go vet` and `gofmt -l` (any unformatted file fails the build).

### TypeScript / React

```sh
cd ui
npm run lint          # eslint
npx eslint . --fix    # fix what it can
npm run build         # tsc -b + vite build, catches type and bundler errors
```

### Comments

Comment *why*, not *what*. The codebase leans on short comments that explain a
non-obvious decision — a failure boundary, a performance trade-off, a deliberate
simplification — and omits them everywhere the code already reads clearly.

Deliberate shortcuts with a known ceiling are marked `ponytail:` and name their upgrade path:

```ts
// ponytail: aggregating a few hundred rows in the browser is free. Past ~5k
// sandboxes, move these rollups behind /api/v1/overview and send the totals.
```

---

## Working on the Chart

The Helm chart in `deploy/helm/sandbox-dashboard/` is the source of truth.
`deploy/install.yaml` is its rendered form, for people who would rather not install Helm.
**The two must agree, and CI fails if they don't.** After changing any template or default:

```sh
make manifests   # regenerates deploy/install.yaml
make helm-lint
```

Commit the regenerated `deploy/install.yaml` alongside the template change.

New values need three things, or the chart is only half-documented:

1. A default and a comment in `values.yaml`
2. An entry in `values.schema.json`, so a typo fails at install time rather than at runtime
3. A row in `deploy/helm/sandbox-dashboard/README.md`

---

## PR Process

### Conventional Commits

All commit messages must follow [Conventional Commits](https://www.conventionalcommits.org/):

```
<type>(<scope>): <short description>

[optional body]
[optional footer]
```

**Types:** `feat`, `fix`, `docs`, `style`, `refactor`, `test`, `chore`, `ci`, `helm`

**Examples:**

```
feat(overview): group the fleet by any sandbox label
fix(prometheus): drop non-finite samples instead of charting NaN
helm: expose podAnnotations on the deployment
docs: add a worked values example
chore(deps): bump recharts to 2.15.4
```

### Submitting a PR

1. Fork the repository and create a branch from `main`:

   ```sh
   git checkout -b feat/my-feature
   ```

2. Make your changes, following the code style guidelines above.

3. Add or update tests as appropriate.

4. Ensure everything passes locally:

   ```sh
   make lint
   make test
   make ui-test
   make build
   make helm-lint
   make manifests && git diff --exit-code deploy/install.yaml
   ```

5. Push and open a PR against `main`. Fill in the PR template.

6. A maintainer will review. Address feedback and the PR will be merged once approved.

### Stacked PRs

If you split work into a stack of dependent PRs, **target every one at `main` and merge them
one at a time**, confirming each PR's base has flipped to `main` before merging the next.
GitHub retargets a stacked PR only *after* its base merges, so back-to-back merges land the
later PRs in each other rather than in `main` — and they all report success either way.

### Scope

Keep PRs small and focused — under ~400 changed lines where possible. Unrelated changes
belong in separate PRs, however small.
