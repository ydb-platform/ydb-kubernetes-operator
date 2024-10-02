package storagenodeset_test

import (
	"context"
	"path/filepath"
	"strconv"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	"github.com/ydb-platform/ydb-kubernetes-operator/api/v1alpha1"
	testobjects "github.com/ydb-platform/ydb-kubernetes-operator/e2e/tests/test-objects"
	"github.com/ydb-platform/ydb-kubernetes-operator/internal/controllers/storage"
	"github.com/ydb-platform/ydb-kubernetes-operator/internal/controllers/storagenodeset"
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
			&storagenodeset.Reconciler{
				Client: k8sClient,
				Scheme: (*mgr).GetScheme(),
			},
		}
	})

	RunSpecs(t, "StorageNodeSet controller medium tests suite")
}

var _ = Describe("StorageNodeSet controller medium tests", func() {
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

	It("Check controller operation through nodeSetSpec inline spec in Storage object", func() {
		storageSample := testobjects.DefaultStorage(filepath.Join("..", "..", "..", "e2e", "tests", "data", "storage-mirror-3-dc-config.yaml"))

		// Test create inline nodeSetSpec in Storage object
		testNodeSetName := "nodeset"
		storageNodeSetAmount := 3
		for idx := 1; idx <= storageNodeSetAmount; idx++ {
			storageSample.Spec.NodeSets = append(storageSample.Spec.NodeSets, v1alpha1.StorageNodeSetSpecInline{
				Name: testNodeSetName + "-" + strconv.Itoa(idx),
				StorageNodeSpec: v1alpha1.StorageNodeSpec{
					Nodes: 1,
				},
			})
		}
		Expect(k8sClient.Create(ctx, storageSample)).Should(Succeed())

		// check that StorageNodeSets was created
		storageNodeSets := v1alpha1.StorageNodeSetList{}
		Eventually(func() bool {
			Expect(k8sClient.List(ctx, &storageNodeSets, client.InNamespace(
				testobjects.YdbNamespace,
			))).Should(Succeed())
			foundStorageNodeSet := make(map[int]bool)
			for _, storageNodeSet := range storageNodeSets.Items {
				for idxNodeSet := 1; idxNodeSet <= storageNodeSetAmount; idxNodeSet++ {
					if storageNodeSet.Name == testobjects.StorageName+"-"+testNodeSetName+"-"+strconv.Itoa(idxNodeSet) {
						foundStorageNodeSet[idxNodeSet] = true
						break
					}
				}
			}
			for idxNodeSet := 1; idxNodeSet <= storageNodeSetAmount; idxNodeSet++ {
				if !foundStorageNodeSet[idxNodeSet] {
					return false
				}
			}
			return true
		}, test.Timeout, test.Interval).Should(BeTrue())

		// check that StatefulSets was created
		storageStatefulSets := appsv1.StatefulSetList{}
		Eventually(func() bool {
			Expect(k8sClient.List(ctx, &storageStatefulSets, client.InNamespace(
				testobjects.YdbNamespace,
			))).Should(Succeed())
			foundStatefulSet := make(map[int]bool)
			for _, statefulSet := range storageStatefulSets.Items {
				for idxNodeSet := 1; idxNodeSet <= storageNodeSetAmount; idxNodeSet++ {
					if statefulSet.Name == testobjects.StorageName+"-"+testNodeSetName+"-"+strconv.Itoa(idxNodeSet) {
						foundStatefulSet[idxNodeSet] = true
						break
					}
				}
			}
			for idxNodeSet := 1; idxNodeSet <= storageNodeSetAmount; idxNodeSet++ {
				if !foundStatefulSet[idxNodeSet] {
					return false
				}
			}
			return true
		}, test.Timeout, test.Interval).Should(BeTrue())
	})
})
