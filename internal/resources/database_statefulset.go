package resources

import (
	"errors"
	"fmt"
	"strconv"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/ydb-platform/ydb-kubernetes-operator/api/v1alpha1"
	"github.com/ydb-platform/ydb-kubernetes-operator/internal/configuration"
	"github.com/ydb-platform/ydb-kubernetes-operator/internal/ptr"
)

type DatabaseStatefulSetBuilder struct {
	*v1alpha1.Database

	Labels  map[string]string
	Storage *v1alpha1.Storage
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
	sts.ObjectMeta.Annotations = CopyDict(b.Spec.AdditionalAnnotations)

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

func (b *DatabaseStatefulSetBuilder) buildEnv() []corev1.EnvVar {
	var envVars []corev1.EnvVar

	envVars = append(envVars, corev1.EnvVar{
		Name: "NODE_NAME", // for `--grpc-public-host` flag
		ValueFrom: &corev1.EnvVarSource{
			FieldRef: &corev1.ObjectFieldSelector{
				APIVersion: "v1",
				FieldPath:  "metadata.name",
			},
		},
	})

	return envVars
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
			Labels:      b.Labels,
			Annotations: CopyDict(b.Spec.AdditionalAnnotations),
		},
		Spec: corev1.PodSpec{
			Containers:                []corev1.Container{b.buildContainer()},
			InitContainers:            b.Spec.InitContainers,
			NodeSelector:              b.Spec.NodeSelector,
			Affinity:                  b.Spec.Affinity,
			Tolerations:               b.Spec.Tolerations,
			TopologySpreadConstraints: b.Spec.TopologySpreadConstraints,

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
	configMapName := b.Name

	volumes := []corev1.Volume{
		{
			Name: configVolumeName,
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{Name: configMapName},
				},
			},
		},
	}

	if b.Spec.Service.GRPC.TLSConfiguration != nil && b.Spec.Service.GRPC.TLSConfiguration.Enabled {
		volumes = append(volumes, buildTLSVolume(grpcTLSVolumeName, b.Spec.Service.GRPC.TLSConfiguration))
	}

	if b.Spec.Service.Interconnect.TLSConfiguration != nil && b.Spec.Service.Interconnect.TLSConfiguration.Enabled {
		volumes = append(volumes, buildTLSVolume(interconnectTLSVolumeName, b.Spec.Service.Interconnect.TLSConfiguration))
	}

	if b.Spec.Encryption != nil && b.Spec.Encryption.Enabled {
		volumes = append(volumes, b.buildEncryptionVolume())
	}

	if b.Spec.Datastreams != nil && b.Spec.Datastreams.Enabled {
		volumes = append(volumes, b.buildDatastreamsIAMServiceAccountKeyVolume())
		if b.Spec.Service.Datastreams.TLSConfiguration != nil && b.Spec.Service.Datastreams.TLSConfiguration.Enabled {
			volumes = append(volumes, buildTLSVolume(datastreamsTLSVolumeName, b.Spec.Service.Datastreams.TLSConfiguration))
		}
	}

	for _, volume := range b.Spec.Volumes {
		volumes = append(volumes, *volume)
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

func (b *DatabaseStatefulSetBuilder) buildEncryptionVolume() corev1.Volume {
	var secretName, secretKey string
	if b.Spec.Encryption.Key != nil {
		secretName = b.Spec.Encryption.Key.Name
		secretKey = b.Spec.Encryption.Key.Key
	} else {
		secretName = b.Name
		secretKey = defaultEncryptionSecretKey
	}

	return corev1.Volume{
		Name: encryptionVolumeName,
		VolumeSource: corev1.VolumeSource{
			Secret: &corev1.SecretVolumeSource{
				SecretName: secretName,
				Items: []corev1.KeyToPath{
					{
						Key:  secretKey,
						Path: configuration.DatabaseEncryptionKeyFile,
					},
				},
			},
		},
	}
}

func (b *DatabaseStatefulSetBuilder) buildDatastreamsIAMServiceAccountKeyVolume() corev1.Volume {
	return corev1.Volume{
		Name: datastreamsIAMServiceAccountKeyVolumeName,
		VolumeSource: corev1.VolumeSource{
			Secret: &corev1.SecretVolumeSource{
				SecretName: b.Spec.Datastreams.IAMServiceAccountKey.Name,
				Items: []corev1.KeyToPath{
					{
						Key:  b.Spec.Datastreams.IAMServiceAccountKey.Key,
						Path: configuration.DatastreamsIAMServiceAccountKeyFile,
					},
				},
			},
		},
	}
}

func (b *DatabaseStatefulSetBuilder) buildContainer() corev1.Container {
	command, args := b.buildContainerArgs()
	container := corev1.Container{
		Name:            "ydb-dynamic",
		Image:           b.Spec.Image.Name,
		ImagePullPolicy: *b.Spec.Image.PullPolicyName,
		Command:         command,
		Args:            args,
		Env:             b.buildEnv(),
		LivenessProbe: &corev1.Probe{
			ProbeHandler: corev1.ProbeHandler{
				TCPSocket: &corev1.TCPSocketAction{
					Port: intstr.FromInt(v1alpha1.GRPCPort),
				},
			},
		},
		VolumeMounts: b.buildVolumeMounts(),
		SecurityContext: &corev1.SecurityContext{
			Privileged: ptr.Bool(false),
			Capabilities: &corev1.Capabilities{
				Add: []corev1.Capability{"SYS_RAWIO"},
			},
		},
	}

	ports := []corev1.ContainerPort{{
		Name: "grpc", ContainerPort: v1alpha1.GRPCPort,
	}, {
		Name: "interconnect", ContainerPort: v1alpha1.InterconnectPort,
	}, {
		Name: "status", ContainerPort: v1alpha1.StatusPort,
	}}

	if b.Spec.Datastreams != nil && b.Spec.Datastreams.Enabled {
		ports = append(ports, corev1.ContainerPort{
			Name: "datastreams", ContainerPort: v1alpha1.DatastreamsPort,
		})
	}

	container.Ports = ports

	if b.Spec.Resources != nil {
		container.Resources = b.Spec.Resources.ContainerResources
	} else if b.Spec.SharedResources != nil {
		container.Resources = b.Spec.SharedResources.ContainerResources
	}
	return container
}

func (b *DatabaseStatefulSetBuilder) buildVolumeMounts() []corev1.VolumeMount {
	var volumeMounts []corev1.VolumeMount

	volumeMounts = append(volumeMounts, corev1.VolumeMount{
		Name:      configVolumeName,
		ReadOnly:  true,
		MountPath: v1alpha1.ConfigDir,
	})

	if b.Spec.Service.GRPC.TLSConfiguration.Enabled {
		volumeMounts = append(volumeMounts, corev1.VolumeMount{
			Name:      grpcTLSVolumeName,
			ReadOnly:  true,
			MountPath: "/tls/grpc", // fixme const
		})
	}

	if b.Spec.Service.Interconnect.TLSConfiguration.Enabled {
		volumeMounts = append(volumeMounts, corev1.VolumeMount{
			Name:      interconnectTLSVolumeName,
			ReadOnly:  true,
			MountPath: "/tls/interconnect", // fixme const
		})
	}

	if b.Spec.Encryption != nil && b.Spec.Encryption.Enabled {
		volumeMounts = append(volumeMounts, corev1.VolumeMount{
			Name:      encryptionVolumeName,
			ReadOnly:  true,
			MountPath: configuration.DatabaseEncryptionKeyPath,
		})
	}

	if b.Spec.Datastreams != nil && b.Spec.Datastreams.Enabled {
		volumeMounts = append(volumeMounts, corev1.VolumeMount{
			Name:      datastreamsIAMServiceAccountKeyVolumeName,
			ReadOnly:  true,
			MountPath: configuration.DatastreamsIAMServiceAccountKeyPath,
		})
		if b.Spec.Service.Datastreams.TLSConfiguration.Enabled {
			volumeMounts = append(volumeMounts, corev1.VolumeMount{
				Name:      datastreamsTLSVolumeName,
				ReadOnly:  true,
				MountPath: "/tls/datastreams", // fixme const
			})
		}
	}

	for _, volume := range b.Spec.Volumes {
		volumeMounts = append(volumeMounts, corev1.VolumeMount{
			Name:      volume.Name,
			MountPath: fmt.Sprintf("%s/%s", wellKnownDirForAdditionalVolumes, volume.Name),
		})
	}

	return volumeMounts
}

func (b *DatabaseStatefulSetBuilder) buildContainerArgs() ([]string, []string) {
	command := []string{fmt.Sprintf("%s/%s", v1alpha1.BinariesDir, v1alpha1.DaemonBinaryName)}

	db := NewDatabase(b.DeepCopy())
	db.Storage = b.Storage

	tenantName := fmt.Sprintf(v1alpha1.TenantNameFormat, b.Spec.Domain, b.Name)

	args := []string{
		"server",

		"--mon-port",
		fmt.Sprintf("%d", v1alpha1.StatusPort),

		"--ic-port",
		fmt.Sprintf("%d", v1alpha1.InterconnectPort),

		"--yaml-config",
		fmt.Sprintf("%s/%s", v1alpha1.ConfigDir, v1alpha1.ConfigFileName),

		"--tenant",
		tenantName,

		"--node-broker",
		db.GetStorageEndpointWithProto(),
	}

	if b.Spec.Service.GRPC.ExternalHost == "" {
		service := fmt.Sprintf(interconnectServiceNameFormat, b.GetName())
		b.Spec.Service.GRPC.ExternalHost = fmt.Sprintf("%s.%s.svc.cluster.local", service, b.GetNamespace()) // FIXME .svc.cluster.local
	}

	publicHostOption := "--grpc-public-host"
	publicPortOption := "--grpc-public-port"

	args = append(
		args,

		publicHostOption,
		fmt.Sprintf("%s.%s", "$(NODE_NAME)", b.Spec.Service.GRPC.ExternalHost), // fixme $(NODE_NAME)

		publicPortOption,
		strconv.Itoa(v1alpha1.GRPCPort),
	)

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
