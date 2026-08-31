{{/*
Chart name, overridable.
*/}}
{{- define "autopilot.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Fully qualified release name, capped at 63 characters so it is a valid label
value even for long release names.
*/}}
{{- define "autopilot.fullname" -}}
{{- if .Values.fullnameOverride }}
{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" }}
{{- else if contains (include "autopilot.name" .) .Release.Name }}
{{- .Release.Name | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- printf "%s-%s" .Release.Name (include "autopilot.name" .) | trunc 63 | trimSuffix "-" }}
{{- end }}
{{- end }}

{{- define "autopilot.labels" -}}
helm.sh/chart: {{ printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{ include "autopilot.selectorLabels" . }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
app.kubernetes.io/component: control-plane
app.kubernetes.io/part-of: inference-fabric-autopilot
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{/*
Selector labels. These end up in an immutable Deployment selector, so they must
never include anything that changes between upgrades — a chart version or an
app version here makes every upgrade fail.
*/}}
{{- define "autopilot.selectorLabels" -}}
app.kubernetes.io/name: {{ include "autopilot.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{- define "autopilot.serviceAccountName" -}}
{{- if .Values.serviceAccount.create }}
{{- default (include "autopilot.fullname" .) .Values.serviceAccount.name }}
{{- else }}
{{- default "default" .Values.serviceAccount.name }}
{{- end }}
{{- end }}

{{/*
Full image reference. The tag falls back to the chart's appVersion so an
install without an explicit tag is still pinned to a release rather than to a
moving "latest".
*/}}
{{- define "autopilot.image" -}}
{{- $registry := .Values.image.registry -}}
{{- $repo := .Values.image.repository -}}
{{- $tag := default .Chart.AppVersion .Values.image.tag -}}
{{- if $registry -}}
{{- printf "%s/%s:%s" $registry $repo $tag -}}
{{- else -}}
{{- printf "%s:%s" $repo $tag -}}
{{- end -}}
{{- end }}
