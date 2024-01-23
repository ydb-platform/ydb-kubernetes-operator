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

type RemoteStorageNodeSetBuilder struct {
	client.Object

	Name   string
	Labels map[string]string

	StorageNodeSetSpec api.StorageNodeSetSpec
}

type RemoteStorageNodeSetResource struct {
	*api.RemoteStorageNodeSet
}

func (b *RemoteStorageNodeSetBuilder) Build(obj client.Object) error {
	dns, ok := obj.(*api.RemoteStorageNodeSet)
	if !ok {
		return errors.New("failed to cast to RemoteStorageNodeSet object")
	}

	if dns.ObjectMeta.Name == "" {
		dns.ObjectMeta.Name = b.Name
	}
	dns.ObjectMeta.Namespace = b.GetNamespace()

	dns.ObjectMeta.Labels = b.Labels
	dns.Spec = b.StorageNodeSetSpec

	return nil
}

func (b *RemoteStorageNodeSetBuilder) Placeholder(cr client.Object) client.Object {
	return &api.RemoteStorageNodeSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      b.Name,
			Namespace: cr.GetNamespace(),
		},
	}
}

func (b *RemoteStorageNodeSetResource) GetResourceBuilders(restConfig *rest.Config) []ResourceBuilder {
	Storage := b.recastRemoteStorageNodeSet()

	var resourceBuilders []ResourceBuilder
	resourceBuilders = append(resourceBuilders,
		&StorageStatefulSetBuilder{
			Storage:    Storage.DeepCopy(),
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

func NewRemoteStorageNodeSet(remoteStorageNodeSet *api.RemoteStorageNodeSet) RemoteStorageNodeSetResource {
	crRemoteStorageNodeSet := remoteStorageNodeSet.DeepCopy()

	return RemoteStorageNodeSetResource{RemoteStorageNodeSet: crRemoteStorageNodeSet}
}

func (b *RemoteStorageNodeSetResource) SetStatusOnFirstReconcile() (bool, ctrl.Result, error) {
	if b.Status.Conditions == nil {
		b.Status.Conditions = []metav1.Condition{}

		if b.Spec.Pause {
			meta.SetStatusCondition(&b.Status.Conditions, metav1.Condition{
				Type:    StoragePausedCondition,
				Status:  "False",
				Reason:  ReasonInProgress,
				Message: "Transitioning RemoteStorageNodeSet to Paused state",
			})

			return Stop, ctrl.Result{RequeueAfter: StatusUpdateRequeueDelay}, nil
		}
	}

	return Continue, ctrl.Result{}, nil
}

func (b *RemoteStorageNodeSetResource) Unwrap() *api.RemoteStorageNodeSet {
	return b.DeepCopy()
}

func (b *RemoteStorageNodeSetResource) recastRemoteStorageNodeSet() *api.Storage {
	return &api.Storage{
		ObjectMeta: metav1.ObjectMeta{
			Name:      b.RemoteStorageNodeSet.Spec.StorageRef.Name,
			Namespace: b.RemoteStorageNodeSet.Spec.StorageRef.Namespace,
			Labels:    b.RemoteStorageNodeSet.Labels,
		},
		Spec: api.StorageSpec{
			StorageClusterSpec: b.Spec.StorageClusterSpec,
			StorageNodeSpec:    b.Spec.StorageNodeSpec,
		},
	}
}
