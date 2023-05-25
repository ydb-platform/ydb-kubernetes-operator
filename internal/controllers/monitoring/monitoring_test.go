package monitoring_test

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	api "github.com/ydb-platform/ydb-kubernetes-operator/api/v1alpha1"
	testobjects "github.com/ydb-platform/ydb-kubernetes-operator/e2e/tests/test-objects"
	"github.com/ydb-platform/ydb-kubernetes-operator/internal/controllers/database"
	"github.com/ydb-platform/ydb-kubernetes-operator/internal/controllers/monitoring"
	"github.com/ydb-platform/ydb-kubernetes-operator/internal/controllers/storage"
	"github.com/ydb-platform/ydb-kubernetes-operator/internal/labels"
	"github.com/ydb-platform/ydb-kubernetes-operator/internal/test"
)

var (
	k8sClient client.Client
	ctx       context.Context
)

func TestMonitoringApis(t *testing.T) {
	RegisterFailHandler(Fail)

	test.SetupK8STestManager(&ctx, &k8sClient, func(mgr *manager.Manager) []test.Reconciler {
		return []test.Reconciler{
			&monitoring.DatabaseMonitoringReconciler{
				Client: k8sClient,
				Scheme: (*mgr).GetScheme(),
			},
			&monitoring.StorageMonitoringReconciler{
				Client: k8sClient,
				Scheme: (*mgr).GetScheme(),
			},
		}
	})
	BeforeEach(func() {
		nsName := testobjects.YdbNamespace

		found := corev1.Namespace{}

		err := k8sClient.Get(ctx, types.NamespacedName{Name: nsName}, &found)
		if err != nil {
			By(fmt.Sprintf("Create %s namespace", nsName))

			// FIXME: check only not found error ?
			namespace := corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: nsName,
				},
			}
			Expect(k8sClient.Create(ctx, &namespace)).Should(Succeed())
		}
	})
	RunSpecs(t, "Monitoring controllers tests")
}

func createMockSvc(name string, parentKind string, parent client.Object) {
	GinkgoHelper()

	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: testobjects.YdbNamespace,
			Labels:    map[string]string{labels.ServiceComponent: "status"},
		},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{
				{
					Name: "status",
					Port: 8765,
				},
			},
		},
	}
	Expect(k8sClient.Create(ctx, svc)).Should(Succeed())

	trueVal := true
	ref := []metav1.OwnerReference{
		{
			Kind:       parentKind,
			Name:       parent.GetName(),
			APIVersion: "v1",
			UID:        parent.GetUID(),
		},
		{
			Kind:       "Service",
			Name:       svc.Name,
			APIVersion: "v1",
			UID:        svc.GetUID(),
			Controller: &trueVal,
		},
	}
	svc.SetOwnerReferences(ref)
	Expect(k8sClient.Update(ctx, svc)).Should(Succeed())
}

func createMockDbAndSvc() {
	GinkgoHelper()

	db := testobjects.DefaultDatabase()
	Expect(k8sClient.Create(ctx, db)).Should(Succeed())

	db.Status.State = string(database.Ready)
	Expect(k8sClient.Status().Update(ctx, db)).Should(Succeed())

	createMockSvc("database-svc-status", "Database", db)
}

func createMockStorageAndSvc() {
	GinkgoHelper()

	stor := testobjects.DefaultStorage(filepath.Join("..", "..", "..", "e2e", "tests", "data", "storage-block-4-2-config.yaml"))
	Expect(k8sClient.Create(ctx, stor)).Should(Succeed())

	stor.Status.State = string(storage.Ready)
	Expect(k8sClient.Status().Update(ctx, stor)).Should(Succeed())

	createMockSvc("storage-svc-status", "Storage", stor)
}

var _ = Describe("Create DatabaseMonitoring", func() {
	When("Database is already ready", func() {
		It("We must create ServiceMonitor", func() {
			createMockDbAndSvc()

			dbMon := api.DatabaseMonitoring{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "database-monitoring",
					Namespace: testobjects.YdbNamespace,
				},
				Spec: api.DatabaseMonitoringSpec{
					DatabaseClusterRef: api.NamespacedRef{
						Name:      testobjects.DatabaseName,
						Namespace: testobjects.YdbNamespace,
					},
				},
			}

			Expect(k8sClient.Create(ctx, &dbMon)).Should(Succeed())

			Eventually(func() bool {
				found := monitoringv1.ServiceMonitorList{}

				Expect(k8sClient.List(ctx, &found, client.InNamespace(
					testobjects.YdbNamespace,
				))).Should(Succeed())

				for _, smon := range found.Items {
					if smon.Name == dbMon.GetName() {
						return true
					}
				}
				return false
			}, 15, 1).Should(BeTrue())
		})
	})
})

var _ = Describe("StorageMonitoring tests", func() {
	When("Database is already ready", func() {
		It("We must create ServiceMonitor", func() {
			createMockStorageAndSvc()

			dbMon := api.StorageMonitoring{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "storage-monitoring",
					Namespace: testobjects.YdbNamespace,
				},
				Spec: api.StorageMonitoringSpec{
					StorageRef: api.NamespacedRef{
						Name:      testobjects.StorageName,
						Namespace: testobjects.YdbNamespace,
					},
				},
			}

			Expect(k8sClient.Create(ctx, &dbMon)).Should(Succeed())

			Eventually(func() bool {
				found := monitoringv1.ServiceMonitorList{}

				Expect(k8sClient.List(ctx, &found, client.InNamespace(
					testobjects.YdbNamespace,
				))).Should(Succeed())

				for _, smon := range found.Items {
					if smon.Name == dbMon.GetName() {
						return true
					}
				}
				return false
			}, 15, 1).Should(BeTrue())
		})
	})
})
