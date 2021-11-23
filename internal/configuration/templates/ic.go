package templates

const InterconnectConfigTemplate = `
StartTcp: true
{{- if .Spec.Service.Interconnect.TLSConfiguration.Enabled }}
EncryptionMode: REQUIRED
{{- end }}
MaxInflightAmountOfDataInKB: 10240
HandshakeTimeoutDuration {
  Seconds: 1
}
`
