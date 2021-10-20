package templates

const BindRootStorageRequestRootInitConfigTemplate = `
ModifyScheme {
  WorkingDir: "/"
  OperationType: ESchemeOpAlterSubDomain
  SubDomain {
    Name: "Root"
    StoragePools {
      Name: "/Root:hdd"
      Kind: "hdd"
    }
  }
}
`
