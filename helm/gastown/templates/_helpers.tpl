{{/*
Gastown chart helpers.
This is a thin wrapper chart — bd-daemon templates come from the subchart.
Agent controller templates are defined here.
*/}}

{{- define "gastown.fullname" -}}
{{- .Release.Name | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels
*/}}
{{- define "gastown.labels" -}}
helm.sh/chart: {{ printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{ include "gastown.selectorLabels" . }}
{{- end }}

{{/*
Selector labels
*/}}
{{- define "gastown.selectorLabels" -}}
app.kubernetes.io/name: {{ .Chart.Name }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/* ===== BD Daemon subchart naming (for cross-chart references) ===== */}}

{{/*
BD Daemon subchart fullname — mirrors bd-daemon.fullname when used as subchart
*/}}
{{- define "gastown.bdDaemon.fullname" -}}
{{- printf "%s-bd-daemon" (include "gastown.fullname" .) | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Daemon fully qualified name — mirrors bd-daemon.daemon.fullname
*/}}
{{- define "gastown.daemon.fullname" -}}
{{- printf "%s-daemon" (include "gastown.bdDaemon.fullname" .) | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Daemon token secret name — mirrors bd-daemon.daemon.tokenSecretName
*/}}
{{- define "gastown.daemon.tokenSecretName" -}}
{{- printf "%s-token" (include "gastown.daemon.fullname" .) }}
{{- end }}

{{/* ===== Agent Controller component helpers ===== */}}

{{/*
Agent Controller fully qualified name
*/}}
{{- define "gastown.agentController.fullname" -}}
{{- printf "%s-agent-controller" (include "gastown.fullname" .) | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Agent Controller labels
*/}}
{{- define "gastown.agentController.labels" -}}
{{ include "gastown.labels" . }}
app.kubernetes.io/component: agent-controller
{{- end }}

{{/*
Agent Controller selector labels
*/}}
{{- define "gastown.agentController.selectorLabels" -}}
{{ include "gastown.selectorLabels" . }}
app.kubernetes.io/component: agent-controller
{{- end }}

{{/*
Agent Controller service account name
*/}}
{{- define "gastown.agentController.serviceAccountName" -}}
{{- if .Values.agentController.serviceAccount.create }}
{{- default (printf "%s-sa" (include "gastown.agentController.fullname" .)) .Values.agentController.serviceAccount.name }}
{{- else }}
{{- default "default" .Values.agentController.serviceAccount.name }}
{{- end }}
{{- end }}

{{/* ===== Coop Broker component helpers ===== */}}

{{/*
Coop Broker fully qualified name
*/}}
{{- define "gastown.coopBroker.fullname" -}}
{{- printf "%s-coop-broker" (include "gastown.fullname" .) | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Coop Broker labels
*/}}
{{- define "gastown.coopBroker.labels" -}}
{{ include "gastown.labels" . }}
app.kubernetes.io/component: coop-broker
{{- end }}

{{/*
Coop Broker selector labels
*/}}
{{- define "gastown.coopBroker.selectorLabels" -}}
{{ include "gastown.selectorLabels" . }}
app.kubernetes.io/component: coop-broker
{{- end }}

{{/*
Coop Broker auth token secret name
*/}}
{{- define "gastown.coopBroker.authTokenSecretName" -}}
{{- if .Values.coopBroker.authTokenSecret }}
{{- .Values.coopBroker.authTokenSecret }}
{{- else }}
{{- include "gastown.daemon.tokenSecretName" . }}
{{- end }}
{{- end }}

{{/*
Coop Broker credential secret name
*/}}
{{- define "gastown.coopBroker.credentialSecretName" -}}
{{- printf "%s-credentials" (include "gastown.coopBroker.fullname" .) }}
{{- end }}

{{/*
Coop Broker NATS URL — auto-wire from bd-daemon NATS if not set
*/}}
{{- define "gastown.coopBroker.natsURL" -}}
{{- if .Values.coopBroker.natsURL }}
{{- .Values.coopBroker.natsURL }}
{{- else if (index .Values "bd-daemon" "nats" "enabled") }}
{{- printf "nats://%s-bd-daemon-nats:4222" .Release.Name }}
{{- end }}
{{- end }}

{{/*
Coop Broker service URL (for agent pods to register with)
*/}}
{{- define "gastown.coopBroker.serviceURL" -}}
{{- printf "http://%s:%d" (include "gastown.coopBroker.fullname" .) (int .Values.coopBroker.service.port) }}
{{- end }}

{{/*
Coop Mux service URL (for dashboard and session management)
*/}}
{{- define "gastown.coopBroker.muxServiceURL" -}}
{{- printf "http://%s:%d" (include "gastown.coopBroker.fullname" .) (int .Values.coopBroker.service.muxPort) }}
{{- end }}
