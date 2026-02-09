{{/*
Expand the name of the chart.
*/}}
{{- define "agent-pod.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
*/}}
{{- define "agent-pod.fullname" -}}
{{- if .Values.fullnameOverride }}
{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- $name := default .Chart.Name .Values.nameOverride }}
{{- if contains $name .Release.Name }}
{{- .Release.Name | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- printf "%s-%s" .Release.Name $name | trunc 63 | trimSuffix "-" }}
{{- end }}
{{- end }}
{{- end }}

{{/*
Create chart name and version as used by the chart label.
*/}}
{{- define "agent-pod.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels
*/}}
{{- define "agent-pod.labels" -}}
helm.sh/chart: {{ include "agent-pod.chart" . }}
{{ include "agent-pod.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{/*
Selector labels
*/}}
{{- define "agent-pod.selectorLabels" -}}
app.kubernetes.io/name: {{ include "agent-pod.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
Service account name
*/}}
{{- define "agent-pod.serviceAccountName" -}}
{{- if .Values.serviceAccount.create }}
{{- default (include "agent-pod.fullname" .) .Values.serviceAccount.name }}
{{- else }}
{{- default "default" .Values.serviceAccount.name }}
{{- end }}
{{- end }}

{{/*
Agent config secret name (for Anthropic API key)
*/}}
{{- define "agent-pod.apiKeySecretName" -}}
{{- printf "%s-api-key" (include "agent-pod.fullname" .) }}
{{- end }}

{{/*
Git credentials secret name
*/}}
{{- define "agent-pod.gitCredentialsSecretName" -}}
{{- printf "%s-git-credentials" (include "agent-pod.fullname" .) }}
{{- end }}
