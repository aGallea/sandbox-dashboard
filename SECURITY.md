# Security Policy

## Reporting a Vulnerability

If you discover a security vulnerability in sandbox-dashboard, please report it responsibly.

**Do not open a public GitHub issue for security vulnerabilities.**

Instead, please email **<asafgallea@gmail.com>** with:

- A description of the vulnerability
- Steps to reproduce
- Potential impact
- Suggested fix (if any)

You should receive an acknowledgment within 48 hours. We will work with you to understand the
issue and coordinate a fix before any public disclosure.

## Supported Versions

| Version | Supported |
| ------- | --------- |
| latest  | Yes       |

## Scope

This policy covers the dashboard binary, the container image, and the Helm chart in this
repository. It does not cover [agent-sandbox][as] itself, Prometheus, or OpenSandbox, which
are maintained elsewhere.

[as]: https://github.com/kubernetes-sigs/agent-sandbox

## Security Considerations

Read these before exposing an install to anyone.

- **The dashboard ships no authentication of its own.** Anyone who can reach it sees the whole
  sandbox fleet — namespaces, node names, images, owners, session IDs. The Service is
  `ClusterIP` by default; put it behind your existing ingress auth (IAP, oauth2-proxy, or
  similar) before exposing it. Do not attach a public LoadBalancer without one. `values.yaml`
  carries commented annotation sets for the common stacks.
- **It is read-only, and has no verbs to be otherwise.** The ClusterRole grants only
  `get`/`list`/`watch` on the four agent-sandbox CRDs plus Pods and Events. There is no write
  path in the code and no write permission in the chart.
- **The OpenSandbox API key** is read from a Secret into `OPENSANDBOX_API_KEY` and used only as
  an outbound request header. Prefer `openSandbox.existingSecret` over `openSandbox.apiKey`,
  which would otherwise put the key in your Helm values and release history.
- **Integration URLs are yours to choose.** `--prometheus-url` and `--opensandbox-url` accept
  any scheme; in-cluster Service URLs are plain HTTP by default. Use HTTPS for anything that
  leaves the cluster.
- **The pod runs unprivileged**: non-root (UID 65532), read-only root filesystem, all
  capabilities dropped, `seccompProfile: RuntimeDefault`, no privilege escalation.
- **No persistence, no outbound telemetry.** There is no database and no state on disk; every
  read goes through the in-process informer cache. The dashboard talks only to the
  kube-apiserver and to the integrations you configure.
