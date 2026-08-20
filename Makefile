.PHONY: all build test lint run ui-install ui-build clean manifests helm-lint

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

# The chart is the source of truth; deploy/install.yaml is its rendered form for
# people who would rather not install Helm. CI fails if the two disagree.
manifests:
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
