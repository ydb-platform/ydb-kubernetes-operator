package templates

const GRpcConfigTemplate = `
StartGRpcProxy: true
Host: "[::]"
GRpcMemoryQuotaBytes: 1073741824
StreamingConfig {
  EnableOutputStreams: true
}
{{- if .Spec.Service.Interconnect.TLSConfiguration.Enabled }}
SslPort: {{ .GRPCPort }}
CA: "/tls/grpc/ca.crt"
Cert: "/tls/grpc/tls.crt"
Key: "/tls/grpc/tls.key"
{{- else }}
Port: {{ .GRPCPort }}
{{- end }}
Services: "legacy"
Services: "experimental"
Services: "yql"
Services: "discovery"
Services: "cms"
Services: "locking"
Services: "kesus"
Services: "scripting"
Services: "s3_internal"
Services: "rate_limiter"
Services: "monitoring"
KeepAliveEnable: true
KeepAliveIdleTimeoutTriggerSec: 90
KeepAliveMaxProbeCount: 3
KeepAliveProbeIntervalSec: 10
`
