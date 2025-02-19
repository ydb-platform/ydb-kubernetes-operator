package labels

// https://kubernetes.io/docs/concepts/overview/working-with-objects/common-labels/
const (
	// NameKey The name of a higher level application this one is part of
	NameKey = "app.kubernetes.io/name"
	// InstanceKey A unique name identifying the instance of an application
	InstanceKey = "app.kubernetes.io/instance"
	// ComponentKey The component within the architecture
	ComponentKey = "app.kubernetes.io/component"
	// PartOfKey The name of a higher level application this one is part of
	PartOfKey = "app.kubernetes.io/part-of"
	// ManagedByKey The tool being used to manage the operation of an application
	ManagedByKey = "app.kubernetes.io/managed-by"

	// ServiceComponent The specialization of a Service resource
	ServiceComponent = "ydb.tech/service-for"
	// StatefulsetComponent The specialization of a Statefulset resource
	StatefulsetComponent = "ydb.tech/statefulset-name"
	// StorageNodeSetComponent The specialization of a StorageNodeSet resource
	StorageNodeSetComponent = "ydb.tech/storage-nodeset"
	// DatabaseNodeSetComponent The specialization of a DatabaseNodeSet resource
	DatabaseNodeSetComponent = "ydb.tech/database-nodeset"
	// RemoteClusterKey The specialization of a remote k8s cluster
	RemoteClusterKey = "ydb.tech/remote-cluster"

	StorageComponent         = "storage-node"
	DynamicComponent         = "dynamic-node"
	BlobstorageInitComponent = "blobstorage-init"

	GRPCComponent         = "grpc"
	InterconnectComponent = "interconnect"
	StatusComponent       = "status"
	DatastreamsComponent  = "datastreams"
)

type Labels map[string]string

func Common(name string, defaultLabels Labels) Labels {
	l := Labels{}

	l.Merge(makeCommonLabels(defaultLabels, name))

	return l
}

func (l Labels) AsMap() map[string]string {
	return l
}

func (l Labels) Copy() Labels {
	res := Labels{}

	for k, v := range l {
		res[k] = v
	}

	return res
}

func (l Labels) Merge(other map[string]string) map[string]string {
	if other == nil {
		return l
	}

	for k, v := range other {
		l[k] = v
	}

	return l
}

func makeCommonLabels(other map[string]string, instance string) map[string]string {
	common := make(map[string]string)

	// keep part-of customized if it was set by high-level app
	var found bool
	if common[PartOfKey], found = other[PartOfKey]; !found {
		common[PartOfKey] = "yandex-database"
	}

	common[NameKey] = "ydb"
	common[InstanceKey] = instance

	common[ManagedByKey] = "ydb-operator"

	if storageNodeSetName, exist := other[StorageNodeSetComponent]; exist {
		common[StorageNodeSetComponent] = storageNodeSetName
	}

	if databaseNodeSetName, exist := other[DatabaseNodeSetComponent]; exist {
		common[DatabaseNodeSetComponent] = databaseNodeSetName
	}

	if remoteCluster, exist := other[RemoteClusterKey]; exist {
		common[RemoteClusterKey] = remoteCluster
	}

	return common
}
