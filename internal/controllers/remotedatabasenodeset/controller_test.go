package remotedatabasenodeset_test

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/kubectl/pkg/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
	"sigs.k8s.io/controller-runtime/pkg/cluster"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	"github.com/ydb-platform/ydb-kubernetes-operator/api/v1alpha1"
	testobjects "github.com/ydb-platform/ydb-kubernetes-operator/e2e/tests/test-objects"
	ydbannotations "github.com/ydb-platform/ydb-kubernetes-operator/internal/annotations"
	. "github.com/ydb-platform/ydb-kubernetes-operator/internal/controllers/constants"
	"github.com/ydb-platform/ydb-kubernetes-operator/internal/controllers/database"
	"github.com/ydb-platform/ydb-kubernetes-operator/internal/controllers/databasenodeset"
	"github.com/ydb-platform/ydb-kubernetes-operator/internal/controllers/remotedatabasenodeset"
	"github.com/ydb-platform/ydb-kubernetes-operator/internal/controllers/storage"
	"github.com/ydb-platform/ydb-kubernetes-operator/internal/resources"
	"github.com/ydb-platform/ydb-kubernetes-operator/internal/test"
)

const (
	testRemoteCluster = "remote-cluster"
	testNodeSetName   = "nodeset"
	testSecretName    = "remote-secret"
)

var (
	localClient  client.Client
	remoteClient client.Client
	localEnv     *envtest.Environment
	remoteEnv    *envtest.Environment
	ctx          context.Context
	cancel       context.CancelFunc
)

func TestRemoteDatabaseNodeSetApis(t *testing.T) {
	RegisterFailHandler(Fail)

	RunSpecs(t, "RemoteDatabaseNodeSet controller tests")
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
	Expect(err).ShouldNot(HaveOccurred())

	localCfg, err := localEnv.Start()
	Expect(err).ToNot(HaveOccurred())
	Expect(localCfg).ToNot(BeNil())

	remoteCfg, err := remoteEnv.Start()
	Expect(err).ToNot(HaveOccurred())
	Expect(remoteCfg).ToNot(BeNil())

	// +kubebuilder:scaffold:scheme

	localManager, err := ctrl.NewManager(localCfg, ctrl.Options{
		MetricsBindAddress: "0",
		Scheme:             scheme.Scheme,
	})
	Expect(err).ShouldNot(HaveOccurred())

	//+kubebuilder:scaffold:scheme

	remoteManager, err := ctrl.NewManager(remoteCfg, ctrl.Options{
		MetricsBindAddress: "0",
		Scheme:             scheme.Scheme,
	})
	Expect(err).ShouldNot(HaveOccurred())

	databaseSelector, err := remotedatabasenodeset.BuildRemoteSelector(testRemoteCluster)
	Expect(err).ShouldNot(HaveOccurred())

	remoteCluster, err := cluster.New(localCfg, func(o *cluster.Options) {
		o.Scheme = scheme.Scheme
		o.NewCache = cache.BuilderWithOptions(cache.Options{
			SelectorsByObject: cache.SelectorsByObject{
				&v1alpha1.RemoteDatabaseNodeSet{}: {Label: databaseSelector},
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
		Log:    logf.Log,
	}).SetupWithManager(localManager)
	Expect(err).ShouldNot(HaveOccurred())

	err = (&database.Reconciler{
		Client: localManager.GetClient(),
		Scheme: localManager.GetScheme(),
		Config: localManager.GetConfig(),
		Log:    logf.Log,
	}).SetupWithManager(localManager)
	Expect(err).ShouldNot(HaveOccurred())

	err = (&databasenodeset.Reconciler{
		Client: localManager.GetClient(),
		Scheme: localManager.GetScheme(),
		Config: localManager.GetConfig(),
		Log:    logf.Log,
	}).SetupWithManager(localManager)
	Expect(err).ShouldNot(HaveOccurred())

	err = (&databasenodeset.Reconciler{
		Client: remoteManager.GetClient(),
		Scheme: remoteManager.GetScheme(),
		Config: remoteManager.GetConfig(),
		Log:    logf.Log,
	}).SetupWithManager(remoteManager)
	Expect(err).ShouldNot(HaveOccurred())

	err = (&remotedatabasenodeset.Reconciler{
		Client: remoteManager.GetClient(),
		Scheme: remoteManager.GetScheme(),
		Log:    logf.Log,
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

var _ = Describe("RemoteDatabaseNodeSet controller tests", func() {
	var localNamespace corev1.Namespace
	var remoteNamespace corev1.Namespace
	var storageSample *v1alpha1.Storage
	var databaseSample *v1alpha1.Database

	BeforeEach(func() {
		storageSample = testobjects.DefaultStorage(filepath.Join("..", "..", "..", "e2e", "tests", "data", "storage-block-4-2-config.yaml"))
		databaseSample = testobjects.DefaultDatabase()
		databaseSample.Spec.NodeSets = append(databaseSample.Spec.NodeSets, v1alpha1.DatabaseNodeSetSpecInline{
			Name: testNodeSetName + "-local",
			DatabaseNodeSpec: v1alpha1.DatabaseNodeSpec{
				Nodes: 4,
			},
		})
		databaseSample.Spec.NodeSets = append(databaseSample.Spec.NodeSets, v1alpha1.DatabaseNodeSetSpecInline{
			Name: testNodeSetName + "-remote",
			Remote: &v1alpha1.RemoteSpec{
				Cluster: testRemoteCluster,
			},
			DatabaseNodeSpec: v1alpha1.DatabaseNodeSpec{
				Nodes: 4,
			},
		})
		databaseSample.Spec.NodeSets = append(databaseSample.Spec.NodeSets, v1alpha1.DatabaseNodeSetSpecInline{
			Name: testNodeSetName + "-remote-dedicated",
			Remote: &v1alpha1.RemoteSpec{
				Cluster: testRemoteCluster,
			},
			DatabaseNodeSpec: v1alpha1.DatabaseNodeSpec{
				Nodes: 4,
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
		foundStorage := v1alpha1.Storage{}
		Eventually(func() bool {
			Expect(localClient.Get(ctx, types.NamespacedName{
				Name:      storageSample.Name,
				Namespace: testobjects.YdbNamespace,
			}, &foundStorage))
			return foundStorage.Status.State == StorageProvisioning
		}, test.Timeout, test.Interval).Should(BeTrue())
		By("set status Ready to Storage...")
		foundStorage.Status.State = StorageReady
		Expect(localClient.Status().Update(ctx, &foundStorage)).Should(Succeed())

		By("set status Ready to Storage...")
		Eventually(func() error {
			foundStorage := v1alpha1.Storage{}
			Expect(localClient.Get(ctx, types.NamespacedName{
				Name:      storageSample.Name,
				Namespace: testobjects.YdbNamespace,
			}, &foundStorage))
			return localClient.Status().Update(ctx, &foundStorage)
		}, test.Timeout, test.Interval).ShouldNot(HaveOccurred())

		By("issuing create Database commands...")
		Expect(localClient.Create(ctx, databaseSample)).Should(Succeed())
		By("checking that Database created on local cluster...")
		foundDatabase := v1alpha1.Database{}
		Eventually(func() bool {
			Expect(localClient.Get(ctx, types.NamespacedName{
				Name:      databaseSample.Name,
				Namespace: testobjects.YdbNamespace,
			}, &foundDatabase))
			return foundDatabase.Status.State == DatabaseProvisioning
		}, test.Timeout, test.Interval).Should(BeTrue())

		By("checking that DatabaseNodeSet created on local cluster...")
		Eventually(func() error {
			foundDatabaseNodeSet := &v1alpha1.DatabaseNodeSet{}
			return localClient.Get(ctx, types.NamespacedName{
				Name:      databaseSample.Name + "-" + testNodeSetName + "-local",
				Namespace: testobjects.YdbNamespace,
			}, foundDatabaseNodeSet)
		}, test.Timeout, test.Interval).ShouldNot(HaveOccurred())

		By("checking that RemoteDatabaseNodeSet created on local cluster...")
		Eventually(func() error {
			foundRemoteDatabaseNodeSet := &v1alpha1.RemoteDatabaseNodeSet{}
			return localClient.Get(ctx, types.NamespacedName{
				Name:      databaseSample.Name + "-" + testNodeSetName + "-remote",
				Namespace: testobjects.YdbNamespace,
			}, foundRemoteDatabaseNodeSet)
		}, test.Timeout, test.Interval).ShouldNot(HaveOccurred())

		By("checking that dedicated RemoteDatabaseNodeSet created on local cluster...")
		Eventually(func() error {
			foundRemoteDatabaseNodeSet := &v1alpha1.RemoteDatabaseNodeSet{}
			return localClient.Get(ctx, types.NamespacedName{
				Name:      databaseSample.Name + "-" + testNodeSetName + "-remote-dedicated",
				Namespace: testobjects.YdbNamespace,
			}, foundRemoteDatabaseNodeSet)
		}, test.Timeout, test.Interval).ShouldNot(HaveOccurred())

		By("checking that DatabaseNodeSet created on remote cluster...")
		Eventually(func() bool {
			foundDatabaseNodeSetOnRemote := v1alpha1.DatabaseNodeSetList{}

			Expect(remoteClient.List(ctx, &foundDatabaseNodeSetOnRemote, client.InNamespace(
				testobjects.YdbNamespace,
			))).Should(Succeed())

			for _, nodeset := range foundDatabaseNodeSetOnRemote.Items {
				if nodeset.Name == databaseSample.Name+"-"+testNodeSetName+"-remote" {
					return true
				}
			}
			return false
		}, test.Timeout, test.Interval).Should(BeTrue())

		By("checking that dedicated DatabaseNodeSet created on remote cluster...")
		Eventually(func() bool {
			foundDatabaseNodeSetOnRemote := v1alpha1.DatabaseNodeSetList{}

			Expect(remoteClient.List(ctx, &foundDatabaseNodeSetOnRemote, client.InNamespace(
				testobjects.YdbNamespace,
			))).Should(Succeed())

			for _, nodeset := range foundDatabaseNodeSetOnRemote.Items {
				if nodeset.Name == databaseSample.Name+"-"+testNodeSetName+"-remote-dedicated" {
					return true
				}
			}
			return false
		}, test.Timeout, test.Interval).Should(BeTrue())
	})

	AfterEach(func() {
		deleteAll(localEnv, localClient, &localNamespace)
		deleteAll(remoteEnv, remoteClient, &localNamespace)
	})

	When("Created RemoteDatabaseNodeSet in k8s-mgmt-cluster", func() {
		It("Should receive status from k8s-data-cluster", func() {
			By("set dedicated DatabaseNodeSet status to Ready on remote cluster...")
			foundDedicatedDatabaseNodeSetOnRemote := v1alpha1.DatabaseNodeSet{}
			Expect(remoteClient.Get(ctx, types.NamespacedName{
				Name:      databaseSample.Name + "-" + testNodeSetName + "-remote-dedicated",
				Namespace: testobjects.YdbNamespace,
			}, &foundDedicatedDatabaseNodeSetOnRemote)).Should(Succeed())

			foundDedicatedDatabaseNodeSetOnRemote.Status.State = DatabaseNodeSetReady
			foundDedicatedDatabaseNodeSetOnRemote.Status.Conditions = append(
				foundDedicatedDatabaseNodeSetOnRemote.Status.Conditions,
				metav1.Condition{
					Type:               DatabaseNodeSetReadyCondition,
					Status:             "True",
					Reason:             ReasonCompleted,
					LastTransitionTime: metav1.NewTime(time.Now()),
					Message:            fmt.Sprintf("Scaled databaseNodeSet to %d successfully", foundDedicatedDatabaseNodeSetOnRemote.Spec.Nodes),
				},
			)
			Expect(remoteClient.Status().Update(ctx, &foundDedicatedDatabaseNodeSetOnRemote)).Should(Succeed())

			By("set DatabaseNodeSet status to Ready on remote cluster...")
			foundDatabaseNodeSetOnRemote := v1alpha1.DatabaseNodeSet{}
			Expect(remoteClient.Get(ctx, types.NamespacedName{
				Name:      databaseSample.Name + "-" + testNodeSetName + "-remote",
				Namespace: testobjects.YdbNamespace,
			}, &foundDatabaseNodeSetOnRemote)).Should(Succeed())

			foundDatabaseNodeSetOnRemote.Status.State = DatabaseNodeSetReady
			foundDatabaseNodeSetOnRemote.Status.Conditions = append(
				foundDatabaseNodeSetOnRemote.Status.Conditions,
				metav1.Condition{
					Type:               DatabaseNodeSetReadyCondition,
					Status:             "True",
					Reason:             ReasonCompleted,
					LastTransitionTime: metav1.NewTime(time.Now()),
					Message:            fmt.Sprintf("Scaled databaseNodeSet to %d successfully", foundDatabaseNodeSetOnRemote.Spec.Nodes),
				},
			)
			Expect(remoteClient.Status().Update(ctx, &foundDatabaseNodeSetOnRemote)).Should(Succeed())

			By("checking that dedicated RemoteDatabaseNodeSet status updated on local cluster...")
			Eventually(func() bool {
				foundRemoteDatabaseNodeSetOnRemote := v1alpha1.RemoteDatabaseNodeSet{}
				Expect(localClient.Get(ctx, types.NamespacedName{
					Name:      databaseSample.Name + "-" + testNodeSetName + "-remote-dedicated",
					Namespace: testobjects.YdbNamespace,
				}, &foundRemoteDatabaseNodeSetOnRemote)).Should(Succeed())

				return meta.IsStatusConditionPresentAndEqual(
					foundRemoteDatabaseNodeSetOnRemote.Status.Conditions,
					DatabaseNodeSetReadyCondition,
					metav1.ConditionTrue,
				) && foundRemoteDatabaseNodeSetOnRemote.Status.State == DatabaseNodeSetReady
			}, test.Timeout, test.Interval).Should(BeTrue())

			By("checking that RemoteDatabaseNodeSet status updated on local cluster...")
			Eventually(func() bool {
				foundRemoteDatabaseNodeSetOnRemote := v1alpha1.RemoteDatabaseNodeSet{}
				Expect(localClient.Get(ctx, types.NamespacedName{
					Name:      databaseSample.Name + "-" + testNodeSetName + "-remote",
					Namespace: testobjects.YdbNamespace,
				}, &foundRemoteDatabaseNodeSetOnRemote)).Should(Succeed())

				return meta.IsStatusConditionPresentAndEqual(
					foundRemoteDatabaseNodeSetOnRemote.Status.Conditions,
					DatabaseNodeSetReadyCondition,
					metav1.ConditionTrue,
				) && foundRemoteDatabaseNodeSetOnRemote.Status.State == DatabaseNodeSetReady
			}, test.Timeout, test.Interval).Should(BeTrue())
		})
	})

	When("Created RemoteDatabaseNodeSet with Secrets in k8s-mgmt-cluster", func() {
		It("Should sync Secrets into k8s-data-cluster", func() {
			By("create simple Secret in Database namespace")
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

			By("checking that Database updated on local cluster...")
			Eventually(func() error {
				foundDatabase := &v1alpha1.Database{}
				Expect(localClient.Get(ctx, types.NamespacedName{
					Name:      databaseSample.Name,
					Namespace: testobjects.YdbNamespace,
				}, foundDatabase))

				foundDatabase.Spec.Secrets = append(
					foundDatabase.Spec.Secrets,
					&corev1.LocalObjectReference{
						Name: testSecretName,
					},
				)
				return localClient.Update(ctx, foundDatabase)
			}, test.Timeout, test.Interval).ShouldNot(HaveOccurred())

			By("checking that Secrets are synced...")
			Eventually(func() error {
				foundRemoteDatabaseNodeSet := &v1alpha1.RemoteDatabaseNodeSet{}
				Expect(localClient.Get(ctx, types.NamespacedName{
					Name:      databaseSample.Name + "-" + testNodeSetName + "-remote",
					Namespace: testobjects.YdbNamespace,
				}, foundRemoteDatabaseNodeSet)).Should(Succeed())

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

				primaryResourceName, exist := remoteSecret.Annotations[ydbannotations.PrimaryResourceDatabaseAnnotation]
				if !exist {
					return fmt.Errorf("annotation %s does not exist on remoteSecret %s", ydbannotations.PrimaryResourceDatabaseAnnotation, remoteSecret.Name)
				}
				if primaryResourceName != foundRemoteDatabaseNodeSet.Spec.DatabaseRef.Name {
					return fmt.Errorf("primaryResourceName %s does not equal databaseRef name %s", primaryResourceName, foundRemoteDatabaseNodeSet.Spec.DatabaseRef.Name)
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

	When("Created RemoteDatabaseNodeSet with Services in k8s-mgmt-cluster", func() {
		It("Should sync Services into k8s-data-cluster", func() {
			By("checking that Services are synced...")
			Eventually(func() error {
				foundRemoteDatabaseNodeSet := &v1alpha1.RemoteDatabaseNodeSet{}
				Expect(localClient.Get(ctx, types.NamespacedName{
					Name:      databaseSample.Name + "-" + testNodeSetName + "-remote",
					Namespace: testobjects.YdbNamespace,
				}, foundRemoteDatabaseNodeSet)).Should(Succeed())

				foundServices := corev1.ServiceList{}
				Expect(localClient.List(ctx, &foundServices, client.InNamespace(
					testobjects.YdbNamespace,
				))).Should(Succeed())
				for _, localService := range foundServices.Items {
					if !strings.HasPrefix(localService.Name, databaseSample.Name) {
						continue
					}
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

			By("checking that dedicated RemoteDatabaseNodeSet RemoteStatus are updated...")
			Eventually(func() bool {
				foundRemoteDatabaseNodeSet := &v1alpha1.RemoteDatabaseNodeSet{}
				Expect(localClient.Get(ctx, types.NamespacedName{
					Name:      databaseSample.Name + "-" + testNodeSetName + "-remote-dedicated",
					Namespace: testobjects.YdbNamespace,
				}, foundRemoteDatabaseNodeSet)).Should(Succeed())

				foundConfigMap := corev1.ConfigMap{}
				Expect(remoteClient.Get(ctx, types.NamespacedName{
					Name:      databaseSample.Name,
					Namespace: testobjects.YdbNamespace,
				}, &foundConfigMap)).Should(Succeed())

				gvk, err := apiutil.GVKForObject(foundConfigMap.DeepCopy(), scheme.Scheme)
				Expect(err).ShouldNot(HaveOccurred())

				for idx := range foundRemoteDatabaseNodeSet.Status.RemoteResources {
					remoteResource := foundRemoteDatabaseNodeSet.Status.RemoteResources[idx]
					if resources.EqualRemoteResourceWithObject(
						&remoteResource,
						testobjects.YdbNamespace,
						foundConfigMap.DeepCopy(),
						gvk,
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

			By("checking that DatabaseNodeSet created on remote cluster...")
			Eventually(func() bool {
				foundDatabaseNodeSet := api.DatabaseNodeSetList{}

				Expect(remoteClient.List(ctx, &foundDatabaseNodeSet, client.InNamespace(
					testobjects.YdbNamespace,
				))).Should(Succeed())

				for _, nodeset := range foundDatabaseNodeSet.Items {
					if nodeset.Name == databaseSample.Name+"-"+testNodeSetName+"-remote" {
						return true
					}
				}
				return false
			}, test.Timeout, test.Interval).Should(BeTrue())
		})
	})

	When("Delete RemoteDatabaseNodeSet in k8s-mgmt-cluster", func() {
		It("Should delete resources in k8s-data-cluster", func() {
			By("delete RemoteDatabaseNodeSet on local cluster...")
			Eventually(func() error {
				foundDatabase := v1alpha1.Database{}
				Expect(localClient.Get(ctx, types.NamespacedName{
					Name:      databaseSample.Name,
					Namespace: testobjects.YdbNamespace,
				}, &foundDatabase)).Should(Succeed())
				foundDatabase.Spec.NodeSets = []v1alpha1.DatabaseNodeSetSpecInline{
					{
						Name: testNodeSetName + "-local",
						DatabaseNodeSpec: v1alpha1.DatabaseNodeSpec{
							Nodes: 4,
						},
					},
					{
						Name: testNodeSetName + "-remote-dedicated",
						Remote: &v1alpha1.RemoteSpec{
							Cluster: testRemoteCluster,
						},
						DatabaseNodeSpec: v1alpha1.DatabaseNodeSpec{
							Nodes: 4,
						},
					},
				}
				return localClient.Update(ctx, &foundDatabase)
			}, test.Timeout, test.Interval).Should(Succeed())

			By("checking that DatabaseNodeSet deleted from remote cluster...")
			Eventually(func() bool {
				foundDatabaseNodeSetOnRemote := v1alpha1.DatabaseNodeSet{}

				err := remoteClient.Get(ctx, types.NamespacedName{
					Name:      databaseSample.Name + "-" + testNodeSetName + "-remote",
					Namespace: testobjects.YdbNamespace,
				}, &foundDatabaseNodeSetOnRemote)

				return apierrors.IsNotFound(err)
			}, test.Timeout, test.Interval).Should(BeTrue())

			By("checking that RemoteDatabaseNodeSet deleted from local cluster...")
			Eventually(func() bool {
				foundRemoteDatabaseNodeSet := v1alpha1.RemoteDatabaseNodeSet{}

				err := localClient.Get(ctx, types.NamespacedName{
					Name:      databaseSample.Name + "-" + testNodeSetName + "-remote",
					Namespace: testobjects.YdbNamespace,
				}, &foundRemoteDatabaseNodeSet)

				return apierrors.IsNotFound(err)
			}, test.Timeout, test.Interval).Should(BeTrue())

			By("checking that Services for dedicated DatabaseNodeSet exisiting in remote cluster...")
			Eventually(func() error {
				foundDedicatedDatabaseNodeSetOnRemote := &v1alpha1.DatabaseNodeSet{}
				Expect(remoteClient.Get(ctx, types.NamespacedName{
					Name:      databaseSample.Name + "-" + testNodeSetName + "-remote-dedicated",
					Namespace: testobjects.YdbNamespace,
				}, foundDedicatedDatabaseNodeSetOnRemote)).Should(Succeed())

				databaseServices := corev1.ServiceList{}
				Expect(localClient.List(ctx, &databaseServices,
					client.InNamespace(testobjects.YdbNamespace),
				)).Should(Succeed())
				for _, databaseService := range databaseServices.Items {
					remoteService := &corev1.Service{}
					if !strings.HasPrefix(databaseService.GetName(), databaseSample.Name) {
						continue
					}
					if err := remoteClient.Get(ctx, types.NamespacedName{
						Name:      databaseService.GetName(),
						Namespace: databaseService.GetNamespace(),
					}, remoteService); err != nil {
						return err
					}
				}
				return nil
			}, test.Timeout, test.Interval).ShouldNot(HaveOccurred())
		})
	})
})

func deleteAll(env *envtest.Environment, k8sClient client.Client, objs ...client.Object) {
	for _, obj := range objs {
		ctx := context.Background()
		clientGo, err := kubernetes.NewForConfig(env.Config)
		Expect(err).ShouldNot(HaveOccurred())
		Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, obj))).Should(Succeed())

		//nolint:nestif
		if ns, ok := obj.(*corev1.Namespace); ok {
			// Normally the kube-controller-manager would handle finalization
			// and garbage collection of namespaces, but with envtest, we aren't
			// running a kube-controller-manager. Instead we're gonna approximate
			// (poorly) the kube-controller-manager by explicitly deleting some
			// resources within the namespace and then removing the `kubernetes`
			// finalizer from the namespace resource so it can finish deleting.
			// Note that any resources within the namespace that we don't
			// successfully delete could reappear if the namespace is ever
			// recreated with the same name.

			// Look up all namespaced resources under the discovery API
			_, apiResources, err := clientGo.Discovery().ServerGroupsAndResources()
			Expect(err).ShouldNot(HaveOccurred())
			namespacedGVKs := make(map[string]schema.GroupVersionKind)
			for _, apiResourceList := range apiResources {
				defaultGV, err := schema.ParseGroupVersion(apiResourceList.GroupVersion)
				Expect(err).ShouldNot(HaveOccurred())
				for _, r := range apiResourceList.APIResources {
					if !r.Namespaced || strings.Contains(r.Name, "/") {
						// skip non-namespaced and subresources
						continue
					}
					gvk := schema.GroupVersionKind{
						Group:   defaultGV.Group,
						Version: defaultGV.Version,
						Kind:    r.Kind,
					}
					if r.Group != "" {
						gvk.Group = r.Group
					}
					if r.Version != "" {
						gvk.Version = r.Version
					}
					namespacedGVKs[gvk.String()] = gvk
				}
			}

			// Delete all namespaced resources in this namespace
			for _, gvk := range namespacedGVKs {
				var u unstructured.Unstructured
				u.SetGroupVersionKind(gvk)
				err := k8sClient.DeleteAllOf(ctx, &u, client.InNamespace(ns.Name))
				Expect(client.IgnoreNotFound(ignoreMethodNotAllowed(err))).ShouldNot(HaveOccurred())
			}

			Eventually(func() error {
				key := client.ObjectKeyFromObject(ns)
				if err := k8sClient.Get(ctx, key, ns); err != nil {
					return client.IgnoreNotFound(err)
				}
				// remove `kubernetes` finalizer
				const kubernetes = "kubernetes"
				finalizers := []corev1.FinalizerName{}
				for _, f := range ns.Spec.Finalizers {
					if f != kubernetes {
						finalizers = append(finalizers, f)
					}
				}
				ns.Spec.Finalizers = finalizers

				// We have to use the k8s.io/client-go library here to expose
				// ability to patch the /finalize subresource on the namespace
				_, err = clientGo.CoreV1().Namespaces().Finalize(ctx, ns, metav1.UpdateOptions{})
				return err
			}, test.Timeout, test.Interval).Should(Succeed())
		}

		Eventually(func() metav1.StatusReason {
			key := client.ObjectKeyFromObject(obj)
			if err := k8sClient.Get(ctx, key, obj); err != nil {
				return apierrors.ReasonForError(err)
			}
			return ""
		}, test.Timeout, test.Interval).Should(Equal(metav1.StatusReasonNotFound))
	}
}

func ignoreMethodNotAllowed(err error) error {
	if err != nil {
		if apierrors.ReasonForError(err) == metav1.StatusReasonMethodNotAllowed {
			return nil
		}
	}
	return err
}
