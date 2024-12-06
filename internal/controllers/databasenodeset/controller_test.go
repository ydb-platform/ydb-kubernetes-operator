package databasenodeset_test

import (
	"context"
	"path/filepath"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	"github.com/ydb-platform/ydb-kubernetes-operator/api/v1alpha1"
	. "github.com/ydb-platform/ydb-kubernetes-operator/internal/controllers/constants"
	"github.com/ydb-platform/ydb-kubernetes-operator/internal/controllers/database"
	"github.com/ydb-platform/ydb-kubernetes-operator/internal/controllers/databasenodeset"
	"github.com/ydb-platform/ydb-kubernetes-operator/internal/controllers/storage"
	"github.com/ydb-platform/ydb-kubernetes-operator/internal/test"
	testobjects "github.com/ydb-platform/ydb-kubernetes-operator/tests/test-k8s-objects"
)

const (
	testDatabaseLabel = "database-label"
	testNodeSetName   = "nodeset"
	testNodeSetLabel  = "nodeset-label"
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
			&database.Reconciler{
				Client: k8sClient,
				Scheme: (*mgr).GetScheme(),
			},
			&databasenodeset.Reconciler{
				Client: k8sClient,
				Scheme: (*mgr).GetScheme(),
			},
		}
	})

	RunSpecs(t, "DatabaseNodeSet controller medium tests suite")
}

var _ = Describe("DatabaseNodeSet controller medium tests", func() {
	var namespace corev1.Namespace
	var storageSample v1alpha1.Storage
	var databaseSample v1alpha1.Database

	BeforeEach(func() {
		namespace = corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: testobjects.YdbNamespace,
			},
		}
		Expect(k8sClient.Create(ctx, &namespace)).Should(Succeed())

		storageSample = *testobjects.DefaultStorage(filepath.Join("..", "..", "..", "tests", "data", "storage-mirror-3-dc-config.yaml"))
		Expect(k8sClient.Create(ctx, &storageSample)).Should(Succeed())

		By("checking that Storage created on local cluster...")
		foundStorage := v1alpha1.Storage{}
		Eventually(func() bool {
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name:      storageSample.Name,
				Namespace: testobjects.YdbNamespace,
			}, &foundStorage))
			return foundStorage.Status.State == StorageInitializing
		}, test.Timeout, test.Interval).Should(BeTrue())

		By("set condition Initialized to Storage...")
		Eventually(func() error {
			foundStorage := v1alpha1.Storage{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name:      storageSample.Name,
				Namespace: testobjects.YdbNamespace,
			}, &foundStorage))
			meta.SetStatusCondition(&foundStorage.Status.Conditions, metav1.Condition{
				Type:   StorageInitializedCondition,
				Status: metav1.ConditionTrue,
				Reason: ReasonCompleted,
			})
			return k8sClient.Status().Update(ctx, &foundStorage)
		}, test.Timeout, test.Interval).ShouldNot(HaveOccurred())

		databaseSample = *testobjects.DefaultDatabase()
		databaseSample.Spec.NodeSets = append(databaseSample.Spec.NodeSets, v1alpha1.DatabaseNodeSetSpecInline{
			Name: testNodeSetName,
			DatabaseNodeSpec: v1alpha1.DatabaseNodeSpec{
				Nodes: 1,
			},
		})

		By("issuing create Database commands...")
		Expect(k8sClient.Create(ctx, &databaseSample)).Should(Succeed())
		By("checking that Database created on local cluster...")
		foundDatabase := v1alpha1.Database{}
		Eventually(func() bool {
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name:      databaseSample.Name,
				Namespace: testobjects.YdbNamespace,
			}, &foundDatabase))
			return foundDatabase.Status.State == DatabaseInitializing
		}, test.Timeout, test.Interval).Should(BeTrue())

		By("checking that DatabaseNodeSet created on local cluster...")
		Eventually(func() error {
			foundDatabaseNodeSet := &v1alpha1.DatabaseNodeSet{}
			return k8sClient.Get(ctx, types.NamespacedName{
				Name:      databaseSample.Name + "-" + testNodeSetName,
				Namespace: testobjects.YdbNamespace,
			}, foundDatabaseNodeSet)
		}, test.Timeout, test.Interval).ShouldNot(HaveOccurred())
	})

	AfterEach(func() {
		Expect(k8sClient.Delete(ctx, &databaseSample)).Should(Succeed())
		Expect(k8sClient.Delete(ctx, &storageSample)).Should(Succeed())
		Expect(k8sClient.Delete(ctx, &namespace)).Should(Succeed())
	})

	It("Check labels and annotations propagation to NodeSet", func() {
		// Test update inline nodeSetSpec in Database object
		testNodeSetName := "nodeset"
		By("checking that Database updated on local cluster...")
		Eventually(func() error {
			foundDatabase := &v1alpha1.Database{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name:      databaseSample.Name,
				Namespace: testobjects.YdbNamespace,
			}, foundDatabase))

			foundDatabase.Labels = map[string]string{
				testDatabaseLabel: "true",
			}

			foundDatabase.Annotations = map[string]string{
				v1alpha1.AnnotationUpdateStrategyOnDelete: "true",
			}

			foundDatabase.Spec.NodeSets = append(foundDatabase.Spec.NodeSets, v1alpha1.DatabaseNodeSetSpecInline{
				Name: testNodeSetName + "-labeled",
				Labels: map[string]string{
					testNodeSetLabel: "true",
				},
				DatabaseNodeSpec: v1alpha1.DatabaseNodeSpec{
					Nodes: 1,
				},
			})

			foundDatabase.Spec.NodeSets = append(foundDatabase.Spec.NodeSets, v1alpha1.DatabaseNodeSetSpecInline{
				Name: testNodeSetName + "-annotated",
				Annotations: map[string]string{
					v1alpha1.AnnotationDataCenter: "envtest",
				},
				DatabaseNodeSpec: v1alpha1.DatabaseNodeSpec{
					Nodes: 1,
				},
			})

			return k8sClient.Update(ctx, foundDatabase)
		}, test.Timeout, test.Interval).ShouldNot(HaveOccurred())

		// check that DatabaseNodeSets was created
		databaseNodeSets := v1alpha1.DatabaseNodeSetList{}
		Eventually(func() bool {
			Expect(k8sClient.List(ctx, &databaseNodeSets, client.InNamespace(
				testobjects.YdbNamespace,
			))).Should(Succeed())
			foundDatabaseNodeSet := make(map[string]bool)
			for _, databaseNodeSet := range databaseNodeSets.Items {
				if databaseNodeSet.Name == testobjects.DatabaseName+"-"+testNodeSetName+"-labeled" {
					foundDatabaseNodeSet["labeled"] = true
					break
				}
			}
			for _, databaseNodeSet := range databaseNodeSets.Items {
				if databaseNodeSet.Name == testobjects.DatabaseName+"-"+testNodeSetName+"-annotated" {
					foundDatabaseNodeSet["annotated"] = true
					break
				}
			}

			return foundDatabaseNodeSet["labeled"] && foundDatabaseNodeSet["annotated"]
		}, test.Timeout, test.Interval).Should(BeTrue())

		// check that StatefulSets was created
		databaseStatefulSets := appsv1.StatefulSetList{}
		Eventually(func() bool {
			Expect(k8sClient.List(ctx, &databaseStatefulSets, client.InNamespace(
				testobjects.YdbNamespace,
			))).Should(Succeed())
			foundStatefulSet := make(map[string]bool)
			for _, statefulSet := range databaseStatefulSets.Items {
				if statefulSet.Name == testobjects.DatabaseName+"-"+testNodeSetName+"-labeled" {
					foundStatefulSet["labeled"] = true
					break
				}
			}

			for _, statefulSet := range databaseStatefulSets.Items {
				if statefulSet.Name == testobjects.DatabaseName+"-"+testNodeSetName+"-annotated" {
					foundStatefulSet["annotated"] = true
					break
				}
			}

			return foundStatefulSet["labeled"] && foundStatefulSet["annotated"]
		}, test.Timeout, test.Interval).Should(BeTrue())

		// check that DatabaseNodeSet was labeled
		labeledDatabaseNodeSet := v1alpha1.DatabaseNodeSet{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{
			Name:      databaseSample.Name + "-" + testNodeSetName + "-labeled",
			Namespace: testobjects.YdbNamespace,
		}, &labeledDatabaseNodeSet)).Should(Succeed())
		Expect(labeledDatabaseNodeSet.Labels[testNodeSetLabel]).Should(Equal("true"))

		// check that DatabaseNodeSet was annotated and args was appended to pods
		annotatedDatabaseNodeSet := v1alpha1.DatabaseNodeSet{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{
			Name:      databaseSample.Name + "-" + testNodeSetName + "-annotated",
			Namespace: testobjects.YdbNamespace,
		}, &annotatedDatabaseNodeSet)).Should(Succeed())
		Expect(annotatedDatabaseNodeSet.Annotations[v1alpha1.AnnotationUpdateStrategyOnDelete]).Should(Equal("true"))
		Expect(annotatedDatabaseNodeSet.Annotations[v1alpha1.AnnotationDataCenter]).Should(Equal("envtest"))

		annotatedDatabaseStatefulSet := appsv1.StatefulSet{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{
			Name:      databaseSample.Name + "-" + testNodeSetName + "-annotated",
			Namespace: testobjects.YdbNamespace,
		}, &annotatedDatabaseStatefulSet)).Should(Succeed())
		Expect(annotatedDatabaseStatefulSet.Spec.UpdateStrategy.Type).Should(Equal(appsv1.OnDeleteStatefulSetStrategyType))
		var dataCenterArgExist bool
		for _, item := range annotatedDatabaseStatefulSet.Spec.Template.Spec.Containers[0].Args {
			if item == "--data-center" {
				dataCenterArgExist = true
			}
		}
		Expect(dataCenterArgExist).Should(BeTrue())
	})
})
