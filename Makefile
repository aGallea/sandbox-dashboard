.PHONY: all build test lint run ui-install ui-build clean

GO := go
BINARY := dashboard
PKG := ./...

all: test build

build:
	$(GO) build -o $(BINARY) ./cmd/dashboard

test:
	$(GO) test -race -count=1 $(PKG)

lint:
	$(GO) vet $(PKG)
	gofmt -l . | tee /dev/stderr | (! grep .)

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
