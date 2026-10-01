package prom

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// namespaceName is the apiserver's own rule for a namespace name. The names fed
// to UsageQueries come from the informer cache rather than from a client, but
// they are interpolated into PromQL, so they are re-validated here instead of
// trusted.
var namespaceName = regexp.MustCompile(`^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`)

// UsageQueries returns the two instant queries behind the usage endpoint: CPU
// cores and working-set bytes, both summed per pod.
//
// The queries are scoped to the namespaces sandboxes actually live in. Measured
// on a 1500-pod cluster holding 169 sandboxes in one namespace, the scoped
// query returned 267 series in 0.7s against 1527 series in 1.6s unscoped — the
// cost should follow the fleet, not the cluster.
//
// An empty namespace list yields unscoped queries; callers with no sandbox pods
// should skip the query entirely rather than ask for the whole cluster.
func UsageQueries(namespaces []string) (cpu, mem string) {
	scope := namespaceMatcher(namespaces)
	return perPod(`rate(container_cpu_usage_seconds_total{%s}[5m])`, scope),
		perPod(`container_memory_working_set_bytes{%s}`, scope)
}

// perPod sums a cAdvisor expression per pod; expr holds one %s for its matchers.
//
// It sums the per-container series, and falls back to the pod-level cgroup for
// pods that have none: gVisor runs all of a pod's containers inside one sandbox,
// so cAdvisor exports only the pod cgroup for it. The two matchers never overlap
// — container="" with image="" is the pod cgroup alone, not the pause container —
// so no pod is counted twice. Parenthesised so callers can join on the result.
func perPod(expr, scope string) string {
	containers := fmt.Sprintf(expr, scope+`container!=""`)
	podCgroup := fmt.Sprintf(expr, scope+`container="",image="",pod!=""`)
	return fmt.Sprintf(`(sum by (namespace, pod) (%s) or sum by (namespace, pod) (%s))`, containers, podCgroup)
}

// namespaceMatcher builds the `namespace=~"a|b",` matcher prefix, sorted so the
// query text is stable, and empty when no name is legal.
func namespaceMatcher(namespaces []string) string {
	valid := make([]string, 0, len(namespaces))
	for _, ns := range namespaces {
		if namespaceName.MatchString(ns) {
			valid = append(valid, ns)
		}
	}
	if len(valid) == 0 {
		return ""
	}
	sort.Strings(valid)
	return fmt.Sprintf(`namespace=~"%s",`, strings.Join(valid, "|"))
}
