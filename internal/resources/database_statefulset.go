package resources

import (
	"errors"
	"fmt"
	"strconv"

	"github.com/ydb-platform/ydb-kubernetes-operator/api/v1alpha1"
	"github.com/ydb-platform/ydb-kubernetes-operator/internal/ptr"
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
	sts.ObjectMeta.Namespace = b.Namespace

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

	podTemplate := corev1.PodTemplateSpec{
		ObjectMeta: metav1.ObjectMeta{
			Labels: b.Labels,
		},
		Spec: corev1.PodSpec{
			Containers:   []corev1.Container{b.buildContainer()},
			NodeSelector: b.Spec.NodeSelector,
			Affinity:     b.Spec.Affinity,
			Tolerations:  b.Spec.Tolerations,

			Volumes: b.buildVolumes(),

			DNSConfig: &corev1.PodDNSConfig{
				Searches: dnsConfigSearches,
			},
		},
	}
	if b.Spec.Image.PullSecret != nil {
		podTemplate.Spec.ImagePullSecrets = []corev1.LocalObjectReference{{Name: *b.Spec.Image.PullSecret}}
	}
	return podTemplate
}

func (b *DatabaseStatefulSetBuilder) buildVolumes() []corev1.Volume {
	var volumes []corev1.Volume

	if b.Spec.ClusterConfig != "" {
		volumes = append(volumes, corev1.Volume{
			Name: configVolumeName,
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{Name: b.Spec.ClusterConfig},
				},
			},
		})
	}

	if b.Spec.Service.GRPC.TLSConfiguration != nil && b.Spec.Service.GRPC.TLSConfiguration.Enabled {
		volumes = append(volumes, buildTLSVolume("grpc-tls-volume", b.Spec.Service.GRPC.TLSConfiguration))
	}

	if b.Spec.Service.Interconnect.TLSConfiguration != nil && b.Spec.Service.Interconnect.TLSConfiguration.Enabled {
		volumes = append(volumes, buildTLSVolume("interconnect-tls-volume", b.Spec.Service.Interconnect.TLSConfiguration)) // fixme const
	}

	return volumes
}

func buildTLSVolume(name string, configuration *v1alpha1.TLSConfiguration) corev1.Volume { // fixme move somewhere?
	volume := corev1.Volume{
		Name: name,
		VolumeSource: corev1.VolumeSource{
			Secret: &corev1.SecretVolumeSource{
				SecretName: configuration.Key.Name, // todo validate that Name is equal for all params
				Items: []corev1.KeyToPath{
					{
						Key:  configuration.CertificateAuthority.Key,
						Path: "ca.crt",
					},
					{
						Key:  configuration.Certificate.Key,
						Path: "tls.crt",
					},
					{
						Key:  configuration.Key.Key,
						Path: "tls.key",
					},
				},
			},
		},
	}

	return volume
}

func (b *DatabaseStatefulSetBuilder) buildContainer() corev1.Container {
	command, args := b.buildContainerArgs()
	container := corev1.Container{
		Name:    "ydb-dynamic",
		Image:   b.Spec.Image.Name,
		Command: command,
		Args:    args,
		LivenessProbe: &corev1.Probe{
			Handler: corev1.Handler{
				TCPSocket: &corev1.TCPSocketAction{
					Port: intstr.FromInt(v1alpha1.GRPCPort),
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

		VolumeMounts: b.buildVolumeMounts(),
	}
	if b.Spec.Resources != nil {
		container.Resources = (*b.Spec.Resources).ContainerResources
	} else if b.Spec.SharedResources != nil {
		container.Resources = (*b.Spec.SharedResources).ContainerResources
	}
	return container
}

func (b *DatabaseStatefulSetBuilder) buildVolumeMounts() []corev1.VolumeMount {
	var volumeMounts []corev1.VolumeMount

	if b.Spec.ClusterConfig != "" {
		volumeMounts = append(volumeMounts, corev1.VolumeMount{
			Name:      configVolumeName,
			ReadOnly:  true,
			MountPath: "/opt/kikimr/cfg",
		})
	}

	if b.Spec.Service.GRPC.TLSConfiguration.Enabled {
		volumeMounts = append(volumeMounts, corev1.VolumeMount{
			Name:      "grpc-tls-volume",
			ReadOnly:  true,
			MountPath: "/tls/grpc",
		})
	}

	if b.Spec.Service.Interconnect.TLSConfiguration.Enabled {
		volumeMounts = append(volumeMounts, corev1.VolumeMount{
			Name:      "interconnect-tls-volume",
			ReadOnly:  true,
			MountPath: "/tls/interconnect",
		})
	}

	return volumeMounts
}

func (b *DatabaseStatefulSetBuilder) buildContainerArgs() ([]string, []string) {
	command := []string{"/opt/kikimr/bin/kikimr"}

	db := NewDatabase(b.DeepCopy())

	tenant_name := fmt.Sprintf(v1alpha1.TenantNameFormat, b.Spec.Domain, b.Name)

	args := []string{
		"server",

		"--mon-port",
		fmt.Sprintf("%d", v1alpha1.StatusPort),

		"--ic-port",
		fmt.Sprintf("%d", v1alpha1.InterconnectPort),

		"--tenant",
		tenant_name,

		"--node-broker",
		db.GetStorageEndpointWithProto(),
	}

	if b.Spec.Service.GRPC.ExternalHost != "" {
		publicHostOption := "--grpc-public-host"
		publicPortOption := "--grpc-public-port"

		args = append(
			args,

			publicHostOption,
			fmt.Sprintf("%s.%s", "$(HOSTNAME)", b.Spec.Service.GRPC.ExternalHost), // fixme $(HOSTNAME)

			publicPortOption,
			strconv.Itoa(v1alpha1.GRPCPort),
		)
	}

	if b.Spec.ClusterConfig != "" {
		command = []string{"/bin/bash"}
		args = []string{
			"-c",
			fmt.Sprintf("export kikimr_tenant='%s' && source /opt/kikimr/cfg/dynamic_server.cfg && exec /opt/kikimr/bin/kikimr ${kikimr_arg}", tenant_name),
		}
	}
	return command, args
}

func (b *DatabaseStatefulSetBuilder) Placeholder(cr client.Object) client.Object {
	return &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cr.GetName(),
			Namespace: cr.GetNamespace(),
		},
	}
}
