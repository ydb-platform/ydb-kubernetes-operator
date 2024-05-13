package resources

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strconv"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"

	api "github.com/ydb-platform/ydb-kubernetes-operator/api/v1alpha1"
	"github.com/ydb-platform/ydb-kubernetes-operator/internal/annotations"
	"github.com/ydb-platform/ydb-kubernetes-operator/internal/labels"
	"github.com/ydb-platform/ydb-kubernetes-operator/internal/ptr"
)

const (
	configVolumeName = "ydb-config"
)

type StorageStatefulSetBuilder struct {
	*api.Storage
	RestConfig *rest.Config

	Name   string
	Labels map[string]string
}

func StringRJust(str, pad string, length int) string {
	for {
		str = pad + str
		if len(str) > length {
			return str[len(str)-length:]
		}
	}
}

func (b *StorageStatefulSetBuilder) GeneratePVCName(index int) string {
	return b.Name + "-" + StringRJust(strconv.Itoa(index), "0", api.DiskNumberMaxDigits)
}

func (b *StorageStatefulSetBuilder) GenerateDeviceName(index int) string {
	return api.DiskPathPrefix + "_" + StringRJust(strconv.Itoa(index), "0", api.DiskNumberMaxDigits)
}

func (b *StorageStatefulSetBuilder) Build(obj client.Object) error {
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
		ServiceName:          fmt.Sprintf(InterconnectServiceNameFormat, b.Storage.Name),
		Template:             b.buildPodTemplateSpec(),
	}

	if value, ok := b.ObjectMeta.Annotations[api.AnnotationUpdateStrategyOnDelete]; ok && value == api.AnnotationValueTrue {
		sts.Spec.UpdateStrategy = appsv1.StatefulSetUpdateStrategy{
			Type: "OnDelete",
		}
	}

	pvcList := make([]corev1.PersistentVolumeClaim, 0, len(b.Spec.DataStore))
	for i, pvcSpec := range b.Spec.DataStore {
		pvcList = append(
			pvcList,
			corev1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name: b.GeneratePVCName(i),
				},
				Spec: pvcSpec,
			},
		)
	}
	sts.Spec.VolumeClaimTemplates = pvcList

	return nil
}

func (b *StorageStatefulSetBuilder) buildPodTemplateSpec() corev1.PodTemplateSpec {
	dnsConfigSearches := []string{
		fmt.Sprintf(api.InterconnectServiceFQDNFormat, b.Storage.Name, b.GetNamespace()),
	}

	podTemplateLabels := CopyDict(b.Labels)
	podTemplateLabels[labels.StorageGeneration] = strconv.FormatInt(b.ObjectMeta.Generation, 10)

	podTemplateAnnotations := CopyDict(b.Spec.AdditionalAnnotations)
	podTemplateAnnotations[annotations.ConfigurationChecksum] = GetConfigurationChecksum(b.Spec.Configuration)

	podTemplate := corev1.PodTemplateSpec{
		ObjectMeta: metav1.ObjectMeta{
			Labels:      podTemplateLabels,
			Annotations: podTemplateAnnotations,
		},
		Spec: corev1.PodSpec{
			Containers:                    []corev1.Container{b.buildContainer()},
			NodeSelector:                  b.Spec.NodeSelector,
			Affinity:                      b.Spec.Affinity,
			Tolerations:                   b.Spec.Tolerations,
			PriorityClassName:             b.Spec.PriorityClassName,
			TopologySpreadConstraints:     b.buildTopologySpreadConstraints(),
			TerminationGracePeriodSeconds: b.Spec.TerminationGracePeriodSeconds,

			Volumes: b.buildVolumes(),

			DNSConfig: &corev1.PodDNSConfig{
				Searches: dnsConfigSearches,
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

	if b.Spec.HostNetwork {
		podTemplate.Spec.HostNetwork = true
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

func (b *StorageStatefulSetBuilder) buildTopologySpreadConstraints() []corev1.TopologySpreadConstraint {
	if len(b.Spec.TopologySpreadConstraints) > 0 {
		return b.Spec.TopologySpreadConstraints
	}

	if b.Spec.Erasure != api.ErasureMirror3DC {
		return []corev1.TopologySpreadConstraint{}
	}

	return []corev1.TopologySpreadConstraint{
		{
			TopologyKey:       corev1.LabelTopologyZone,
			WhenUnsatisfiable: corev1.DoNotSchedule,
			LabelSelector:     &metav1.LabelSelector{MatchLabels: b.Labels},
			MaxSkew:           1,
		},
		{
			TopologyKey:       corev1.LabelHostname,
			WhenUnsatisfiable: corev1.DoNotSchedule,
			LabelSelector:     &metav1.LabelSelector{MatchLabels: b.Labels},
			MaxSkew:           1,
		},
	}
}

func (b *StorageStatefulSetBuilder) buildVolumes() []corev1.Volume {
	configMapName := b.Storage.Name

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

	for _, volume := range b.Spec.Volumes {
		volumes = append(volumes, *volume)
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

func (b *StorageStatefulSetBuilder) buildCaStorePatchingInitContainer() corev1.Container {
	command, args := buildCAStorePatchingCommandArgs(
		b.Spec.CABundle,
		b.Spec.Service.GRPC,
		b.Spec.Service.Interconnect,
	)
	containerResources := corev1.ResourceRequirements{}
	if b.Spec.Resources != nil {
		containerResources = *b.Spec.Resources
	}
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
		Resources:    containerResources,
	}
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

func (b *StorageStatefulSetBuilder) buildCaStorePatchingInitContainerVolumeMounts() []corev1.VolumeMount {
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
	return volumeMounts
}

func (b *StorageStatefulSetBuilder) buildContainer() corev1.Container { // todo add init container for sparse files?
	command, args := b.buildContainerArgs()
	containerResources := corev1.ResourceRequirements{}
	if b.Spec.Resources != nil {
		containerResources = *b.Spec.Resources
	}
	imagePullPolicy := corev1.PullIfNotPresent
	if b.Spec.Image.PullPolicyName != nil {
		imagePullPolicy = *b.Spec.Image.PullPolicyName
	}

	container := corev1.Container{
		Name:            "ydb-storage",
		Image:           b.Spec.Image.Name,
		ImagePullPolicy: imagePullPolicy,
		Command:         command,
		Args:            args,

		SecurityContext: &corev1.SecurityContext{
			Privileged: ptr.Bool(false),
			Capabilities: &corev1.Capabilities{
				Add: []corev1.Capability{"SYS_RAWIO"},
			},
		},

		Ports: []corev1.ContainerPort{{
			Name: "grpc", ContainerPort: api.GRPCPort,
		}, {
			Name: "interconnect", ContainerPort: api.InterconnectPort,
		}, {
			Name: "status", ContainerPort: api.StatusPort,
		}},

		VolumeMounts: b.buildVolumeMounts(),
		Resources:    containerResources,
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

	var volumeDeviceList []corev1.VolumeDevice // todo decide on PVC volumeMode?
	var volumeMountList []corev1.VolumeMount
	for i, spec := range b.Spec.DataStore {
		if *spec.VolumeMode == corev1.PersistentVolumeFilesystem {
			volumeMountList = append(
				volumeMountList,
				corev1.VolumeMount{
					Name:      b.GeneratePVCName(i),
					MountPath: api.DiskFilePath,
				},
			)
		}
		if *spec.VolumeMode == corev1.PersistentVolumeBlock {
			volumeDeviceList = append(
				volumeDeviceList,
				corev1.VolumeDevice{
					Name:       b.GeneratePVCName(i),
					DevicePath: b.GenerateDeviceName(i),
				},
			)
		}
	}
	container.VolumeDevices = append(container.VolumeDevices, volumeDeviceList...)
	container.VolumeMounts = append(container.VolumeMounts, volumeMountList...)

	return container
}

func (b *StorageStatefulSetBuilder) buildVolumeMounts() []corev1.VolumeMount {
	volumeMounts := []corev1.VolumeMount{
		{
			Name:      configVolumeName,
			ReadOnly:  true,
			MountPath: fmt.Sprintf("%s/%s", api.ConfigDir, api.ConfigFileName),
			SubPath:   api.ConfigFileName,
		},
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

	for _, volume := range b.Spec.Volumes {
		volumeMounts = append(volumeMounts, corev1.VolumeMount{
			Name:      volume.Name,
			MountPath: fmt.Sprintf("%s/%s", wellKnownDirForAdditionalVolumes, volume.Name),
		})
	}

	return volumeMounts
}

func (b *StorageStatefulSetBuilder) buildContainerArgs() ([]string, []string) {
	command := []string{fmt.Sprintf("%s/%s", api.BinariesDir, api.DaemonBinaryName)}
	var args []string

	args = append(args,
		"server",

		"--mon-port",
		fmt.Sprintf("%d", api.StatusPort),

		"--ic-port",
		fmt.Sprintf("%d", api.InterconnectPort),

		"--yaml-config",
		fmt.Sprintf("%s/%s", api.ConfigDir, api.ConfigFileName),

		"--node",
		"static",

		"--label",
		fmt.Sprintf("%s=%s", api.LabelDeploymentKey, api.LabelDeploymentValueKubernetes),
	)

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

	return command, args
}

func (b *StorageStatefulSetBuilder) Placeholder(cr client.Object) client.Object {
	return &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cr.GetName(),
			Namespace: cr.GetNamespace(),
		},
	}
}
