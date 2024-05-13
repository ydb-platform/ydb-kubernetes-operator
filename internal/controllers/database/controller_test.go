package database_test

import (
	"context"
	"errors"
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
	testobjects "github.com/ydb-platform/ydb-kubernetes-operator/e2e/tests/test-objects"
	. "github.com/ydb-platform/ydb-kubernetes-operator/internal/controllers/constants"
	"github.com/ydb-platform/ydb-kubernetes-operator/internal/controllers/database"
	"github.com/ydb-platform/ydb-kubernetes-operator/internal/controllers/storage"
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
			&database.Reconciler{
				Client: k8sClient,
				Scheme: (*mgr).GetScheme(),
			},
		}
	})

	RunSpecs(t, "Database controller medium tests suite")
}

var _ = Describe("Database controller medium tests", func() {
	var namespace corev1.Namespace
	var storageSample v1alpha1.Storage

	BeforeEach(func() {
		namespace = corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: testobjects.YdbNamespace,
			},
		}
		Expect(k8sClient.Create(ctx, &namespace)).Should(Succeed())
		storageSample = *testobjects.DefaultStorage(filepath.Join("..", "..", "..", "e2e", "tests", "data", "storage-block-4-2-config.yaml"))
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

		By("set status Ready to Storage...")
		Eventually(func() error {
			foundStorage := v1alpha1.Storage{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name:      storageSample.Name,
				Namespace: testobjects.YdbNamespace,
			}, &foundStorage))
			foundStorage.Status.State = StorageReady
			return k8sClient.Status().Update(ctx, &foundStorage)
		}, test.Timeout, test.Interval).ShouldNot(HaveOccurred())
	})

	AfterEach(func() {
		Expect(k8sClient.Delete(ctx, &storageSample)).Should(Succeed())
		Expect(k8sClient.Delete(ctx, &namespace)).Should(Succeed())
	})

	It("Checking field propagation to objects", func() {
		By("Check that Shared Database was created...")
		databaseSample := *testobjects.DefaultDatabase()
		databaseSample.Spec.SharedResources = &v1alpha1.DatabaseResources{
			StorageUnits: []v1alpha1.StorageUnit{
				{
					UnitKind: "ssd",
					Count:    1,
				},
			},
		}
		Expect(k8sClient.Create(ctx, &databaseSample)).Should(Succeed())

		By("Check that StatefulSet was created...")
		databaseStatefulSet := appsv1.StatefulSet{}
		foundStatefulSets := appsv1.StatefulSetList{}
		Eventually(func() error {
			err := k8sClient.List(ctx, &foundStatefulSets, client.InNamespace(
				testobjects.YdbNamespace))
			if err != nil {
				return err
			}
			for idx, statefulSet := range foundStatefulSets.Items {
				if statefulSet.Name == testobjects.DatabaseName {
					databaseStatefulSet = foundStatefulSets.Items[idx]
					return nil
				}
			}
			return errors.New("failed to find StatefulSet")
		}, test.Timeout, test.Interval).ShouldNot(HaveOccurred())

		By("Check that args `--label` propagated to pods...", func() {
			podContainerArgs := databaseStatefulSet.Spec.Template.Spec.Containers[0].Args
			var labelArgKey string
			var labelArgValue string
			for idx, arg := range podContainerArgs {
				if arg == "--label" {
					labelArgKey = strings.Split(podContainerArgs[idx+1], "=")[0]
					labelArgValue = strings.Split(podContainerArgs[idx+1], "=")[1]
					if labelArgKey == v1alpha1.LabelDeploymentKey {
						Expect(labelArgValue).Should(BeEquivalentTo(v1alpha1.LabelDeploymentValueKubernetes))
					}
					if labelArgKey == v1alpha1.LabelSharedDatabaseKey {
						Expect(labelArgValue).Should(BeEquivalentTo(v1alpha1.LabelSharedDatabaseValueTrue))
					}
				}
			}
		})
	})
})
