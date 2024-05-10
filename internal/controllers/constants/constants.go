package constants

import "time"

type (
	ClusterState        string
	RemoteResourceState string
)

const (
	StorageKind               = "Storage"
	StorageNodeSetKind        = "StorageNodeSet"
	RemoteStorageNodeSetKind  = "RemoteStorageNodeSet"
	DatabaseKind              = "Database"
	DatabaseNodeSetKind       = "DatabaseNodeSet"
	RemoteDatabaseNodeSetKind = "RemoteDatabaseNodeSet"

	// For backward compatibility
	OldStorageInitializedCondition  = "StorageReady"
	OldDatabaseInitializedCondition = "TenantInitialized"

	StoragePausedCondition       = "StoragePaused"
	StorageInitializedCondition  = "StorageInitialized"
	DatabasePausedCondition      = "DatabasePaused"
	DatabaseInitializedCondition = "DatabaseInitialized"

	NodeSetPreparedCondition    = "NodeSetPreparing"
	NodeSetProvisionedCondition = "NodeSetProvisioning"
	NodeSetReadyCondition       = "NodeSetReady"
	NodeSetPausedCondition      = "NodeSetPaused"

	RemoteResourceSyncedCondition = "ResourceSynced"

	Stop     = true
	Continue = false

	ReasonInProgress  = "InProgress"
	ReasonNotRequired = "NotRequired"
	ReasonCompleted   = "Completed"

	DefaultRequeueDelay                = 10 * time.Second
	StatusUpdateRequeueDelay           = 1 * time.Second
	SelfCheckRequeueDelay              = 30 * time.Second
	StorageInitializationRequeueDelay  = 30 * time.Second
	DatabaseInitializationRequeueDelay = 30 * time.Second

	DatabasePending      ClusterState = "Pending"
	DatabasePreparing    ClusterState = "Preparing"
	DatabaseProvisioning ClusterState = "Provisioning"
	DatabaseInitializing ClusterState = "Initializing"
	DatabaseReady        ClusterState = "Ready"
	DatabasePaused       ClusterState = "Paused"

	DatabaseNodeSetPending      ClusterState = "Pending"
	DatabaseNodeSetPreparing    ClusterState = "Preparing"
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
	StorageNodeSetPreparing    ClusterState = "Preparing"
	StorageNodeSetProvisioning ClusterState = "Provisioning"
	StorageNodeSetReady        ClusterState = "Ready"
	StorageNodeSetPaused       ClusterState = "Paused"

	ResourceSyncPending RemoteResourceState = "Pending"
	ResourceSyncSuccess RemoteResourceState = "Synced"

	StorageAwaitRequeueDelay        = 30 * time.Second
	SharedDatabaseAwaitRequeueDelay = 30 * time.Second

	OwnerControllerField = ".metadata.controller"
	DatabaseRefField     = ".spec.databaseRef.name"
	StorageRefField      = ".spec.storageRef.name"
	SecretField          = ".spec.secrets"
)
