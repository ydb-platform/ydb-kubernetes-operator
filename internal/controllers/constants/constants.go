package constants

import "time"

type ClusterState string

const (
	PausedState  = "Paused"
	FrozenState  = "Frozen"
	RunningState = "Running"

	StoragePausedCondition = "StoragePaused"
	StoragePausedReason    = "PauseIsSet"

	DatabasePausedCondition = "DatabasePaused"
	DatabasePausedReason    = "PauseIsSet"

	Stop     = true
	Continue = false

	ReasonInProgress  = "InProgress"
	ReasonNotRequired = "NotRequired"
	ReasonCompleted   = "Completed"

	DefaultRequeueDelay               = 10 * time.Second
	StatusUpdateRequeueDelay          = 1 * time.Second
	SelfCheckRequeueDelay             = 30 * time.Second
	StorageInitializationRequeueDelay = 5 * time.Second

	DatabasePending      ClusterState = "Pending"
	DatabaseProvisioning ClusterState = "Provisioning"
	DatabaseInitializing ClusterState = "Initializing"
	DatabaseReady        ClusterState = "Ready"
	DatabasePaused       ClusterState = "Paused"

	DatabaseTenantInitializedCondition        = "TenantInitialized"
	DatabaseTenantInitializedReasonInProgress = ReasonInProgress
	DatabaseTenantInitializedReasonCompleted  = ReasonCompleted

	StoragePending      ClusterState = "Pending"
	StoragePreparing    ClusterState = "Preparing"
	StorageProvisioning ClusterState = "Provisioning"
	StorageInitializing ClusterState = "Initializing"
	StorageReady        ClusterState = "Ready"
	StoragePaused       ClusterState = "Paused"

	StorageInitializedCondition        = "StorageReady"
	StorageInitializedReasonInProgress = ReasonInProgress
	StorageInitializedReasonCompleted  = ReasonCompleted

	TenantCreationRequeueDelay      = 30 * time.Second
	StorageAwaitRequeueDelay        = 30 * time.Second
	SharedDatabaseAwaitRequeueDelay = 30 * time.Second
)
