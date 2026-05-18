# agent-sandbox-dashboard

Lightweight read-only operational dashboard for [kubernetes-sigs/agent-sandbox](https://github.com/kubernetes-sigs/agent-sandbox).

**Status:** M3 complete (`v0.3.0`). M4 (Docker + kustomize install) next.

See `docs/design.md` (forthcoming) for architecture.

## Development

```bash
make test          # run all tests
make run           # run against $KUBECONFIG
make build         # build the binary
```

## License

Apache-2.0
