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

const (
	configVolumeName = "ydb-config"
)

type StorageStatefulSetBuilder struct {
	*v1alpha1.Storage

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
	sts.ObjectMeta.Namespace = b.Namespace
	sts.ObjectMeta.Annotations = CopyDict(b.Spec.AdditionalAnnotations)

	sts.Spec = appsv1.StatefulSetSpec{
		Replicas: ptr.Int32(b.Spec.Nodes),
		Selector: &metav1.LabelSelector{
			MatchLabels: b.Labels,
		},
		PodManagementPolicy:  appsv1.ParallelPodManagement,
		RevisionHistoryLimit: ptr.Int32(10),
		ServiceName:          fmt.Sprintf(interconnectServiceNameFormat, b.GetName()),
		Template:             b.buildPodTemplateSpec(),
	}

	var pvcList []corev1.PersistentVolumeClaim
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
		fmt.Sprintf(
			"%s-interconnect.%s.svc.cluster.local", // fixme .svc.cluster.local should not be hardcoded
			b.Name,
			b.Namespace,
		),
	}

	podTemplate := corev1.PodTemplateSpec{
		ObjectMeta: metav1.ObjectMeta{
			Labels:      b.Labels,
			Annotations: CopyDict(b.Spec.AdditionalAnnotations),
		},
		Spec: corev1.PodSpec{
			Containers:     []corev1.Container{b.buildContainer()},
			NodeSelector:   b.Spec.NodeSelector,
			Affinity:       b.Spec.Affinity,
			Tolerations:    b.Spec.Tolerations,

			Volumes: b.buildVolumes(),

			DNSConfig: &corev1.PodDNSConfig{
				Searches: dnsConfigSearches,
			},
			TopologySpreadConstraints: b.buildTopologySpreadConstraints(),
		},
	}

	// InitContainer only needed for CaBundle manipulation for now,
	// may be probably used for other stuff later
	if b.Spec.CaBundle != "" {
		podTemplate.Spec.InitContainers = append(
			[]corev1.Container{b.buildInitContainer()},
			b.Spec.InitContainers...,
		)
	} else {
		podTemplate.Spec.InitContainers = b.Spec.InitContainers
	}

	if b.Spec.HostNetwork {
		podTemplate.Spec.HostNetwork = true
		podTemplate.Spec.DNSPolicy = corev1.DNSClusterFirstWithHostNet
	}
	if b.Spec.Image.PullSecret != nil {
		podTemplate.Spec.ImagePullSecrets = []corev1.LocalObjectReference{{Name: *b.Spec.Image.PullSecret}}
	}
	return podTemplate
}

func (b *StorageStatefulSetBuilder) buildTopologySpreadConstraints() []corev1.TopologySpreadConstraint {
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

	if b.areAnyCertificatesAddedToStore() {
		volumes = append(volumes, corev1.Volume{
			Name: initMainSharedCertsVolumeName,
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{},
			},
		})

		volumes = append(volumes, corev1.Volume{
			Name: initMainSharedSourceDirVolumeName,
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{},
			},
		})
	}

	if b.Spec.CaBundle != "" {
		volumes = append(volumes, corev1.Volume{
			Name: caBundleVolumeName,
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{Name: caBundleConfigMap},
				},
			},
		})
	}

	return volumes
}

func (b *StorageStatefulSetBuilder) buildInitContainer() corev1.Container {
	command, args := b.buildInitContainerArgs()

	container := corev1.Container{
		Name:            "ydb-storage-init-container",
		Image:           b.Spec.Image.Name,
		ImagePullPolicy: *b.Spec.Image.PullPolicyName,
		Command:         command,
		Args:            args,

		SecurityContext: &corev1.SecurityContext{
			RunAsUser: new(int64),
		},

		VolumeMounts: b.buildInitContainerVolumeMounts(),
		Resources:    b.Spec.Resources,
	}

	return container
}

func (b *StorageStatefulSetBuilder) areAnyCertificatesAddedToStore() bool {
	return b.Spec.CaBundle != "" ||
		b.Spec.Service.GRPC.TLSConfiguration.Enabled ||
		b.Spec.Service.Interconnect.TLSConfiguration.Enabled
}

func (b *StorageStatefulSetBuilder) buildInitContainerVolumeMounts() []corev1.VolumeMount {
	volumeMounts := []corev1.VolumeMount{}

	if b.areAnyCertificatesAddedToStore() {
		volumeMounts = append(volumeMounts, corev1.VolumeMount{
			Name:      initMainSharedSourceDirVolumeName,
			MountPath: defaultPathForLocalCerts,
		})

		volumeMounts = append(volumeMounts, corev1.VolumeMount{
			Name:      initMainSharedCertsVolumeName,
			MountPath: systemSslStorePath,
		})
	}

	if b.Spec.CaBundle != "" {
		volumeMounts = append(volumeMounts, corev1.VolumeMount{
			Name:      caBundleVolumeName,
			ReadOnly:  true,
			MountPath: temporaryPathForCertsInInit,
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

	container := corev1.Container{
		Name:            "ydb-storage",
		Image:           b.Spec.Image.Name,
		ImagePullPolicy: *b.Spec.Image.PullPolicyName,
		Command:         command,
		Args:            args,
		LivenessProbe: &corev1.Probe{
			Handler: corev1.Handler{
				TCPSocket: &corev1.TCPSocketAction{
					Port: intstr.FromInt(v1alpha1.GRPCPort),
				},
			},
		},

		SecurityContext: &corev1.SecurityContext{
			Privileged: ptr.Bool(false),
		},

		Ports: []corev1.ContainerPort{{
			Name: "grpc", ContainerPort: v1alpha1.GRPCPort,
		}, {
			Name: "interconnect", ContainerPort: v1alpha1.InterconnectPort,
		}, {
			Name: "status", ContainerPort: v1alpha1.StatusPort,
		}},

		VolumeMounts: b.buildVolumeMounts(),
		Resources:    b.Spec.Resources,
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
			Name:      initMainSharedSourceDirVolumeName,
			MountPath: defaultPathForLocalCerts,
		})

		volumeMounts = append(volumeMounts, corev1.VolumeMount{
			Name:      initMainSharedCertsVolumeName,
			MountPath: systemSslStorePath,
		})
	}

	return volumeMounts
}

func (b *StorageStatefulSetBuilder) buildInitContainerArgs() ([]string, []string) {
	command := []string{"/bin/bash", "-c"}

	arg := ""

	if b.Spec.CaBundle != "" {
		arg += fmt.Sprintf("cp %s/* %s/ &&", temporaryPathForCertsInInit, defaultPathForLocalCerts)
	}

	if b.Spec.Service.GRPC.TLSConfiguration.Enabled {
		arg += fmt.Sprintf("cp /tls/grpc/ca.crt %s/", defaultPathForLocalCerts) // fixme const
	}

	if b.Spec.Service.Interconnect.TLSConfiguration.Enabled {
		arg += fmt.Sprintf("cp /tls/interconnect/ca.crt %s/", defaultPathForLocalCerts) // fixme const
	}

	if arg != "" {
		arg += fmt.Sprintf("%s && %s && %s",
			"apt-get update",
			"apt-get install -y ca-certificates",
			"update-ca-certificates",
		)
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
