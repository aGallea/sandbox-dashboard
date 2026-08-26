.PHONY: all build test lint run ui-install ui-build ui-test clean manifests helm-lint

GO := go
BINARY := dashboard
PKG := ./cmd/... ./internal/...

all: test build

build: ui-build
	@trap 'rm -rf internal/ui/dist && mkdir -p internal/ui/dist && touch internal/ui/dist/.gitkeep' EXIT INT TERM; \
		rm -rf internal/ui/dist && \
		cp -r ui/dist internal/ui/dist && \
		$(GO) build -o $(BINARY) ./cmd/dashboard

test:
	$(GO) test -race -count=1 $(PKG)

lint:
	$(GO) vet $(PKG)
	gofmt -l cmd internal | tee /dev/stderr | (! grep .)

run: build
	./$(BINARY) --kubeconfig=$${KUBECONFIG:-$$HOME/.kube/config}

ui-install:
	cd ui && npm ci

ui-build: ui-install
	cd ui && npm run build

ui-test: ui-install
	cd ui && npm test
	cd ui && npm run lint

# The chart is the source of truth; deploy/install.yaml is its rendered form for
# people who would rather not install Helm. CI fails if the two disagree.
manifests:
	@# Helm 4 puts a blank line before every `---`, so regenerating with it churns
	@# three lines that have nothing to do with your change — and CI, which pins
	@# 3.16.2, then fails on a diff you cannot see the point of. Fail here instead.
	@HELM_MAJOR=$$(helm version --template '{{.Version}}' 2>/dev/null | sed 's/^v//' | cut -d. -f1); \
	  [ "$$HELM_MAJOR" = "3" ] || { \
	    echo "manifests: needs Helm 3 (CI pins v3.16.2); found $$(helm version --short)." >&2; \
	    echo "  Helm 4 renders the same chart with extra blank lines, which CI rejects." >&2; \
	    exit 1; \
	  }
	@CHART_VERSION=$$(grep '^version:' deploy/helm/sandbox-dashboard/Chart.yaml | awk '{print $$2}'); \
	{ \
	  sed -e "s/@CHART_VERSION@/$$CHART_VERSION/" deploy/install.yaml.header; \
	  helm template sandbox-dashboard deploy/helm/sandbox-dashboard --namespace default; \
	} > deploy/install.yaml

helm-lint:
	helm lint deploy/helm/sandbox-dashboard
	helm template sandbox-dashboard deploy/helm/sandbox-dashboard > /dev/null
	helm template sandbox-dashboard deploy/helm/sandbox-dashboard \
	  --set prometheus.url=http://prometheus.monitoring.svc:9090 \
	  --set openSandbox.url=http://opensandbox-server.default.svc \
	  --set openSandbox.existingSecret=osb-key \
	  --set ingress.enabled=true --set listenPort=9090 > /dev/null
	helm template sandbox-dashboard deploy/helm/sandbox-dashboard \
	  -f deploy/helm/sandbox-dashboard/values-example.yaml > /dev/null
	# watchNamespaces must swap the ClusterRole for a Role per namespace. Getting
	# this backwards installs cleanly and then 403s at list time, so assert it.
	@set -e; out=$$(helm template sandbox-dashboard deploy/helm/sandbox-dashboard \
	  --set 'watchNamespaces={default,team-a}'); \
	  echo "$$out" | grep -q 'kind: ClusterRole' && { echo "::error::watchNamespaces still rendered a ClusterRole"; exit 1; } || true; \
	  [ "$$(echo "$$out" | grep -c '^kind: Role$$')" = "2" ] || { echo "::error::expected one Role per watched namespace"; exit 1; }; \
	  [ "$$(echo "$$out" | grep -c '^kind: RoleBinding$$')" = "2" ] || { echo "::error::expected one RoleBinding per watched namespace"; exit 1; }; \
	  echo "$$out" | grep -q 'AGENT_SANDBOX_DASHBOARD_WATCH_NAMESPACES' || { echo "::error::the pod was not told which namespaces to watch"; exit 1; }; \
	  echo "namespace-scoped RBAC renders correctly"

clean:
	rm -f $(BINARY)
	rm -rf ui/dist ui/node_modules

.PHONY: test-integration
test-integration:
	KUBEBUILDER_ASSETS="$$(setup-envtest use 1.31.x --bin-dir=$$HOME/.local/share/envtest -p path)" \
		$(GO) test -tags=integration -count=1 ./internal/server/...

.PHONY: docker
docker:
	docker build -t sandbox-dashboard:local .
