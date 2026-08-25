{{/*
Chart name, overridable.
*/}}
{{- define "sandbox-dashboard.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{/*
Fully qualified name. Release-scoped by default so two installs in one cluster
do not collide — which matters more than usual here, because the ClusterRole and
its binding are cluster-scoped objects.
*/}}
{{- define "sandbox-dashboard.fullname" -}}
{{- if .Values.fullnameOverride -}}
{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" -}}
{{- else -}}
{{- $name := default .Chart.Name .Values.nameOverride -}}
{{- if contains $name .Release.Name -}}
{{- .Release.Name | trunc 63 | trimSuffix "-" -}}
{{- else -}}
{{- printf "%s-%s" .Release.Name $name | trunc 63 | trimSuffix "-" -}}
{{- end -}}
{{- end -}}
{{- end -}}

{{- define "sandbox-dashboard.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "sandbox-dashboard.labels" -}}
helm.sh/chart: {{ include "sandbox-dashboard.chart" . }}
{{ include "sandbox-dashboard.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
app.kubernetes.io/component: dashboard
{{- end -}}

{{- define "sandbox-dashboard.selectorLabels" -}}
app.kubernetes.io/name: {{ include "sandbox-dashboard.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end -}}

{{- define "sandbox-dashboard.serviceAccountName" -}}
{{- if .Values.serviceAccount.create -}}
{{- default (include "sandbox-dashboard.fullname" .) .Values.serviceAccount.name -}}
{{- else -}}
{{- default "default" .Values.serviceAccount.name -}}
{{- end -}}
{{- end -}}

{{/*
Name of the Secret holding the OpenSandbox API key: the one the operator already
has, or the one this chart creates from an inline value.
*/}}
{{- define "sandbox-dashboard.osbSecretName" -}}
{{- if .Values.openSandbox.existingSecret -}}
{{- .Values.openSandbox.existingSecret -}}
{{- else -}}
{{- printf "%s-opensandbox" (include "sandbox-dashboard.fullname" .) -}}
{{- end -}}
{{- end -}}

{{/*
The read-only rules the dashboard needs, shared by the ClusterRole and the
per-namespace Roles so the two cannot drift apart.

The leading comment is emitted, not a template comment: deploy/install.yaml is
a published artifact and readers of it should see why there are no write verbs.
*/}}
{{- define "sandbox-dashboard.rbacRules" -}}
# Read-only throughout: the dashboard never writes, and has no verbs to.
- apiGroups: ["agents.x-k8s.io"]
  resources: ["sandboxes"]
  verbs: ["get", "list", "watch"]
- apiGroups: ["extensions.agents.x-k8s.io"]
  resources: ["sandboxtemplates", "sandboxclaims", "sandboxwarmpools"]
  verbs: ["get", "list", "watch"]
- apiGroups: [""]
  resources: ["pods", "events"]
  verbs: ["get", "list", "watch"]
{{- end }}
