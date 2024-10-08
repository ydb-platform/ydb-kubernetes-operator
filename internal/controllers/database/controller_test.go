package database_test

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
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
	env       *envtest.Environment
)

func TestAPIs(t *testing.T) {
	RegisterFailHandler(Fail)

	env = test.SetupK8STestManager(&ctx, &k8sClient, func(mgr *manager.Manager) []test.Reconciler {
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
		storageSample = *testobjects.DefaultStorage(filepath.Join("..", "..", "..", "e2e", "tests", "data", "storage-mirror-3-dc-config.yaml"))
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
	})

	AfterEach(func() {
		Expect(k8sClient.Delete(ctx, &storageSample)).Should(Succeed())
		Expect(k8sClient.Delete(ctx, &namespace)).Should(Succeed())
		test.DeleteAllObjects(env, k8sClient, &namespace)
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

		By("Check encryption for Database...")
		foundDatabase := v1alpha1.Database{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{
			Name:      databaseSample.Name,
			Namespace: testobjects.YdbNamespace,
		}, &foundDatabase))

		By("Update Database and enable encryption...")
		foundDatabase.Spec.Encryption = &v1alpha1.EncryptionConfig{Enabled: true}
		Expect(k8sClient.Update(ctx, &foundDatabase)).Should(Succeed())

		By("Check that encryption secret was created...")
		encryptionSecret := corev1.Secret{}
		Eventually(func() error {
			return k8sClient.Get(ctx, types.NamespacedName{
				Name:      databaseSample.Name,
				Namespace: testobjects.YdbNamespace,
			}, &encryptionSecret)
		}, test.Timeout, test.Interval).ShouldNot(HaveOccurred())
		encryptionData := encryptionSecret.Data

		By("Check that arg `--key-file` was added to StatefulSet...")
		databaseStatefulSet = appsv1.StatefulSet{}
		Eventually(func() error {
			Expect(k8sClient.List(ctx,
				&foundStatefulSets,
				client.InNamespace(testobjects.YdbNamespace),
			)).ShouldNot(HaveOccurred())
			for idx, statefulSet := range foundStatefulSets.Items {
				if statefulSet.Name == testobjects.DatabaseName {
					databaseStatefulSet = foundStatefulSets.Items[idx]
					break
				}
			}
			podContainerArgs := databaseStatefulSet.Spec.Template.Spec.Containers[0].Args
			encryptionKeyConfigPath := fmt.Sprintf("%s/%s", v1alpha1.ConfigDir, v1alpha1.DatabaseEncryptionKeyConfigFile)
			for idx, arg := range podContainerArgs {
				if arg == "--key-file" {
					if podContainerArgs[idx+1] == encryptionKeyConfigPath {
						return nil
					}
					return fmt.Errorf(
						"Found arg `--key-file=%s` for encryption does not match with expected path: %s",
						podContainerArgs[idx+1],
						encryptionKeyConfigPath,
					)
				}
			}
			return errors.New("Failed to find arg `--key-file` for encryption in StatefulSet")
		}, test.Timeout, test.Interval).ShouldNot(HaveOccurred())

		By("Update Database encryption pin...")
		Expect(k8sClient.Get(ctx, types.NamespacedName{
			Name:      databaseSample.Name,
			Namespace: testobjects.YdbNamespace,
		}, &foundDatabase))
		pin := "Ignore"
		foundDatabase.Spec.Encryption = &v1alpha1.EncryptionConfig{
			Enabled: true,
			Pin:     &pin,
		}
		Expect(k8sClient.Update(ctx, &foundDatabase)).Should(Succeed())

		By("Check that Secret for encryption was not changed...")
		Consistently(func(g Gomega) bool {
			g.Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name:      databaseSample.Name,
				Namespace: testobjects.YdbNamespace,
			}, &encryptionSecret))
			return reflect.DeepEqual(encryptionData, encryptionSecret.Data)
		}, test.Timeout, test.Interval).Should(BeTrue())
	})

	It("Check iPDiscovery flag works", func() {
		getDBSts := func(generation int64) appsv1.StatefulSet {
			sts := appsv1.StatefulSet{}

			Eventually(
				func() error {
					objectKey := types.NamespacedName{
						Name:      testobjects.DatabaseName,
						Namespace: testobjects.YdbNamespace,
					}
					err := k8sClient.Get(ctx, objectKey, &sts)
					if err != nil {
						return err
					}

					if sts.Generation <= generation {
						return fmt.Errorf("sts is too old (generation=%d)", sts.Generation)
					}
					return nil
				},
			).WithTimeout(test.Timeout).WithPolling(test.Interval).Should(Succeed())

			return sts
		}
		getDB := func() v1alpha1.Database {
			found := v1alpha1.Database{}
			Eventually(func() error {
				return k8sClient.Get(ctx,
					types.NamespacedName{
						Name:      testobjects.DatabaseName,
						Namespace: testobjects.YdbNamespace,
					},
					&found,
				)
			}, test.Timeout, test.Interval).Should(Succeed())
			return found
		}

		db := *testobjects.DefaultDatabase()

		Expect(k8sClient.Create(ctx, &db)).Should(Succeed())

		sts := getDBSts(0)
		args := sts.Spec.Template.Spec.Containers[0].Args

		Expect(args).To(ContainElements([]string{"--grpc-public-host"}))
		Expect(args).ToNot(ContainElements([]string{"--grpc-public-address-v6", "--grpc-public-address-v4", "--grpc-public-target-name-override"}))

		db = getDB()
		db.Spec.Service.GRPC.IPDiscovery = &v1alpha1.IPDiscovery{
			Enabled:  true,
			IPFamily: corev1.IPv6Protocol,
		}

		Expect(k8sClient.Update(ctx, &db)).Should(Succeed())

		sts = getDBSts(sts.Generation)
		args = sts.Spec.Template.Spec.Containers[0].Args

		Expect(args).To(ContainElements([]string{"--grpc-public-address-v6"}))
		Expect(args).ToNot(ContainElements([]string{"--grpc-public-target-name-override"}))

		db = getDB()

		db.Spec.Service.GRPC.IPDiscovery = &v1alpha1.IPDiscovery{
			Enabled:            true,
			IPFamily:           corev1.IPv4Protocol,
			TargetNameOverride: "a.b.c.d",
		}

		Expect(k8sClient.Update(ctx, &db)).Should(Succeed())

		sts = getDBSts(sts.Generation)
		args = sts.Spec.Template.Spec.Containers[0].Args

		Expect(args).To(ContainElements([]string{"--grpc-public-address-v4", "--grpc-public-target-name-override"}))
	})
})
