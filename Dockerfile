# syntax=docker/dockerfile:1.7

# ---- 1. ui builder ----
FROM --platform=$BUILDPLATFORM node:20-alpine AS ui-builder
WORKDIR /src/ui
COPY ui/package.json ui/package-lock.json ./
RUN npm ci
COPY ui/ ./
RUN npm run build

# ---- 2. go builder ----
FROM --platform=$BUILDPLATFORM golang:1.26-alpine AS go-builder
WORKDIR /src
ENV CGO_ENABLED=0 GOFLAGS=-trimpath
COPY go.mod go.sum ./
RUN go mod download
COPY . ./
# Embed the UI assets the same way `make build` does.
COPY --from=ui-builder /src/ui/dist ./internal/ui/dist
ARG TARGETOS TARGETARCH
RUN GOOS=$TARGETOS GOARCH=$TARGETARCH go build -ldflags="-s -w" -o /out/dashboard ./cmd/dashboard

# ---- 3. runtime ----
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=go-builder /out/dashboard /usr/local/bin/dashboard
USER 65532:65532
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/dashboard"]
