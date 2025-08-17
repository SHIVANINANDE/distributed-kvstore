{{/*
Expand the name of the chart.
*/}}
{{- define "kvstore.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
We truncate at 63 chars because some Kubernetes name fields are limited to this (by the DNS naming spec).
If release name contains chart name it will be used as a full name.
*/}}
{{- define "kvstore.fullname" -}}
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
{{- define "kvstore.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels
*/}}
{{- define "kvstore.labels" -}}
helm.sh/chart: {{ include "kvstore.chart" . }}
{{ include "kvstore.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
environment: {{ .Values.global.environment }}
{{- end }}

{{/*
Selector labels
*/}}
{{- define "kvstore.selectorLabels" -}}
app.kubernetes.io/name: {{ include "kvstore.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/component: server
{{- end }}

{{/*
Create the name of the service account to use
*/}}
{{- define "kvstore.serviceAccountName" -}}
{{- if .Values.serviceAccount.create }}
{{- default (include "kvstore.fullname" .) .Values.serviceAccount.name }}
{{- else }}
{{- default "default" .Values.serviceAccount.name }}
{{- end }}
{{- end }}

{{/*
Create the image name
*/}}
{{- define "kvstore.image" -}}
{{- $registry := .Values.image.registry | default .Values.global.imageRegistry }}
{{- $repository := .Values.image.repository | default .Values.global.imageRepository }}
{{- $tag := .Values.image.tag | default .Values.global.imageTag | default .Chart.AppVersion }}
{{- if $registry }}
{{- printf "%s/%s:%s" $registry $repository $tag }}
{{- else }}
{{- printf "%s:%s" $repository $tag }}
{{- end }}
{{- end }}

{{/*
Create the image pull policy
*/}}
{{- define "kvstore.imagePullPolicy" -}}
{{- .Values.image.pullPolicy | default .Values.global.imagePullPolicy }}
{{- end }}

{{/*
Create headless service name
*/}}
{{- define "kvstore.headlessServiceName" -}}
{{- printf "%s-headless" (include "kvstore.fullname" .) }}
{{- end }}

{{/*
Create storage class name
*/}}
{{- define "kvstore.storageClassName" -}}
{{- if .Values.persistence.storageClass }}
{{- if (eq "-" .Values.persistence.storageClass) }}
{{- printf "" }}
{{- else }}
{{- printf "%s" .Values.persistence.storageClass }}
{{- end }}
{{- end }}
{{- end }}

{{/*
Create TLS secret name
*/}}
{{- define "kvstore.tlsSecretName" -}}
{{- if .Values.security.tls.secretName }}
{{- .Values.security.tls.secretName }}
{{- else }}
{{- printf "%s-tls" (include "kvstore.fullname" .) }}
{{- end }}
{{- end }}

{{/*
Validate cluster size
*/}}
{{- define "kvstore.validateClusterSize" -}}
{{- if lt (.Values.cluster.size | int) 1 }}
{{- fail "cluster.size must be at least 1" }}
{{- end }}
{{- if gt (.Values.cluster.size | int) 20 }}
{{- fail "cluster.size cannot exceed 20" }}
{{- end }}
{{- end }}

{{/*
Validate replication factor
*/}}
{{- define "kvstore.validateReplicationFactor" -}}
{{- if lt (.Values.cluster.replicationFactor | int) 1 }}
{{- fail "cluster.replicationFactor must be at least 1" }}
{{- end }}
{{- if gt (.Values.cluster.replicationFactor | int) (.Values.cluster.size | int) }}
{{- fail "cluster.replicationFactor cannot exceed cluster.size" }}
{{- end }}
{{- end }}

{{/*
Calculate minimum available pods for PDB
*/}}
{{- define "kvstore.pdbMinAvailable" -}}
{{- if .Values.podDisruptionBudget.minAvailable }}
{{- .Values.podDisruptionBudget.minAvailable }}
{{- else }}
{{- $size := .Values.cluster.size | int }}
{{- if eq $size 1 }}
{{- 1 }}
{{- else if eq $size 2 }}
{{- 1 }}
{{- else }}
{{- sub $size 1 }}
{{- end }}
{{- end }}
{{- end }}