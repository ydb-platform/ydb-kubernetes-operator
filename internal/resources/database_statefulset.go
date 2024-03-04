package resources

import (
	"errors"
	"fmt"
	"log"
	"regexp"
	"strconv"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/ydb-platform/ydb-kubernetes-operator/api/v1alpha1"
	"github.com/ydb-platform/ydb-kubernetes-operator/internal/ptr"
)

type DatabaseStatefulSetBuilder struct {
	*v1alpha1.Database
	RestConfig *rest.Config

	Name   string
	Labels map[string]string
}

var annotationDataCenterPattern = regexp.MustCompile("^[a-zA-Z]([a-zA-Z0-9_-]*[a-zA-Z0-9])?$")

func (b *DatabaseStatefulSetBuilder) Build(obj client.Object) error {
	sts, ok := obj.(*appsv1.StatefulSet)
	if !ok {
		return errors.New("failed to cast to StatefulSet object")
	}

	if sts.ObjectMeta.Name == "" {
		sts.ObjectMeta.Name = b.Name
	}
	sts.ObjectMeta.Namespace = b.GetNamespace()
	sts.ObjectMeta.Annotations = CopyDict(b.Spec.AdditionalAnnotations)

	replicas := ptr.Int32(b.Spec.Nodes)
	if b.Spec.Pause {
		replicas = ptr.Int32(0)
	}

	sts.Spec = appsv1.StatefulSetSpec{
		Replicas: replicas,
		Selector: &metav1.LabelSelector{
			MatchLabels: b.Labels,
		},
		PodManagementPolicy:  appsv1.ParallelPodManagement,
		RevisionHistoryLimit: ptr.Int32(10),
		ServiceName:          fmt.Sprintf(interconnectServiceNameFormat, b.Database.Name),
		Template:             b.buildPodTemplateSpec(),
	}

	if value, ok := b.ObjectMeta.Annotations[v1alpha1.AnnotationUpdateStrategyOnDelete]; ok && value == v1alpha1.AnnotationValueTrue {
		sts.Spec.UpdateStrategy = appsv1.StatefulSetUpdateStrategy{
			Type: "OnDelete",
		}
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
	podTemplate := corev1.PodTemplateSpec{
		ObjectMeta: metav1.ObjectMeta{
			Labels:      b.Labels,
			Annotations: CopyDict(b.Spec.AdditionalAnnotations),
		},
		Spec: corev1.PodSpec{
			Containers:                    []corev1.Container{b.buildContainer()},
			NodeSelector:                  b.Spec.NodeSelector,
			Affinity:                      b.Spec.Affinity,
			Tolerations:                   b.Spec.Tolerations,
			PriorityClassName:             b.Spec.PriorityClassName,
			TopologySpreadConstraints:     b.Spec.TopologySpreadConstraints,
			TerminationGracePeriodSeconds: b.Spec.TerminationGracePeriodSeconds,

			Volumes: b.buildVolumes(),

			DNSConfig: &corev1.PodDNSConfig{
				Searches: []string{
					fmt.Sprintf(v1alpha1.InterconnectServiceFQDNFormat, b.Spec.StorageClusterRef.Name, b.Spec.StorageClusterRef.Namespace),
				},
			},
		},
	}

	// InitContainer only needed for CaBundle manipulation for now,
	// may be probably used for other stuff later
	if b.areAnyCertificatesAddedToStore() {
		podTemplate.Spec.InitContainers = append(
			[]corev1.Container{b.buildCaStorePatchingInitContainer()},
			b.Spec.InitContainers...,
		)
	} else {
		podTemplate.Spec.InitContainers = b.Spec.InitContainers
	}

	if b.Spec.Image.PullSecret != nil {
		podTemplate.Spec.ImagePullSecrets = []corev1.LocalObjectReference{{Name: *b.Spec.Image.PullSecret}}
	}

	if value, ok := b.ObjectMeta.Annotations[v1alpha1.AnnotationUpdateDNSPolicy]; ok {
		switch value {
		case string(corev1.DNSClusterFirstWithHostNet), string(corev1.DNSClusterFirst), string(corev1.DNSDefault), string(corev1.DNSNone):
			podTemplate.Spec.DNSPolicy = corev1.DNSPolicy(value)
		case "":
			podTemplate.Spec.DNSPolicy = corev1.DNSClusterFirst
		default:
		}
	}

	return podTemplate
}

func (b *DatabaseStatefulSetBuilder) buildVolumes() []corev1.Volume {
	configMapName := b.Database.Name

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

	if b.Spec.Service.GRPC.TLSConfiguration.Enabled {
		volumes = append(volumes, buildTLSVolume(grpcTLSVolumeName, b.Spec.Service.GRPC.TLSConfiguration))
	}

	if b.Spec.Service.Interconnect.TLSConfiguration.Enabled {
		volumes = append(volumes, buildTLSVolume(interconnectTLSVolumeName, b.Spec.Service.Interconnect.TLSConfiguration))
	}

	if b.Spec.Encryption != nil && b.Spec.Encryption.Enabled {
		volumes = append(volumes, b.buildEncryptionVolume())
	}

	if b.Spec.Datastreams != nil && b.Spec.Datastreams.Enabled {
		volumes = append(volumes, b.buildDatastreamsIAMServiceAccountKeyVolume())
		if b.Spec.Service.Datastreams.TLSConfiguration.Enabled {
			volumes = append(volumes, buildTLSVolume(datastreamsTLSVolumeName, b.Spec.Service.Datastreams.TLSConfiguration))
		}
	}

	for _, volume := range b.Spec.Volumes {
		volumes = append(volumes, *volume)
	}

	for _, secret := range b.Spec.Secrets {
		volumes = append(volumes, corev1.Volume{
			Name: secret.Name,
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName: secret.Name,
				},
			},
		})
	}

	if b.areAnyCertificatesAddedToStore() {
		volumes = append(volumes, corev1.Volume{
			Name: systemCertsVolumeName,
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{},
			},
		})

		volumes = append(volumes, corev1.Volume{
			Name: localCertsVolumeName,
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{},
			},
		})
	}

	return volumes
}

func (b *DatabaseStatefulSetBuilder) buildCaStorePatchingInitContainer() corev1.Container {
	command, args := b.buildCaStorePatchingInitContainerArgs()
	imagePullPolicy := corev1.PullIfNotPresent
	if b.Spec.Image.PullPolicyName != nil {
		imagePullPolicy = *b.Spec.Image.PullPolicyName
	}
	container := corev1.Container{
		Name:            "ydb-storage-init-container",
		Image:           b.Spec.Image.Name,
		ImagePullPolicy: imagePullPolicy,
		Command:         command,
		Args:            args,
		SecurityContext: &corev1.SecurityContext{
			RunAsUser: new(int64),
		},

		VolumeMounts: b.buildCaStorePatchingInitContainerVolumeMounts(),
	}
	var containerResources corev1.ResourceRequirements
	if b.Spec.Resources != nil {
		containerResources = b.Spec.Resources.ContainerResources
	} else if b.Spec.SharedResources != nil {
		containerResources = b.Spec.SharedResources.ContainerResources
	}
	container.Resources = containerResources

	if len(b.Spec.CABundle) > 0 {
		container.Env = []corev1.EnvVar{
			{
				Name:  caBundleEnvName,
				Value: b.Spec.CABundle,
			},
		}
	}
	return container
}

func (b *DatabaseStatefulSetBuilder) areAnyCertificatesAddedToStore() bool {
	return len(b.Spec.CABundle) > 0 ||
		b.Spec.Service.GRPC.TLSConfiguration.Enabled ||
		b.Spec.Service.Interconnect.TLSConfiguration.Enabled ||
		b.Spec.Service.Datastreams.TLSConfiguration.Enabled
}

func (b *DatabaseStatefulSetBuilder) buildCaStorePatchingInitContainerVolumeMounts() []corev1.VolumeMount {
	volumeMounts := []corev1.VolumeMount{}

	if b.areAnyCertificatesAddedToStore() {
		volumeMounts = append(volumeMounts, corev1.VolumeMount{
			Name:      localCertsVolumeName,
			MountPath: localCertsDir,
		})

		volumeMounts = append(volumeMounts, corev1.VolumeMount{
			Name:      systemCertsVolumeName,
			MountPath: systemCertsDir,
		})
	}

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

	if b.Spec.Service.Datastreams.TLSConfiguration.Enabled {
		volumeMounts = append(volumeMounts, corev1.VolumeMount{
			Name:      datastreamsTLSVolumeName,
			ReadOnly:  true,
			MountPath: "/tls/datastreams", // fixme const
		})
	}
	return volumeMounts
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
						Path: v1alpha1.DatabaseEncryptionKeyFile,
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
						Path: v1alpha1.DatastreamsIAMServiceAccountKeyFile,
					},
				},
			},
		},
	}
}

func (b *DatabaseStatefulSetBuilder) buildContainer() corev1.Container {
	command, args := b.buildContainerArgs()
	imagePullPolicy := corev1.PullIfNotPresent
	if b.Spec.Image.PullPolicyName != nil {
		imagePullPolicy = *b.Spec.Image.PullPolicyName
	}
	container := corev1.Container{
		Name:            "ydb-dynamic",
		Image:           b.Spec.Image.Name,
		ImagePullPolicy: imagePullPolicy,
		Command:         command,
		Args:            args,
		Env:             b.buildEnv(),

		VolumeMounts: b.buildVolumeMounts(),
		SecurityContext: &corev1.SecurityContext{
			Privileged: ptr.Bool(false),
			Capabilities: &corev1.Capabilities{
				Add: []corev1.Capability{"SYS_RAWIO"},
			},
		},
	}

	if value, ok := b.ObjectMeta.Annotations[v1alpha1.AnnotationDisableLivenessProbe]; !ok || value != v1alpha1.AnnotationValueTrue {
		container.LivenessProbe = &corev1.Probe{
			ProbeHandler: corev1.ProbeHandler{
				TCPSocket: &corev1.TCPSocketAction{
					Port: intstr.FromInt(v1alpha1.GRPCPort),
				},
			},
		}
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
			MountPath: v1alpha1.DatabaseEncryptionKeyPath,
		})
	}

	if b.Spec.Datastreams != nil && b.Spec.Datastreams.Enabled {
		volumeMounts = append(volumeMounts, corev1.VolumeMount{
			Name:      datastreamsIAMServiceAccountKeyVolumeName,
			ReadOnly:  true,
			MountPath: v1alpha1.DatastreamsIAMServiceAccountKeyPath,
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

	if b.areAnyCertificatesAddedToStore() {
		volumeMounts = append(volumeMounts, corev1.VolumeMount{
			Name:      localCertsVolumeName,
			MountPath: localCertsDir,
		})

		volumeMounts = append(volumeMounts, corev1.VolumeMount{
			Name:      systemCertsVolumeName,
			MountPath: systemCertsDir,
		})
	}

	for _, secret := range b.Spec.Secrets {
		volumeMounts = append(volumeMounts, corev1.VolumeMount{
			Name:      secret.Name,
			MountPath: fmt.Sprintf("%s/%s", wellKnownDirForAdditionalSecrets, secret.Name),
		})
	}

	return volumeMounts
}

func (b *DatabaseStatefulSetBuilder) buildCaStorePatchingInitContainerArgs() ([]string, []string) {
	command := []string{"/bin/bash", "-c"}

	arg := ""

	if len(b.Spec.CABundle) > 0 {
		arg += fmt.Sprintf("printf $%s | base64 --decode > %s/%s && ", caBundleEnvName, localCertsDir, caBundleFileName)
	}

	if b.Spec.Service.GRPC.TLSConfiguration.Enabled {
		arg += fmt.Sprintf("cp /tls/grpc/ca.crt %s/grpcRoot.crt && ", localCertsDir) // fixme const
	}

	if b.Spec.Service.Interconnect.TLSConfiguration.Enabled {
		arg += fmt.Sprintf("cp /tls/interconnect/ca.crt %s/interconnectRoot.crt && ", localCertsDir) // fixme const
	}

	if arg != "" {
		arg += updateCACertificatesBin
	}

	args := []string{arg}

	return command, args
}

func (b *DatabaseStatefulSetBuilder) buildContainerArgs() ([]string, []string) {
	command := []string{fmt.Sprintf("%s/%s", v1alpha1.BinariesDir, v1alpha1.DaemonBinaryName)}

	args := []string{
		"server",

		"--mon-port",
		fmt.Sprintf("%d", v1alpha1.StatusPort),

		"--ic-port",
		fmt.Sprintf("%d", v1alpha1.InterconnectPort),

		"--yaml-config",
		fmt.Sprintf("%s/%s", v1alpha1.ConfigDir, v1alpha1.ConfigFileName),

		"--tenant",
		b.GetDatabasePath(),

		"--node-broker",
		b.Spec.StorageEndpoint,
	}

	for _, secret := range b.Spec.Secrets {
		exists, err := checkSecretHasField(
			b.GetNamespace(),
			secret.Name,
			v1alpha1.YdbAuthToken,
			b.RestConfig,
		)

		if err != nil {
			log.Default().Printf("Failed to inspect a secret %s: %s\n", secret.Name, err.Error())
		} else if exists {
			args = append(args,
				"--auth-token-file",
				fmt.Sprintf(
					"%s/%s/%s",
					wellKnownDirForAdditionalSecrets,
					secret.Name,
					v1alpha1.YdbAuthToken,
				),
			)
		}
	}

	publicHostOption := "--grpc-public-host"
	publicHost := fmt.Sprintf(v1alpha1.InterconnectServiceFQDNFormat, b.Database.Name, b.GetNamespace()) // FIXME .svc.cluster.local
	if b.Spec.Service.GRPC.ExternalHost != "" {
		publicHost = b.Spec.Service.GRPC.ExternalHost
	}
	publicPortOption := "--grpc-public-port"
	publicPort := v1alpha1.GRPCPort

	args = append(
		args,

		publicHostOption,
		fmt.Sprintf("%s.%s", "$(NODE_NAME)", publicHost), // fixme $(NODE_NAME)

		publicPortOption,
		strconv.Itoa(publicPort),
	)

	if value, ok := b.ObjectMeta.Annotations[v1alpha1.AnnotationDataCenter]; ok {
		if annotationDataCenterPattern.MatchString(value) {
			args = append(args,
				"--data-center",
				value,
			)
		}
	}

	if value, ok := b.ObjectMeta.Annotations[v1alpha1.AnnotationNodeHost]; ok {
		args = append(args,
			"--node-host",
			value,
		)
	}

	if value, ok := b.ObjectMeta.Annotations[v1alpha1.AnnotationNodeDomain]; ok {
		args = append(args,
			"--node-domain",
			value,
		)
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
