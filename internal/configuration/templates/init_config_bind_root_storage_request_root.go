package templates

const BindRootStorageRequestRootInitConfigTemplate = `
ModifyScheme {
  WorkingDir: "/"
  OperationType: ESchemeOpAlterSubDomain
  SubDomain {
    Name: "root"
    StoragePools {
      Name: "/root:hdd"
      Kind: "hdd"
    }
  }
}
`
