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
			"%s-interconnect.%s.svc.cluster.local",
			b.Name,
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
			TopologySpreadConstraints: b.buildTopologySpreadConstraints(),
		},
	}
	if b.Spec.Image.PullSecret != nil {
		podTemplate.Spec.ImagePullSecrets = []corev1.LocalObjectReference{{Name: *b.Spec.Image.PullSecret}}
	}
	return podTemplate
}

func (b *StorageStatefulSetBuilder) buildTopologySpreadConstraints() []corev1.TopologySpreadConstraint {
	if b.Spec.Erasure == v1alpha1.ErasureBlock42 {
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
	if b.Spec.ClusterConfig != "" {
		configMapName = b.Spec.ClusterConfig
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
		volumes = append(volumes, buildTLSVolume("grpc-tls-volume", b.Spec.Service.GRPC.TLSConfiguration))
	}

	if b.Spec.Service.Interconnect.TLSConfiguration.Enabled {
		volumes = append(volumes, buildTLSVolume("interconnect-tls-volume", b.Spec.Service.Interconnect.TLSConfiguration)) // fixme const
	}

	return volumes
}

func (b *StorageStatefulSetBuilder) buildContainer() corev1.Container {
	command, args := b.buildContainerArgs()

	container := corev1.Container{
		Name:    "ydb-storage",
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

	var volumeDeviceList []corev1.VolumeDevice
	for i := range b.Spec.DataStore {
		volumeDeviceList = append(
			volumeDeviceList,
			corev1.VolumeDevice{
				Name:       b.GeneratePVCName(i),
				DevicePath: b.GenerateDeviceName(i),
			},
		)
	}
	container.VolumeDevices = volumeDeviceList

	return container
}

func (b *StorageStatefulSetBuilder) buildVolumeMounts() []corev1.VolumeMount {
	volumeMounts := []corev1.VolumeMount{
		{
			Name:      configVolumeName,
			ReadOnly:  true,
			MountPath: "/opt/kikimr/cfg",
		},
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

func (b *StorageStatefulSetBuilder) buildContainerArgs() ([]string, []string) {
	command := []string{"/opt/kikimr/bin/start.sh"}
	var args []string

	if b.Spec.ClusterConfig != "" {
		command = []string{"/bin/bash"}
		args = append(args, "-c", "source /opt/kikimr/cfg/kikimr.cfg && exec /opt/kikimr/bin/kikimr ${kikimr_arg}")
	}

	if b.Spec.Service.Interconnect.TLSConfiguration.Enabled {
		args = append(args,
			"--ca",
			"/tls/interconnect/ca.crt",
			"--cert",
			"/tls/interconnect/tls.crt",
			"--key",
			"/tls/interconnect/tls.key",
		)
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
