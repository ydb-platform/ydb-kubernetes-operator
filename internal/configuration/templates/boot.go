package templates

const BootstrapConfigTemplate = `
Tablet {
  Type: FLAT_HIVE
  Node: 1
  Node: 2
  Node: 3
  Node: 4
  Node: 5
  Node: 6
  Node: 7
  Node: 8
  Info {
    TabletID: 72057594037968897
    Channels {
      Channel: 0
      History {
        FromGeneration: 0
        GroupID: 0
      }
      ChannelErasureName: "block-4-2"
    }
    Channels {
      Channel: 1
      History {
        FromGeneration: 0
        GroupID: 0
      }
      ChannelErasureName: "block-4-2"
    }
    Channels {
      Channel: 2
      History {
        FromGeneration: 0
        GroupID: 0
      }
      ChannelErasureName: "block-4-2"
    }
  }
}
Tablet {
  Type: FLAT_BS_CONTROLLER
  Node: 1
  Node: 2
  Node: 3
  Node: 4
  Node: 5
  Node: 6
  Node: 7
  Node: 8
  Info {
    TabletID: 72057594037932033
    Channels {
      Channel: 0
      History {
        FromGeneration: 0
        GroupID: 0
      }
      ChannelErasureName: "block-4-2"
    }
    Channels {
      Channel: 1
      History {
        FromGeneration: 0
        GroupID: 0
      }
      ChannelErasureName: "block-4-2"
    }
    Channels {
      Channel: 2
      History {
        FromGeneration: 0
        GroupID: 0
      }
      ChannelErasureName: "block-4-2"
    }
  }
}
Tablet {
  Type: FLAT_SCHEMESHARD
  Node: 1
  Node: 2
  Node: 3
  Node: 4
  Node: 5
  Node: 6
  Node: 7
  Node: 8
  Info {
    TabletID: 72057594046678944
    Channels {
      Channel: 0
      History {
        FromGeneration: 0
        GroupID: 0
      }
      ChannelErasureName: "block-4-2"
    }
    Channels {
      Channel: 1
      History {
        FromGeneration: 0
        GroupID: 0
      }
      ChannelErasureName: "block-4-2"
    }
    Channels {
      Channel: 2
      History {
        FromGeneration: 0
        GroupID: 0
      }
      ChannelErasureName: "block-4-2"
    }
  }
}
Tablet {
  Type: FLAT_TX_COORDINATOR
  Node: 1
  Node: 2
  Node: 3
  Node: 4
  Node: 5
  Node: 6
  Node: 7
  Node: 8
  Info {
    TabletID: 72057594046316545
    Channels {
      Channel: 0
      History {
        FromGeneration: 0
        GroupID: 0
      }
      ChannelErasureName: "block-4-2"
    }
    Channels {
      Channel: 1
      History {
        FromGeneration: 0
        GroupID: 0
      }
      ChannelErasureName: "block-4-2"
    }
    Channels {
      Channel: 2
      History {
        FromGeneration: 0
        GroupID: 0
      }
      ChannelErasureName: "block-4-2"
    }
  }
}
Tablet {
  Type: FLAT_TX_COORDINATOR
  Node: 1
  Node: 2
  Node: 3
  Node: 4
  Node: 5
  Node: 6
  Node: 7
  Node: 8
  Info {
    TabletID: 72057594046316546
    Channels {
      Channel: 0
      History {
        FromGeneration: 0
        GroupID: 0
      }
      ChannelErasureName: "block-4-2"
    }
    Channels {
      Channel: 1
      History {
        FromGeneration: 0
        GroupID: 0
      }
      ChannelErasureName: "block-4-2"
    }
    Channels {
      Channel: 2
      History {
        FromGeneration: 0
        GroupID: 0
      }
      ChannelErasureName: "block-4-2"
    }
  }
}
Tablet {
  Type: FLAT_TX_COORDINATOR
  Node: 1
  Node: 2
  Node: 3
  Node: 4
  Node: 5
  Node: 6
  Node: 7
  Node: 8
  Info {
    TabletID: 72057594046316547
    Channels {
      Channel: 0
      History {
        FromGeneration: 0
        GroupID: 0
      }
      ChannelErasureName: "block-4-2"
    }
    Channels {
      Channel: 1
      History {
        FromGeneration: 0
        GroupID: 0
      }
      ChannelErasureName: "block-4-2"
    }
    Channels {
      Channel: 2
      History {
        FromGeneration: 0
        GroupID: 0
      }
      ChannelErasureName: "block-4-2"
    }
  }
}
Tablet {
  Type: TX_MEDIATOR
  Node: 1
  Node: 2
  Node: 3
  Node: 4
  Node: 5
  Node: 6
  Node: 7
  Node: 8
  Info {
    TabletID: 72057594046382081
    Channels {
      Channel: 0
      History {
        FromGeneration: 0
        GroupID: 0
      }
      ChannelErasureName: "block-4-2"
    }
    Channels {
      Channel: 1
      History {
        FromGeneration: 0
        GroupID: 0
      }
      ChannelErasureName: "block-4-2"
    }
    Channels {
      Channel: 2
      History {
        FromGeneration: 0
        GroupID: 0
      }
      ChannelErasureName: "block-4-2"
    }
  }
}
Tablet {
  Type: TX_MEDIATOR
  Node: 1
  Node: 2
  Node: 3
  Node: 4
  Node: 5
  Node: 6
  Node: 7
  Node: 8
  Info {
    TabletID: 72057594046382082
    Channels {
      Channel: 0
      History {
        FromGeneration: 0
        GroupID: 0
      }
      ChannelErasureName: "block-4-2"
    }
    Channels {
      Channel: 1
      History {
        FromGeneration: 0
        GroupID: 0
      }
      ChannelErasureName: "block-4-2"
    }
    Channels {
      Channel: 2
      History {
        FromGeneration: 0
        GroupID: 0
      }
      ChannelErasureName: "block-4-2"
    }
  }
}
Tablet {
  Type: TX_MEDIATOR
  Node: 1
  Node: 2
  Node: 3
  Node: 4
  Node: 5
  Node: 6
  Node: 7
  Node: 8
  Info {
    TabletID: 72057594046382083
    Channels {
      Channel: 0
      History {
        FromGeneration: 0
        GroupID: 0
      }
      ChannelErasureName: "block-4-2"
    }
    Channels {
      Channel: 1
      History {
        FromGeneration: 0
        GroupID: 0
      }
      ChannelErasureName: "block-4-2"
    }
    Channels {
      Channel: 2
      History {
        FromGeneration: 0
        GroupID: 0
      }
      ChannelErasureName: "block-4-2"
    }
  }
}
Tablet {
  Type: TX_ALLOCATOR
  Node: 1
  Node: 2
  Node: 3
  Node: 4
  Node: 5
  Node: 6
  Node: 7
  Node: 8
  Info {
    TabletID: 72057594046447617
    Channels {
      Channel: 0
      History {
        FromGeneration: 0
        GroupID: 0
      }
      ChannelErasureName: "block-4-2"
    }
    Channels {
      Channel: 1
      History {
        FromGeneration: 0
        GroupID: 0
      }
      ChannelErasureName: "block-4-2"
    }
    Channels {
      Channel: 2
      History {
        FromGeneration: 0
        GroupID: 0
      }
      ChannelErasureName: "block-4-2"
    }
  }
}
Tablet {
  Type: TX_ALLOCATOR
  Node: 1
  Node: 2
  Node: 3
  Node: 4
  Node: 5
  Node: 6
  Node: 7
  Node: 8
  Info {
    TabletID: 72057594046447618
    Channels {
      Channel: 0
      History {
        FromGeneration: 0
        GroupID: 0
      }
      ChannelErasureName: "block-4-2"
    }
    Channels {
      Channel: 1
      History {
        FromGeneration: 0
        GroupID: 0
      }
      ChannelErasureName: "block-4-2"
    }
    Channels {
      Channel: 2
      History {
        FromGeneration: 0
        GroupID: 0
      }
      ChannelErasureName: "block-4-2"
    }
  }
}
Tablet {
  Type: TX_ALLOCATOR
  Node: 1
  Node: 2
  Node: 3
  Node: 4
  Node: 5
  Node: 6
  Node: 7
  Node: 8
  Info {
    TabletID: 72057594046447619
    Channels {
      Channel: 0
      History {
        FromGeneration: 0
        GroupID: 0
      }
      ChannelErasureName: "block-4-2"
    }
    Channels {
      Channel: 1
      History {
        FromGeneration: 0
        GroupID: 0
      }
      ChannelErasureName: "block-4-2"
    }
    Channels {
      Channel: 2
      History {
        FromGeneration: 0
        GroupID: 0
      }
      ChannelErasureName: "block-4-2"
    }
  }
}
Tablet {
  Type: CMS
  Node: 1
  Node: 2
  Node: 3
  Node: 4
  Node: 5
  Node: 6
  Node: 7
  Node: 8
  Info {
    TabletID: 72057594037936128
    Channels {
      Channel: 0
      History {
        FromGeneration: 0
        GroupID: 0
      }
      ChannelErasureName: "block-4-2"
    }
    Channels {
      Channel: 1
      History {
        FromGeneration: 0
        GroupID: 0
      }
      ChannelErasureName: "block-4-2"
    }
    Channels {
      Channel: 2
      History {
        FromGeneration: 0
        GroupID: 0
      }
      ChannelErasureName: "block-4-2"
    }
  }
}
Tablet {
  Type: NODE_BROKER
  Node: 1
  Node: 2
  Node: 3
  Node: 4
  Node: 5
  Node: 6
  Node: 7
  Node: 8
  Info {
    TabletID: 72057594037936129
    Channels {
      Channel: 0
      History {
        FromGeneration: 0
        GroupID: 0
      }
      ChannelErasureName: "block-4-2"
    }
    Channels {
      Channel: 1
      History {
        FromGeneration: 0
        GroupID: 0
      }
      ChannelErasureName: "block-4-2"
    }
    Channels {
      Channel: 2
      History {
        FromGeneration: 0
        GroupID: 0
      }
      ChannelErasureName: "block-4-2"
    }
  }
}
Tablet {
  Type: TENANT_SLOT_BROKER
  Node: 1
  Node: 2
  Node: 3
  Node: 4
  Node: 5
  Node: 6
  Node: 7
  Node: 8
  Info {
    TabletID: 72057594037936130
    Channels {
      Channel: 0
      History {
        FromGeneration: 0
        GroupID: 0
      }
      ChannelErasureName: "block-4-2"
    }
    Channels {
      Channel: 1
      History {
        FromGeneration: 0
        GroupID: 0
      }
      ChannelErasureName: "block-4-2"
    }
    Channels {
      Channel: 2
      History {
        FromGeneration: 0
        GroupID: 0
      }
      ChannelErasureName: "block-4-2"
    }
  }
}
Tablet {
  Type: CONSOLE
  Node: 1
  Node: 2
  Node: 3
  Node: 4
  Node: 5
  Node: 6
  Node: 7
  Node: 8
  Info {
    TabletID: 72057594037936131
    Channels {
      Channel: 0
      History {
        FromGeneration: 0
        GroupID: 0
      }
      ChannelErasureName: "block-4-2"
    }
    Channels {
      Channel: 1
      History {
        FromGeneration: 0
        GroupID: 0
      }
      ChannelErasureName: "block-4-2"
    }
    Channels {
      Channel: 2
      History {
        FromGeneration: 0
        GroupID: 0
      }
      ChannelErasureName: "block-4-2"
    }
  }
}
ResourceBroker {
  Queues {
    Name: "queue_default"
    Weight: 30
    Limit {
      Resource: 2
    }
  }
  Queues {
    Name: "queue_compaction_gen0"
    Weight: 100
    Limit {
      Resource: 10
    }
  }
  Queues {
    Name: "queue_compaction_gen1"
    Weight: 100
    Limit {
      Resource: 6
    }
  }
  Queues {
    Name: "queue_compaction_gen2"
    Weight: 100
    Limit {
      Resource: 3
    }
  }
  Queues {
    Name: "queue_compaction_gen3"
    Weight: 100
    Limit {
      Resource: 3
    }
  }
  Queues {
    Name: "queue_transaction"
    Weight: 100
    Limit {
      Resource: 4
    }
  }
  Queues {
    Name: "queue_background_compaction"
    Weight: 10
    Limit {
      Resource: 1
    }
  }
  Queues {
    Name: "queue_scan"
    Weight: 100
    Limit {
      Resource: 10
    }
  }
  Queues {
    Name: "queue_kqp_resource_manager"
    Weight: 30
    Limit {
      Resource: 4
      Resource: 10737418240
    }
  }
  Queues {
    Name: "queue_build_index"
    Weight: 100
    Limit {
      Resource: 10
    }
  }
  Tasks {
    Name: "unknown"
    QueueName: "queue_default"
    DefaultDuration: 60000000
  }
  Tasks {
    Name: "compaction_gen0"
    QueueName: "queue_compaction_gen0"
    DefaultDuration: 10000000
  }
  Tasks {
    Name: "compaction_gen1"
    QueueName: "queue_compaction_gen1"
    DefaultDuration: 30000000
  }
  Tasks {
    Name: "compaction_gen2"
    QueueName: "queue_compaction_gen2"
    DefaultDuration: 120000000
  }
  Tasks {
    Name: "compaction_gen3"
    QueueName: "queue_compaction_gen3"
    DefaultDuration: 600000000
  }
  Tasks {
    Name: "transaction"
    QueueName: "queue_transaction"
    DefaultDuration: 600000000
  }
  Tasks {
    Name: "background_compaction"
    QueueName: "queue_background_compaction"
    DefaultDuration: 60000000
  }
  Tasks {
    Name: "background_compaction_gen0"
    QueueName: "queue_background_compaction"
    DefaultDuration: 10000000
  }
  Tasks {
    Name: "background_compaction_gen1"
    QueueName: "queue_background_compaction"
    DefaultDuration: 20000000
  }
  Tasks {
    Name: "background_compaction_gen2"
    QueueName: "queue_background_compaction"
    DefaultDuration: 60000000
  }
  Tasks {
    Name: "background_compaction_gen3"
    QueueName: "queue_background_compaction"
    DefaultDuration: 300000000
  }
  Tasks {
    Name: "scan"
    QueueName: "queue_scan"
    DefaultDuration: 300000000
  }
  Tasks {
    Name: "kqp_query"
    QueueName: "queue_kqp_resource_manager"
    DefaultDuration: 600000000
  }
  Tasks {
    Name: "build_index"
    QueueName: "queue_build_index"
    DefaultDuration: 600000000
  }
  ResourceLimit {
    Resource: 20
    Resource: 17179869184
  }
}
SharedCacheConfig {
  MemoryLimit: 10000000000
}
`
