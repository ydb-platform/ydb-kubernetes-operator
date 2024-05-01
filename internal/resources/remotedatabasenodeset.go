package resources

import (
	"errors"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"

	api "github.com/ydb-platform/ydb-kubernetes-operator/api/v1alpha1"
	ydbannotations "github.com/ydb-platform/ydb-kubernetes-operator/internal/annotations"
	. "github.com/ydb-platform/ydb-kubernetes-operator/internal/controllers/constants" //nolint:revive,stylecheck
)

type RemoteDatabaseNodeSetBuilder struct {
	client.Object

	Name        string
	Labels      map[string]string
	Annotations map[string]string

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
	dns.ObjectMeta.Annotations = b.Annotations

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

	nodeSetAnnotations := CopyDict(b.Annotations)
	delete(nodeSetAnnotations, ydbannotations.LastAppliedAnnotation)

	resourceBuilders = append(resourceBuilders,
		&DatabaseNodeSetBuilder{
			Object: b,

			Name:        b.Name,
			Labels:      b.Labels,
			Annotations: nodeSetAnnotations,

			DatabaseNodeSetSpec: b.Spec,
		},
	)

	return resourceBuilders
}

func NewRemoteDatabaseNodeSet(remoteDatabaseNodeSet *api.RemoteDatabaseNodeSet) RemoteDatabaseNodeSetResource {
	crRemoteDatabaseNodeSet := remoteDatabaseNodeSet.DeepCopy()

	return RemoteDatabaseNodeSetResource{RemoteDatabaseNodeSet: crRemoteDatabaseNodeSet}
}

func (b *RemoteDatabaseNodeSetResource) GetRemoteObjects(
	scheme *runtime.Scheme,
) []client.Object {
	remoteObjects := []client.Object{}

	// sync Secrets
	for _, secret := range b.Spec.Secrets {
		remoteObjects = append(remoteObjects,
			&corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      secret.Name,
					Namespace: b.Namespace,
				},
			})
	}

	// sync ConfigMap
	remoteObjects = append(remoteObjects,
		&corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      b.Spec.DatabaseRef.Name,
				Namespace: b.Namespace,
			},
		})

	// sync Services
	remoteObjects = append(remoteObjects,
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
		remoteObjects = append(remoteObjects,
			&corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      fmt.Sprintf(DatastreamsServiceNameFormat, b.Spec.DatabaseRef.Name),
					Namespace: b.Namespace,
				},
			})
	}

	for _, remoteObj := range remoteObjects {
		remoteObjGVK, _ := apiutil.GVKForObject(remoteObj, scheme)
		remoteObj.GetObjectKind().SetGroupVersionKind(remoteObjGVK)
	}

	return remoteObjects
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

func (b *RemoteDatabaseNodeSetResource) UnsetPrimaryResourceAnnotations(obj client.Object) {
	annotations := make(map[string]string)
	for key, value := range obj.GetAnnotations() {
		if key != annotations[ydbannotations.PrimaryResourceDatabaseAnnotation] {
			annotations[key] = value
		}
	}
	obj.SetAnnotations(annotations)
}

func (b *RemoteDatabaseNodeSetResource) CreateRemoteResourceStatus(
	remoteObj client.Object,
) {
	b.Status.RemoteResources = append(
		b.Status.RemoteResources,
		api.RemoteResource{
			Group:      remoteObj.GetObjectKind().GroupVersionKind().Group,
			Version:    remoteObj.GetObjectKind().GroupVersionKind().Version,
			Kind:       remoteObj.GetObjectKind().GroupVersionKind().Kind,
			Name:       remoteObj.GetName(),
			State:      ResourceSyncPending,
			Conditions: []metav1.Condition{},
		},
	)
	meta.SetStatusCondition(
		&b.Status.RemoteResources[len(b.Status.RemoteResources)-1].Conditions,
		metav1.Condition{
			Type:   RemoteResourceSyncedCondition,
			Status: "Unknown",
			Reason: ReasonInProgress,
		},
	)
}

func (b *RemoteDatabaseNodeSetResource) UpdateRemoteResourceStatus(
	remoteResource *api.RemoteResource,
	status metav1.ConditionStatus,
	resourceVersion string,
) {
	if status == metav1.ConditionFalse {
		meta.SetStatusCondition(&remoteResource.Conditions,
			metav1.Condition{
				Type:    RemoteResourceSyncedCondition,
				Status:  metav1.ConditionFalse,
				Reason:  ReasonInProgress,
				Message: fmt.Sprintf("Failed to sync remoteObject to resourceVersion %s", resourceVersion),
			})
		remoteResource.State = ResourceSyncPending
	}

	if status == metav1.ConditionTrue {
		meta.SetStatusCondition(&remoteResource.Conditions,
			metav1.Condition{
				Type:    RemoteResourceSyncedCondition,
				Status:  metav1.ConditionTrue,
				Reason:  ReasonCompleted,
				Message: fmt.Sprintf("Successfully synced remoteObject to resourceVersion %s", resourceVersion),
			})
		remoteResource.State = ResourceSyncSuccess
	}
}

func (b *RemoteDatabaseNodeSetResource) RemoveRemoteResourceStatus(remoteObj client.Object) {
	var idxRemoteObj int
	for idx := range b.Status.RemoteResources {
		if EqualRemoteResourceWithObject(&b.Status.RemoteResources[idx], remoteObj) {
			idxRemoteObj = idx
			break
		}
	}
	b.Status.RemoteResources = append(
		b.Status.RemoteResources[:idxRemoteObj],
		b.Status.RemoteResources[idxRemoteObj+1:]...,
	)
}
