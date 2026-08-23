---
name: Bug report
about: Report something that isn't working correctly
title: "bug: "
labels: bug
assignees: ""
---

## Describe the bug

A clear and concise description of what the bug is.

## To reproduce

Steps to reproduce the behavior:

1. ...
2. ...
3. ...

## Expected behavior

What you expected to happen.

## Actual behavior

What actually happened.

## Environment

| Field                  | Value                                        |
| ---------------------- | -------------------------------------------- |
| Dashboard version      | (image tag, e.g. 0.4.0)                      |
| Chart version          | (`helm list`, e.g. 0.4.0 — or "install.yaml") |
| Install method         | Helm / `deploy/install.yaml` / built from source |
| Kubernetes version     | (`kubectl version --short`)                  |
| Cluster type           | GKE / EKS / AKS / kind / minikube / other    |
| agent-sandbox version  | (e.g. v0.3.0)                                |
| Prometheus configured  | yes / no                                     |
| OpenSandbox configured | yes / no                                     |
| Browser                | (if the bug is in the UI)                    |

## Logs

<details>
<summary>Dashboard logs (<code>kubectl logs deploy/sandbox-dashboard</code>)</summary>

```
paste logs here
```

</details>

<details>
<summary>Browser console (if the bug is in the UI)</summary>

```
paste console output here
```

</details>

## Additional context

Any other context, screenshots, or information that might be relevant. Please redact
namespace, node, image and session names if your cluster data is sensitive.
