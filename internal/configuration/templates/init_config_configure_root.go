package templates

const ConfigureRootInitConfigTemplate = `
ConfigureRequest {
  Actions {
    AddConfigItem {
      ConfigItem {
        Config {
          {{- if hasKey .Amap "sys.txt"}}
          ActorSystemConfig {
            {{- (index .Amap "sys.txt") | indent 12 }}
          }
          {{- end }}
          {{- if hasKey .Amap "log.txt"}}
          LogConfig {
            {{- (index .Amap "log.txt") | indent 12 }}
          }
          {{- end }}
          {{- if hasKey .Amap "names.txt"}}
          NameserviceConfig {
            {{- (index .Amap "names.txt") | indent 12 }}
          }
          {{- end }}
          {{- if hasKey .Amap "ic.txt"}}
          InterconnectConfig {
            {{- (index .Amap "ic.txt") | indent 12 }}
          }
          {{- end }}
          {{- if hasKey .Amap "domains.txt"}}
          DomainsConfig {
            {{- (index .Amap "domains.txt") | indent 12 }}
          }
          {{- end }}
          {{- if hasKey .Amap "bs.txt"}}
          BlobStorageConfig {
            {{- (index .Amap "bs.txt") | indent 12 }}
          }
          {{- end }}
          {{- if hasKey .Amap "channels.txt"}}
          ChannelProfileConfig {
            {{- (index .Amap "channels.txt") | indent 12 }}
          }
          {{- end }}
          {{- if hasKey .Amap "boot.txt"}}
          BootstrapConfig {
            {{- (index .Amap "boot.txt") | indent 12 }}
          }
          {{- end }}
          {{- if hasKey .Amap "vdisk.txt"}}
          VDiskConfig {
            {{- (index .Amap "vdisk.txt") | indent 12 }}
          }
          {{- end }}
          {{- if hasKey .Amap "kqp.txt"}}
          KQPConfig {
            {{- (index .Amap "kqp.txt") | indent 12 }}
          }
          {{- end }}
          {{- if hasKey .Amap "grpc.txt"}}
          GRpcConfig {
            {{- (index .Amap "grpc.txt") | indent 12 }}
          }
          {{- end }}
          {{- if hasKey .Amap "feature_flags.txt"}}
          FeatureFlags {
            {{- (index .Amap "feature_flags.txt") | indent 12 }}
          }
          {{- end }}
          {{- if hasKey .Amap "pq.txt"}}
          PQConfig {
            {{- (index .Amap "pq.txt") | indent 12 }}
          }
          {{- end }}
          {{- if hasKey .Amap "auth.txt"}}
          AuthConfig {
            {{- (index .Amap "auth.txt") | indent 12 }}
          }
          {{- end }}
        }
      }
      EnableAutoSplit: true
    }
  }
}
`
