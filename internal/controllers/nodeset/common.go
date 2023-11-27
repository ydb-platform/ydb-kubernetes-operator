package nodeset

import (
	"time"
)

const (
	Pending      NodeSetState = "Pending"
	Provisioning NodeSetState = "Provisioning"
	Ready        NodeSetState = "Ready"

	DefaultRequeueDelay      = 10 * time.Second
	StatusUpdateRequeueDelay = 1 * time.Second

	Stop     = true
	Continue = false
)

type NodeSetState string
