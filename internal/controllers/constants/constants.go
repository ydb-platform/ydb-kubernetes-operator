package constants

import "time"

type ClusterState string
type RemoteResourceState string

const (
	StoragePausedCondition             = "StoragePaused"
	StorageInitializedCondition        = "StorageReady"
	StorageNodeSetReadyCondition       = "StorageNodeSetReady"
	DatabasePausedCondition            = "DatabasePaused"
	DatabaseTenantInitializedCondition = "TenantInitialized"
	DatabaseNodeSetReadyCondition      = "DatabaseNodeSetReady"
	RemoteResourceSyncedCondition      = "ResourceSynced"

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
	DatabasePreparing    ClusterState = "Preparing"
	DatabaseProvisioning ClusterState = "Provisioning"
	DatabaseInitializing ClusterState = "Initializing"
	DatabaseReady        ClusterState = "Ready"
	DatabasePaused       ClusterState = "Paused"

	DatabaseNodeSetPending      ClusterState = "Pending"
	DatabaseNodeSetProvisioning ClusterState = "Provisioning"
	DatabaseNodeSetReady        ClusterState = "Ready"
	DatabaseNodeSetPaused       ClusterState = "Paused"

	StoragePending      ClusterState = "Pending"
	StoragePreparing    ClusterState = "Preparing"
	StorageProvisioning ClusterState = "Provisioning"
	StorageInitializing ClusterState = "Initializing"
	StorageReady        ClusterState = "Ready"
	StoragePaused       ClusterState = "Paused"

	StorageNodeSetPending      ClusterState = "Pending"
	StorageNodeSetProvisioning ClusterState = "Provisioning"
	StorageNodeSetReady        ClusterState = "Ready"
	StorageNodeSetPaused       ClusterState = "Paused"

	ResourceSyncPending RemoteResourceState = "Pending"
	ResourceSyncSuccess RemoteResourceState = "Synced"

	TenantCreationRequeueDelay      = 30 * time.Second
	StorageAwaitRequeueDelay        = 30 * time.Second
	SharedDatabaseAwaitRequeueDelay = 30 * time.Second

	OwnerControllerField = ".metadata.controller"
	DatabaseRefField     = ".spec.databaseRef.name"
	StorageRefField      = ".spec.storageRef.name"
	SecretField          = ".spec.secrets"
)
