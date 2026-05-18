# agent-sandbox-dashboard

Lightweight read-only operational dashboard for [kubernetes-sigs/agent-sandbox](https://github.com/kubernetes-sigs/agent-sandbox).

**Status:** M1 complete (`v0.1.0-alpha.1`). M2 (lists + detail) next.

See `docs/design.md` (forthcoming) for architecture.

## Development

```bash
make test          # run all tests
make run           # run against $KUBECONFIG
make build         # build the binary
```

## License

Apache-2.0
