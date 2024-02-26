package resources

import (
	"errors"
	"fmt"

	"github.com/ydb-platform/ydb-kubernetes-operator/api/v1alpha1"
	api "github.com/ydb-platform/ydb-kubernetes-operator/api/v1alpha1"
	"github.com/ydb-platform/ydb-kubernetes-operator/internal/labels"
	"github.com/ydb-platform/ydb-kubernetes-operator/internal/ptr"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
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
		BackoffLimit:          ptr.Int32(10),
		Suspend:               ptr.Bool(true),
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

func GetInitJobBuilder(storage *api.Storage) ResourceBuilder {
	jobName := fmt.Sprintf(InitJobNameFormat, storage.Name)

	jobLabels := labels.Common(jobName, make(map[string]string))
	jobAnnotations := make(map[string]string)

	if storage.Spec.InitJob != nil {
		jobLabels.Merge(storage.Spec.InitJob.AdditionalLabels)
		jobAnnotations = CopyDict(storage.Spec.InitJob.AdditionalAnnotations)
	}

	return &StorageInitJobBuilder{
		Storage: storage,

		Name: jobName,

		Labels:      jobLabels,
		Annotations: jobAnnotations,
	}
}

func (b *StorageInitJobBuilder) buildInitJobPodTemplateSpec() corev1.PodTemplateSpec {
	podTemplate := corev1.PodTemplateSpec{
		ObjectMeta: metav1.ObjectMeta{
			Labels:      b.Labels,
			Annotations: b.Annotations,
		},
		Spec: corev1.PodSpec{
			Containers:    []corev1.Container{b.buildInitJobContainer()},
			Volumes:       b.buildInitJobVolumes(),
			RestartPolicy: corev1.RestartPolicyOnFailure,
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
	if b.AreAnyCertificatesAddedToStore() {
		podTemplate.Spec.InitContainers = append(
			[]corev1.Container{b.buildCaStorePatchingInitContainer()},
			b.Spec.InitContainers...,
		)
	}

	if b.Spec.Image.PullSecret != nil {
		podTemplate.Spec.ImagePullSecrets = []corev1.LocalObjectReference{{Name: *b.Spec.Image.PullSecret}}
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
		authTokenSecretName := fmt.Sprintf(OperatorTokenSecretNameFromat, b.Storage.Name)
		volumes = append(volumes,
			corev1.Volume{
				Name: operatorTokenVolumeName,
				VolumeSource: corev1.VolumeSource{
					Secret: &corev1.SecretVolumeSource{
						SecretName: authTokenSecretName,
					},
				},
			})
	}

	if b.AreAnyCertificatesAddedToStore() {
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

	var authEnabled bool
	if b.Spec.OperatorConnection != nil {
		authEnabled = true
	}

	command, args := b.BuildBlobStorageInitCommandArgs(authEnabled)

	container := corev1.Container{
		Name:            "ydb-init-blobstorage",
		Image:           b.Spec.Image.Name,
		ImagePullPolicy: imagePullPolicy,
		Command:         command,
		Args:            args,

		VolumeMounts: b.buildJobVolumeMounts(),
		Resources:    *b.Spec.InitJob.Resources,
	}

	return container
}

func (b *StorageInitJobBuilder) buildJobVolumeMounts() []corev1.VolumeMount {
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
			MountPath: fmt.Sprintf("%s/%s", v1alpha1.CustomCertsDir, v1alpha1.GRPCCertsDirName),
		})
	}

	if b.Spec.OperatorConnection != nil {
		volumeMounts = append(volumeMounts, corev1.VolumeMount{
			Name:      operatorTokenVolumeName,
			ReadOnly:  true,
			MountPath: v1alpha1.OperatorTokenFilePath,
			SubPath:   v1alpha1.OperatorTokenFileName,
		})
	}

	if b.AreAnyCertificatesAddedToStore() {
		volumeMounts = append(volumeMounts, corev1.VolumeMount{
			Name:      localCertsVolumeName,
			MountPath: v1alpha1.LocalCertsDir,
		})

		volumeMounts = append(volumeMounts, corev1.VolumeMount{
			Name:      systemCertsVolumeName,
			MountPath: v1alpha1.SystemCertsDir,
		})
	}

	return volumeMounts
}

func (b *StorageInitJobBuilder) buildCaStorePatchingInitContainer() corev1.Container {
	command, args := b.BuildCAStorePatchingCommandArgs()
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
				Name:  v1alpha1.CABundleEnvName,
				Value: b.Spec.CABundle,
			},
		}
	}
	return container
}

func (b *StorageInitJobBuilder) buildCaStorePatchingInitContainerVolumeMounts() []corev1.VolumeMount {
	volumeMounts := []corev1.VolumeMount{}

	if b.AreAnyCertificatesAddedToStore() {
		volumeMounts = append(volumeMounts, corev1.VolumeMount{
			Name:      localCertsVolumeName,
			MountPath: v1alpha1.LocalCertsDir,
		})

		volumeMounts = append(volumeMounts, corev1.VolumeMount{
			Name:      systemCertsVolumeName,
			MountPath: v1alpha1.SystemCertsDir,
		})
	}

	if b.Spec.Service.GRPC.TLSConfiguration.Enabled {
		volumeMounts = append(volumeMounts, corev1.VolumeMount{
			Name:      grpcTLSVolumeName,
			ReadOnly:  true,
			MountPath: fmt.Sprintf("%s/%s", v1alpha1.CustomCertsDir, v1alpha1.GRPCCertsDirName),
		})
	}

	return volumeMounts
}
