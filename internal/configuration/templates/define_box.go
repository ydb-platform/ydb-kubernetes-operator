package templates

const DefineBoxTemplate = `
Command {
  DefineHostConfig {
    HostConfigId: 1
    Drive {
      Path: "/dev/kikimr_ssd_00"
    }
  }
}
Command {
  DefineBox {
    BoxId: 1
    {{- range $i := iter .Spec.Nodes }}
    Host {
      Key {
        Fqdn: "{{ $.ObjectMeta.Name }}-{{ $i }}"
        IcPort: {{ $.InterconnectPort }}
      }
      HostConfigId: 1
    }
    {{- end }}
  }
}
`
