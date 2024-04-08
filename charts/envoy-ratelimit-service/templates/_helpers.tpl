{{- define "envoy-ratelimit-service.pod.extraSpecs" -}}
{{- if .Values.priorityClassName }}
priorityClassName: {{ .Values.priorityClassName }}
{{- end }}

{{- if .Values.tolerations }}
tolerations:
{{ toYaml .Values.tolerations }}
{{- end }}

{{- if .Values.nodeSelector }}
nodeSelector:
{{ toYaml .Values.nodeSelector | nindent 2 }}
{{- end }}

{{- if .Values.affinity }}
affinity:
{{ toYaml .Values.affinity | nindent 2 }}
{{- end }}

{{- end -}}
