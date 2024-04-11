package storage_test

import (
	"context"
	"fmt"
	"path/filepath"
	"strconv"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	"github.com/ydb-platform/ydb-kubernetes-operator/api/v1alpha1"
	testobjects "github.com/ydb-platform/ydb-kubernetes-operator/e2e/tests/test-objects"
	"github.com/ydb-platform/ydb-kubernetes-operator/internal/annotations"
	"github.com/ydb-platform/ydb-kubernetes-operator/internal/controllers/storage"
	"github.com/ydb-platform/ydb-kubernetes-operator/internal/labels"
	"github.com/ydb-platform/ydb-kubernetes-operator/internal/resources"
	"github.com/ydb-platform/ydb-kubernetes-operator/internal/test"
)

var (
	k8sClient client.Client
	ctx       context.Context
)

func TestAPIs(t *testing.T) {
	RegisterFailHandler(Fail)

	test.SetupK8STestManager(&ctx, &k8sClient, func(mgr *manager.Manager) []test.Reconciler {
		return []test.Reconciler{
			&storage.Reconciler{
				Client: k8sClient,
				Scheme: (*mgr).GetScheme(),
			},
		}
	})

	RunSpecs(t, "Storage controller medium tests suite")
}

var _ = Describe("Storage controller medium tests", func() {
	var namespace corev1.Namespace

	BeforeEach(func() {
		namespace = corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: testobjects.YdbNamespace,
			},
		}
		Expect(k8sClient.Create(ctx, &namespace)).Should(Succeed())
	})

	AfterEach(func() {
		Expect(k8sClient.Delete(ctx, &namespace)).Should(Succeed())
	})

	It("Check volume has been propagated to pods", func() {
		storageSample := testobjects.DefaultStorage(filepath.Join("..", "..", "..", "e2e", "tests", "data", "storage-block-4-2-config.yaml"))

		tmpFilesDir := "/tmp/mounted_volume"
		testVolumeName := "sample-volume"
		testVolumeMountPath := fmt.Sprintf("%v/volume", tmpFilesDir)

		HostPathDirectoryType := corev1.HostPathDirectory

		storageSample.Spec.Volumes = append(storageSample.Spec.Volumes, &corev1.Volume{
			Name: testVolumeName,
			VolumeSource: corev1.VolumeSource{
				HostPath: &corev1.HostPathVolumeSource{
					Path: testVolumeMountPath,
					Type: &HostPathDirectoryType,
				},
			},
		})

		Expect(k8sClient.Create(ctx, storageSample)).Should(Succeed())

		storageStatefulSets := appsv1.StatefulSetList{}
		Eventually(func() bool {
			Expect(k8sClient.List(ctx, &storageStatefulSets, client.InNamespace(
				testobjects.YdbNamespace,
			))).Should(Succeed())
			foundStatefulSet := false
			for _, statefulSet := range storageStatefulSets.Items {
				if statefulSet.Name == testobjects.StorageName {
					foundStatefulSet = true
					break
				}
			}
			return foundStatefulSet
		}, test.Timeout, test.Interval).Should(BeTrue())

		storageSS := storageStatefulSets.Items[0]
		volumes := storageSS.Spec.Template.Spec.Volumes
		// Pod Template always has `ydb-config` mounted as a volume, plus in
		// this test it also has our test volume. So two in total:
		Expect(len(volumes)).To(Equal(1 + 1))

		foundVolume := false
		for _, volume := range volumes {
			if volume.Name == testVolumeName {
				foundVolume = true
				Expect(volume.VolumeSource.HostPath.Path).To(Equal(testVolumeMountPath))
			}
		}
		Expect(foundVolume).To(BeTrue())
	})

	It("Check that label and annotation propagated to pods", func() {
		storageSample := testobjects.DefaultStorage(filepath.Join("..", "..", "..", "e2e", "tests", "data", "storage-block-4-2-config.yaml"))

		Expect(k8sClient.Create(ctx, storageSample)).Should(Succeed())

		foundStorage := v1alpha1.Storage{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{
			Name:      testobjects.StorageName,
			Namespace: testobjects.YdbNamespace,
		}, &foundStorage)).Should(Succeed())

		storagePods := corev1.PodList{}
		Eventually(func() bool {
			Expect(k8sClient.List(ctx, &storagePods,
				client.InNamespace(testobjects.YdbNamespace),
				client.MatchingLabels{
					labels.InstanceKey:  testobjects.StorageName,
					labels.ComponentKey: labels.StorageComponent,
				}),
			).Should(Succeed())

			foundStorageGenerationLabel := false
			for _, storagePods := range storagePods.Items {
				if storagePods.Labels[labels.StorageGeneration] == strconv.FormatInt(foundStorage.ObjectMeta.Generation, 10) {
					foundStorageGenerationLabel = true
				} else {
					foundStorageGenerationLabel = false
					break
				}
			}

			foundConfigurationChecksumAnnotation := false
			for _, storagePods := range storagePods.Items {
				if storagePods.Annotations[annotations.ConfigurationChecksum] == resources.GetConfigurationChecksum(foundStorage.Spec.Configuration) {
					foundConfigurationChecksumAnnotation = true
				} else {
					foundConfigurationChecksumAnnotation = false
					break
				}
			}

			return foundStorageGenerationLabel && foundConfigurationChecksumAnnotation
		}, test.Timeout, test.Interval).Should(BeTrue())
	})
})
