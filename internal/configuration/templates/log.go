package templates

const LogConfigTemplate = `
Entry {
  Component: "FLAT_TX_SCHEMESHARD"
  Level: 7
}
Entry {
  Component: "TENANT_SLOT_BROKER"
  Level: 7
}
Entry {
  Component: "TX_DATASHARD"
  Level: 5
}
Entry {
  Component: "HIVE"
  Level: 7
}
Entry {
  Component: "LOCAL"
  Level: 7
}
Entry {
  Component: "TX_PROXY"
  Level: 5
}
Entry {
  Component: "BS_CONTROLLER"
  Level: 3
}
`
