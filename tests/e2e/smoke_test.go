package tests

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v1alpha1 "github.com/ydb-platform/ydb-kubernetes-operator/api/v1alpha1"
	. "github.com/ydb-platform/ydb-kubernetes-operator/internal/controllers/constants"
	"github.com/ydb-platform/ydb-kubernetes-operator/internal/resources"
	"github.com/ydb-platform/ydb-kubernetes-operator/internal/test"
	testobjects "github.com/ydb-platform/ydb-kubernetes-operator/tests/test-k8s-objects"
	. "github.com/ydb-platform/ydb-kubernetes-operator/tests/test-utils"
)

func emptyStorageDefaultFields(storage *v1alpha1.Storage) {
	storage.Spec.Image = nil
	storage.Spec.Resources = nil
	storage.Spec.Service = nil
	storage.Spec.Monitoring = nil
}

func emptyDatabaseDefaultFields(database *v1alpha1.Database) {
	database.Spec.StorageClusterRef.Namespace = ""
	database.Spec.Image = nil
	database.Spec.Service = nil
	database.Spec.Domain = ""
	database.Spec.Path = ""
	database.Spec.Encryption = nil
	database.Spec.Datastreams = nil
	database.Spec.Monitoring = nil
	database.Spec.StorageEndpoint = ""
}

var _ = Describe("Operator smoke test", func() {
	var ctx context.Context
	var namespace corev1.Namespace

	var storageSample *v1alpha1.Storage
	var databaseSample *v1alpha1.Database

	BeforeEach(func() {
		storageSample = testobjects.DefaultStorage(filepath.Join("..", "data", "storage-mirror-3-dc-config.yaml"))
		databaseSample = testobjects.DefaultDatabase()

		ctx = context.Background()
		namespace = corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: testobjects.YdbNamespace,
			},
		}
		Expect(k8sClient.Create(ctx, &namespace)).Should(Succeed())
		Eventually(func(g Gomega) bool {
			namespaceList := corev1.NamespaceList{}
			g.Expect(k8sClient.List(ctx, &namespaceList)).Should(Succeed())
			for _, namespace := range namespaceList.Items {
				if namespace.GetName() == testobjects.YdbNamespace {
					return true
				}
			}
			return false
		}, Timeout, Interval).Should(BeTrue())
		InstallLocalOperatorWithHelm(testobjects.YdbNamespace)
	})

	It("Check webhook defaulter", func() {
		emptyStorageDefaultFields(storageSample)
		Expect(k8sClient.Create(ctx, storageSample)).Should(Succeed())
		defer DeleteStorageSafely(ctx, k8sClient, storageSample)

		emptyDatabaseDefaultFields(databaseSample)
		Expect(k8sClient.Create(ctx, databaseSample)).Should(Succeed())
		defer func() {
			Expect(k8sClient.Delete(ctx, databaseSample)).Should(Succeed())
		}()
	})

	It("Check webhook defaulter with dynconfig and nodeSets", func() {
		storageSample = testobjects.DefaultStorage(filepath.Join("..", "data", "storage-mirror-3-dc-dynconfig.yaml"))
		emptyStorageDefaultFields(storageSample)
		storageSample.Spec.NodeSets = []v1alpha1.StorageNodeSetSpecInline{
			{
				Name:            "storage-nodeset-1",
				StorageNodeSpec: v1alpha1.StorageNodeSpec{Nodes: 1},
			},
			{
				Name:            "storage-nodeset-2",
				StorageNodeSpec: v1alpha1.StorageNodeSpec{Nodes: 2},
			},
		}
		Expect(k8sClient.Create(ctx, storageSample)).Should(Succeed())
		defer DeleteStorageSafely(ctx, k8sClient, storageSample)
	})

	It("general smoke pipeline, create storage + database", func() {
		By("issuing create commands...")
		Expect(k8sClient.Create(ctx, storageSample)).Should(Succeed())
		defer DeleteStorageSafely(ctx, k8sClient, storageSample)
		Expect(k8sClient.Create(ctx, databaseSample)).Should(Succeed())
		defer func() {
			Expect(k8sClient.Delete(ctx, databaseSample)).Should(Succeed())
		}()

		By("waiting until Storage is ready...")
		WaitUntilStorageReady(ctx, k8sClient, storageSample.Name, testobjects.YdbNamespace)

		By("checking that all the storage pods are running and ready...")
		CheckPodsRunningAndReady(ctx, k8sClient, "ydb-cluster", "kind-storage", storageSample.Spec.Nodes)

		By("waiting until database is ready...")
		WaitUntilDatabaseReady(ctx, k8sClient, databaseSample.Name, testobjects.YdbNamespace)

		By("checking that all the database pods are running and ready...")
		CheckPodsRunningAndReady(ctx, k8sClient, "ydb-cluster", "kind-database", databaseSample.Spec.Nodes)

		database := v1alpha1.Database{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{
			Name:      databaseSample.Name,
			Namespace: testobjects.YdbNamespace,
		}, &database)).Should(Succeed())
		storageEndpoint := database.Spec.StorageEndpoint

		databasePods := corev1.PodList{}
		Expect(k8sClient.List(ctx, &databasePods, client.InNamespace(testobjects.YdbNamespace), client.MatchingLabels{
			"ydb-cluster": "kind-database",
		})).Should(Succeed())
		podName := databasePods.Items[0].Name

		By("bring YDB CLI inside ydb database pod...")
		BringYdbCliToPod(podName, testobjects.YdbNamespace)

		By("execute simple query inside ydb database pod...")
		databasePath := DatabasePathWithDefaultDomain(databaseSample)
		ExecuteSimpleTableE2ETest(podName, testobjects.YdbNamespace, storageEndpoint, databasePath)
	})

	It("pause and un-pause Storage, should destroy and bring up Pods", func() {
		By("issuing create commands...")
		Expect(k8sClient.Create(ctx, storageSample)).Should(Succeed())
		defer DeleteStorageSafely(ctx, k8sClient, storageSample)

		By("waiting until Storage is ready...")
		WaitUntilStorageReady(ctx, k8sClient, storageSample.Name, testobjects.YdbNamespace)

		By("checking that all the storage pods are running and ready...")
		CheckPodsRunningAndReady(ctx, k8sClient, "ydb-cluster", "kind-storage", storageSample.Spec.Nodes)

		By("setting storage pause to Paused...")
		storage := v1alpha1.Storage{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{
			Name:      storageSample.Name,
			Namespace: testobjects.YdbNamespace,
		}, &storage)).Should(Succeed())

		storage.Spec.Pause = true
		Expect(k8sClient.Update(ctx, &storage)).Should(Succeed())

		By("expecting all Pods to die...")
		storagePods := corev1.PodList{}
		Eventually(func(g Gomega) bool {
			g.Expect(k8sClient.List(ctx, &storagePods, client.InNamespace(testobjects.YdbNamespace), client.MatchingLabels{
				"ydb-cluster": "kind-storage",
			})).Should(Succeed())

			return len(storagePods.Items) == 0
		}, Timeout, Interval).Should(BeTrue())

		By("setting storage pause back to Running...")
		storage = v1alpha1.Storage{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{
			Name:      storageSample.Name,
			Namespace: testobjects.YdbNamespace,
		}, &storage)).Should(Succeed())

		storage.Spec.Pause = false
		Expect(k8sClient.Update(ctx, &storage)).Should(Succeed())

		By("expecting storage to become ready again...")
		WaitUntilStorageReady(ctx, k8sClient, storageSample.Name, testobjects.YdbNamespace)

		By("checking that all the storage pods are running and ready...")
		CheckPodsRunningAndReady(ctx, k8sClient, "ydb-cluster", "kind-storage", storageSample.Spec.Nodes)
	})

	It("freeze + delete StatefulSet + un-freeze Storage", func() {
		By("issuing create commands...")
		Expect(k8sClient.Create(ctx, storageSample)).Should(Succeed())
		defer DeleteStorageSafely(ctx, k8sClient, storageSample)

		By("waiting until Storage is ready...")
		WaitUntilStorageReady(ctx, k8sClient, storageSample.Name, testobjects.YdbNamespace)

		By("checking that all the storage pods are running and ready...")
		CheckPodsRunningAndReady(ctx, k8sClient, "ydb-cluster", "kind-storage", storageSample.Spec.Nodes)

		By("setting storage operatorSync to false...")
		storage := v1alpha1.Storage{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{
			Name:      storageSample.Name,
			Namespace: testobjects.YdbNamespace,
		}, &storage)).Should(Succeed())

		storage.Spec.OperatorSync = false
		Expect(k8sClient.Update(ctx, &storage)).Should(Succeed())

		By("expecting all Pods to stay alive for a while...")
		Consistently(func(g Gomega) bool {
			storagePods := corev1.PodList{}
			g.Expect(k8sClient.List(ctx, &storagePods, client.InNamespace(testobjects.YdbNamespace), client.MatchingLabels{
				"ydb-cluster": "kind-storage",
			})).Should(Succeed())

			return len(storagePods.Items) == int(storageSample.Spec.Nodes)
		}, test.Timeout, test.Interval).Should(BeTrue())

		By("deleting a StatefulSet...")
		statefulSet := v1.StatefulSet{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{
			Name:      storageSample.Name,
			Namespace: testobjects.YdbNamespace,
		}, &statefulSet)).Should(Succeed())
		Expect(k8sClient.Delete(ctx, &statefulSet)).Should(Succeed())

		By("storage pods must all go down...")
		Eventually(func(g Gomega) bool {
			storagePods := corev1.PodList{}
			g.Expect(k8sClient.List(ctx, &storagePods, client.InNamespace(testobjects.YdbNamespace), client.MatchingLabels{
				"ydb-cluster": "kind-storage",
			})).Should(Succeed())

			return len(storagePods.Items) == 0
		}, Timeout, Interval).Should(BeTrue())
		By("... and then storage pods must not restart for a while...")
		Consistently(func(g Gomega) bool {
			storagePods := corev1.PodList{}
			g.Expect(k8sClient.List(ctx, &storagePods, client.InNamespace(testobjects.YdbNamespace), client.MatchingLabels{
				"ydb-cluster": "kind-storage",
			})).Should(Succeed())

			return len(storagePods.Items) == 0
		}, test.Timeout, test.Interval).Should(BeTrue())

		By("setting storage freeze back to Running...")
		storage = v1alpha1.Storage{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{
			Name:      storageSample.Name,
			Namespace: testobjects.YdbNamespace,
		}, &storage)).Should(Succeed())

		storage.Spec.OperatorSync = true
		Expect(k8sClient.Update(ctx, &storage)).Should(Succeed())

		By("expecting storage to become ready again...")
		WaitUntilStorageReady(ctx, k8sClient, storageSample.Name, testobjects.YdbNamespace)

		By("checking that all the storage pods are running and ready...")
		CheckPodsRunningAndReady(ctx, k8sClient, "ydb-cluster", "kind-storage", storageSample.Spec.Nodes)
	})

	It("create storage and database with nodeSets", func() {
		By("issuing create commands...")
		storageSample = testobjects.DefaultStorage(filepath.Join("..", "data", "storage-mirror-3-dc-config.yaml"))
		testNodeSetName := "nodeset"
		for idx := 1; idx <= 3; idx++ {
			storageSample.Spec.NodeSets = append(storageSample.Spec.NodeSets, v1alpha1.StorageNodeSetSpecInline{
				Name: testNodeSetName + "-" + strconv.Itoa(idx),
				StorageNodeSpec: v1alpha1.StorageNodeSpec{
					Nodes: 1,
				},
			})
			databaseSample.Spec.NodeSets = append(databaseSample.Spec.NodeSets, v1alpha1.DatabaseNodeSetSpecInline{
				Name: testNodeSetName + "-" + strconv.Itoa(idx),
				DatabaseNodeSpec: v1alpha1.DatabaseNodeSpec{
					Nodes: 1,
				},
			})
		}
		Expect(k8sClient.Create(ctx, storageSample)).Should(Succeed())
		defer DeleteStorageSafely(ctx, k8sClient, storageSample)
		Expect(k8sClient.Create(ctx, databaseSample)).Should(Succeed())
		defer func() {
			Expect(k8sClient.Delete(ctx, databaseSample)).Should(Succeed())
		}()

		By("waiting until Storage is ready...")
		WaitUntilStorageReady(ctx, k8sClient, storageSample.Name, testobjects.YdbNamespace)

		By("checking that all the storage pods are running and ready...")
		CheckPodsRunningAndReady(ctx, k8sClient, "ydb-cluster", "kind-storage", storageSample.Spec.Nodes)

		By("waiting until database is ready...")
		WaitUntilDatabaseReady(ctx, k8sClient, databaseSample.Name, testobjects.YdbNamespace)

		By("checking that all the database pods are running and ready...")
		CheckPodsRunningAndReady(ctx, k8sClient, "ydb-cluster", "kind-database", databaseSample.Spec.Nodes)

		database := v1alpha1.Database{}
		databasePods := corev1.PodList{}
		By("modify nodeSetSpec inline to check inheritance...")
		Eventually(func(g Gomega) error {
			g.Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name:      databaseSample.Name,
				Namespace: testobjects.YdbNamespace,
			}, &database)).Should(Succeed())
			database.Spec.Nodes = 2
			database.Spec.NodeSets = []v1alpha1.DatabaseNodeSetSpecInline{
				{
					Name: testNodeSetName + "-" + strconv.Itoa(1),
					DatabaseNodeSpec: v1alpha1.DatabaseNodeSpec{
						Nodes: 1,
					},
				},
				{
					Name: testNodeSetName + "-" + strconv.Itoa(2),
					DatabaseNodeSpec: v1alpha1.DatabaseNodeSpec{
						Nodes: 1,
					},
				},
			}
			return k8sClient.Update(ctx, &database)
		}, Timeout, Interval).Should(BeNil())

		By("expecting databaseNodeSet pods deletion...")
		Eventually(func(g Gomega) bool {
			g.Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name:      databaseSample.Name,
				Namespace: testobjects.YdbNamespace,
			}, &database)).Should(Succeed())

			g.Expect(k8sClient.List(ctx, &databasePods, client.InNamespace(testobjects.YdbNamespace), client.MatchingLabels{
				"ydb-cluster": "kind-database",
			})).Should(Succeed())
			return len(databasePods.Items) == int(database.Spec.Nodes)
		}, Timeout, Interval).Should(BeTrue())

		storageEndpoint := database.Spec.StorageEndpoint
		Expect(k8sClient.List(ctx, &databasePods, client.InNamespace(testobjects.YdbNamespace), client.MatchingLabels{
			"ydb-cluster": "kind-database",
		})).Should(Succeed())
		podName := databasePods.Items[0].Name

		By("bring YDB CLI inside ydb database pod...")
		BringYdbCliToPod(podName, testobjects.YdbNamespace)

		By("execute simple query inside ydb database pod...")
		databasePath := DatabasePathWithDefaultDomain(databaseSample)
		ExecuteSimpleTableE2ETest(podName, testobjects.YdbNamespace, storageEndpoint, databasePath)
	})

	It("operatorConnection check, create storage with default staticCredentials", func() {
		By("issuing create commands...")
		storageSample = testobjects.DefaultStorage(filepath.Join("..", "data", "storage-mirror-3-dc-config-staticCreds.yaml"))
		storageSample.Spec.OperatorConnection = &v1alpha1.ConnectionOptions{
			StaticCredentials: &v1alpha1.StaticCredentialsAuth{
				Username: "root",
			},
		}
		Expect(k8sClient.Create(ctx, storageSample)).Should(Succeed())
		defer DeleteStorageSafely(ctx, k8sClient, storageSample)

		By("waiting until Storage is ready...")
		WaitUntilStorageReady(ctx, k8sClient, storageSample.Name, testobjects.YdbNamespace)

		By("checking that all the storage pods are running and ready...")
		CheckPodsRunningAndReady(ctx, k8sClient, "ydb-cluster", "kind-storage", storageSample.Spec.Nodes)
	})

	It("storage.State goes Pending -> Preparing -> Initializing -> Provisioning -> Ready", func() {
		Expect(k8sClient.Create(ctx, storageSample)).Should(Succeed())
		defer DeleteStorageSafely(ctx, k8sClient, storageSample)

		By("waiting until Storage is ready...")
		WaitUntilStorageReady(ctx, k8sClient, storageSample.Name, testobjects.YdbNamespace)

		By("tracking storage state changes...")
		events, err := clientset.CoreV1().Events(testobjects.YdbNamespace).List(context.Background(),
			metav1.ListOptions{TypeMeta: metav1.TypeMeta{Kind: "Storage"}})
		Expect(err).ShouldNot(HaveOccurred())

		allowedChanges := map[ClusterState]ClusterState{
			StoragePending:      StoragePreparing,
			StoragePreparing:    StorageInitializing,
			StorageInitializing: StorageProvisioning,
			StorageProvisioning: StorageReady,
		}

		re := regexp.MustCompile(`Storage moved from ([a-zA-Z]+) to ([a-zA-Z]+)`)
		for _, event := range events.Items {
			if event.Reason == "StatusChanged" {
				match := re.FindStringSubmatch(event.Message)
				Expect(allowedChanges[ClusterState(match[1])]).Should(BeEquivalentTo(ClusterState(match[2])))
			}
		}
	})

	FIt("using grpcs for storage connection", func() {
		By("create storage certificate secret...")
		storageCert := testobjects.StorageCertificate()
		Expect(k8sClient.Create(ctx, storageCert)).Should(Succeed())
		defer func() {
			Expect(k8sClient.Delete(ctx, storageCert)).Should(Succeed())
		}()
		By("create database certificate secret...")
		databaseCert := testobjects.DatabaseCertificate()
		Expect(k8sClient.Create(ctx, databaseCert)).Should(Succeed())
		defer func() {
			Expect(k8sClient.Delete(ctx, databaseCert)).Should(Succeed())
		}()

		By("create storage...")
		storageSample = testobjects.DefaultStorage(filepath.Join("..", "data", "storage-mirror-3-dc-config-tls.yaml"))
		storageSample.Spec.Service.GRPC.TLSConfiguration = testobjects.TLSConfiguration(
			testobjects.StorageCertificateSecretName,
		)
		Expect(k8sClient.Create(ctx, storageSample)).Should(Succeed())
		defer DeleteStorageSafely(ctx, k8sClient, storageSample)

		By("waiting until Storage is ready...")
		WaitUntilStorageReady(ctx, k8sClient, storageSample.Name, testobjects.YdbNamespace)

		By("checking that all the storage pods are running and ready...")
		CheckPodsRunningAndReady(ctx, k8sClient, "ydb-cluster", "kind-storage", storageSample.Spec.Nodes)

		By("create database...")
		databaseSample.Spec.Service.GRPC.TLSConfiguration = testobjects.TLSConfiguration(
			testobjects.DatabaseCertificateSecretName,
		)
		Expect(k8sClient.Create(ctx, databaseSample)).Should(Succeed())
		defer func() {
			Expect(k8sClient.Delete(ctx, databaseSample)).Should(Succeed())
		}()

		By("waiting until database is ready...")
		WaitUntilDatabaseReady(ctx, k8sClient, databaseSample.Name, testobjects.YdbNamespace)

		By("checking that all the database pods are running and ready...")
		CheckPodsRunningAndReady(ctx, k8sClient, "ydb-cluster", "kind-database", databaseSample.Spec.Nodes)

		database := v1alpha1.Database{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{
			Name:      databaseSample.Name,
			Namespace: testobjects.YdbNamespace,
		}, &database)).Should(Succeed())
		storageEndpoint := database.Spec.StorageEndpoint

		databasePods := corev1.PodList{}
		Expect(k8sClient.List(ctx, &databasePods,
			client.InNamespace(testobjects.YdbNamespace),
			client.MatchingLabels{"ydb-cluster": "kind-database"}),
		).Should(Succeed())
		podName := databasePods.Items[0].Name

		By("bring YDB CLI inside ydb database pod...")
		BringYdbCliToPod(podName, testobjects.YdbNamespace)

		By("execute simple query inside ydb database pod...")
		databasePath := DatabasePathWithDefaultDomain(databaseSample)
		ExecuteSimpleTableE2ETest(podName, testobjects.YdbNamespace, storageEndpoint, databasePath)
	})

	It("Check that Storage deleted after Database...", func() {
		By("create storage...")
		Expect(k8sClient.Create(ctx, storageSample)).Should(Succeed())
		defer DeleteStorageSafely(ctx, k8sClient, storageSample)

		By("create database...")
		Expect(k8sClient.Create(ctx, databaseSample)).Should(Succeed())

		By("waiting until Storage is ready...")
		WaitUntilStorageReady(ctx, k8sClient, storageSample.Name, testobjects.YdbNamespace)

		By("checking that all the storage pods are running and ready...")
		CheckPodsRunningAndReady(ctx, k8sClient, "ydb-cluster", "kind-storage", storageSample.Spec.Nodes)

		By("waiting until Database is ready...")
		WaitUntilDatabaseReady(ctx, k8sClient, databaseSample.Name, testobjects.YdbNamespace)

		By("checking that all the database pods are running and ready...")
		CheckPodsRunningAndReady(ctx, k8sClient, "ydb-cluster", "kind-database", databaseSample.Spec.Nodes)

		By("delete Storage...")
		Expect(k8sClient.Delete(ctx, storageSample)).Should(Succeed())

		By("checking that Storage deletionTimestamp is not nil...")
		Eventually(func() bool {
			foundStorage := v1alpha1.Storage{}
			err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      storageSample.Name,
				Namespace: testobjects.YdbNamespace,
			}, &foundStorage)
			if err != nil {
				return false
			}
			return !foundStorage.DeletionTimestamp.IsZero()
		}, Timeout, test.Interval).Should(BeTrue())

		By("checking that Storage is present in cluster...")
		Consistently(func() error {
			foundStorage := v1alpha1.Storage{}
			err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      storageSample.Name,
				Namespace: testobjects.YdbNamespace,
			}, &foundStorage)
			return err
		}, test.Timeout, test.Interval).ShouldNot(HaveOccurred())

		By("delete Database...")
		Expect(k8sClient.Delete(ctx, databaseSample)).Should(Succeed())

		By("checking that Storage deleted from cluster...")
		Eventually(func() bool {
			foundStorage := v1alpha1.Storage{}
			err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      storageSample.Name,
				Namespace: testobjects.YdbNamespace,
			}, &foundStorage)
			return apierrors.IsNotFound(err)
		}, Timeout, test.Interval).Should(BeTrue())
	})

	It("check storage with dynconfig", func() {
		By("create storage...")
		storageSample = testobjects.DefaultStorage(filepath.Join("..", "data", "storage-mirror-3-dc-dynconfig.yaml"))

		Expect(k8sClient.Create(ctx, storageSample)).Should(Succeed())
		defer DeleteStorageSafely(ctx, k8sClient, storageSample)

		storage := v1alpha1.Storage{}
		By("waiting until StorageInitialized condition is true...")
		Eventually(func(g Gomega) bool {
			g.Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name:      storageSample.Name,
				Namespace: testobjects.YdbNamespace,
			}, &storage)).Should(Succeed())

			condition := meta.FindStatusCondition(storage.Status.Conditions, StorageInitializedCondition)
			if condition != nil {
				return condition.Status == metav1.ConditionTrue
			}

			return false
		}, Timeout, Interval).Should(BeTrue())

		By("waiting until ReplaceConfig condition is true...")
		Eventually(func(g Gomega) bool {
			g.Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name:      storageSample.Name,
				Namespace: testobjects.YdbNamespace,
			}, &storage)).Should(Succeed())

			condition := meta.FindStatusCondition(storage.Status.Conditions, ReplaceConfigOperationCondition)
			if condition != nil && condition.ObservedGeneration == storage.Generation {
				return condition.Status == metav1.ConditionTrue
			}

			return false
		}, Timeout, Interval).Should(BeTrue())

		By("waiting until ConfigurationSynced condition is true...")
		Eventually(func(g Gomega) bool {
			g.Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name:      storageSample.Name,
				Namespace: testobjects.YdbNamespace,
			}, &storage)).Should(Succeed())

			condition := meta.FindStatusCondition(storage.Status.Conditions, ConfigurationSyncedCondition)
			if condition != nil && condition.ObservedGeneration == storage.Generation {
				return condition.Status == metav1.ConditionTrue
			}

			return false
		}, Timeout, Interval).Should(BeTrue())
	})

	FIt("TLS for status service", func() {
		tlsHTTPCheck := func(port int, serverName string) error {
			url := fmt.Sprintf("https://localhost:%d/", port)
			cert, err := os.ReadFile(testobjects.TestCAPath)
			Expect(err).ShouldNot(HaveOccurred())

			certPool := x509.NewCertPool()
			ok := certPool.AppendCertsFromPEM(cert)
			Expect(ok).To(BeTrue())

			tlsConfig := &tls.Config{
				MinVersion: tls.VersionTLS12,
				RootCAs:    certPool,
				ServerName: serverName,
			}

			transport := &http.Transport{TLSClientConfig: tlsConfig}
			client := &http.Client{
				Transport: transport,
				Timeout:   test.Timeout,
			}
			resp, err := client.Get(url)
			if err != nil {
				var netError net.Error
				var opError *net.OpError

				// for database: operator sets database ready status before the database is actually can server requests.
				if errors.As(err, &netError) && netError.Timeout() || errors.As(err, &opError) || errors.Is(err, io.EOF) {
					return err
				}

				Expect(err).ShouldNot(HaveOccurred())

			}

			Expect(resp).To(HaveHTTPStatus(http.StatusOK))

			return nil
		}

		By("create storage certificate secret...")
		storageCert := testobjects.StorageCertificate()
		Expect(k8sClient.Create(ctx, storageCert)).Should(Succeed())
		defer func() {
			Expect(k8sClient.Delete(ctx, storageCert)).Should(Succeed())
		}()
		By("create database certificate secret...")
		databaseCert := testobjects.DatabaseCertificate()
		Expect(k8sClient.Create(ctx, databaseCert)).Should(Succeed())
		defer func() {
			Expect(k8sClient.Delete(ctx, databaseCert)).Should(Succeed())
		}()

		By("create storage...")
		storageSample.Spec.Service.Status.TLSConfiguration = testobjects.TLSConfiguration(
			testobjects.StorageCertificateSecretName,
		)
		Expect(k8sClient.Create(ctx, storageSample)).Should(Succeed())
		defer DeleteStorageSafely(ctx, k8sClient, storageSample)

		By("create database...")
		databaseSample.Spec.Nodes = 1
		databaseSample.Spec.Service.Status.TLSConfiguration = testobjects.TLSConfiguration(
			testobjects.DatabaseCertificateSecretName,
		)
		Expect(k8sClient.Create(ctx, databaseSample)).Should(Succeed())
		defer func() {
			Expect(k8sClient.Delete(ctx, databaseSample)).Should(Succeed())
		}()

		By("waiting until Storage is ready...")
		WaitUntilStorageReady(ctx, k8sClient, storageSample.Name, testobjects.YdbNamespace)

		By("checking that all the storage pods are running and ready...")
		CheckPodsRunningAndReady(ctx, k8sClient, "ydb-cluster", "kind-storage", storageSample.Spec.Nodes)

		By("forward storage status port and check that we can check TLS response")
		PortForward(ctx,
			fmt.Sprintf(resources.StatusServiceNameFormat, storageSample.Name), storageSample.Namespace,
			"storage-grpc.ydb.svc.cluster.local", v1alpha1.StatusPort, tlsHTTPCheck,
		)

		By("waiting until database is ready...")
		WaitUntilDatabaseReady(ctx, k8sClient, databaseSample.Name, testobjects.YdbNamespace)

		By("checking that all the database pods are running and ready...")
		CheckPodsRunningAndReady(ctx, k8sClient, "ydb-cluster", "kind-database", databaseSample.Spec.Nodes)

		By("forward database status port and check that we can check TLS response")
		PortForward(ctx,
			fmt.Sprintf(resources.StatusServiceNameFormat, databaseSample.Name), databaseSample.Namespace,
			"database-grpc.ydb.svc.cluster.local", v1alpha1.StatusPort, tlsHTTPCheck,
		)
	})

	It("Check encryption for Database", func() {
		By("create storage...")
		Expect(k8sClient.Create(ctx, storageSample)).Should(Succeed())
		defer DeleteStorageSafely(ctx, k8sClient, storageSample)
		By("create database...")
		databaseSample.Spec.Encryption = &v1alpha1.EncryptionConfig{
			Enabled: true,
		}
		Expect(k8sClient.Create(ctx, databaseSample)).Should(Succeed())
		defer func() {
			Expect(k8sClient.Delete(ctx, databaseSample)).Should(Succeed())
		}()

		By("waiting until Storage is ready...")
		WaitUntilStorageReady(ctx, k8sClient, storageSample.Name, testobjects.YdbNamespace)

		By("checking that all the storage pods are running and ready...")
		CheckPodsRunningAndReady(ctx, k8sClient, "ydb-cluster", "kind-storage", storageSample.Spec.Nodes)

		By("waiting until database is ready...")
		WaitUntilDatabaseReady(ctx, k8sClient, databaseSample.Name, testobjects.YdbNamespace)

		By("checking that all the database pods are running and ready...")
		CheckPodsRunningAndReady(ctx, k8sClient, "ydb-cluster", "kind-database", databaseSample.Spec.Nodes)

		database := v1alpha1.Database{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{
			Name:      databaseSample.Name,
			Namespace: testobjects.YdbNamespace,
		}, &database)).Should(Succeed())
		storageEndpoint := database.Spec.StorageEndpoint

		databasePods := corev1.PodList{}
		Expect(k8sClient.List(ctx, &databasePods,
			client.InNamespace(testobjects.YdbNamespace),
			client.MatchingLabels{"ydb-cluster": "kind-database"}),
		).Should(Succeed())
		podName := databasePods.Items[0].Name

		By("bring YDB CLI inside ydb database pod...")
		BringYdbCliToPod(podName, testobjects.YdbNamespace)

		By("execute simple query inside ydb database pod...")
		databasePath := DatabasePathWithDefaultDomain(databaseSample)
		ExecuteSimpleTableE2ETest(podName, testobjects.YdbNamespace, storageEndpoint, databasePath)
	})

	AfterEach(func() {
		UninstallOperatorWithHelm(testobjects.YdbNamespace)
		Expect(k8sClient.Delete(ctx, &namespace)).Should(Succeed())
		Eventually(func(g Gomega) bool {
			namespaceList := corev1.NamespaceList{}
			g.Expect(k8sClient.List(ctx, &namespaceList)).Should(Succeed())
			for _, namespace := range namespaceList.Items {
				if namespace.GetName() == testobjects.YdbNamespace {
					return false
				}
			}
			return true
		}, Timeout, Interval).Should(BeTrue())
	})
})
