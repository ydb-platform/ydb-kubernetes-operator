package templates

const StorageInitScriptTemplate = `
set -eu

echo DefineBox.txt
cat /opt/kikimr/cfg/DefineBox.txt
{{- if .Spec.Service.Interconnect.TLSConfiguration.Enabled }}
/opt/kikimr/bin/kikimr -s grpcs://{{ .Name }}-grpc.{{ .Namespace }}.svc.cluster.local:2135 admin bs config invoke --proto-file /opt/kikimr/cfg/DefineBox.txt
{{- else }}
/opt/kikimr/bin/kikimr admin bs config invoke --proto-file /opt/kikimr/cfg/DefineBox.txt
{{- end }}

echo DefineStoragePools.txt
cat /opt/kikimr/cfg/DefineStoragePools.txt
{{- if .Spec.Service.Interconnect.TLSConfiguration.Enabled }}
/opt/kikimr/bin/kikimr -s grpcs://{{ .Name }}-grpc.{{ .Namespace }}.svc.cluster.local:2135 admin bs config invoke --proto-file /opt/kikimr/cfg/DefineStoragePools.txt
{{- else }}
/opt/kikimr/bin/kikimr admin bs config invoke --proto-file /opt/kikimr/cfg/DefineStoragePools.txt
{{- end }}
`
