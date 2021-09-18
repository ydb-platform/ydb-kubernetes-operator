package resources

import (
	"errors"
	"fmt"

	"github.com/ydb-platform/ydb-kubernetes-operator/api/v1alpha1"
	"github.com/ydb-platform/ydb-kubernetes-operator/pkg/ptr"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type DatabaseStatefulSetBuilder struct {
	*v1alpha1.Database

	Labels map[string]string
}

func (b *DatabaseStatefulSetBuilder) Build(obj client.Object) error {
	sts, ok := obj.(*appsv1.StatefulSet)
	if !ok {
		return errors.New("failed to cast to StatefulSet object")
	}

	if sts.ObjectMeta.Name == "" {
		sts.ObjectMeta.Name = b.Name
	}
	sts.ObjectMeta.Namespace = b.Namespace // fixme should we really slap namespace on any object?

	sts.Spec = appsv1.StatefulSetSpec{
		Replicas: ptr.Int32(b.Spec.Nodes),
		Selector: &metav1.LabelSelector{
			MatchLabels: b.Labels,
		},
		PodManagementPolicy:  appsv1.ParallelPodManagement,
		RevisionHistoryLimit: ptr.Int32(10),
		ServiceName:          fmt.Sprintf(interconnectServiceNameFormat, b.Name),
		Template:             b.buildPodTemplateSpec(),
	}

	return nil
}

func (b *DatabaseStatefulSetBuilder) buildPodTemplateSpec() corev1.PodTemplateSpec {
	dnsConfigSearches := []string{
		fmt.Sprintf(
			"%s-interconnect.%s.svc.cluster.local",
			b.Spec.StorageClusterRef.Name,
			b.Namespace,
		),
	}

	return corev1.PodTemplateSpec{
		ObjectMeta: metav1.ObjectMeta{
			Labels: b.Labels,
		},
		Spec: corev1.PodSpec{
			Containers:   []corev1.Container{b.buildContainer()},
			NodeSelector: b.Spec.NodeSelector,
			Affinity:     b.Spec.Affinity,
			Tolerations:  b.Spec.Tolerations,

			DNSConfig: &corev1.PodDNSConfig{
				Searches: dnsConfigSearches,
			},
		},
	}
}

func (b *DatabaseStatefulSetBuilder) buildContainer() corev1.Container {
	db := NewDatabase(b.DeepCopy())

	container := corev1.Container{
		Name:    "ydb-dynamic",
		Image:   b.Spec.Image.Name,
		Command: []string{"/opt/kikimr/bin/kikimr"},
		Args: []string{
			"server",

			"--mon-port",
			fmt.Sprintf("%d", v1alpha1.StatusPort),

			"--ic-port",
			fmt.Sprintf("%d", v1alpha1.InterconnectPort),

			"--tenant",
			fmt.Sprintf(TenantPathFormat, b.Name),

			"--node-broker",
			db.GetStorageEndpoint(),
		},

		LivenessProbe: &corev1.Probe{
			Handler: corev1.Handler{
				TCPSocket: &corev1.TCPSocketAction{
					Port: intstr.FromInt(v1alpha1.GRPCPort), // fixme
				},
			},
		},

		Ports: []corev1.ContainerPort{{
			Name: "grpc", ContainerPort: v1alpha1.GRPCPort,
		}, {
			Name: "interconnect", ContainerPort: v1alpha1.InterconnectPort,
		}, {
			Name: "status", ContainerPort: v1alpha1.StatusPort,
		}},

		Resources: b.Spec.Resources,
	}

	return container
}

func (b *DatabaseStatefulSetBuilder) Placeholder(cr client.Object) client.Object {
	return &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cr.GetName(),
			Namespace: cr.GetNamespace(),
		},
	}
}
