package resources

import (
	"errors"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	api "github.com/ydb-platform/ydb-kubernetes-operator/api/v1alpha1"
	. "github.com/ydb-platform/ydb-kubernetes-operator/internal/controllers/constants" //nolint:revive,stylecheck
)

type StorageNodeSetBuilder struct {
	client.Object

	Name               string
	Labels             map[string]string
	StorageNodeSetSpec api.StorageNodeSetSpec
}

type StorageNodeSetResource struct {
	*api.StorageNodeSet
}

func (b *StorageNodeSetBuilder) Build(obj client.Object) error {
	sns, ok := obj.(*api.StorageNodeSet)
	if !ok {
		return errors.New("failed to cast to StorageNodeSet object")
	}

	if sns.ObjectMeta.Name == "" {
		sns.ObjectMeta.Name = b.Name
	}
	sns.ObjectMeta.Namespace = b.GetNamespace()

	sns.ObjectMeta.Labels = b.Labels
	sns.Spec = b.StorageNodeSetSpec

	return nil
}

func (b *StorageNodeSetBuilder) Placeholder(cr client.Object) client.Object {
	return &api.StorageNodeSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      b.Name,
			Namespace: cr.GetNamespace(),
		},
	}
}

func (b *StorageNodeSetResource) GetResourceBuilders(restConfig *rest.Config) []ResourceBuilder {
	storage := b.recastStorageNodeSet()

	var resourceBuilders []ResourceBuilder
	resourceBuilders = append(
		resourceBuilders,
		&StorageStatefulSetBuilder{
			Storage:    storage.DeepCopy(),
			RestConfig: restConfig,

			Name:   b.Name,
			Labels: b.Labels,
		},
		&ConfigMapBuilder{
			Object: b,

			Name: b.Name,
			Data: map[string]string{
				api.ConfigFileName: b.Spec.Configuration,
			},
			Labels: b.Labels,
		},
	)

	return resourceBuilders
}

func NewStorageNodeSet(storageNodeSet *api.StorageNodeSet) StorageNodeSetResource {
	crStorageNodeSet := storageNodeSet.DeepCopy()

	return StorageNodeSetResource{
		StorageNodeSet: crStorageNodeSet,
	}
}

func (b *StorageNodeSetResource) SetStatusOnFirstReconcile() (bool, ctrl.Result, error) {
	if b.Status.Conditions == nil {
		b.Status.Conditions = []metav1.Condition{}

		if b.Spec.Pause {
			meta.SetStatusCondition(&b.Status.Conditions, metav1.Condition{
				Type:    string(StorageNodeSetPaused),
				Status:  "True",
				Reason:  ReasonCompleted,
				Message: "State DatabaseNodeSet set to Paused",
			})

			return Stop, ctrl.Result{RequeueAfter: StatusUpdateRequeueDelay}, nil
		}
	}

	return Continue, ctrl.Result{}, nil
}

func (b *StorageNodeSetResource) Unwrap() *api.StorageNodeSet {
	return b.DeepCopy()
}

func (b *StorageNodeSetResource) recastStorageNodeSet() api.Storage {
	return api.Storage{
		ObjectMeta: metav1.ObjectMeta{
			Name:      b.StorageNodeSet.Spec.StorageRef.Name,
			Namespace: b.StorageNodeSet.Spec.StorageRef.Namespace,
			Labels:    b.StorageNodeSet.Labels,
		},
		Spec: api.StorageSpec{
			Nodes:                     b.StorageNodeSet.Spec.Nodes,
			Configuration:             b.StorageNodeSet.Spec.Configuration, // TODO: migrate to configmapRef
			Erasure:                   b.StorageNodeSet.Spec.Erasure,       // TODO: get from configuration
			DataStore:                 b.StorageNodeSet.Spec.DataStore,
			Service:                   b.StorageNodeSet.Spec.Service,
			Resources:                 b.StorageNodeSet.Spec.Resources,
			Image:                     b.StorageNodeSet.Spec.Image,
			InitContainers:            b.StorageNodeSet.Spec.InitContainers,
			CABundle:                  b.StorageNodeSet.Spec.CABundle, // TODO: migrate to trust-manager
			Secrets:                   b.StorageNodeSet.Spec.Secrets,
			Volumes:                   b.StorageNodeSet.Spec.Volumes,
			HostNetwork:               b.StorageNodeSet.Spec.HostNetwork,
			NodeSelector:              b.StorageNodeSet.Spec.NodeSelector,
			Affinity:                  b.StorageNodeSet.Spec.Affinity,
			Tolerations:               b.StorageNodeSet.Spec.Tolerations,
			TopologySpreadConstraints: b.StorageNodeSet.Spec.TopologySpreadConstraints,
			AdditionalLabels:          b.StorageNodeSet.Spec.AdditionalLabels,
			AdditionalAnnotations:     b.StorageNodeSet.Spec.AdditionalAnnotations,
			PriorityClassName:         b.StorageNodeSet.Spec.PriorityClassName,
		},
	}
}
