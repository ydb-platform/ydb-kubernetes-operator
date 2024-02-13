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
	"sigs.k8s.io/controller-runtime/pkg/cluster"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	api "github.com/ydb-platform/ydb-kubernetes-operator/api/v1alpha1"
	testobjects "github.com/ydb-platform/ydb-kubernetes-operator/e2e/tests/test-objects"
	. "github.com/ydb-platform/ydb-kubernetes-operator/internal/controllers/constants"
	"github.com/ydb-platform/ydb-kubernetes-operator/internal/controllers/database"
	"github.com/ydb-platform/ydb-kubernetes-operator/internal/controllers/databasenodeset"
	"github.com/ydb-platform/ydb-kubernetes-operator/internal/controllers/remotedatabasenodeset"
	"github.com/ydb-platform/ydb-kubernetes-operator/internal/controllers/storage"
	"github.com/ydb-platform/ydb-kubernetes-operator/internal/test"
)

const (
	testRemoteCluster = "remote-cluster"
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

	err := api.AddToScheme(scheme.Scheme)
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
				&api.RemoteDatabaseNodeSet{}: {Label: databaseSelector},
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

	err = (&database.Reconciler{
		Client: localManager.GetClient(),
		Scheme: localManager.GetScheme(),
		Config: localManager.GetConfig(),
	}).SetupWithManager(localManager)
	Expect(err).ShouldNot(HaveOccurred())

	err = (&databasenodeset.Reconciler{
		Client: localManager.GetClient(),
		Scheme: localManager.GetScheme(),
		Config: localManager.GetConfig(),
	}).SetupWithManager(localManager)
	Expect(err).ShouldNot(HaveOccurred())

	err = (&databasenodeset.Reconciler{
		Client: remoteManager.GetClient(),
		Scheme: remoteManager.GetScheme(),
		Config: remoteManager.GetConfig(),
	}).SetupWithManager(remoteManager)
	Expect(err).ShouldNot(HaveOccurred())

	err = (&remotedatabasenodeset.Reconciler{
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

var _ = Describe("RemoteDatabaseNodeSet controller tests", func() {
	var localNamespace corev1.Namespace
	var remoteNamespace corev1.Namespace
	var storageSample *api.Storage
	var databaseSample *api.Database

	BeforeEach(func() {
		storageSample = testobjects.DefaultStorage(filepath.Join("..", "..", "..", "e2e", "tests", "data", "storage-block-4-2-config.yaml"))
		databaseSample = testobjects.DefaultDatabase()
		databaseSample.Spec.NodeSets = append(databaseSample.Spec.NodeSets, api.DatabaseNodeSetSpecInline{
			Name: testNodeSetName + "-local",
			DatabaseNodeSpec: api.DatabaseNodeSpec{
				Nodes: 4,
			},
		})
		databaseSample.Spec.NodeSets = append(databaseSample.Spec.NodeSets, api.DatabaseNodeSetSpecInline{
			Name: testNodeSetName + "-remote",
			Remote: &api.RemoteSpec{
				Cluster: testRemoteCluster,
			},
			DatabaseNodeSpec: api.DatabaseNodeSpec{
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
		foundStorage := api.Storage{}
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

		By("issuing create Database commands...")
		Expect(localClient.Create(ctx, databaseSample)).Should(Succeed())
		By("checking that Database created on local cluster...")
		foundDatabase := api.Database{}
		Eventually(func() bool {
			Expect(localClient.Get(ctx, types.NamespacedName{
				Name:      databaseSample.Name,
				Namespace: testobjects.YdbNamespace,
			}, &foundDatabase))
			return foundDatabase.Status.State == DatabaseProvisioning
		}, test.Timeout, test.Interval).Should(BeTrue())
	})

	AfterEach(func() {
		deleteAll(localEnv, localClient, &localNamespace)
		deleteAll(remoteEnv, remoteClient, &localNamespace)
	})

	When("Create Database with RemoteDatabaseNodeSet in k8s-mgmt-cluster", func() {
		It("Should create DatabaseNodeSet and sync resources in k8s-data-cluster", func() {
			By("checking that DatabaseNodeSet created on local cluster...")
			Eventually(func() bool {
				foundDatabaseNodeSet := api.DatabaseNodeSetList{}

				Expect(localClient.List(ctx, &foundDatabaseNodeSet, client.InNamespace(
					testobjects.YdbNamespace,
				))).Should(Succeed())

				for _, nodeset := range foundDatabaseNodeSet.Items {
					if nodeset.Name == databaseSample.Name+"-"+testNodeSetName+"-local" {
						return true
					}
				}
				return false
			}, test.Timeout, test.Interval).Should(BeTrue())

			By("checking that RemoteDatabaseNodeSet created on local cluster...")
			Eventually(func() bool {
				foundRemoteDatabaseNodeSet := api.RemoteDatabaseNodeSetList{}

				Expect(localClient.List(ctx, &foundRemoteDatabaseNodeSet, client.InNamespace(
					testobjects.YdbNamespace,
				))).Should(Succeed())

				for _, nodeset := range foundRemoteDatabaseNodeSet.Items {
					if nodeset.Name == databaseSample.Name+"-"+testNodeSetName+"-remote" {
						return true
					}
				}
				return false
			}, test.Timeout, test.Interval).Should(BeTrue())

			By("checking that DatabaseNodeSet created on remote cluster...")
			Eventually(func() bool {
				foundDatabaseNodeSetOnRemote := api.DatabaseNodeSetList{}

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

			By("set DatabaseNodeSet status to Ready on remote cluster...")
			foundDatabaseNodeSetOnRemote := api.DatabaseNodeSet{}
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

			By("checking that RemoteDatabaseNodeSet status updated on local cluster...")
			Eventually(func() bool {
				foundRemoteDatabaseNodeSetOnRemote := api.RemoteDatabaseNodeSet{}
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
	When("Delete database with RemoteDatabaseNodeSet in k8s-mgmt-cluster", func() {
		It("Should delete all resources in k8s-data-cluster", func() {
			By("checking that RemoteDatabaseNodeSet created on local cluster...")
			Eventually(func() bool {
				foundRemoteDatabaseNodeSet := api.RemoteDatabaseNodeSetList{}

				Expect(localClient.List(ctx, &foundRemoteDatabaseNodeSet, client.InNamespace(
					testobjects.YdbNamespace,
				))).Should(Succeed())

				for _, nodeset := range foundRemoteDatabaseNodeSet.Items {
					if nodeset.Name == databaseSample.Name+"-"+testNodeSetName+"-remote" {
						return true
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

			By("delete RemoteDatabaseNodeSet on local cluster...")
			Eventually(func() error {
				foundDatabase := api.Database{}
				Expect(localClient.Get(ctx, types.NamespacedName{
					Name:      databaseSample.Name,
					Namespace: testobjects.YdbNamespace,
				}, &foundDatabase)).Should(Succeed())
				foundDatabase.Spec.NodeSets = []api.DatabaseNodeSetSpecInline{
					{
						Name: testNodeSetName + "-local",
						DatabaseNodeSpec: api.DatabaseNodeSpec{
							Nodes: 4,
						},
					},
				}
				return localClient.Update(ctx, &foundDatabase)
			}, test.Timeout, test.Interval).Should(Succeed())

			By("checking that DatabaseNodeSet deleted from remote cluster...")
			Eventually(func() bool {
				foundDatabaseNodeSetOnRemote := api.DatabaseNodeSet{}

				err := remoteClient.Get(ctx, types.NamespacedName{
					Name:      databaseSample.Name + "-" + testNodeSetName + "-remote",
					Namespace: testobjects.YdbNamespace,
				}, &foundDatabaseNodeSetOnRemote)

				return apierrors.IsNotFound(err)
			}, test.Timeout, test.Interval).Should(BeTrue())

			By("checking that RemoteDatabaseNodeSet deleted from local cluster...")
			Eventually(func() bool {
				foundRemoteDatabaseNodeSet := api.RemoteDatabaseNodeSet{}

				err := localClient.Get(ctx, types.NamespacedName{
					Name:      databaseSample.Name + "-" + testNodeSetName + "-remote",
					Namespace: testobjects.YdbNamespace,
				}, &foundRemoteDatabaseNodeSet)

				return apierrors.IsNotFound(err)
			}, test.Timeout, test.Interval).Should(BeTrue())
		})
	})
})

func deleteAll(env *envtest.Environment, k8sClient client.Client, objs ...client.Object) {
	for _, obj := range objs {
		ctx := context.Background()
		clientGo, err := kubernetes.NewForConfig(env.Config)
		Expect(err).ShouldNot(HaveOccurred())
		Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, obj))).Should(Succeed())

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
