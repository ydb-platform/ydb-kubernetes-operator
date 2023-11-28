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
	if sts.ObjectMeta.Name == "" {
		sts.ObjectMeta.Name = b.Spec.DatabaseRef + "-" + b.GetName()
	}
	sts.ObjectMeta.Labels = b.Labels

	// Number of nodes (pods) in the set
	sts.Spec.Replicas = ptr.Int32(b.Spec.Nodes)

	// if b.Spec.Image != nil {
	// 	for _, container := range sts.Spec.Template.Spec.Containers {
	// 		container.Image = b.Spec.Image.Name
	// 		if b.Spec.Image.PullPolicyName != nil {
	// 			container.ImagePullPolicy = *b.Spec.Image.PullPolicyName
	// 		}
	// 	}
	// 	if b.Spec.Image.PullSecret != nil {
	// 		sts.Spec.Template.Spec.ImagePullSecrets = []corev1.LocalObjectReference{{Name: *b.Spec.Image.PullSecret}}
	// 	}
	// }

	if b.Spec.Resources != nil {
		for _, container := range sts.Spec.Template.Spec.Containers {
			container.Resources = *b.Spec.Resources
		}
	}

	if b.Spec.NodeSelector != nil {
		sts.Spec.Template.Spec.NodeSelector = b.Spec.NodeSelector
	}

	if b.Spec.Affinity != nil {
		sts.Spec.Template.Spec.Affinity = b.Spec.Affinity
	}

	if b.Spec.Tolerations != nil {
		sts.Spec.Template.Spec.Tolerations = b.Spec.Tolerations
	}

	if b.Spec.TopologySpreadConstraints != nil {
		sts.Spec.Template.Spec.TopologySpreadConstraints = b.Spec.TopologySpreadConstraints
	}

	return nil
}

func (b *DatabaseNodeSetStatefulSetBuilder) Placeholder(cr client.Object) client.Object {
	return &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      b.Spec.DatabaseRef + "-" + b.GetName(),
			Namespace: cr.GetNamespace(),
		},
	}
}
