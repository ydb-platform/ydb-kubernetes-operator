package storage_test

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	testobjects "github.com/ydb-platform/ydb-kubernetes-operator/e2e/tests/test-objects"
	"github.com/ydb-platform/ydb-kubernetes-operator/internal/controllers/storage"
	"github.com/ydb-platform/ydb-kubernetes-operator/internal/test"
)

var (
	k8sClient client.Client
	ctx       context.Context
)

const (
	Timeout  = time.Second * 600
	Interval = time.Second * 5
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
		}, Timeout, Interval).Should(BeTrue())

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
})
