package templates

const DomainsConfigTemplate = `
Domain {
  DomainId: 1
  SchemeRoot: 72057594046678944
  SSId: 1
  HiveUid: 1
  PlanResolution: 10
  Name: "{{ .Domain }}"
  StoragePoolTypes {
    Kind: "hdd"
    PoolConfig {
      BoxId: 1
      ErasureSpecies: "block-4-2"
      VDiskKind: "Default"
      Kind: "hdd"
      PDiskFilter {
        Property {
          Type: ROT
        }
      }
    }
  }
  ExplicitMediators: 72057594046382081
  ExplicitMediators: 72057594046382082
  ExplicitMediators: 72057594046382083
  ExplicitCoordinators: 72057594046316545
  ExplicitCoordinators: 72057594046316546
  ExplicitCoordinators: 72057594046316547
  ExplicitAllocators: 72057594046447617
  ExplicitAllocators: 72057594046447618
  ExplicitAllocators: 72057594046447619
}
StateStorage {
  SSId: 1
  Ring {
    NToSelect: 5
    Node: 1
    Node: 2
    Node: 3
    Node: 4
    Node: 5
    Node: 6
    Node: 7
    Node: 8
  }
}
HiveConfig {
  HiveUid: 1
  Hive: 72057594037968897
}
ForbidImplicitStoragePools: true
`
