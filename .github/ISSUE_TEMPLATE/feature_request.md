---
name: Feature request
about: Suggest a new feature or improvement
title: "feat: "
labels: enhancement
assignees: ""
---

## Problem / motivation

What question about your sandbox fleet can you not answer today? Who would benefit?

## Proposed solution

Describe the feature you'd like to see. Be as specific as possible.

## Alternatives considered

Have you considered any alternative approaches? Why do you prefer this one?

## Affected component(s)

- [ ] API / Go backend
- [ ] UI (overview, sandbox list, metrics)
- [ ] Prometheus integration
- [ ] OpenSandbox integration
- [ ] Helm chart / deployment
- [ ] Documentation
- [ ] CI / tooling

## Does it need data the dashboard cannot see today?

<!--
The dashboard is read-only and reads agent-sandbox CRDs, Pods and Events, plus optionally
Prometheus and OpenSandbox. A feature needing anything else — a write, a new RBAC verb, a
new dependency — is a bigger conversation, so say so here.
-->

- [ ] No — it can be derived from what is already fetched
- [ ] Yes: \_\_\_

## Additional context

Any mockups, examples, or links to relevant prior art.
