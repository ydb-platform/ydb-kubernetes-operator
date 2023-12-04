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
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	"github.com/ydb-platform/ydb-kubernetes-operator/api/v1alpha1"
	testobjects "github.com/ydb-platform/ydb-kubernetes-operator/e2e/tests/test-objects"
	"github.com/ydb-platform/ydb-kubernetes-operator/internal/controllers/storage"
	"github.com/ydb-platform/ydb-kubernetes-operator/internal/controllers/storagenodeset"
	"github.com/ydb-platform/ydb-kubernetes-operator/internal/test"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
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
			&storagenodeset.StorageNodeSetReconciler{
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
		storageSample := testobjects.DefaultStorage(filepath.Join("..", "..", "..", "e2e", "tests", "data", "storage-block-4-2-config.yaml"))

		// Test create inline nodeSetSpec in Storage object
		testNodeSetName := "nodeSet"
		for idx := 1; idx <= 4; idx++ {
			storageSample.Spec.NodeSet = append(storageSample.Spec.NodeSet, v1alpha1.StorageNodeSetSpecInline{
				Name:  testNodeSetName + "-" + strconv.Itoa(idx),
				Nodes: 2,
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
				for idxNodeSet := 1; idxNodeSet <= 4; idxNodeSet++ {
					if storageNodeSet.Name == testobjects.StorageName+"-"+testNodeSetName+"-"+strconv.Itoa(idxNodeSet) {
						foundStorageNodeSet[idxNodeSet] = true
						break
					}
				}
			}
			for idxNodeSet := 1; idxNodeSet <= 4; idxNodeSet++ {
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
				for idxNodeSet := 1; idxNodeSet <= 4; idxNodeSet++ {
					if statefulSet.Name == testobjects.StorageName+"-"+testNodeSetName+"-"+strconv.Itoa(idxNodeSet) {
						foundStatefulSet[idxNodeSet] = true
						break
					}
				}
			}
			for idxNodeSet := 1; idxNodeSet <= 4; idxNodeSet++ {
				if !foundStatefulSet[idxNodeSet] {
					return false
				}
			}
			return true
		}, test.Timeout, test.Interval).Should(BeTrue())

		// Test edit inline nodeSetSpec in Storage object
		storageNodeSet := v1alpha1.StorageNodeSet{}
		storageStatefulSet := appsv1.StatefulSet{}
		Expect(k8sClient.Get(ctx,
			types.NamespacedName{
				Name:      testobjects.StorageName + "-" + testNodeSetName + "-" + strconv.Itoa(1),
				Namespace: testobjects.YdbNamespace,
			},
			&storageNodeSet)).Should(Succeed())
		objPatch := storageSample.DeepCopy()
		for idx, storageNodeSet := range objPatch.Spec.NodeSet {
			if storageNodeSet.Name == testobjects.StorageName+"-"+testNodeSetName+"-"+strconv.Itoa(1) {
				objPatch.Spec.NodeSet[idx].Nodes = 4
				break
			}
		}
		nodeSetPatch := client.StrategicMergeFrom(objPatch)
		Expect(k8sClient.Patch(ctx, storageSample, nodeSetPatch)).Should(Succeed())

		// check that StorageNodeSet ".spec.nodes" was changed
		Eventually(func() bool {
			Expect(k8sClient.Get(ctx,
				types.NamespacedName{
					Name:      testobjects.StorageName + "-" + testNodeSetName + "-" + strconv.Itoa(1),
					Namespace: testobjects.YdbNamespace,
				},
				&storageNodeSet)).Should(Succeed())
			return storageNodeSet.Spec.Nodes == 4
		}, test.Timeout, test.Interval).Should(BeTrue())

		// check that StatefulSet ".spec.replicas" was changed
		Eventually(func() bool {
			Expect(k8sClient.Get(ctx,
				types.NamespacedName{
					Name:      testobjects.StorageName + "-" + testNodeSetName + "-" + strconv.Itoa(1),
					Namespace: testobjects.YdbNamespace,
				},
				&storageStatefulSet)).Should(Succeed())
			return *storageStatefulSet.Spec.Replicas == 4
		}, test.Timeout, test.Interval).Should(BeTrue())

		// Test delete inline nodeSetSpec in Storage object
		objPatch = storageSample.DeepCopy()
		for idx, storageNodeSet := range objPatch.Spec.NodeSet {
			if storageNodeSet.Name == testobjects.StorageName+"-"+testNodeSetName+"-"+strconv.Itoa(1) {
				objPatch.Spec.NodeSet = append(objPatch.Spec.NodeSet[:idx], objPatch.Spec.NodeSet[idx+1:]...)
				break
			}
		}
		nodeSetPatch = client.StrategicMergeFrom(objPatch)
		Expect(k8sClient.Patch(ctx, storageSample, nodeSetPatch)).Should(Succeed())

		// check that StorageNodeSet was deleted
		Eventually(func() bool {
			err := k8sClient.Get(ctx,
				types.NamespacedName{
					Name:      testobjects.StorageName + "-" + testNodeSetName + "-" + strconv.Itoa(1),
					Namespace: testobjects.YdbNamespace,
				},
				&storageNodeSet)
			return apierrors.IsNotFound(err)
		}, test.Timeout, test.Interval).Should(BeTrue())

		// check that  StatefulSet was deleted
		Eventually(func() bool {
			err := k8sClient.Get(ctx,
				types.NamespacedName{
					Name:      testobjects.StorageName + "-" + testNodeSetName + "-" + strconv.Itoa(1),
					Namespace: testobjects.YdbNamespace,
				},
				&storageStatefulSet)
			return apierrors.IsNotFound(err)
		}, test.Timeout, test.Interval).Should(BeTrue())
	})
})
