package templates

const ConsoleConfigRootInitConfigTemplate = `
SetConfigRequest {
  Config {
    TenantsConfig {
      AvailabilityZoneKinds {
        Kind: "any"
      }
      AvailabilityZoneKinds {
        Kind: "single"
        SlotLocation {
          CollocationGroup: 1
          ForceCollocation: true
        }
      }
      AvailabilityZoneKinds {
        Kind: "0"
        DataCenterName: "0"
      }
      AvailabilityZoneSets {
        Name: "all"
        ZoneKinds: "any"
        ZoneKinds: "single"
        ZoneKinds: "0"
      }
      ComputationalUnitKinds {
        Kind: "slot"
        TenantSlotType: "default"
        AvailabilityZoneSet: "all"
      }
    }
    ConfigsConfig {
      UsageScopeRestrictions {
        AllowedNodeIdUsageScopeKinds: 2
        AllowedHostUsageScopeKinds: 2
        AllowedTenantUsageScopeKinds: 2
        AllowedTenantUsageScopeKinds: 29
        AllowedTenantUsageScopeKinds: 38
        AllowedTenantUsageScopeKinds: 45
        AllowedTenantUsageScopeKinds: 1
        AllowedNodeTypeUsageScopeKinds: 2
        AllowedNodeTypeUsageScopeKinds: 45
      }
    }
  }
}
`
