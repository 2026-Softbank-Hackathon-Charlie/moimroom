{{- define "moimroom.fullname" -}}
{{- if contains .Chart.Name .Release.Name -}}
{{- .Release.Name | trunc 63 | trimSuffix "-" -}}
{{- else -}}
{{- printf "%s-%s" .Release.Name .Chart.Name | trunc 63 | trimSuffix "-" -}}
{{- end -}}
{{- end -}}

{{- define "moimroom.selectorLabels" -}}
app.kubernetes.io/name: {{ .Chart.Name }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end -}}

{{- define "moimroom.labels" -}}
{{ include "moimroom.selectorLabels" . }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
helm.sh/chart: {{ printf "%s-%s" .Chart.Name .Chart.Version }}
{{- end -}}

{{- define "moimroom.dbSecret" -}}
{{- .Values.database.secretName | default (printf "%s-db-app" .Release.Name) -}}
{{- end -}}

{{- define "moimroom.s3Secret" -}}
{{- .Values.s3.secretName | default (printf "%s-s3" .Release.Name) -}}
{{- end -}}

{{- define "moimroom.appSecret" -}}
{{- .Values.appSecret.name | default (printf "%s-app" .Release.Name) -}}
{{- end -}}
