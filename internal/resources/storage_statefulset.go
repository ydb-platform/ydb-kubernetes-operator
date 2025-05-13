package resources

import (
	"errors"
	"fmt"
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

const (
	configVolumeName = "ydb-config"
)

type StorageStatefulSetBuilder struct {
	*api.Storage
	RestConfig *rest.Config

	Name        string
	Labels      map[string]string
	Annotations map[string]string
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

func (b *StorageStatefulSetBuilder) buildPodTemplateLabels() labels.Labels {
	podTemplateLabels := labels.Labels{}

	podTemplateLabels.Merge(b.Labels)
	podTemplateLabels.Merge(map[string]string{labels.StatefulsetComponent: b.Name})
	podTemplateLabels.Merge(b.Spec.AdditionalPodLabels)

	return podTemplateLabels
}

func (b *StorageStatefulSetBuilder) buildPodTemplateSpec() corev1.PodTemplateSpec {
	podTemplateLabels := b.buildPodTemplateLabels()

	domain := api.DefaultDomainName
	if dnsAnnotation, ok := b.GetAnnotations()[api.DNSDomainAnnotation]; ok {
		domain = dnsAnnotation
	}
	dnsConfigSearches := []string{
		fmt.Sprintf(api.InterconnectServiceFQDNFormat, b.Storage.Name, b.GetNamespace(), domain),
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
		volumes = append(volumes, buildTLSVolume(GRPCTLSVolumeName, b.Spec.Service.GRPC.TLSConfiguration))
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
		b.Spec.Service.Status,
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
			Name:      GRPCTLSVolumeName,
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

func (b *StorageStatefulSetBuilder) buildContainerPorts() []corev1.ContainerPort {
	podPorts := []corev1.ContainerPort{{
		Name: "interconnect", ContainerPort: api.InterconnectPort,
	}, {
		Name: "status", ContainerPort: api.StatusPort,
	}}

	firstGRPCPort := corev1.ContainerPort{
		Name:          "grpc",
		ContainerPort: api.GRPCPort,
	}

	overrideGRPCPort := b.Spec.StorageClusterSpec.Service.GRPC.Port
	if overrideGRPCPort != 0 {
		firstGRPCPort.ContainerPort = overrideGRPCPort
	}

	podPorts = append(podPorts, firstGRPCPort)

	additionalPort := b.Spec.StorageClusterSpec.Service.GRPC.AdditionalPort
	if additionalPort != 0 {
		podPorts = append(podPorts, corev1.ContainerPort{
			Name:          "additional-grpc",
			ContainerPort: additionalPort,
		})
	}

	return podPorts
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

		SecurityContext: mergeSecurityContextWithDefaults(b.Spec.SecurityContext),

		Ports: b.buildContainerPorts(),

		VolumeMounts: b.buildVolumeMounts(),
		Resources:    containerResources,
	}

	livenessProbePort := api.GRPCPort
	if b.Spec.Service.GRPC.Port != 0 {
		livenessProbePort = int(b.Spec.Service.GRPC.Port)
	}

	if value, ok := b.ObjectMeta.Annotations[api.AnnotationDisableLivenessProbe]; !ok || value != api.AnnotationValueTrue {
		container.LivenessProbe = &corev1.Probe{
			ProbeHandler: corev1.ProbeHandler{
				TCPSocket: &corev1.TCPSocketAction{
					Port: intstr.FromInt(livenessProbePort),
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
			Name:      GRPCTLSVolumeName,
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

	if b.Spec.Service.Status.TLSConfiguration.Enabled {
		args = append(args,
			"--mon-cert",
			fmt.Sprintf("%s/%s", statusTLSVolumeMountPath, statusBundleFileName),
		)
	}

	authTokenSecretName := api.AuthTokenSecretName
	authTokenSecretKey := api.AuthTokenSecretKey
	if value, ok := b.ObjectMeta.Annotations[api.AnnotationAuthTokenSecretName]; ok {
		authTokenSecretName = value
	}
	if value, ok := b.ObjectMeta.Annotations[api.AnnotationAuthTokenSecretKey]; ok {
		authTokenSecretKey = value
	}
	for _, secret := range b.Spec.Secrets {
		if secret.Name == authTokenSecretName {
			args = append(args,
				api.AuthTokenFileArg,
				fmt.Sprintf(
					"%s/%s/%s",
					wellKnownDirForAdditionalSecrets,
					authTokenSecretName,
					authTokenSecretKey,
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
