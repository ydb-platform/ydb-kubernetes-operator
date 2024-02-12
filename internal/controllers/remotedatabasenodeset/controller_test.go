package remotedatabasenodeset_test

import (
	"context"
	"fmt"
	"path/filepath"
	"runtime"
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
	"github.com/ydb-platform/ydb-kubernetes-operator/internal/controllers/database"
	"github.com/ydb-platform/ydb-kubernetes-operator/internal/controllers/databasenodeset"
	"github.com/ydb-platform/ydb-kubernetes-operator/internal/controllers/remotedatabasenodeset"
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

	RunSpecs(t, "RemoteDatabaseNodeSet controller tests")
}

var _ = BeforeSuite(func() {
	By("bootstrapping test environment")

	ctx, cancel = context.WithCancel(context.TODO())

	logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))

	_, curfile, _, _ := runtime.Caller(0) //nolint:dogsled
	localEnv = &envtest.Environment{
		CRDDirectoryPaths: []string{
			filepath.Join(curfile, filepath.Join("..", "..", "..", "deploy", "ydb-operator", "crds")),
		},
		ErrorIfCRDPathMissing: true,
	}
	remoteEnv = &envtest.Environment{
		CRDDirectoryPaths: []string{
			filepath.Join(curfile, filepath.Join("..", "..", "..", "deploy", "ydb-operator", "crds")),
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

	err = (&database.Reconciler{
		Client:   localManager.GetClient(),
		Scheme:   localManager.GetScheme(),
		Config:   localManager.GetConfig(),
		Recorder: localManager.GetEventRecorderFor("ydb-operator"),
	}).SetupWithManager(localManager)
	Expect(err).ShouldNot(HaveOccurred())

	err = (&databasenodeset.Reconciler{
		Client:   localManager.GetClient(),
		Scheme:   localManager.GetScheme(),
		Config:   localManager.GetConfig(),
		Recorder: localManager.GetEventRecorderFor("ydb-operator"),
	}).SetupWithManager(localManager)
	Expect(err).ShouldNot(HaveOccurred())

	err = (&databasenodeset.Reconciler{
		Client:   remoteManager.GetClient(),
		Scheme:   remoteManager.GetScheme(),
		Config:   remoteManager.GetConfig(),
		Recorder: remoteManager.GetEventRecorderFor("ydb-operator"),
	}).SetupWithManager(remoteManager)
	Expect(err).ShouldNot(HaveOccurred())

	err = (&remotedatabasenodeset.Reconciler{
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

var _ = Describe("RemoteDatabaseNodeSet controller tests", func() {
	var localNamespace corev1.Namespace
	var remoteNamespace corev1.Namespace
	var storageSample *api.Storage
	var databaseSample *api.Database

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

	When("Create database with RemoteDatabaseNodeSet in k8s-mgmt-cluster", func() {
		It("Should create databaseNodeSet and sync resources in k8s-data-cluster", func() {
			By("issuing create commands...")
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
			Expect(localClient.Create(ctx, storageSample)).Should(Succeed())
			Expect(localClient.Create(ctx, databaseSample)).Should(Succeed())

			By("checking that Storage created on local cluster...")
			Eventually(func() error {
				foundStorage := api.Storage{}

				return localClient.Get(ctx, types.NamespacedName{
					Name:      storageSample.Name,
					Namespace: testobjects.YdbNamespace,
				}, &foundStorage)

			}, test.Timeout, test.Interval).ShouldNot(HaveOccurred())

			By("checking that databaseNodeSet created on local cluster...")
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

			By("checking that databaseNodeSet created on remote cluster...")
			Eventually(func() bool {
				founddatabaseNodeSetOnRemote := api.DatabaseNodeSetList{}

				Expect(remoteClient.List(ctx, &founddatabaseNodeSetOnRemote, client.InNamespace(
					testobjects.YdbNamespace,
				))).Should(Succeed())

				for _, nodeset := range founddatabaseNodeSetOnRemote.Items {
					if nodeset.Name == databaseSample.Name+"-"+testNodeSetName+"-remote" {
						return true
					}
				}
				return false
			}, test.Timeout, test.Interval).Should(BeTrue())

			By("Set databaseNodeSet status to Ready on remote cluster...")
			founddatabaseNodeSetOnRemote := api.DatabaseNodeSet{}
			Expect(remoteClient.Get(ctx, types.NamespacedName{
				Name:      databaseSample.Name + "-" + testNodeSetName + "-remote",
				Namespace: testobjects.YdbNamespace,
			}, &founddatabaseNodeSetOnRemote)).Should(Succeed())

			founddatabaseNodeSetOnRemote.Status.State = DatabaseNodeSetReady
			founddatabaseNodeSetOnRemote.Status.Conditions = append(
				founddatabaseNodeSetOnRemote.Status.Conditions,
				metav1.Condition{
					Type:    DatabaseNodeSetReadyCondition,
					Status:  "True",
					Reason:  ReasonCompleted,
					Message: fmt.Sprintf("Scaled databaseNodeSet to %d successfully", founddatabaseNodeSetOnRemote.Spec.Nodes),
				},
			)
			Expect(remoteClient.Status().Update(ctx, &founddatabaseNodeSetOnRemote)).Should(Succeed())

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
			By("issuing create commands...")
			storageSample = testobjects.DefaultStorage(filepath.Join("..", "..", "..", "e2e", "tests", "data", "storage-block-4-2-config.yaml"))
			databaseSample = testobjects.DefaultDatabase()
			databaseSample.Spec.NodeSets = append(databaseSample.Spec.NodeSets, api.DatabaseNodeSetSpecInline{
				Name: testNodeSetName + "-remote",
				Remote: &api.RemoteSpec{
					Cluster: testRemoteCluster,
				},
				DatabaseNodeSpec: api.DatabaseNodeSpec{
					Nodes: 4,
				},
			})
			Expect(localClient.Create(ctx, storageSample)).Should(Succeed())
			Expect(localClient.Create(ctx, databaseSample)).Should(Succeed())

			By("checking that Storage created on local cluster...")
			Eventually(func() error {
				foundStorage := api.Storage{}

				return localClient.Get(ctx, types.NamespacedName{
					Name:      storageSample.Name,
					Namespace: testobjects.YdbNamespace,
				}, &foundStorage)

			}, test.Timeout, test.Interval).ShouldNot(HaveOccurred())

			By("checking that DatabaseNodeSet created on remote cluster...")
			Eventually(func() bool {
				founddatabaseNodeSetOnRemote := api.DatabaseNodeSetList{}

				Expect(remoteClient.List(ctx, &founddatabaseNodeSetOnRemote, client.InNamespace(
					testobjects.YdbNamespace,
				))).Should(Succeed())

				for _, nodeset := range founddatabaseNodeSetOnRemote.Items {
					if nodeset.Name == databaseSample.Name+"-"+testNodeSetName+"-remote" {
						return true
					}
				}
				return false
			}, test.Timeout, test.Interval).Should(BeTrue())

			By("Delete Database on local cluster...")
			founddatabase := api.Database{}
			Expect(localClient.Get(ctx, types.NamespacedName{
				Name:      databaseSample.Name,
				Namespace: testobjects.YdbNamespace,
			}, &founddatabase)).Should(Succeed())

			Expect(localClient.Delete(ctx, &founddatabase))

			By("checking that DatabaseNodeSets deleted from remote cluster...")
			Eventually(func() bool {
				founddatabaseNodeSetOnRemote := api.DatabaseNodeSetList{}

				Expect(remoteClient.List(ctx, &founddatabaseNodeSetOnRemote, client.InNamespace(
					testobjects.YdbNamespace,
				))).Should(Succeed())

				return len(founddatabaseNodeSetOnRemote.Items) == 0
			}, test.Timeout, test.Interval).Should(BeTrue())

			By("checking that Database deleted from local cluster...")
			Eventually(func() bool {
				founddatabases := api.DatabaseList{}

				Expect(remoteClient.List(ctx, &founddatabases, client.InNamespace(
					testobjects.YdbNamespace,
				))).Should(Succeed())

				return len(founddatabases.Items) == 0
			}, test.Timeout, test.Interval).Should(BeTrue())
		})
	})
})
