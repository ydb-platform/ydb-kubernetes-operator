package remotestoragenodeset_test

import (
	"context"
	"path/filepath"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
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

func TestRemoteNodeSetApis(t *testing.T) {
	RegisterFailHandler(Fail)

	RunSpecs(t, "RemoteStorageNodeSet controller tests")
}

var _ = BeforeSuite(func() {
	By("bootstrapping test environment")

	ctx, cancel = context.WithCancel(context.TODO())

	logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))

	localEnv = &envtest.Environment{
		CRDDirectoryPaths: []string{filepath.Join("..", "..", "..", "deploy", "ydb-operator", "crds")},
	}
	remoteEnv = &envtest.Environment{
		CRDDirectoryPaths: []string{filepath.Join("..", "..", "..", "deploy", "ydb-operator", "crds")},
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
		Client:   localManager.GetClient(),
		Scheme:   localManager.GetScheme(),
		Config:   localManager.GetConfig(),
		Recorder: localManager.GetEventRecorderFor("ydb-operator"),
	}).SetupWithManager(localManager)
	Expect(err).ShouldNot(HaveOccurred())

	err = (&storagenodeset.Reconciler{
		Client:   localManager.GetClient(),
		Scheme:   localManager.GetScheme(),
		Config:   localManager.GetConfig(),
		Recorder: localManager.GetEventRecorderFor("ydb-operator"),
	}).SetupWithManager(localManager)
	Expect(err).ShouldNot(HaveOccurred())

	err = (&storagenodeset.Reconciler{
		Client:   remoteManager.GetClient(),
		Scheme:   remoteManager.GetScheme(),
		Config:   remoteManager.GetConfig(),
		Recorder: remoteManager.GetEventRecorderFor("ydb-operator"),
	}).SetupWithManager(remoteManager)
	Expect(err).ShouldNot(HaveOccurred())

	err = (&remotestoragenodeset.Reconciler{
		Client:         remoteManager.GetClient(),
		RemoteClient:   localManager.GetClient(),
		Scheme:         remoteManager.GetScheme(),
		RemoteRecorder: remoteManager.GetEventRecorderFor("ydb-operator"),
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
	})

	AfterEach(func() {
		Expect(localClient.Delete(ctx, &localNamespace)).Should(Succeed())
		Expect(remoteClient.Delete(ctx, &remoteNamespace)).Should(Succeed())
	})

	When("Create Storage with RemoteStorageNodeSet in k8s-mgmt-cluster", func() {
		It("Should create StorageNodeSet in k8s-data-cluster", func() {
			By("issuing create commands...")
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
			Expect(localClient.Create(ctx, storageSample)).Should(Succeed())

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
				foundStorageNodeSetInRemote := api.StorageNodeSetList{}

				Expect(remoteClient.List(ctx, &foundStorageNodeSetInRemote, client.InNamespace(
					testobjects.YdbNamespace,
				))).Should(Succeed())

				for _, nodeset := range foundStorageNodeSetInRemote.Items {
					if nodeset.Name == storageSample.Name+"-"+testNodeSetName+"-remote" {
						return true
					}
				}
				return false
			}, test.Timeout, test.Interval).Should(BeTrue())

			By("checking that StorageNodeSet status is Ready on remote cluster...")
			Eventually(func() bool {
				foundStorageNodeSetOnRemote := api.StorageNodeSet{}
				Expect(remoteClient.Get(ctx, types.NamespacedName{
					Name:      storageSample.Name + "-" + testNodeSetName + "-remote",
					Namespace: testobjects.YdbNamespace,
				}, &foundStorageNodeSetOnRemote)).Should(Succeed())

				return meta.IsStatusConditionPresentAndEqual(
					foundStorageNodeSetOnRemote.Status.Conditions,
					StorageNodeSetReadyCondition,
					metav1.ConditionTrue,
				) && foundStorageNodeSetOnRemote.Status.State == StorageNodeSetReady
			}, test.Timeout, test.Interval).Should(BeTrue())

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
})
