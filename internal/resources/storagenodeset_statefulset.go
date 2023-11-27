package resources

import (
	"errors"

	"github.com/ydb-platform/ydb-kubernetes-operator/api/v1alpha1"
	"github.com/ydb-platform/ydb-kubernetes-operator/internal/ptr"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type StorageNodeSetStatefulSetBuilder struct {
	*v1alpha1.StorageNodeSet

	StorageStatefulSetBuilder *StorageStatefulSetBuilder
}

func (b *StorageNodeSetStatefulSetBuilder) Build(obj client.Object) error {
	b.StorageStatefulSetBuilder.Build(obj)

	sts, ok := obj.(*appsv1.StatefulSet)
	if !ok {
		return errors.New("failed to cast to StatefulSet object")
	}

	// Meta information
	sts.ObjectMeta.Name = b.StorageNodeSet.Spec.StorageRef + "-" + b.StorageNodeSet.Name
	sts.ObjectMeta.Labels = b.StorageNodeSet.Labels

	// Number of nodes (pods) in the set
	sts.Spec.Replicas = ptr.Int32(b.StorageNodeSet.Spec.Nodes)

	// if b.StorageNodeSet.Spec.Image != nil {
	// 	for _, container := range sts.Spec.Template.Spec.Containers {
	// 		container.Image = b.StorageNodeSet.Spec.Image.Name
	// 		if b.StorageNodeSet.Spec.Image.PullPolicyName != nil {
	// 			container.ImagePullPolicy = *b.StorageNodeSet.Spec.Image.PullPolicyName
	// 		}
	// 	}
	// 	if b.StorageNodeSet.Spec.Image.PullSecret != nil {
	// 		sts.Spec.Template.Spec.ImagePullSecrets = []corev1.LocalObjectReference{{Name: *b.Spec.Image.PullSecret}}
	// 	}
	// }

	if b.StorageNodeSet.Spec.DataStore != nil {
		pvcList := make([]corev1.PersistentVolumeClaim, 0, len(b.Spec.DataStore))
		for i, pvcSpec := range b.StorageNodeSet.Spec.DataStore {
			pvcList = append(
				pvcList,
				corev1.PersistentVolumeClaim{
					ObjectMeta: metav1.ObjectMeta{
						Name: b.StorageStatefulSetBuilder.GeneratePVCName(i),
					},
					Spec: pvcSpec,
				},
			)
		}
		sts.Spec.VolumeClaimTemplates = pvcList
	}

	if b.StorageNodeSet.Spec.Resources != nil {
		for _, container := range sts.Spec.Template.Spec.Containers {
			container.Resources = *b.StorageNodeSet.Spec.Resources
		}
	}

	if b.StorageNodeSet.Spec.NodeSelector != nil {
		sts.Spec.Template.Spec.NodeSelector = b.StorageNodeSet.Spec.NodeSelector
	}

	if b.StorageNodeSet.Spec.Affinity != nil {
		sts.Spec.Template.Spec.Affinity = b.StorageNodeSet.Spec.Affinity
	}

	if b.StorageNodeSet.Spec.Tolerations != nil {
		sts.Spec.Template.Spec.Tolerations = b.StorageNodeSet.Spec.Tolerations
	}

	if b.StorageNodeSet.Spec.TopologySpreadConstraints != nil {
		sts.Spec.Template.Spec.TopologySpreadConstraints = b.StorageNodeSet.Spec.TopologySpreadConstraints
	}

	return nil
}

func (b *StorageNodeSetStatefulSetBuilder) Placeholder(cr client.Object) client.Object {
	return &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cr.GetName(),
			Namespace: cr.GetNamespace(),
		},
	}
}
