package resources

import (
	"errors"

	"github.com/ydb-platform/ydb-kubernetes-operator/api/v1alpha1"
	"github.com/ydb-platform/ydb-kubernetes-operator/internal/ptr"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type DatabaseNodeSetStatefulSetBuilder struct {
	*v1alpha1.DatabaseNodeSet

	DatabaseStatefulSetBuilder *DatabaseStatefulSetBuilder
}

func (b *DatabaseNodeSetStatefulSetBuilder) Build(obj client.Object) error {
	b.DatabaseStatefulSetBuilder.Build(obj)

	sts, ok := obj.(*appsv1.StatefulSet)
	if !ok {
		return errors.New("failed to cast to StatefulSet object")
	}

	// Meta information
	sts.ObjectMeta.Name = b.DatabaseNodeSet.Spec.DatabaseRef + "-" + b.DatabaseNodeSet.Name
	sts.ObjectMeta.Labels = b.DatabaseNodeSet.Labels

	// Number of nodes (pods) in the set
	sts.Spec.Replicas = ptr.Int32(b.DatabaseNodeSet.Spec.Nodes)

	// if b.DatabaseNodeSet.Spec.Image != nil {
	// 	for _, container := range sts.Spec.Template.Spec.Containers {
	// 		container.Image = b.DatabaseNodeSet.Spec.Image.Name
	// 		if b.DatabaseNodeSet.Spec.Image.PullPolicyName != nil {
	// 			container.ImagePullPolicy = *b.DatabaseNodeSet.Spec.Image.PullPolicyName
	// 		}
	// 	}
	// 	if b.DatabaseNodeSet.Spec.Image.PullSecret != nil {
	// 		sts.Spec.Template.Spec.ImagePullSecrets = []corev1.LocalObjectReference{{Name: *b.Spec.Image.PullSecret}}
	// 	}
	// }

	if b.DatabaseNodeSet.Spec.Resources != nil {
		for _, container := range sts.Spec.Template.Spec.Containers {
			container.Resources = *b.DatabaseNodeSet.Spec.Resources
		}
	}

	if b.DatabaseNodeSet.Spec.NodeSelector != nil {
		sts.Spec.Template.Spec.NodeSelector = b.DatabaseNodeSet.Spec.NodeSelector
	}

	if b.DatabaseNodeSet.Spec.Affinity != nil {
		sts.Spec.Template.Spec.Affinity = b.DatabaseNodeSet.Spec.Affinity
	}

	if b.DatabaseNodeSet.Spec.Tolerations != nil {
		sts.Spec.Template.Spec.Tolerations = b.DatabaseNodeSet.Spec.Tolerations
	}

	if b.DatabaseNodeSet.Spec.TopologySpreadConstraints != nil {
		sts.Spec.Template.Spec.TopologySpreadConstraints = b.DatabaseNodeSet.Spec.TopologySpreadConstraints
	}

	return nil
}

func (b *DatabaseNodeSetStatefulSetBuilder) Placeholder(cr client.Object) client.Object {
	return &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cr.GetName(),
			Namespace: cr.GetNamespace(),
		},
	}
}
