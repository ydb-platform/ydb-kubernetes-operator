package resources

import (
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

	"github.com/ydb-platform/ydb-kubernetes-operator/api/v1alpha1"
	"github.com/ydb-platform/ydb-kubernetes-operator/internal/ptr"
)

const (
	configVolumeName = "ydb-config"
)

type StorageStatefulSetBuilder struct {
	*v1alpha1.Storage
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
	return b.Name + "-" + StringRJust(strconv.Itoa(index), "0", v1alpha1.DiskNumberMaxDigits)
}

func (b *StorageStatefulSetBuilder) GenerateDeviceName(index int) string {
	return v1alpha1.DiskPathPrefix + "_" + StringRJust(strconv.Itoa(index), "0", v1alpha1.DiskNumberMaxDigits)
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
		ServiceName:          fmt.Sprintf(interconnectServiceNameFormat, b.Storage.Name),
		Template:             b.buildPodTemplateSpec(),
	}

	if value, ok := b.ObjectMeta.Annotations[v1alpha1.AnnotationUpdateStrategyOnDelete]; ok && value == v1alpha1.AnnotationValueTrue {
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
		fmt.Sprintf(v1alpha1.InterconnectServiceFQDNFormat, b.Storage.Name, b.GetNamespace()),
	}
	podTemplate := corev1.PodTemplateSpec{
		ObjectMeta: metav1.ObjectMeta{
			Labels:      b.Labels,
			Annotations: CopyDict(b.Spec.AdditionalAnnotations),
		},
		Spec: corev1.PodSpec{
			Containers:                []corev1.Container{b.buildContainer()},
			NodeSelector:              b.Spec.NodeSelector,
			Affinity:                  b.Spec.Affinity,
			Tolerations:               b.Spec.Tolerations,
			PriorityClassName:         b.Spec.PriorityClassName,
			TopologySpreadConstraints: b.buildTopologySpreadConstraints(),

			Volumes: b.buildVolumes(),

			DNSConfig: &corev1.PodDNSConfig{
				Searches: dnsConfigSearches,
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

	if b.Spec.HostNetwork {
		podTemplate.Spec.HostNetwork = true
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

func (b *StorageStatefulSetBuilder) buildTopologySpreadConstraints() []corev1.TopologySpreadConstraint {
	if len(b.Spec.TopologySpreadConstraints) > 0 {
		return b.Spec.TopologySpreadConstraints
	}

	if b.Spec.Erasure != v1alpha1.ErasureMirror3DC {
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

func (b *StorageStatefulSetBuilder) buildCaStorePatchingInitContainer() corev1.Container {
	command, args := b.buildCaStorePatchingInitContainerArgs()
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

func (b *StorageStatefulSetBuilder) areAnyCertificatesAddedToStore() bool {
	return len(b.Spec.CABundle) > 0 ||
		b.Spec.Service.GRPC.TLSConfiguration.Enabled ||
		b.Spec.Service.Interconnect.TLSConfiguration.Enabled
}

func (b *StorageStatefulSetBuilder) buildCaStorePatchingInitContainerVolumeMounts() []corev1.VolumeMount {
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
			Name: "grpc", ContainerPort: v1alpha1.GRPCPort,
		}, {
			Name: "interconnect", ContainerPort: v1alpha1.InterconnectPort,
		}, {
			Name: "status", ContainerPort: v1alpha1.StatusPort,
		}},

		VolumeMounts: b.buildVolumeMounts(),
		Resources:    containerResources,
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

	var volumeDeviceList []corev1.VolumeDevice // todo decide on PVC volumeMode?
	var volumeMountList []corev1.VolumeMount
	for i, spec := range b.Spec.DataStore {
		if *spec.VolumeMode == corev1.PersistentVolumeFilesystem {
			volumeMountList = append(
				volumeMountList,
				corev1.VolumeMount{
					Name:      b.GeneratePVCName(i),
					MountPath: v1alpha1.DiskFilePath,
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
			MountPath: v1alpha1.ConfigDir,
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

	for _, volume := range b.Spec.Volumes {
		volumeMounts = append(volumeMounts, corev1.VolumeMount{
			Name:      volume.Name,
			MountPath: fmt.Sprintf("%s/%s", wellKnownDirForAdditionalVolumes, volume.Name),
		})
	}

	return volumeMounts
}

func (b *StorageStatefulSetBuilder) buildCaStorePatchingInitContainerArgs() ([]string, []string) {
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

func (b *StorageStatefulSetBuilder) buildContainerArgs() ([]string, []string) {
	command := []string{fmt.Sprintf("%s/%s", v1alpha1.BinariesDir, v1alpha1.DaemonBinaryName)}
	var args []string

	args = append(args,
		"server",

		"--mon-port",
		fmt.Sprintf("%d", v1alpha1.StatusPort),

		"--ic-port",
		fmt.Sprintf("%d", v1alpha1.InterconnectPort),

		"--yaml-config",
		fmt.Sprintf("%s/%s", v1alpha1.ConfigDir, v1alpha1.ConfigFileName),

		"--node",
		"static",
	)

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
