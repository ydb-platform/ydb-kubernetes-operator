package templates

const GRpcConfigTemplate = `
StartGRpcProxy: true
Host: "[::]"
Port: {{ .GRPCPort }}
GRpcMemoryQuotaBytes: 1073741824
StreamingConfig {
  EnableOutputStreams: true
}
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
