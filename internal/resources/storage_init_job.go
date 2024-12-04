package resources

import (
	"errors"
	"fmt"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	api "github.com/ydb-platform/ydb-kubernetes-operator/api/v1alpha1"
	"github.com/ydb-platform/ydb-kubernetes-operator/internal/ptr"
)

type StorageInitJobBuilder struct {
	*api.Storage

	Name string

	Labels      map[string]string
	Annotations map[string]string
}

func (b *StorageInitJobBuilder) Build(obj client.Object) error {
	job, ok := obj.(*batchv1.Job)
	if !ok {
		return errors.New("failed to cast to Job object")
	}

	if job.ObjectMeta.Name == "" {
		job.ObjectMeta.Name = b.Name
	}
	job.ObjectMeta.Namespace = b.GetNamespace()
	job.ObjectMeta.Labels = b.Labels
	job.ObjectMeta.Annotations = b.Annotations

	job.Spec = batchv1.JobSpec{
		Parallelism:           ptr.Int32(1),
		Completions:           ptr.Int32(1),
		ActiveDeadlineSeconds: ptr.Int64(300),
		BackoffLimit:          ptr.Int32(6),
		Template:              b.buildInitJobPodTemplateSpec(),
	}

	return nil
}

func (b *StorageInitJobBuilder) Placeholder(cr client.Object) client.Object {
	return &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      b.Name,
			Namespace: cr.GetNamespace(),
		},
	}
}

func (b *StorageInitJobBuilder) buildInitJobPodTemplateSpec() corev1.PodTemplateSpec {
	domain := api.DefaultDomainName
	if dnsAnnotation, ok := b.GetAnnotations()[api.DNSDomainAnnotation]; ok {
		domain = dnsAnnotation
	}
	dnsConfigSearches := []string{
		fmt.Sprintf(api.InterconnectServiceFQDNFormat, b.Storage.Name, b.GetNamespace(), domain),
	}
	podTemplate := corev1.PodTemplateSpec{
		ObjectMeta: metav1.ObjectMeta{
			Labels:      b.Labels,
			Annotations: b.Annotations,
		},
		Spec: corev1.PodSpec{
			Containers:    []corev1.Container{b.buildInitJobContainer()},
			Volumes:       b.buildInitJobVolumes(),
			RestartPolicy: corev1.RestartPolicyNever,
			DNSConfig: &corev1.PodDNSConfig{
				Searches: dnsConfigSearches,
			},
		},
	}

	if b.Spec.InitJob != nil {
		if b.Spec.InitJob.NodeSelector != nil {
			podTemplate.Spec.NodeSelector = b.Spec.InitJob.NodeSelector
		}

		if b.Spec.InitJob.Affinity != nil {
			podTemplate.Spec.Affinity = b.Spec.InitJob.Affinity
		}

		if b.Spec.InitJob.Tolerations != nil {
			podTemplate.Spec.Tolerations = b.Spec.InitJob.Tolerations
		}
	}

	// InitContainer only needed for CaBundle manipulation for now,
	// may be probably used for other stuff later
	if b.AnyCertificatesAdded() {
		podTemplate.Spec.InitContainers = append(
			[]corev1.Container{b.buildCaStorePatchingInitContainer()},
			b.Spec.InitContainers...,
		)
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

func (b *StorageInitJobBuilder) buildInitJobVolumes() []corev1.Volume {
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

	if b.Spec.OperatorConnection != nil {
		secretName := fmt.Sprintf(OperatorTokenSecretNameFormat, b.Storage.Name)
		volumes = append(volumes,
			corev1.Volume{
				Name: operatorTokenVolumeName,
				VolumeSource: corev1.VolumeSource{
					Secret: &corev1.SecretVolumeSource{
						SecretName: secretName,
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

func (b *StorageInitJobBuilder) buildInitJobContainer() corev1.Container { // todo add init container for sparse files?
	imagePullPolicy := corev1.PullIfNotPresent
	if b.Spec.Image.PullPolicyName != nil {
		imagePullPolicy = *b.Spec.Image.PullPolicyName
	}

	command, args := b.buildBlobStorageInitCommandArgs()

	container := corev1.Container{
		Name:            "ydb-init-blobstorage",
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

		VolumeMounts: b.buildJobVolumeMounts(),
		Resources:    corev1.ResourceRequirements{},
	}

	if b.Spec.InitJob != nil {
		if b.Spec.InitJob.Resources != nil {
			container.Resources = *b.Spec.InitJob.Resources
		}
	}

	return container
}

func (b *StorageInitJobBuilder) buildJobVolumeMounts() []corev1.VolumeMount {
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
			MountPath: grpcTLSVolumeMountPath,
		})
	}

	if b.Spec.OperatorConnection != nil {
		secretName := fmt.Sprintf(OperatorTokenSecretNameFormat, b.Storage.Name)
		volumeMounts = append(volumeMounts, corev1.VolumeMount{
			Name:      operatorTokenVolumeName,
			ReadOnly:  true,
			MountPath: fmt.Sprintf("%s/%s", wellKnownDirForAdditionalSecrets, secretName),
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

	return volumeMounts
}

func (b *StorageInitJobBuilder) buildCaStorePatchingInitContainer() corev1.Container {
	command, args := buildCAStorePatchingCommandArgs(
		b.Spec.CABundle,
		b.Spec.Service.GRPC,
		b.Spec.Service.Interconnect,
		api.StatusService{TLSConfiguration: &api.TLSConfiguration{Enabled: false}},
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
		Resources:    corev1.ResourceRequirements{},
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

func (b *StorageInitJobBuilder) buildCaStorePatchingInitContainerVolumeMounts() []corev1.VolumeMount {
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

	return volumeMounts
}

func (b *StorageInitJobBuilder) buildBlobStorageInitCommandArgs() ([]string, []string) {
	command := []string{
		fmt.Sprintf("%s/%s", api.BinariesDir, api.DaemonBinaryName),
	}

	args := []string{}
	if b.Storage.Spec.OperatorConnection != nil {
		secretName := fmt.Sprintf(OperatorTokenSecretNameFormat, b.Storage.Name)
		args = append(
			args,
			"-f",
			fmt.Sprintf("%s/%s/%s", wellKnownDirForAdditionalSecrets, secretName, wellKnownNameForOperatorToken),
		)
	}

	endpoint := b.Storage.GetStorageEndpointWithProto()
	args = append(
		args,
		"-s",
		endpoint,
	)

	args = append(
		args,
		"admin", "blobstorage", "config", "init", "--yaml-file",
		fmt.Sprintf("%s/%s", api.ConfigDir, api.ConfigFileName),
	)

	return command, args
}
