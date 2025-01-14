package storage_test

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
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
	"github.com/ydb-platform/ydb-kubernetes-operator/internal/annotations"
	"github.com/ydb-platform/ydb-kubernetes-operator/internal/controllers/storage"
	"github.com/ydb-platform/ydb-kubernetes-operator/internal/labels"
	"github.com/ydb-platform/ydb-kubernetes-operator/internal/resources"
	"github.com/ydb-platform/ydb-kubernetes-operator/internal/test"
	testobjects "github.com/ydb-platform/ydb-kubernetes-operator/tests/test-k8s-objects"
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

	It("Checking field propagation to objects", func() {
		storageSample := testobjects.DefaultStorage(filepath.Join("..", "..", "..", "tests", "data", "storage-mirror-3-dc-config.yaml"))

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

		By("Check volume has been propagated to pods...")
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

		By("Check that configuration checksum annotation propagated to pods...", func() {
			podAnnotations := storageSS.Spec.Template.Annotations

			foundStorage := v1alpha1.Storage{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name:      testobjects.StorageName,
				Namespace: testobjects.YdbNamespace,
			}, &foundStorage)).Should(Succeed())

			foundConfigurationChecksumAnnotation := false
			if podAnnotations[annotations.ConfigurationChecksum] == resources.SHAChecksum(foundStorage.Spec.Configuration) {
				foundConfigurationChecksumAnnotation = true
			}
			Expect(foundConfigurationChecksumAnnotation).To(BeTrue())
		})

		By("Check that args with --label propagated to pods...", func() {
			podContainerArgs := storageSS.Spec.Template.Spec.Containers[0].Args
			var labelArgKey string
			var labelArgValue string
			for idx, arg := range podContainerArgs {
				if arg == "--label" {
					labelArgKey = strings.Split(podContainerArgs[idx+1], "=")[0]
					labelArgValue = strings.Split(podContainerArgs[idx+1], "=")[1]
				}
			}
			Expect(labelArgKey).Should(BeEquivalentTo(v1alpha1.LabelDeploymentKey))
			Expect(labelArgValue).Should(BeEquivalentTo(v1alpha1.LabelDeploymentValueKubernetes))
		})

		By("Check that statefulset podTemplate labels remain immutable...", func() {
			testLabelKey := "ydb-label"
			testLabelValue := "test"
			By("set additional labels to Storage...")
			Eventually(func() error {
				foundStorage := v1alpha1.Storage{}
				Expect(k8sClient.Get(ctx, types.NamespacedName{
					Name:      storageSample.Name,
					Namespace: testobjects.YdbNamespace,
				}, &foundStorage))
				additionalLabels := resources.CopyDict(foundStorage.Spec.AdditionalLabels)
				additionalLabels[testLabelKey] = testLabelValue
				foundStorage.Spec.AdditionalLabels = additionalLabels
				return k8sClient.Update(ctx, &foundStorage)
			}, test.Timeout, test.Interval).ShouldNot(HaveOccurred())

			By("check that additional labels was added...")
			foundStatefulSets := appsv1.StatefulSetList{}
			Eventually(func() error {
				err := k8sClient.List(ctx, &foundStatefulSets,
					client.InNamespace(testobjects.YdbNamespace),
				)
				if err != nil {
					return err
				}
				value := foundStatefulSets.Items[0].Labels[testLabelKey]
				if value != testLabelValue {
					return fmt.Errorf("label value of `%s` in StatefulSet does not equal `%s`. Current labels: %s", testLabelKey, testLabelValue, foundStatefulSets.Items[0].Labels)
				}
				return nil
			}, test.Timeout, test.Interval).ShouldNot(HaveOccurred())

			By("check that StatefulSet selector was not updated...")
			Expect(*foundStatefulSets.Items[0].Spec.Selector).Should(BeEquivalentTo(
				metav1.LabelSelector{
					MatchLabels: map[string]string{
						labels.StatefulsetComponent: storageSample.Name,
					},
				},
			))
		})

		By("Check that additionalPodLabels propagated into podTemplate...", func() {
			testLabelKey := "ydb-pod-label"
			testLabelValue := "test-podTemplate"
			By("set additional pod labels to Storage...")
			Eventually(func() error {
				foundStorage := v1alpha1.Storage{}
				Expect(k8sClient.Get(ctx, types.NamespacedName{
					Name:      storageSample.Name,
					Namespace: testobjects.YdbNamespace,
				}, &foundStorage))
				foundStorage.Spec.AdditionalPodLabels = make(map[string]string)
				foundStorage.Spec.AdditionalPodLabels[testLabelKey] = testLabelValue
				return k8sClient.Update(ctx, &foundStorage)
			}, test.Timeout, test.Interval).ShouldNot(HaveOccurred())

			By("check that additional pod labels was added...")
			foundStatefulSets := appsv1.StatefulSetList{}
			Eventually(func() error {
				err := k8sClient.List(ctx, &foundStatefulSets,
					client.InNamespace(testobjects.YdbNamespace),
				)
				if err != nil {
					return err
				}
				value := foundStatefulSets.Items[0].Spec.Template.Labels[testLabelKey]
				if value != testLabelValue {
					return fmt.Errorf("label value of `%s` in StatefulSet does not equal `%s`. Current labels: %s", testLabelKey, testLabelValue, foundStatefulSets.Items[0].Labels)
				}
				return nil
			}, test.Timeout, test.Interval).ShouldNot(HaveOccurred())
		})

		By("check that delete StatefulSet event was detected...", func() {
			foundStatefulSets := appsv1.StatefulSetList{}
			Expect(k8sClient.List(ctx, &foundStatefulSets, client.InNamespace(testobjects.YdbNamespace))).ShouldNot(HaveOccurred())
			Expect(len(foundStatefulSets.Items)).Should(Equal(1))
			Expect(k8sClient.Delete(ctx, &foundStatefulSets.Items[0])).ShouldNot(HaveOccurred())
			Eventually(func() int {
				Expect(k8sClient.List(ctx, &foundStatefulSets, client.InNamespace(testobjects.YdbNamespace))).ShouldNot(HaveOccurred())
				return len(foundStatefulSets.Items)
			}, test.Timeout, test.Interval).Should(Equal(1))
		})
	})
})
