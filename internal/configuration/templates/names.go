package templates

const NamesTemplate = `
{{- range $i := iter .Spec.Nodes }}
Node {
  NodeId: {{ add $i 1 }}
  Port: {{ $.InterconnectPort }}
  Host: "{{ $.ObjectMeta.Name }}-{{ $i }}"
  InterconnectHost: "{{ $.ObjectMeta.Name }}-{{ $i }}.{{ $.ObjectMeta.Name }}-interconnect.{{ $.ObjectMeta.Namespace }}.svc.cluster.local"
  WalleLocation {
    DataCenter: "az-1"
    Rack: "rack-{{ add $i 1 }}"
    Body: {{ add 12340 $i }}
  }
}
{{- end }}
ClusterUUID: "ydb:{{ .ObjectMeta.UID }}"
AcceptUUID: "ydb:{{ .ObjectMeta.UID }}"
`
