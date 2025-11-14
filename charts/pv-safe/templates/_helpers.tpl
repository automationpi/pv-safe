{{/*
Expand the name of the chart.
*/}}
{{- define "pv-safe.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
*/}}
{{- define "pv-safe.fullname" -}}
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
{{- define "pv-safe.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels
*/}}
{{- define "pv-safe.labels" -}}
helm.sh/chart: {{ include "pv-safe.chart" . }}
{{ include "pv-safe.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- with .Values.commonLabels }}
{{ toYaml . }}
{{- end }}
{{- end }}

{{/*
Selector labels
*/}}
{{- define "pv-safe.selectorLabels" -}}
app.kubernetes.io/name: {{ include "pv-safe.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
app: pv-safe-webhook
{{- end }}

{{/*
Create the name of the service account to use
*/}}
{{- define "pv-safe.serviceAccountName" -}}
{{- if .Values.serviceAccount.create }}
{{- default (include "pv-safe.fullname" .) .Values.serviceAccount.name }}
{{- else }}
{{- default "default" .Values.serviceAccount.name }}
{{- end }}
{{- end }}

{{/*
Create the namespace name
*/}}
{{- define "pv-safe.namespace" -}}
{{- default "pv-safe-system" .Values.namespace.name }}
{{- end }}

{{/*
Create the certificate issuer name
*/}}
{{- define "pv-safe.issuerName" -}}
{{- if .Values.certificate.issuer.create }}
{{- printf "%s-selfsigned" (include "pv-safe.fullname" .) }}
{{- else }}
{{- .Values.certificate.issuer.name }}
{{- end }}
{{- end }}
