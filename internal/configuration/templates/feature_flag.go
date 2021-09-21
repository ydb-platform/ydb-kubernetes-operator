package templates

const FeatureFlagTemplate = `
EnableSeparateSolomonShardForPDisk: true
AllowConsistentOperationsForSchemeShard: true
EnableExternalSubdomains: true
SendSchemaVersionToDatashard: true
EnableSchemeBoardCache: true
EnableSystemViews: true
EnableExternalHive: true
UseSchemeBoardCacheForSchemeRequests: true
CompileMinikqlWithVersion: true
ReadTableWithSnapshot: true
EnableOfflineSlaves: true
AllowOnlineIndexBuild: true
EnablePersistentQueryStats: true
DisableDataShardBarrier: true
EnableTtlOnIndexedTables: true
`
