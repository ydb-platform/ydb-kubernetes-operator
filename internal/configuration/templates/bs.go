package templates

const BlobStorageConfigTemplate = `
ServiceSet {
  PDisks {
    NodeID: 1
    PDiskID: 1
	{{- if eq ((index .Spec.DataStore 0).VolumeMode | deref | toString) "Block" }}
    Path: "/dev/kikimr_ssd_00"
	{{- end }}
	{{- if eq ((index .Spec.DataStore 0).VolumeMode | deref | toString) "Filesystem" }}
    Path: "/data/kikimr"
	{{- end }}
    PDiskGuid: 7072714480107857776
    PDiskCategory: 0
  }
  PDisks {
    NodeID: 2
    PDiskID: 1
	{{- if eq ((index .Spec.DataStore 0).VolumeMode | deref | toString) "Block" }}
    Path: "/dev/kikimr_ssd_00"
	{{- end }}
	{{- if eq ((index .Spec.DataStore 0).VolumeMode | deref | toString) "Filesystem" }}
    Path: "/data/kikimr"
	{{- end }}
    PDiskGuid: 16291166822083105651
    PDiskCategory: 0
  }
  PDisks {
    NodeID: 3
    PDiskID: 1
	{{- if eq ((index .Spec.DataStore 0).VolumeMode | deref | toString) "Block" }}
    Path: "/dev/kikimr_ssd_00"
	{{- end }}
	{{- if eq ((index .Spec.DataStore 0).VolumeMode | deref | toString) "Filesystem" }}
    Path: "/data/kikimr"
	{{- end }}
    PDiskGuid: 6662963332485859848
    PDiskCategory: 0
  }
  PDisks {
    NodeID: 4
    PDiskID: 1
	{{- if eq ((index .Spec.DataStore 0).VolumeMode | deref | toString) "Block" }}
    Path: "/dev/kikimr_ssd_00"
	{{- end }}
	{{- if eq ((index .Spec.DataStore 0).VolumeMode | deref | toString) "Filesystem" }}
    Path: "/data/kikimr"
	{{- end }}
    PDiskGuid: 17826674922888324531
    PDiskCategory: 0
  }
  PDisks {
    NodeID: 5
    PDiskID: 1
	{{- if eq ((index .Spec.DataStore 0).VolumeMode | deref | toString) "Block" }}
    Path: "/dev/kikimr_ssd_00"
	{{- end }}
	{{- if eq ((index .Spec.DataStore 0).VolumeMode | deref | toString) "Filesystem" }}
    Path: "/data/kikimr"
	{{- end }}
    PDiskGuid: 7072509400554621355
    PDiskCategory: 0
  }
  PDisks {
    NodeID: 6
    PDiskID: 1
	{{- if eq ((index .Spec.DataStore 0).VolumeMode | deref | toString) "Block" }}
    Path: "/dev/kikimr_ssd_00"
	{{- end }}
	{{- if eq ((index .Spec.DataStore 0).VolumeMode | deref | toString) "Filesystem" }}
    Path: "/data/kikimr"
	{{- end }}
    PDiskGuid: 2048839831911463872
    PDiskCategory: 0
  }
  PDisks {
    NodeID: 7
    PDiskID: 1
	{{- if eq ((index .Spec.DataStore 0).VolumeMode | deref | toString) "Block" }}
    Path: "/dev/kikimr_ssd_00"
	{{- end }}
	{{- if eq ((index .Spec.DataStore 0).VolumeMode | deref | toString) "Filesystem" }}
    Path: "/data/kikimr"
	{{- end }}
    PDiskGuid: 16453561994362958550
    PDiskCategory: 0
  }
  PDisks {
    NodeID: 8
    PDiskID: 1
	{{- if eq ((index .Spec.DataStore 0).VolumeMode | deref | toString) "Block" }}
    Path: "/dev/kikimr_ssd_00"
	{{- end }}
	{{- if eq ((index .Spec.DataStore 0).VolumeMode | deref | toString) "Filesystem" }}
    Path: "/data/kikimr"
	{{- end }}
    PDiskGuid: 13131501771972430830
    PDiskCategory: 0
  }
  VDisks {
    VDiskID {
      GroupID: 0
      GroupGeneration: 1
      Ring: 0
      Domain: 0
      VDisk: 0
    }
    VDiskLocation {
      NodeID: 1
      PDiskID: 1
      VDiskSlotID: 0
      PDiskGuid: 7072714480107857776
    }
    VDiskKind: Default
  }
  VDisks {
    VDiskID {
      GroupID: 0
      GroupGeneration: 1
      Ring: 0
      Domain: 1
      VDisk: 0
    }
    VDiskLocation {
      NodeID: 2
      PDiskID: 1
      VDiskSlotID: 0
      PDiskGuid: 16291166822083105651
    }
    VDiskKind: Default
  }
  VDisks {
    VDiskID {
      GroupID: 0
      GroupGeneration: 1
      Ring: 0
      Domain: 2
      VDisk: 0
    }
    VDiskLocation {
      NodeID: 3
      PDiskID: 1
      VDiskSlotID: 0
      PDiskGuid: 6662963332485859848
    }
    VDiskKind: Default
  }
  VDisks {
    VDiskID {
      GroupID: 0
      GroupGeneration: 1
      Ring: 0
      Domain: 3
      VDisk: 0
    }
    VDiskLocation {
      NodeID: 4
      PDiskID: 1
      VDiskSlotID: 0
      PDiskGuid: 17826674922888324531
    }
    VDiskKind: Default
  }
  VDisks {
    VDiskID {
      GroupID: 0
      GroupGeneration: 1
      Ring: 0
      Domain: 4
      VDisk: 0
    }
    VDiskLocation {
      NodeID: 5
      PDiskID: 1
      VDiskSlotID: 0
      PDiskGuid: 7072509400554621355
    }
    VDiskKind: Default
  }
  VDisks {
    VDiskID {
      GroupID: 0
      GroupGeneration: 1
      Ring: 0
      Domain: 5
      VDisk: 0
    }
    VDiskLocation {
      NodeID: 6
      PDiskID: 1
      VDiskSlotID: 0
      PDiskGuid: 2048839831911463872
    }
    VDiskKind: Default
  }
  VDisks {
    VDiskID {
      GroupID: 0
      GroupGeneration: 1
      Ring: 0
      Domain: 6
      VDisk: 0
    }
    VDiskLocation {
      NodeID: 7
      PDiskID: 1
      VDiskSlotID: 0
      PDiskGuid: 16453561994362958550
    }
    VDiskKind: Default
  }
  VDisks {
    VDiskID {
      GroupID: 0
      GroupGeneration: 1
      Ring: 0
      Domain: 7
      VDisk: 0
    }
    VDiskLocation {
      NodeID: 8
      PDiskID: 1
      VDiskSlotID: 0
      PDiskGuid: 13131501771972430830
    }
    VDiskKind: Default
  }
  Groups {
    GroupID: 0
    GroupGeneration: 1
    ErasureSpecies: 4
    Rings {
      FailDomains {
        VDiskLocations {
          NodeID: 1
          PDiskID: 1
          VDiskSlotID: 0
          PDiskGuid: 7072714480107857776
        }
      }
      FailDomains {
        VDiskLocations {
          NodeID: 2
          PDiskID: 1
          VDiskSlotID: 0
          PDiskGuid: 16291166822083105651
        }
      }
      FailDomains {
        VDiskLocations {
          NodeID: 3
          PDiskID: 1
          VDiskSlotID: 0
          PDiskGuid: 6662963332485859848
        }
      }
      FailDomains {
        VDiskLocations {
          NodeID: 4
          PDiskID: 1
          VDiskSlotID: 0
          PDiskGuid: 17826674922888324531
        }
      }
      FailDomains {
        VDiskLocations {
          NodeID: 5
          PDiskID: 1
          VDiskSlotID: 0
          PDiskGuid: 7072509400554621355
        }
      }
      FailDomains {
        VDiskLocations {
          NodeID: 6
          PDiskID: 1
          VDiskSlotID: 0
          PDiskGuid: 2048839831911463872
        }
      }
      FailDomains {
        VDiskLocations {
          NodeID: 7
          PDiskID: 1
          VDiskSlotID: 0
          PDiskGuid: 16453561994362958550
        }
      }
      FailDomains {
        VDiskLocations {
          NodeID: 8
          PDiskID: 1
          VDiskSlotID: 0
          PDiskGuid: 13131501771972430830
        }
      }
    }
  }
  AvailabilityDomains: 1
}
`
