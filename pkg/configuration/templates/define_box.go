package templates

//fixme IcPort

const DefineBoxTemplate = `
Command {
  DefineHostConfig {
    HostConfigId: 1
    Drive {
      Path: "/dev/kikimr_ssd_01"
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
