{{- define "omastx.name" -}}
{{- .Chart.Name -}}
{{- end -}}

{{- define "omastx.fullname" -}}
{{- if contains .Chart.Name .Release.Name -}}
{{- .Release.Name | trunc 63 | trimSuffix "-" -}}
{{- else -}}
{{- printf "%s-%s" .Release.Name .Chart.Name | trunc 63 | trimSuffix "-" -}}
{{- end -}}
{{- end -}}

{{- define "omastx.labels" -}}
helm.sh/chart: {{ printf "%s-%s" .Chart.Name .Chart.Version }}
app.kubernetes.io/name: {{ include "omastx.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end -}}

{{- define "omastx.backend.fullname" -}}
{{ include "omastx.fullname" . }}-backend
{{- end -}}

{{- define "omastx.frontend.fullname" -}}
{{ include "omastx.fullname" . }}-frontend
{{- end -}}

{{- define "omastx.postgres.fullname" -}}
{{ include "omastx.fullname" . }}-postgres
{{- end -}}

{{- define "omastx.existingSecret" -}}
{{- required "existingSecret is required: create a Secret with OMASTX_MASTER_KEY, OMASTX_ADMIN_EMAIL, OMASTX_ADMIN_PASSWORD and DATABASE_URL (or POSTGRES_PASSWORD with postgres.internal.enabled), then set existingSecret to its name" .Values.existingSecret -}}
{{- end -}}
