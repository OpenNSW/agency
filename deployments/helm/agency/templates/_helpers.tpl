{{/*
Expand the name of the chart.
*/}}
{{- define "agency.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" -}}
{{- end }}

{{/*
Create a default fully qualified app name.
*/}}
{{- define "agency.fullname" -}}
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

{{/*
Create chart name and version as used by the chart label.
*/}}
{{- define "agency.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels
*/}}
{{- define "agency.labels" -}}
helm.sh/chart: {{ include "agency.chart" . }}
{{ include "agency.selectorLabels" . }}
{{- if .Values.image.tag }}
app.kubernetes.io/version: {{ .Values.image.tag | quote }}
{{- else if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{/*
Selector labels
*/}}
{{- define "agency.selectorLabels" -}}
app.kubernetes.io/name: {{ include "agency.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
Fail the render if the user's own volumes/volumeMounts reuse the "config"
name, which is reserved for the ConfigMap generated from .Values.config /
.Values.configFiles (see templates/configmap.yaml).
*/}}
{{- define "agency.configVolumeGuard" -}}
{{- if or .Values.config .Values.configFiles }}
{{- range .Values.volumes }}
{{- if eq .name "config" }}
{{- fail "volumes: \"config\" is reserved for the generated config ConfigMap — rename this volume" }}
{{- end }}
{{- end }}
{{- range .Values.volumeMounts }}
{{- if eq .name "config" }}
{{- fail "volumeMounts: \"config\" is reserved for the generated config ConfigMap — rename this volumeMount" }}
{{- end }}
{{- end }}
{{- end }}
{{- end }}

{{/*
Env entries the chart injects automatically ahead of .Values.env, currently
just CONFIG_PATH when .Values.config is set (unless the caller already
defines env.CONFIG_PATH themselves).
*/}}
{{- define "agency.autoEnv" -}}
{{- if and .Values.config (not (hasKey .Values.env "CONFIG_PATH")) }}
- name: CONFIG_PATH
  value: {{ printf "%s/config.yaml" .Values.configMountPath | quote }}
{{- end }}
{{- end }}

{{/*
checksum/config annotation value — forces a rollout when .Values.config /
.Values.configFiles content changes, since the Deployment otherwise has no
reference to the generated ConfigMap's contents.
*/}}
{{- define "agency.configChecksum" -}}
{{- list .Values.config .Values.configFiles | toYaml | sha256sum -}}
{{- end }}
