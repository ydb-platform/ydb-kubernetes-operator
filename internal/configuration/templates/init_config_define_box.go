package templates

const DefineBoxInitConfigTemplate = `
Command {
  DefineHostConfig {
    HostConfigId: 1
    Drive {
	  {{- if eq ((index .Spec.DataStore 0).VolumeMode | deref | toString) "Block" }}
	  Path: "/dev/kikimr_ssd_00"
	  {{- end }}
	  {{- if eq ((index .Spec.DataStore 0).VolumeMode | deref | toString) "Filesystem" }}
	  Path: "/data/kikimr"
	  {{- end }}
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
