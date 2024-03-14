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

type RemoteDatabaseNodeSetBuilder struct {
	client.Object

	Name   string
	Labels map[string]string

	DatabaseNodeSetSpec api.DatabaseNodeSetSpec
}

type RemoteDatabaseNodeSetResource struct {
	*api.RemoteDatabaseNodeSet
}

func (b *RemoteDatabaseNodeSetBuilder) Build(obj client.Object) error {
	dns, ok := obj.(*api.RemoteDatabaseNodeSet)
	if !ok {
		return errors.New("failed to cast to RemoteDatabaseNodeSet object")
	}

	if dns.ObjectMeta.Name == "" {
		dns.ObjectMeta.Name = b.Name
	}
	dns.ObjectMeta.Namespace = b.GetNamespace()

	dns.ObjectMeta.Labels = b.Labels
	dns.Spec = b.DatabaseNodeSetSpec

	return nil
}

func (b *RemoteDatabaseNodeSetBuilder) Placeholder(cr client.Object) client.Object {
	return &api.RemoteDatabaseNodeSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      b.Name,
			Namespace: cr.GetNamespace(),
		},
	}
}

func (b *RemoteDatabaseNodeSetResource) GetResourceBuilders() []ResourceBuilder {
	var resourceBuilders []ResourceBuilder

	resourceBuilders = append(resourceBuilders,
		&DatabaseNodeSetBuilder{
			Object: b,

			Name:   b.Name,
			Labels: b.Labels,

			DatabaseNodeSetSpec: b.Spec,
		},
	)

	return resourceBuilders
}

func NewRemoteDatabaseNodeSet(remoteDatabaseNodeSet *api.RemoteDatabaseNodeSet) RemoteDatabaseNodeSetResource {
	crRemoteDatabaseNodeSet := remoteDatabaseNodeSet.DeepCopy()

	return RemoteDatabaseNodeSetResource{RemoteDatabaseNodeSet: crRemoteDatabaseNodeSet}
}

func (b *RemoteDatabaseNodeSetResource) GetRemoteObjects() []client.Object {
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
				Name:      b.Spec.DatabaseRef.Name,
				Namespace: b.Namespace,
			},
		})

	// sync Services
	objects = append(objects,
		&corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      fmt.Sprintf(GRPCServiceNameFormat, b.Spec.DatabaseRef.Name),
				Namespace: b.Namespace,
			},
		},
		&corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      fmt.Sprintf(InterconnectServiceNameFormat, b.Spec.DatabaseRef.Name),
				Namespace: b.Namespace,
			},
		},
		&corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      fmt.Sprintf(StatusServiceNameFormat, b.Spec.DatabaseRef.Name),
				Namespace: b.Namespace,
			},
		},
	)
	if b.Spec.Datastreams != nil && b.Spec.Datastreams.Enabled {
		objects = append(objects,
			&corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      fmt.Sprintf(DatastreamsServiceNameFormat, b.Spec.DatabaseRef.Name),
					Namespace: b.Namespace,
				},
			})
	}

	return objects
}

func (b *RemoteDatabaseNodeSetResource) SetPrimaryResourceAnnotations(obj client.Object) {
	annotations := make(map[string]string)
	for key, value := range obj.GetAnnotations() {
		annotations[key] = value
	}

	if _, exist := annotations[ydbannotations.PrimaryResourceDatabaseAnnotation]; !exist {
		annotations[ydbannotations.PrimaryResourceDatabaseAnnotation] = b.Spec.DatabaseRef.Name
	}

	obj.SetAnnotations(annotations)
}

func (b *RemoteDatabaseNodeSetResource) SetRemoteResourceStatus(remoteObj client.Object, remoteObjGVK schema.GroupVersionKind) {
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

func (b *RemoteDatabaseNodeSetResource) RemoveRemoteResourceStatus(remoteObj client.Object, remoteObjGVK schema.GroupVersionKind) {
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
