package resources

import (
	"context"
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

	api "github.com/ydb-platform/ydb-kubernetes-operator/api/v1alpha1"
	"github.com/ydb-platform/ydb-kubernetes-operator/internal/labels"
	"github.com/ydb-platform/ydb-kubernetes-operator/internal/ptr"
)

type DatabaseStatefulSetBuilder struct {
	*api.Database
	RestConfig *rest.Config

	Name        string
	Labels      map[string]string
	Annotations map[string]string
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

	sts.ObjectMeta.Labels = b.Labels
	sts.ObjectMeta.Annotations = b.Annotations

	replicas := ptr.Int32(b.Spec.Nodes)
	if b.Spec.Pause {
		replicas = ptr.Int32(0)
	}

	sts.Spec = appsv1.StatefulSetSpec{
		Replicas: replicas,
		Selector: &metav1.LabelSelector{
			MatchLabels: map[string]string{
				labels.StatefulsetComponent: b.Name,
			},
		},
		PodManagementPolicy:  appsv1.ParallelPodManagement,
		RevisionHistoryLimit: ptr.Int32(10),
		ServiceName:          fmt.Sprintf(InterconnectServiceNameFormat, b.Database.Name),
		Template:             b.buildPodTemplateSpec(),
	}

	if value, ok := b.ObjectMeta.Annotations[api.AnnotationUpdateStrategyOnDelete]; ok && value == api.AnnotationValueTrue {
		sts.Spec.UpdateStrategy = appsv1.StatefulSetUpdateStrategy{
			Type: "OnDelete",
		}
	}

	return nil
}

func (b *DatabaseStatefulSetBuilder) buildEnv() []corev1.EnvVar {
	var envVars []corev1.EnvVar

	envVars = append(envVars,
		corev1.EnvVar{
			Name: "NODE_NAME", // for `--grpc-public-host` flag
			ValueFrom: &corev1.EnvVarSource{
				FieldRef: &corev1.ObjectFieldSelector{
					APIVersion: "v1",
					FieldPath:  "metadata.name",
				},
			},
		},
		corev1.EnvVar{
			Name: "POD_IP", // for `--grpc-public-address-<ip-family>` flag
			ValueFrom: &corev1.EnvVarSource{
				FieldRef: &corev1.ObjectFieldSelector{
					APIVersion: "v1",
					FieldPath:  "status.podIP",
				},
			},
		},
	)

	return envVars
}

func (b *DatabaseStatefulSetBuilder) buildPodTemplateLabels() labels.Labels {
	podTemplateLabels := labels.Labels{}

	podTemplateLabels.Merge(b.Labels)
	podTemplateLabels.Merge(map[string]string{labels.StatefulsetComponent: b.Name})
	podTemplateLabels.Merge(b.Spec.AdditionalPodLabels)

	return podTemplateLabels
}

func (b *DatabaseStatefulSetBuilder) buildPodTemplateSpec() corev1.PodTemplateSpec {
	podTemplateLabels := b.buildPodTemplateLabels()

	domain := api.DefaultDomainName
	if dnsAnnotation, ok := b.GetAnnotations()[api.DNSDomainAnnotation]; ok {
		domain = dnsAnnotation
	}
	podTemplate := corev1.PodTemplateSpec{
		ObjectMeta: metav1.ObjectMeta{
			Labels:      podTemplateLabels,
			Annotations: b.Annotations,
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
					fmt.Sprintf(api.InterconnectServiceFQDNFormat, b.Spec.StorageClusterRef.Name, b.Spec.StorageClusterRef.Namespace, domain),
				},
			},
		},
	}

	// InitContainer only needed for CaBundle manipulation for now,
	// may be probably used for other stuff later
	if b.AnyCertificatesAdded() {
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

	if value, ok := b.ObjectMeta.Annotations[api.AnnotationUpdateDNSPolicy]; ok {
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
	configMapName := b.Spec.StorageClusterRef.Name
	if b.Spec.Configuration != "" {
		configMapName = b.GetName()
	}

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

	if b.Spec.Service.Status.TLSConfiguration.Enabled {
		volumes = append(volumes,
			buildTLSVolume(statusOriginTLSVolumeName, b.Spec.Service.Status.TLSConfiguration),
			corev1.Volume{
				Name: statusTLSVolumeName,
				VolumeSource: corev1.VolumeSource{
					EmptyDir: &corev1.EmptyDirVolumeSource{},
				},
			},
		)
	}

	if b.Spec.Encryption != nil && b.Spec.Encryption.Enabled {
		volumes = append(volumes, b.buildEncryptionVolumes()...)
	}

	if b.Spec.Datastreams != nil && b.Spec.Datastreams.Enabled {
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

	if b.AnyCertificatesAdded() {
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
	command, args := buildCAStorePatchingCommandArgs(
		b.Spec.CABundle,
		b.Spec.Service.GRPC,
		b.Spec.Service.Interconnect,
		b.Spec.Service.Status,
	)
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

func (b *DatabaseStatefulSetBuilder) buildCaStorePatchingInitContainerVolumeMounts() []corev1.VolumeMount {
	volumeMounts := []corev1.VolumeMount{}

	if b.AnyCertificatesAdded() {
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
			MountPath: grpcTLSVolumeMountPath,
		})
	}

	if b.Spec.Service.Interconnect.TLSConfiguration.Enabled {
		volumeMounts = append(volumeMounts, corev1.VolumeMount{
			Name:      interconnectTLSVolumeName,
			ReadOnly:  true,
			MountPath: interconnectTLSVolumeMountPath,
		})
	}

	if b.Spec.Service.Datastreams.TLSConfiguration.Enabled {
		volumeMounts = append(volumeMounts, corev1.VolumeMount{
			Name:      datastreamsTLSVolumeName,
			ReadOnly:  true,
			MountPath: datastreamsTLSVolumeMountPath,
		})
	}

	if b.Spec.Service.Status.TLSConfiguration.Enabled {
		volumeMounts = append(volumeMounts, corev1.VolumeMount{
			Name:      statusOriginTLSVolumeName,
			ReadOnly:  true,
			MountPath: statusOriginTLSVolumeMountPath,
		})
		volumeMounts = append(volumeMounts, corev1.VolumeMount{
			Name:      statusTLSVolumeName,
			MountPath: statusTLSVolumeMountPath,
		})
	}

	return volumeMounts
}

func buildTLSVolume(name string, configuration *api.TLSConfiguration) corev1.Volume { // fixme move somewhere?
	volume := corev1.Volume{
		Name: name,
		VolumeSource: corev1.VolumeSource{
			Secret: &corev1.SecretVolumeSource{
				SecretName: configuration.Key.Name, // todo validate that Name is equal for all params
				Items: []corev1.KeyToPath{
					{
						Key:  configuration.CertificateAuthority.Key,
						Path: wellKnownNameForTLSCertificateAuthority,
					},
					{
						Key:  configuration.Certificate.Key,
						Path: wellKnownNameForTLSCertificate,
					},
					{
						Key:  configuration.Key.Key,
						Path: wellKnownNameForTLSPrivateKey,
					},
				},
			},
		},
	}

	return volume
}

func (b *DatabaseStatefulSetBuilder) buildEncryptionVolumes() []corev1.Volume {
	var secretName, secretKey string
	if b.Spec.Encryption.Key != nil {
		secretName = b.Spec.Encryption.Key.Name
		secretKey = b.Spec.Encryption.Key.Key
	} else {
		secretName = b.Name
		secretKey = wellKnownNameForEncryptionKeySecret
	}

	encryptionKeySecret := corev1.Volume{
		Name: encryptionKeySecretVolumeName,
		VolumeSource: corev1.VolumeSource{
			Secret: &corev1.SecretVolumeSource{
				SecretName: secretName,
				Items: []corev1.KeyToPath{
					{
						Key:  secretKey,
						Path: api.DatabaseEncryptionKeySecretFile,
					},
				},
			},
		},
	}

	encryptionKeyConfig := corev1.Volume{
		Name: encryptionKeyConfigVolumeName,
		VolumeSource: corev1.VolumeSource{
			ConfigMap: &corev1.ConfigMapVolumeSource{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: fmt.Sprintf(EncryptionKeyConfigNameFormat, b.GetName()),
				},
			},
		},
	}

	return []corev1.Volume{encryptionKeySecret, encryptionKeyConfig}
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

	if value, ok := b.ObjectMeta.Annotations[api.AnnotationDisableLivenessProbe]; !ok || value != api.AnnotationValueTrue {
		container.LivenessProbe = &corev1.Probe{
			ProbeHandler: corev1.ProbeHandler{
				TCPSocket: &corev1.TCPSocketAction{
					Port: intstr.FromInt(api.GRPCPort),
				},
			},
		}
	}

	ports := []corev1.ContainerPort{{
		Name: "grpc", ContainerPort: api.GRPCPort,
	}, {
		Name: "interconnect", ContainerPort: api.InterconnectPort,
	}, {
		Name: "status", ContainerPort: api.StatusPort,
	}}

	if b.Spec.Datastreams != nil && b.Spec.Datastreams.Enabled {
		ports = append(ports, corev1.ContainerPort{
			Name: "datastreams", ContainerPort: api.DatastreamsPort,
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
		MountPath: fmt.Sprintf("%s/%s", api.ConfigDir, api.ConfigFileName),
		SubPath:   api.ConfigFileName,
	})

	if b.Spec.Service.GRPC.TLSConfiguration.Enabled {
		volumeMounts = append(volumeMounts, corev1.VolumeMount{
			Name:      grpcTLSVolumeName,
			ReadOnly:  true,
			MountPath: grpcTLSVolumeMountPath,
		})
	}

	if b.Spec.Service.Interconnect.TLSConfiguration.Enabled {
		volumeMounts = append(volumeMounts, corev1.VolumeMount{
			Name:      interconnectTLSVolumeName,
			ReadOnly:  true,
			MountPath: interconnectTLSVolumeMountPath,
		})
	}

	if b.Spec.Service.Status.TLSConfiguration.Enabled {
		volumeMounts = append(volumeMounts, corev1.VolumeMount{
			Name:      statusTLSVolumeName,
			ReadOnly:  true,
			MountPath: statusTLSVolumeMountPath,
		})
	}

	if b.Spec.Encryption != nil && b.Spec.Encryption.Enabled {
		volumeMounts = append(volumeMounts, corev1.VolumeMount{
			Name:      encryptionKeyConfigVolumeName,
			ReadOnly:  true,
			MountPath: fmt.Sprintf("%s/%s", api.ConfigDir, api.DatabaseEncryptionKeyConfigFile),
			SubPath:   api.DatabaseEncryptionKeyConfigFile,
		})

		volumeMounts = append(volumeMounts, corev1.VolumeMount{
			Name:      encryptionKeySecretVolumeName,
			ReadOnly:  true,
			MountPath: fmt.Sprintf("%s/%s", wellKnownDirForAdditionalSecrets, api.DatabaseEncryptionKeySecretDir),
		})
	}

	if b.Spec.Datastreams != nil && b.Spec.Datastreams.Enabled {
		if b.Spec.Service.Datastreams.TLSConfiguration.Enabled {
			volumeMounts = append(volumeMounts, corev1.VolumeMount{
				Name:      datastreamsTLSVolumeName,
				ReadOnly:  true,
				MountPath: datastreamsTLSVolumeMountPath,
			})
		}
	}

	for _, volume := range b.Spec.Volumes {
		volumeMounts = append(volumeMounts, corev1.VolumeMount{
			Name:      volume.Name,
			MountPath: fmt.Sprintf("%s/%s", wellKnownDirForAdditionalVolumes, volume.Name),
		})
	}

	if b.AnyCertificatesAdded() {
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

func (b *DatabaseStatefulSetBuilder) buildContainerArgs() ([]string, []string) {
	domain := api.DefaultDomainName
	if dnsAnnotation, ok := b.GetAnnotations()[api.DNSDomainAnnotation]; ok {
		domain = dnsAnnotation
	}
	command := []string{fmt.Sprintf("%s/%s", api.BinariesDir, api.DaemonBinaryName)}

	args := []string{
		"server",

		"--mon-port",
		fmt.Sprintf("%d", api.StatusPort),

		"--ic-port",
		fmt.Sprintf("%d", api.InterconnectPort),

		"--yaml-config",
		fmt.Sprintf("%s/%s", api.ConfigDir, api.ConfigFileName),

		"--tenant",
		b.GetDatabasePath(),

		"--node-broker",
		b.Spec.StorageEndpoint,

		"--label",
		fmt.Sprintf("%s=%s", api.LabelDeploymentKey, api.LabelDeploymentValueKubernetes),
	}

	if b.Spec.SharedResources != nil {
		args = append(args,
			"--label",
			fmt.Sprintf("%s=%s", api.LabelSharedDatabaseKey, api.LabelSharedDatabaseValueTrue),
		)
	} else {
		args = append(args,
			"--label",
			fmt.Sprintf("%s=%s", api.LabelSharedDatabaseKey, api.LabelSharedDatabaseValueFalse),
		)
	}

	if b.Spec.Encryption != nil && b.Spec.Encryption.Enabled {
		args = append(args,
			"--key-file",
			fmt.Sprintf("%s/%s", api.ConfigDir, api.DatabaseEncryptionKeyConfigFile),
		)
	}

	// hotfix KIKIMR-16728
	if b.Spec.Service.GRPC.TLSConfiguration.Enabled {
		args = append(args,
			"--grpc-cert",
			fmt.Sprintf("%s/%s", grpcTLSVolumeMountPath, wellKnownNameForTLSCertificate),
			"--grpc-key",
			fmt.Sprintf("%s/%s", grpcTLSVolumeMountPath, wellKnownNameForTLSPrivateKey),
			"--grpc-ca",
			fmt.Sprintf("%s/%s", systemCertsDir, caCertificatesFileName),
		)
	}

	if b.Spec.Service.Status.TLSConfiguration.Enabled {
		args = append(args,
			"--mon-cert",
			fmt.Sprintf("%s/%s", statusTLSVolumeMountPath, statusBundleFileName),
		)
	}

	for _, secret := range b.Spec.Secrets {
		exist, err := CheckSecretKey(
			context.Background(),
			b.GetNamespace(),
			b.RestConfig,
			&corev1.SecretKeySelector{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: secret.Name,
				},
				Key: api.YdbAuthToken,
			},
		)
		if err != nil {
			log.Default().Printf("Failed to inspect a secret %s: %s\n", secret.Name, err.Error())
			continue
		}
		if exist {
			args = append(args,
				"--auth-token-file",
				fmt.Sprintf(
					"%s/%s/%s",
					wellKnownDirForAdditionalSecrets,
					secret.Name,
					api.YdbAuthToken,
				),
			)
		}
	}

	publicHostOption := "--grpc-public-host"
	publicHost := fmt.Sprintf(api.InterconnectServiceFQDNFormat, b.Database.Name, b.GetNamespace(), domain) // FIXME .svc.cluster.local

	if b.Spec.Service.GRPC.ExternalHost != "" {
		publicHost = b.Spec.Service.GRPC.ExternalHost
	}
	if value, ok := b.ObjectMeta.Annotations[api.AnnotationGRPCPublicHost]; ok {
		publicHost = value
	}

	if b.Spec.Service.GRPC.IPDiscovery != nil && b.Spec.Service.GRPC.IPDiscovery.Enabled {
		targetNameOverride := b.Spec.Service.GRPC.IPDiscovery.TargetNameOverride
		ipFamilyArg := "--grpc-public-address-v4"

		if b.Spec.Service.GRPC.IPDiscovery.IPFamily == corev1.IPv6Protocol {
			ipFamilyArg = "--grpc-public-address-v6"
		}

		args = append(
			args,

			ipFamilyArg,
			"$(POD_IP)",
		)
		if targetNameOverride != "" {
			args = append(
				args,
				"--grpc-public-target-name-override",
				fmt.Sprintf("%s.%s", "$(NODE_NAME)", targetNameOverride),
			)
		}
	}

	publicPortOption := "--grpc-public-port"
	publicPort := api.GRPCPort

	args = append(
		args,

		publicHostOption,
		fmt.Sprintf("%s.%s", "$(NODE_NAME)", publicHost), // fixme $(NODE_NAME)

		publicPortOption,
		strconv.Itoa(publicPort),
	)

	if value, ok := b.ObjectMeta.Annotations[api.AnnotationDataCenter]; ok {
		if annotationDataCenterPattern.MatchString(value) {
			args = append(args,
				"--data-center",
				value,
			)
		}
	}

	if value, ok := b.ObjectMeta.Annotations[api.AnnotationNodeHost]; ok {
		args = append(args,
			"--node-host",
			value,
		)
	}

	if value, ok := b.ObjectMeta.Annotations[api.AnnotationNodeDomain]; ok {
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
