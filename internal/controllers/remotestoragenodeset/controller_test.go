package remotestoragenodeset_test

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
	"github.com/ydb-platform/ydb-kubernetes-operator/internal/controllers/remotestoragenodeset"
	"github.com/ydb-platform/ydb-kubernetes-operator/internal/controllers/storage"
	"github.com/ydb-platform/ydb-kubernetes-operator/internal/controllers/storagenodeset"
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

	err := api.AddToScheme(scheme.Scheme)
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
				&api.RemoteStorageNodeSet{}: {Label: storageSelector},
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
	var storageSample *api.Storage

	BeforeEach(func() {
		storageSample = testobjects.DefaultStorage(filepath.Join("..", "..", "..", "e2e", "tests", "data", "storage-block-4-2-config.yaml"))
		storageSample.Spec.NodeSets = append(storageSample.Spec.NodeSets, api.StorageNodeSetSpecInline{
			Name: testNodeSetName + "-local",
			StorageNodeSpec: api.StorageNodeSpec{
				Nodes: 4,
			},
		})
		storageSample.Spec.NodeSets = append(storageSample.Spec.NodeSets, api.StorageNodeSetSpecInline{
			Name: testNodeSetName + "-remote",
			Remote: &api.RemoteSpec{
				Cluster: testRemoteCluster,
			},
			StorageNodeSpec: api.StorageNodeSpec{
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

		By("checking that StorageNodeSet created on local cluster...")
		Eventually(func() bool {
			foundStorageNodeSet := api.StorageNodeSetList{}

			Expect(localClient.List(ctx, &foundStorageNodeSet, client.InNamespace(
				testobjects.YdbNamespace,
			))).Should(Succeed())

			for _, nodeset := range foundStorageNodeSet.Items {
				if nodeset.Name == storageSample.Name+"-"+testNodeSetName+"-local" {
					return true
				}
			}
			return false
		}, test.Timeout, test.Interval).Should(BeTrue())

		By("checking that RemoteStorageNodeSet created on local cluster...")
		Eventually(func() bool {
			foundRemoteStorageNodeSet := api.RemoteStorageNodeSetList{}

			Expect(localClient.List(ctx, &foundRemoteStorageNodeSet, client.InNamespace(
				testobjects.YdbNamespace,
			))).Should(Succeed())

			for _, nodeset := range foundRemoteStorageNodeSet.Items {
				if nodeset.Name == storageSample.Name+"-"+testNodeSetName+"-remote" {
					return true
				}
			}
			return false
		}, test.Timeout, test.Interval).Should(BeTrue())

		By("checking that StorageNodeSet created on remote cluster...")
		Eventually(func() bool {
			foundStorageNodeSetOnRemote := api.StorageNodeSetList{}

			Expect(remoteClient.List(ctx, &foundStorageNodeSetOnRemote, client.InNamespace(
				testobjects.YdbNamespace,
			))).Should(Succeed())

			for _, nodeset := range foundStorageNodeSetOnRemote.Items {
				if nodeset.Name == storageSample.Name+"-"+testNodeSetName+"-remote" {
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

	When("Create Storage with RemoteStorageNodeSet in k8s-mgmt-cluster", func() {
		It("Should create StorageNodeSet and sync resources in k8s-data-cluster", func() {
			By("set StorageNodeSet status to Ready on remote cluster...")
			foundStorageNodeSetOnRemote := api.StorageNodeSet{}
			Expect(remoteClient.Get(ctx, types.NamespacedName{
				Name:      storageSample.Name + "-" + testNodeSetName + "-remote",
				Namespace: testobjects.YdbNamespace,
			}, &foundStorageNodeSetOnRemote)).Should(Succeed())

			foundStorageNodeSetOnRemote.Status.State = StorageNodeSetReady
			foundStorageNodeSetOnRemote.Status.Conditions = append(
				foundStorageNodeSetOnRemote.Status.Conditions,
				metav1.Condition{
					Type:               StorageNodeSetReadyCondition,
					Status:             "True",
					Reason:             ReasonCompleted,
					LastTransitionTime: metav1.NewTime(time.Now()),
					Message:            fmt.Sprintf("Scaled StorageNodeSet to %d successfully", foundStorageNodeSetOnRemote.Spec.Nodes),
				},
			)
			Expect(remoteClient.Status().Update(ctx, &foundStorageNodeSetOnRemote)).Should(Succeed())

			By("checking that RemoteStorageNodeSet status updated on local cluster...")
			Eventually(func() bool {
				foundRemoteStorageNodeSetOnRemote := api.RemoteStorageNodeSet{}
				Expect(localClient.Get(ctx, types.NamespacedName{
					Name:      storageSample.Name + "-" + testNodeSetName + "-remote",
					Namespace: testobjects.YdbNamespace,
				}, &foundRemoteStorageNodeSetOnRemote)).Should(Succeed())

				return meta.IsStatusConditionPresentAndEqual(
					foundRemoteStorageNodeSetOnRemote.Status.Conditions,
					StorageNodeSetReadyCondition,
					metav1.ConditionTrue,
				) && foundRemoteStorageNodeSetOnRemote.Status.State == StorageNodeSetReady
			}, test.Timeout, test.Interval).Should(BeTrue())
		})
	})
	When("Delete Storage with RemoteStorageNodeSet in k8s-mgmt-cluster", func() {
		It("Should delete all resources in k8s-data-cluster", func() {
			By("delete RemoteStorageNodeSet on local cluster...")
			Eventually(func() error {
				foundStorage := api.Storage{}
				Expect(localClient.Get(ctx, types.NamespacedName{
					Name:      storageSample.Name,
					Namespace: testobjects.YdbNamespace,
				}, &foundStorage)).Should(Succeed())
				foundStorage.Spec.NodeSets = []api.StorageNodeSetSpecInline{
					{
						Name: testNodeSetName + "-local",
						StorageNodeSpec: api.StorageNodeSpec{
							Nodes: 4,
						},
					},
				}
				return localClient.Update(ctx, &foundStorage)
			}, test.Timeout, test.Interval).Should(Succeed())

			By("checking that StorageNodeSet deleted from remote cluster...")
			Eventually(func() bool {
				foundStorageNodeSetOnRemote := api.StorageNodeSet{}

				err := remoteClient.Get(ctx, types.NamespacedName{
					Name:      storageSample.Name + "-" + testNodeSetName + "-remote",
					Namespace: testobjects.YdbNamespace,
				}, &foundStorageNodeSetOnRemote)

				return apierrors.IsNotFound(err)
			}, test.Timeout, test.Interval).Should(BeTrue())

			By("checking that RemoteStorageNodeSet deleted from local cluster...")
			Eventually(func() bool {
				foundRemoteStorageNodeSet := api.RemoteStorageNodeSet{}

				err := localClient.Get(ctx, types.NamespacedName{
					Name:      storageSample.Name + "-" + testNodeSetName + "-remote",
					Namespace: testobjects.YdbNamespace,
				}, &foundRemoteStorageNodeSet)

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
