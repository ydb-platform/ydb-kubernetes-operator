package resources

import (
	"errors"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"

	api "github.com/ydb-platform/ydb-kubernetes-operator/api/v1alpha1"
	ydbannotations "github.com/ydb-platform/ydb-kubernetes-operator/internal/annotations"
	. "github.com/ydb-platform/ydb-kubernetes-operator/internal/controllers/constants" //nolint:revive,stylecheck
)

type RemoteStorageNodeSetBuilder struct {
	client.Object

	Name        string
	Labels      map[string]string
	Annotations map[string]string

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
	dns.ObjectMeta.Annotations = b.Annotations

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

func (b *RemoteStorageNodeSetResource) GetResourceBuilders() []ResourceBuilder {
	var resourceBuilders []ResourceBuilder

	resourceBuilders = append(resourceBuilders,
		&StorageNodeSetBuilder{
			Object: b,

			Name:   b.Name,
			Labels: b.Labels,

			StorageNodeSetSpec: b.Spec,
		},
	)
	return resourceBuilders
}

func NewRemoteStorageNodeSet(remoteStorageNodeSet *api.RemoteStorageNodeSet) RemoteStorageNodeSetResource {
	crRemoteStorageNodeSet := remoteStorageNodeSet.DeepCopy()

	return RemoteStorageNodeSetResource{RemoteStorageNodeSet: crRemoteStorageNodeSet}
}

func (b *RemoteStorageNodeSetResource) GetRemoteObjects() []client.Object {
	objects := []client.Object{}

	// sync Secrets
	for _, secret := range b.Spec.Secrets {
		objects = append(objects,
			&corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      secret.Name,
					Namespace: b.Namespace,
				},
			})
	}

	// sync ConfigMap
	objects = append(objects,
		&corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      b.Spec.StorageRef.Name,
				Namespace: b.Namespace,
			},
		})

	// sync Services
	objects = append(objects,
		&corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      fmt.Sprintf(GRPCServiceNameFormat, b.Spec.StorageRef.Name),
				Namespace: b.Namespace,
			},
		},
		&corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      fmt.Sprintf(InterconnectServiceNameFormat, b.Spec.StorageRef.Name),
				Namespace: b.Namespace,
			},
		},
		&corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      fmt.Sprintf(StatusServiceNameFormat, b.Spec.StorageRef.Name),
				Namespace: b.Namespace,
			},
		},
	)

	return objects
}

func (b *RemoteStorageNodeSetResource) SetPrimaryResourceAnnotations(obj client.Object) {
	annotations := make(map[string]string)
	for key, value := range obj.GetAnnotations() {
		annotations[key] = value
	}

	if _, exist := annotations[ydbannotations.PrimaryResourceStorageAnnotation]; !exist {
		annotations[ydbannotations.PrimaryResourceStorageAnnotation] = b.Spec.StorageRef.Name
	}

	obj.SetAnnotations(annotations)
}

func (b *RemoteStorageNodeSetResource) SetRemoteResourceStatus(remoteObj client.Object, remoteObjGVK schema.GroupVersionKind) {
	for idx := range b.Status.RemoteResources {
		if EqualRemoteResourceWithObject(&b.Status.RemoteResources[idx], b.Namespace, remoteObj, remoteObjGVK) {
			meta.SetStatusCondition(&b.Status.RemoteResources[idx].Conditions,
				metav1.Condition{
					Type:    RemoteResourceSyncedCondition,
					Status:  "True",
					Reason:  ReasonCompleted,
					Message: fmt.Sprintf("Resource updated with resourceVersion %s", remoteObj.GetResourceVersion()),
				})
			b.Status.RemoteResources[idx].State = ResourceSyncSuccess
		}
	}
}

func (b *RemoteStorageNodeSetResource) RemoveRemoteResourceStatus(remoteObj client.Object, remoteObjGVK schema.GroupVersionKind) {
	syncedResources := append([]api.RemoteResource{}, b.Status.RemoteResources...)
	for idx := range syncedResources {
		if EqualRemoteResourceWithObject(&syncedResources[idx], b.Namespace, remoteObj, remoteObjGVK) {
			b.Status.RemoteResources = append(
				b.Status.RemoteResources[:idx],
				b.Status.RemoteResources[idx+1:]...,
			)
			break
		}
	}
}
