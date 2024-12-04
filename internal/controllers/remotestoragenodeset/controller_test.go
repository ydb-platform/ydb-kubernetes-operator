package remotestoragenodeset_test

import (
	"context"
	"fmt"
	"path/filepath"
	"reflect"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/kubectl/pkg/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/cluster"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	"github.com/ydb-platform/ydb-kubernetes-operator/api/v1alpha1"
	testobjects "github.com/ydb-platform/ydb-kubernetes-operator/e2e/tests/test-objects"
	ydbannotations "github.com/ydb-platform/ydb-kubernetes-operator/internal/annotations"
	. "github.com/ydb-platform/ydb-kubernetes-operator/internal/controllers/constants"
	"github.com/ydb-platform/ydb-kubernetes-operator/internal/controllers/remotestoragenodeset"
	"github.com/ydb-platform/ydb-kubernetes-operator/internal/controllers/storage"
	"github.com/ydb-platform/ydb-kubernetes-operator/internal/controllers/storagenodeset"
	"github.com/ydb-platform/ydb-kubernetes-operator/internal/resources"
	"github.com/ydb-platform/ydb-kubernetes-operator/internal/test"
)

const (
	testRemoteCluster = "remote-cluster"
	testSecretName    = "remote-secret"
	testNodeSetName   = "nodeset"
)

var (
	localClient  client.Client
	remoteClient client.Client
	localEnv     *envtest.Environment
	remoteEnv    *envtest.Environment
	ctx          context.Context
	cancel       context.CancelFunc
)

func TestRemoteStorageNodeSetApis(t *testing.T) {
	RegisterFailHandler(Fail)

	RunSpecs(t, "RemoteStorageNodeSet controller tests")
}

var _ = BeforeSuite(func() {
	By("bootstrapping test environment")

	ctx, cancel = context.WithCancel(context.TODO())

	logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))

	localEnv = &envtest.Environment{
		CRDDirectoryPaths: []string{
			filepath.Join("..", "..", "..", "deploy", "ydb-operator", "crds"),
		},
		ErrorIfCRDPathMissing: true,
	}
	remoteEnv = &envtest.Environment{
		CRDDirectoryPaths: []string{
			filepath.Join("..", "..", "..", "deploy", "ydb-operator", "crds"),
		},
		ErrorIfCRDPathMissing: true,
	}

	err := v1alpha1.AddToScheme(scheme.Scheme)
	Expect(err).ToNot(HaveOccurred())
	// +kubebuilder:scaffold:scheme

	localCfg, err := localEnv.Start()
	Expect(err).ToNot(HaveOccurred())
	Expect(localCfg).ToNot(BeNil())

	remoteCfg, err := remoteEnv.Start()
	Expect(err).ToNot(HaveOccurred())
	Expect(remoteCfg).ToNot(BeNil())

	localManager, err := ctrl.NewManager(localCfg, ctrl.Options{
		MetricsBindAddress: "0",
		Scheme:             scheme.Scheme,
	})
	Expect(err).ShouldNot(HaveOccurred())

	remoteManager, err := ctrl.NewManager(remoteCfg, ctrl.Options{
		MetricsBindAddress: "0",
		Scheme:             scheme.Scheme,
	})
	Expect(err).ShouldNot(HaveOccurred())

	storageSelector, err := remotestoragenodeset.BuildRemoteSelector(testRemoteCluster)
	Expect(err).ShouldNot(HaveOccurred())

	remoteCluster, err := cluster.New(localCfg, func(o *cluster.Options) {
		o.Scheme = scheme.Scheme
		o.NewCache = cache.BuilderWithOptions(cache.Options{
			SelectorsByObject: cache.SelectorsByObject{
				&v1alpha1.RemoteStorageNodeSet{}: {Label: storageSelector},
			},
		})
	})
	Expect(err).ShouldNot(HaveOccurred())

	err = remoteManager.Add(remoteCluster)
	Expect(err).ShouldNot(HaveOccurred())

	err = (&storage.Reconciler{
		Client: localManager.GetClient(),
		Scheme: localManager.GetScheme(),
		Config: localManager.GetConfig(),
	}).SetupWithManager(localManager)
	Expect(err).ShouldNot(HaveOccurred())

	err = (&storagenodeset.Reconciler{
		Client: localManager.GetClient(),
		Scheme: localManager.GetScheme(),
		Config: localManager.GetConfig(),
	}).SetupWithManager(localManager)
	Expect(err).ShouldNot(HaveOccurred())

	err = (&storagenodeset.Reconciler{
		Client: remoteManager.GetClient(),
		Scheme: remoteManager.GetScheme(),
		Config: remoteManager.GetConfig(),
	}).SetupWithManager(remoteManager)
	Expect(err).ShouldNot(HaveOccurred())

	err = (&remotestoragenodeset.Reconciler{
		Client: remoteManager.GetClient(),
		Scheme: remoteManager.GetScheme(),
	}).SetupWithManager(remoteManager, &remoteCluster)
	Expect(err).ShouldNot(HaveOccurred())

	go func() {
		defer GinkgoRecover()
		err = localManager.Start(ctx)
		Expect(err).ShouldNot(HaveOccurred())
	}()

	go func() {
		defer GinkgoRecover()
		err = remoteManager.Start(ctx)
		Expect(err).ShouldNot(HaveOccurred())
	}()

	localClient = localManager.GetClient()
	Expect(localClient).ToNot(BeNil())
	remoteClient = remoteManager.GetClient()
	Expect(remoteClient).ToNot(BeNil())
})

var _ = AfterSuite(func() {
	By("tearing down the test environment")
	cancel()
	err := localEnv.Stop()
	Expect(err).ToNot(HaveOccurred())
	err = remoteEnv.Stop()
	Expect(err).ToNot(HaveOccurred())
})

var _ = Describe("RemoteStorageNodeSet controller tests", func() {
	var localNamespace corev1.Namespace
	var remoteNamespace corev1.Namespace
	var storageSample *v1alpha1.Storage

	BeforeEach(func() {
		storageSample = testobjects.DefaultStorage(filepath.Join("..", "..", "..", "e2e", "tests", "data", "storage-mirror-3-dc-config.yaml"))
		storageSample.Spec.NodeSets = append(storageSample.Spec.NodeSets, v1alpha1.StorageNodeSetSpecInline{
			Name: testNodeSetName + "-local",
			StorageNodeSpec: v1alpha1.StorageNodeSpec{
				Nodes: 1,
			},
		})
		storageSample.Spec.NodeSets = append(storageSample.Spec.NodeSets, v1alpha1.StorageNodeSetSpecInline{
			Name: testNodeSetName + "-remote-static",
			Remote: &v1alpha1.RemoteSpec{
				Cluster: testRemoteCluster,
			},
			StorageNodeSpec: v1alpha1.StorageNodeSpec{
				Nodes: 1,
			},
		})
		storageSample.Spec.NodeSets = append(storageSample.Spec.NodeSets, v1alpha1.StorageNodeSetSpecInline{
			Name: testNodeSetName + "-remote",
			Remote: &v1alpha1.RemoteSpec{
				Cluster: testRemoteCluster,
			},
			StorageNodeSpec: v1alpha1.StorageNodeSpec{
				Nodes: 1,
			},
		})

		By("issuing create Namespace commands...")
		localNamespace = corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: testobjects.YdbNamespace,
			},
		}
		remoteNamespace = corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: testobjects.YdbNamespace,
			},
		}
		Expect(localClient.Create(ctx, &localNamespace)).Should(Succeed())
		Expect(remoteClient.Create(ctx, &remoteNamespace)).Should(Succeed())

		By("issuing create Storage commands...")
		Expect(localClient.Create(ctx, storageSample)).Should(Succeed())

		By("checking that Storage created on local cluster...")
		Eventually(func() bool {
			foundStorage := v1alpha1.Storage{}
			Expect(localClient.Get(ctx, types.NamespacedName{
				Name:      storageSample.Name,
				Namespace: testobjects.YdbNamespace,
			}, &foundStorage))
			return foundStorage.Status.State == StorageInitializing
		}, test.Timeout, test.Interval).Should(BeTrue())

		By("checking that StorageNodeSet created on local cluster...")
		Eventually(func() error {
			foundStorageNodeSet := &v1alpha1.StorageNodeSet{}
			return localClient.Get(ctx, types.NamespacedName{
				Name:      storageSample.Name + "-" + testNodeSetName + "-local",
				Namespace: testobjects.YdbNamespace,
			}, foundStorageNodeSet)
		}, test.Timeout, test.Interval).ShouldNot(HaveOccurred())

		By("checking that RemoteStorageNodeSet created on local cluster...")
		Eventually(func() error {
			foundRemoteStorageNodeSet := &v1alpha1.RemoteStorageNodeSet{}
			return localClient.Get(ctx, types.NamespacedName{
				Name:      storageSample.Name + "-" + testNodeSetName + "-remote",
				Namespace: testobjects.YdbNamespace,
			}, foundRemoteStorageNodeSet)
		}, test.Timeout, test.Interval).ShouldNot(HaveOccurred())

		By("checking that static RemoteStorageNodeSet created on local cluster...")
		Eventually(func() error {
			foundStaticRemoteStorageNodeSet := &v1alpha1.RemoteStorageNodeSet{}
			return localClient.Get(ctx, types.NamespacedName{
				Name:      storageSample.Name + "-" + testNodeSetName + "-remote-static",
				Namespace: testobjects.YdbNamespace,
			}, foundStaticRemoteStorageNodeSet)
		}, test.Timeout, test.Interval).ShouldNot(HaveOccurred())

		By("checking that StorageNodeSet created on remote cluster...")
		Eventually(func() error {
			foundStorageNodeSetOnRemote := &v1alpha1.StorageNodeSet{}
			return remoteClient.Get(ctx, types.NamespacedName{
				Name:      storageSample.Name + "-" + testNodeSetName + "-remote",
				Namespace: testobjects.YdbNamespace,
			}, foundStorageNodeSetOnRemote)
		}, test.Timeout, test.Interval).ShouldNot(HaveOccurred())

		By("checking that static StorageNodeSet created on remote cluster...")
		Eventually(func() error {
			foundStaticStorageNodeSetOnRemote := &v1alpha1.StorageNodeSet{}
			return remoteClient.Get(ctx, types.NamespacedName{
				Name:      storageSample.Name + "-" + testNodeSetName + "-remote-static",
				Namespace: testobjects.YdbNamespace,
			}, foundStaticStorageNodeSetOnRemote)
		}, test.Timeout, test.Interval).ShouldNot(HaveOccurred())
	})

	AfterEach(func() {
		Expect(localClient.Delete(ctx, storageSample)).Should(Succeed())
		test.DeleteAllObjects(localEnv, localClient, &localNamespace)
		test.DeleteAllObjects(remoteEnv, remoteClient, &localNamespace)
	})

	When("Created RemoteStorageNodeSet in k8s-mgmt-cluster", func() {
		It("Should receive status from k8s-data-cluster", func() {
			By("checking that static StorageNodeSet status updated on remote cluster...")
			Eventually(func() bool {
				foundStaticRemoteStorageNodeSetOnRemote := v1alpha1.StorageNodeSet{}
				Expect(remoteClient.Get(ctx, types.NamespacedName{
					Name:      storageSample.Name + "-" + testNodeSetName + "-remote-static",
					Namespace: testobjects.YdbNamespace,
				}, &foundStaticRemoteStorageNodeSetOnRemote)).Should(Succeed())

				return meta.IsStatusConditionPresentAndEqual(
					foundStaticRemoteStorageNodeSetOnRemote.Status.Conditions,
					NodeSetPreparedCondition,
					metav1.ConditionTrue,
				)
			}, test.Timeout, test.Interval).Should(BeTrue())

			By("checking that static RemoteStorageNodeSet status updated on local cluster...")
			Eventually(func() bool {
				foundStaticRemoteStorageNodeSet := v1alpha1.RemoteStorageNodeSet{}
				Expect(localClient.Get(ctx, types.NamespacedName{
					Name:      storageSample.Name + "-" + testNodeSetName + "-remote-static",
					Namespace: testobjects.YdbNamespace,
				}, &foundStaticRemoteStorageNodeSet)).Should(Succeed())

				return meta.IsStatusConditionPresentAndEqual(
					foundStaticRemoteStorageNodeSet.Status.Conditions,
					NodeSetPreparedCondition,
					metav1.ConditionTrue,
				)
			}, test.Timeout, test.Interval).Should(BeTrue())

			By("checking that StorageNodeSet status updated on remote cluster...")
			Eventually(func() bool {
				foundRemoteStorageNodeSetOnRemote := v1alpha1.StorageNodeSet{}
				Expect(remoteClient.Get(ctx, types.NamespacedName{
					Name:      storageSample.Name + "-" + testNodeSetName + "-remote",
					Namespace: testobjects.YdbNamespace,
				}, &foundRemoteStorageNodeSetOnRemote)).Should(Succeed())

				return meta.IsStatusConditionPresentAndEqual(
					foundRemoteStorageNodeSetOnRemote.Status.Conditions,
					NodeSetPreparedCondition,
					metav1.ConditionTrue,
				)
			}, test.Timeout, test.Interval).Should(BeTrue())

			By("checking that RemoteStorageNodeSet status updated on local cluster...")
			Eventually(func() bool {
				foundRemoteStorageNodeSet := v1alpha1.RemoteStorageNodeSet{}
				Expect(localClient.Get(ctx, types.NamespacedName{
					Name:      storageSample.Name + "-" + testNodeSetName + "-remote",
					Namespace: testobjects.YdbNamespace,
				}, &foundRemoteStorageNodeSet)).Should(Succeed())

				return meta.IsStatusConditionPresentAndEqual(
					foundRemoteStorageNodeSet.Status.Conditions,
					NodeSetPreparedCondition,
					metav1.ConditionTrue,
				)
			}, test.Timeout, test.Interval).Should(BeTrue())
		})
	})
	When("Created RemoteStorageNodeSet with Secrets in k8s-mgmt-cluster", func() {
		It("Should sync Secrets into k8s-data-cluster", func() {
			By("create simple Secret in Storage namespace")
			simpleSecret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testSecretName,
					Namespace: testobjects.YdbNamespace,
				},
				StringData: map[string]string{
					"message": "Hello from k8s-mgmt-cluster",
				},
			}
			Expect(localClient.Create(ctx, simpleSecret))

			By("checking that Storage updated on local cluster...")
			Eventually(func() error {
				foundStorage := &v1alpha1.Storage{}
				Expect(localClient.Get(ctx, types.NamespacedName{
					Name:      storageSample.Name,
					Namespace: testobjects.YdbNamespace,
				}, foundStorage))

				foundStorage.Spec.Secrets = append(
					foundStorage.Spec.Secrets,
					&corev1.LocalObjectReference{
						Name: testSecretName,
					},
				)
				return localClient.Update(ctx, foundStorage)
			}, test.Timeout, test.Interval).ShouldNot(HaveOccurred())

			By("checking that Secrets are synced...")
			Eventually(func() error {
				foundRemoteStorageNodeSet := &v1alpha1.RemoteStorageNodeSet{}
				Expect(localClient.Get(ctx, types.NamespacedName{
					Name:      storageSample.Name + "-" + testNodeSetName + "-remote",
					Namespace: testobjects.YdbNamespace,
				}, foundRemoteStorageNodeSet)).Should(Succeed())

				localSecret := &corev1.Secret{}
				err := localClient.Get(ctx, types.NamespacedName{
					Name:      testSecretName,
					Namespace: testobjects.YdbNamespace,
				}, localSecret)
				if err != nil {
					return err
				}

				remoteSecret := &corev1.Secret{}
				err = remoteClient.Get(ctx, types.NamespacedName{
					Name:      testSecretName,
					Namespace: testobjects.YdbNamespace,
				}, remoteSecret)
				if err != nil {
					return err
				}

				primaryResourceName, exist := remoteSecret.Annotations[ydbannotations.PrimaryResourceStorageAnnotation]
				if !exist {
					return fmt.Errorf("annotation %s does not exist on remoteSecret %s", ydbannotations.PrimaryResourceStorageAnnotation, remoteSecret.Name)
				}
				if primaryResourceName != foundRemoteStorageNodeSet.Spec.StorageRef.Name {
					return fmt.Errorf("primaryResourceName %s does not equal storageRef name %s", primaryResourceName, foundRemoteStorageNodeSet.Spec.StorageRef.Name)
				}

				remoteRV, exist := remoteSecret.Annotations[ydbannotations.RemoteResourceVersionAnnotation]
				if !exist {
					return fmt.Errorf("annotation %s does not exist on remoteSecret %s", ydbannotations.RemoteResourceVersionAnnotation, remoteSecret.Name)
				}
				if localSecret.GetResourceVersion() != remoteRV {
					return fmt.Errorf("localRV %s does not equal remoteRV %s", localSecret.GetResourceVersion(), remoteRV)
				}

				return nil
			}, test.Timeout, test.Interval).ShouldNot(HaveOccurred())
		})
	})
	When("Created RemoteStorageNodeSet with Services in k8s-mgmt-cluster", func() {
		It("Should sync Services into k8s-data-cluster", func() {
			By("checking that Services are synced...")
			Eventually(func() error {
				foundRemoteStorageNodeSet := &v1alpha1.RemoteStorageNodeSet{}
				Expect(localClient.Get(ctx, types.NamespacedName{
					Name:      storageSample.Name + "-" + testNodeSetName + "-remote",
					Namespace: testobjects.YdbNamespace,
				}, foundRemoteStorageNodeSet)).Should(Succeed())

				foundServices := corev1.ServiceList{}
				Expect(localClient.List(ctx, &foundServices, client.InNamespace(
					testobjects.YdbNamespace,
				))).Should(Succeed())
				for _, localService := range foundServices.Items {
					remoteService := &corev1.Service{}
					err := remoteClient.Get(ctx, types.NamespacedName{
						Name:      localService.Name,
						Namespace: localService.Namespace,
					}, remoteService)
					if err != nil {
						return err
					}
				}
				return nil
			}, test.Timeout, test.Interval).ShouldNot(HaveOccurred())

			By("checking that static RemoteStorageNodeSet RemoteResource status are updated...")
			Eventually(func() bool {
				foundRemoteStorageNodeSet := &v1alpha1.RemoteStorageNodeSet{}
				Expect(localClient.Get(ctx, types.NamespacedName{
					Name:      storageSample.Name + "-" + testNodeSetName + "-remote-static",
					Namespace: testobjects.YdbNamespace,
				}, foundRemoteStorageNodeSet)).Should(Succeed())

				foundConfigMap := corev1.ConfigMap{}
				Expect(remoteClient.Get(ctx, types.NamespacedName{
					Name:      storageSample.Name,
					Namespace: testobjects.YdbNamespace,
				}, &foundConfigMap)).Should(Succeed())

				for idx := range foundRemoteStorageNodeSet.Status.RemoteResources {
					remoteResource := foundRemoteStorageNodeSet.Status.RemoteResources[idx]
					if resources.EqualRemoteResourceWithObject(
						&remoteResource,
						foundConfigMap.DeepCopy(),
					) {
						if meta.IsStatusConditionPresentAndEqual(
							remoteResource.Conditions,
							RemoteResourceSyncedCondition,
							metav1.ConditionTrue,
						) {
							return true
						}
					}
				}
				return false
			}, test.Timeout, test.Interval).Should(BeTrue())

			By("checking that static RemoteStorageNodeSet status are synced...")
			Eventually(func() bool {
				foundStorageNodeSet := &v1alpha1.StorageNodeSet{}
				Expect(remoteClient.Get(ctx, types.NamespacedName{
					Name:      storageSample.Name + "-" + testNodeSetName + "-remote-static",
					Namespace: testobjects.YdbNamespace,
				}, foundStorageNodeSet)).Should(Succeed())

				foundRemoteStorageNodeSet := &v1alpha1.RemoteStorageNodeSet{}
				Expect(localClient.Get(ctx, types.NamespacedName{
					Name:      storageSample.Name + "-" + testNodeSetName + "-remote-static",
					Namespace: testobjects.YdbNamespace,
				}, foundRemoteStorageNodeSet)).Should(Succeed())

				if foundStorageNodeSet.Status.State != foundRemoteStorageNodeSet.Status.State {
					return false
				}
				return reflect.DeepEqual(foundStorageNodeSet.Status.Conditions, foundRemoteStorageNodeSet.Status.Conditions)
			}, test.Timeout, test.Interval).Should(BeTrue())

			By("checking that RemoteStorageNodeSet RemoteResource status are updated...")
			Eventually(func() bool {
				foundRemoteStorageNodeSet := &v1alpha1.RemoteStorageNodeSet{}
				Expect(localClient.Get(ctx, types.NamespacedName{
					Name:      storageSample.Name + "-" + testNodeSetName + "-remote",
					Namespace: testobjects.YdbNamespace,
				}, foundRemoteStorageNodeSet)).Should(Succeed())

				foundConfigMap := corev1.ConfigMap{}
				Expect(remoteClient.Get(ctx, types.NamespacedName{
					Name:      storageSample.Name,
					Namespace: testobjects.YdbNamespace,
				}, &foundConfigMap)).Should(Succeed())

				for idx := range foundRemoteStorageNodeSet.Status.RemoteResources {
					remoteResource := foundRemoteStorageNodeSet.Status.RemoteResources[idx]
					if resources.EqualRemoteResourceWithObject(
						&remoteResource,
						foundConfigMap.DeepCopy(),
					) {
						if meta.IsStatusConditionPresentAndEqual(
							remoteResource.Conditions,
							RemoteResourceSyncedCondition,
							metav1.ConditionTrue,
						) {
							return true
						}
					}
				}
				return false
			}, test.Timeout, test.Interval).Should(BeTrue())

			By("checking that RemoteStorageNodeSet status are synced...")
			Eventually(func() bool {
				foundStorageNodeSet := &v1alpha1.StorageNodeSet{}
				Expect(remoteClient.Get(ctx, types.NamespacedName{
					Name:      storageSample.Name + "-" + testNodeSetName + "-remote",
					Namespace: testobjects.YdbNamespace,
				}, foundStorageNodeSet)).Should(Succeed())

				foundRemoteStorageNodeSet := &v1alpha1.RemoteStorageNodeSet{}
				Expect(localClient.Get(ctx, types.NamespacedName{
					Name:      storageSample.Name + "-" + testNodeSetName + "-remote",
					Namespace: testobjects.YdbNamespace,
				}, foundRemoteStorageNodeSet)).Should(Succeed())

				if foundStorageNodeSet.Status.State != foundRemoteStorageNodeSet.Status.State {
					return false
				}
				return reflect.DeepEqual(foundStorageNodeSet.Status.Conditions, foundRemoteStorageNodeSet.Status.Conditions)
			}, test.Timeout, test.Interval).Should(BeTrue())
		})
	})
	When("Delete Storage with RemoteStorageNodeSet in k8s-mgmt-cluster", func() {
		It("Should delete all resources in k8s-data-cluster", func() {
			By("delete RemoteStorageNodeSet on local cluster...")
			Eventually(func() error {
				foundStorage := v1alpha1.Storage{}
				Expect(localClient.Get(ctx, types.NamespacedName{
					Name:      storageSample.Name,
					Namespace: testobjects.YdbNamespace,
				}, &foundStorage)).Should(Succeed())
				foundStorage.Spec.NodeSets = []v1alpha1.StorageNodeSetSpecInline{
					{
						Name: testNodeSetName + "-local",
						StorageNodeSpec: v1alpha1.StorageNodeSpec{
							Nodes: 2,
						},
					},
					{
						Name: testNodeSetName + "-remote-static",
						Remote: &v1alpha1.RemoteSpec{
							Cluster: testRemoteCluster,
						},
						StorageNodeSpec: v1alpha1.StorageNodeSpec{
							Nodes: 1,
						},
					},
				}
				return localClient.Update(ctx, &foundStorage)
			}, test.Timeout, test.Interval).Should(Succeed())

			By("checking that StorageNodeSet deleted from remote cluster...")
			Eventually(func() bool {
				foundStorageNodeSetOnRemote := v1alpha1.StorageNodeSet{}

				err := remoteClient.Get(ctx, types.NamespacedName{
					Name:      storageSample.Name + "-" + testNodeSetName + "-remote",
					Namespace: testobjects.YdbNamespace,
				}, &foundStorageNodeSetOnRemote)

				return apierrors.IsNotFound(err)
			}, test.Timeout, test.Interval).Should(BeTrue())

			By("checking that RemoteStorageNodeSet deleted from local cluster...")
			Eventually(func() bool {
				foundRemoteStorageNodeSet := v1alpha1.RemoteStorageNodeSet{}

				err := localClient.Get(ctx, types.NamespacedName{
					Name:      storageSample.Name + "-" + testNodeSetName + "-remote",
					Namespace: testobjects.YdbNamespace,
				}, &foundRemoteStorageNodeSet)

				return apierrors.IsNotFound(err)
			}, test.Timeout, test.Interval).Should(BeTrue())

			By("checking that Services for static RemoteStorageNodeSet exisiting in remote cluster...")
			Eventually(func() error {
				foundStaticStorageNodeSetOnRemote := &v1alpha1.StorageNodeSet{}
				Expect(remoteClient.Get(ctx, types.NamespacedName{
					Name:      storageSample.Name + "-" + testNodeSetName + "-remote-static",
					Namespace: testobjects.YdbNamespace,
				}, foundStaticStorageNodeSetOnRemote)).Should(Succeed())

				storageServices := corev1.ServiceList{}
				Expect(localClient.List(ctx, &storageServices,
					client.InNamespace(testobjects.YdbNamespace),
				)).Should(Succeed())
				for _, storageService := range storageServices.Items {
					remoteService := &corev1.Service{}
					if err := remoteClient.Get(ctx, types.NamespacedName{
						Name:      storageService.GetName(),
						Namespace: storageService.GetNamespace(),
					}, remoteService); err != nil {
						return err
					}
				}
				return nil
			}, test.Timeout, test.Interval).ShouldNot(HaveOccurred())
		})
	})
})
