package templates

const CMSInitScriptTemplate = `
set -eu

echo Console-Config-Root.txt
cat /opt/kikimr/cfg/Console-Config-Root.txt
{{- if .Spec.Service.Interconnect.TLSConfiguration.Enabled }}
/opt/kikimr/bin/kikimr -s grpcs://{{ .Name }}-grpc.{{ .Namespace }}.svc.cluster.local:2135 admin console execute --domain=root --retry=10 /opt/kikimr/cfg/Console-Config-Root.txt
{{- else }}
/opt/kikimr/bin/kikimr admin console execute --domain={{ .Domain }} --retry=10 /opt/kikimr/cfg/Console-Config-Root.txt
{{- end }}

echo Configure-Root.txt
cat /opt/kikimr/cfg/Configure-Root.txt
{{- if .Spec.Service.Interconnect.TLSConfiguration.Enabled }}
/opt/kikimr/bin/kikimr -s grpcs://{{ .Name }}-grpc.{{ .Namespace }}.svc.cluster.local:2135 admin console execute --domain={{ .Domain }} --retry=10 /opt/kikimr/cfg/Configure-Root.txt
{{- else }}
/opt/kikimr/bin/kikimr admin console execute --domain=root --retry=10 /opt/kikimr/cfg/Configure-Root.txt
{{- end }}
`
