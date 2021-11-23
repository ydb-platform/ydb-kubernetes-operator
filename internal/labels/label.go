package labels

import (
	"github.com/ydb-platform/ydb-kubernetes-operator/api/v1alpha1"
)

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

	StorageComponent = "storage-node"
	DynamicComponent = "dynamic-node"
)

type Labels map[string]string

func Common(name string, defaultLabels Labels) Labels {
	l := Labels{}

	l.Merge(makeCommonLabels(defaultLabels, name))

	return l
}

func ClusterLabels(cluster *v1alpha1.Storage) Labels {
	l := Common(cluster.Name, cluster.Labels)

	l.Merge(cluster.Spec.AdditionalLabels)
	l.Merge(map[string]string{
		ComponentKey: StorageComponent,
	})

	return l
}

func DatabaseLabels(database *v1alpha1.Database) Labels {
	l := Common(database.Name, database.Labels)

	l.Merge(database.Spec.AdditionalLabels)
	l.Merge(map[string]string{
		ComponentKey: DynamicComponent,
	})

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

func (l Labels) Merge(other map[string]string) {
	if other == nil {
		return
	}

	for k, v := range other {
		l[k] = v
	}
}

func (l Labels) MergeInPlace(other map[string]string) map[string]string {
	result := l.Copy()

	for k, v := range other {
		result[k] = v
	}

	return result
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

	return common
}
