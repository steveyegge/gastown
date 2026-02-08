{{/*
Gastown chart helpers.
This is a thin wrapper chart — all templates come from the bd-daemon subchart.
*/}}

{{- define "gastown.fullname" -}}
{{- .Release.Name | trunc 63 | trimSuffix "-" }}
{{- end }}
