.PHONY: all build test lint run ui-install ui-build clean

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

clean:
	rm -f $(BINARY)
	rm -rf ui/dist ui/node_modules

.PHONY: test-integration
test-integration:
	KUBEBUILDER_ASSETS="$$(setup-envtest use 1.31.x --bin-dir=$$HOME/.local/share/envtest -p path)" \
		$(GO) test -tags=integration -count=1 ./internal/server/...
