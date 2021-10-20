package templates

const DefineStoragePoolsInitConfigTemplate = `
Command {
  DefineStoragePool {
    BoxId: 1
    StoragePoolId: 1
    Name: "/root:hdd"
    ErasureSpecies: "block-4-2"
    VDiskKind: "Default"
    Kind: "hdd"
    NumGroups: 7
    PDiskFilter {
      Property {
        Type: ROT
      }
    }
  }
}

`
