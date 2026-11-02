{{/* IPFS_API_TOKEN from the Secret of templates/api-token-secret.yaml. */}}
{{- define "ipfs.apiTokenEnv" -}}
- name: IPFS_API_TOKEN
  valueFrom:
    secretKeyRef:
      name: {{ printf "%s-ipfs-api" .Release.Name | quote }}
      key: token
{{- end }}
