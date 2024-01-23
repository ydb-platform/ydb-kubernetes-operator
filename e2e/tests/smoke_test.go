package tests

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v1alpha1 "github.com/ydb-platform/ydb-kubernetes-operator/api/v1alpha1"
	testobjects "github.com/ydb-platform/ydb-kubernetes-operator/e2e/tests/test-objects"
	. "github.com/ydb-platform/ydb-kubernetes-operator/internal/controllers/constants"
)

const (
	Timeout  = time.Second * 600
	Interval = time.Second * 5
)

var HostPathDirectoryType corev1.HostPathType = "Directory"

func podIsReady(conditions []corev1.PodCondition) bool {
	for _, condition := range conditions {
		if condition.Type == testobjects.ReadyStatus && condition.Status == "True" {
			return true
		}
	}
	return false
}

func execInPod(namespace string, name string, cmd []string) (string, error) {
	args := []string{
		"-n",
		namespace,
		"exec",
		name,
		"--",
	}
	args = append(args, cmd...)
	result := exec.Command("kubectl", args...)
	stdout, err := result.Output()
	return string(stdout), err
}

func bringYdbCliToPod(namespace string, name string, ydbHome string) error {
	args := []string{
		"-n",
		namespace,
		"cp",
		fmt.Sprintf("%v/ydb/bin/ydb", os.ExpandEnv("$HOME")),
		fmt.Sprintf("%v:%v/ydb", name, ydbHome),
	}
	result := exec.Command("kubectl", args...)
	_, err := result.Output()
	return err
}

func installOperatorWithHelm(namespace string) bool {
	args := []string{
		"-n",
		namespace,
		"install",
		"--wait",
		"ydb-operator",
		filepath.Join("..", "..", "deploy", "ydb-operator"),
		"-f",
		filepath.Join("..", "operator-values.yaml"),
	}
	result := exec.Command("helm", args...)
	stdout, err := result.Output()
	if err != nil {
		return false
	}

	return strings.Contains(string(stdout), "deployed")
}

func uninstallOperatorWithHelm(namespace string) bool {
	args := []string{
		"-n",
		namespace,
		"uninstall",
		"--wait",
		"ydb-operator",
	}
	result := exec.Command("helm", args...)
	stdout, err := result.Output()
	if err != nil {
		return false
	}

	return strings.Contains(string(stdout), "uninstalled")
}

func waitUntilStorageReady(ctx context.Context, storageName, storageNamespace string) {
	storage := v1alpha1.Storage{}
	Eventually(func(g Gomega) bool {
		g.Expect(k8sClient.Get(ctx, types.NamespacedName{
			Name:      storageName,
			Namespace: storageNamespace,
		}, &storage)).Should(Succeed())

		return meta.IsStatusConditionPresentAndEqual(
			storage.Status.Conditions,
			StorageInitializedCondition,
			metav1.ConditionTrue,
		) && storage.Status.State == testobjects.ReadyStatus
	}, Timeout, Interval).Should(BeTrue())
}

func waitUntilDatabaseReady(ctx context.Context, databaseName, databaseNamespace string) {
	database := v1alpha1.Database{}
	Eventually(func(g Gomega) bool {
		g.Expect(k8sClient.Get(ctx, types.NamespacedName{
			Name:      databaseName,
			Namespace: databaseNamespace,
		}, &database)).Should(Succeed())

		return meta.IsStatusConditionPresentAndEqual(
			database.Status.Conditions,
			DatabaseTenantInitializedCondition,
			metav1.ConditionTrue,
		) && database.Status.State == testobjects.ReadyStatus
	}, Timeout, Interval).Should(BeTrue())
}

func checkPodsRunningAndReady(ctx context.Context, podLabelKey, podLabelValue string, nPods int32) {
	Eventually(func(g Gomega) bool {
		pods := corev1.PodList{}
		g.Expect(k8sClient.List(ctx, &pods, client.InNamespace(testobjects.YdbNamespace), client.MatchingLabels{
			podLabelKey: podLabelValue,
		})).Should(Succeed())
		g.Expect(len(pods.Items)).Should(BeEquivalentTo(nPods))
		for _, pod := range pods.Items {
			g.Expect(pod.Status.Phase).To(BeEquivalentTo("Running"))
			g.Expect(podIsReady(pod.Status.Conditions)).To(BeTrue())
		}
		return true
	}, Timeout, Interval).Should(BeTrue())
}

var _ = Describe("Operator smoke test", func() {
	var ctx context.Context
	var namespace corev1.Namespace

	var storageSample *v1alpha1.Storage
	var databaseSample *v1alpha1.Database

	BeforeEach(func() {
		storageSample = testobjects.DefaultStorage(filepath.Join(".", "data", "storage-block-4-2-config.yaml"))
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
		Expect(installOperatorWithHelm(testobjects.YdbNamespace)).Should(BeTrue())
	})

	It("general smoke pipeline, create storage + database", func() {
		By("issuing create commands...")
		Expect(k8sClient.Create(ctx, storageSample)).Should(Succeed())
		defer func() {
			Expect(k8sClient.Delete(ctx, storageSample)).Should(Succeed())
		}()
		Expect(k8sClient.Create(ctx, databaseSample)).Should(Succeed())
		defer func() {
			Expect(k8sClient.Delete(ctx, databaseSample)).Should(Succeed())
		}()

		By("waiting until Storage is ready...")
		waitUntilStorageReady(ctx, storageSample.Name, testobjects.YdbNamespace)

		By("checking that all the storage pods are running and ready...")
		checkPodsRunningAndReady(ctx, "ydb-cluster", "kind-storage", storageSample.Spec.Nodes)

		By("waiting until database is ready...")
		waitUntilDatabaseReady(ctx, databaseSample.Name, testobjects.YdbNamespace)

		By("checking that all the database pods are running and ready...")
		checkPodsRunningAndReady(ctx, "ydb-cluster", "kind-database", databaseSample.Spec.Nodes)

		databasePods := corev1.PodList{}
		err := k8sClient.List(ctx, &databasePods, client.InNamespace(testobjects.YdbNamespace), client.MatchingLabels{
			"ydb-cluster": "kind-database",
		})
		Expect(err).To(BeNil())
		firstDBPod := databasePods.Items[0].Name

		Expect(bringYdbCliToPod(testobjects.YdbNamespace, firstDBPod, testobjects.YdbHome)).To(Succeed())

		Eventually(func(g Gomega) {
			out, err := execInPod(testobjects.YdbNamespace, firstDBPod, []string{
				fmt.Sprintf("%v/ydb", testobjects.YdbHome),
				"-d",
				"/" + testobjects.DefaultDomain,
				"-e",
				"grpc://localhost:2135",
				"yql",
				"-s",
				"select 1",
			})

			g.Expect(err).To(BeNil())

			// `yql` gives output in the following format:
			// ┌─────────┐
			// | column0 |
			// ├─────────┤
			// | 1       |
			// └─────────┘
			g.Expect(strings.ReplaceAll(out, "\n", "")).
				To(MatchRegexp(".*column0.*1.*"))
		})
	})

	It("pause and un-pause Storage, should destroy and bring up Pods", func() {
		By("issuing create commands...")
		Expect(k8sClient.Create(ctx, storageSample)).Should(Succeed())
		defer func() {
			Expect(k8sClient.Delete(ctx, storageSample)).Should(Succeed())
		}()

		By("waiting until Storage is ready...")
		waitUntilStorageReady(ctx, storageSample.Name, testobjects.YdbNamespace)

		By("checking that all the storage pods are running and ready...")
		checkPodsRunningAndReady(ctx, "ydb-cluster", "kind-storage", storageSample.Spec.Nodes)

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
		waitUntilStorageReady(ctx, storageSample.Name, testobjects.YdbNamespace)

		By("checking that all the storage pods are running and ready...")
		checkPodsRunningAndReady(ctx, "ydb-cluster", "kind-storage", storageSample.Spec.Nodes)
	})

	It("freeze + delete StatefulSet + un-freeze Storage", func() {
		By("issuing create commands...")
		Expect(k8sClient.Create(ctx, storageSample)).Should(Succeed())
		defer func() {
			Expect(k8sClient.Delete(ctx, storageSample)).Should(Succeed())
		}()

		By("waiting until Storage is ready...")
		waitUntilStorageReady(ctx, storageSample.Name, testobjects.YdbNamespace)

		By("checking that all the storage pods are running and ready...")
		checkPodsRunningAndReady(ctx, "ydb-cluster", "kind-storage", storageSample.Spec.Nodes)

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
		}, 20*time.Second, Interval).Should(BeTrue())

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
		}, 20*time.Second, Interval).Should(BeTrue())

		By("setting storage freeze back to Running...")
		storage = v1alpha1.Storage{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{
			Name:      storageSample.Name,
			Namespace: testobjects.YdbNamespace,
		}, &storage)).Should(Succeed())

		storage.Spec.OperatorSync = true
		Expect(k8sClient.Update(ctx, &storage)).Should(Succeed())

		By("expecting storage to become ready again...")
		waitUntilStorageReady(ctx, storageSample.Name, testobjects.YdbNamespace)

		By("checking that all the storage pods are running and ready...")
		checkPodsRunningAndReady(ctx, "ydb-cluster", "kind-storage", storageSample.Spec.Nodes)

		By("database can be healthily created after Frozen storage...")
		Expect(k8sClient.Create(ctx, databaseSample)).Should(Succeed())
		defer func() {
			Expect(k8sClient.Delete(ctx, databaseSample)).Should(Succeed())
		}()
		By("waiting until database is ready...")
		waitUntilDatabaseReady(ctx, databaseSample.Name, testobjects.YdbNamespace)
		By("checking that all the database pods are running and ready...")
		checkPodsRunningAndReady(ctx, "ydb-cluster", "kind-database", databaseSample.Spec.Nodes)
	})

	It("create storage and database with nodeSets", func() {
		By("issuing create commands...")
		storageSample = testobjects.DefaultStorage(filepath.Join(".", "data", "storage-block-4-2-config-nodeSets.yaml"))
		databaseSample = testobjects.DefaultDatabase()
		testNodeSetName := "nodeset"
		for idx := 1; idx <= 2; idx++ {
			storageSample.Spec.NodeSets = append(storageSample.Spec.NodeSets, v1alpha1.StorageNodeSetSpecInline{
				Name: testNodeSetName + "-" + strconv.Itoa(idx),
				StorageNodeSpec: v1alpha1.StorageNodeSpec{
					Nodes: 4,
				},
			})
			databaseSample.Spec.NodeSets = append(databaseSample.Spec.NodeSets, v1alpha1.DatabaseNodeSetSpecInline{
				Name: testNodeSetName + "-" + strconv.Itoa(idx),
				DatabaseNodeSpec: v1alpha1.DatabaseNodeSpec{
					Nodes: 4,
				},
			})
		}
		Expect(k8sClient.Create(ctx, storageSample)).Should(Succeed())
		defer func() {
			Expect(k8sClient.Delete(ctx, storageSample)).Should(Succeed())
		}()
		Expect(k8sClient.Create(ctx, databaseSample)).Should(Succeed())
		defer func() {
			Expect(k8sClient.Delete(ctx, databaseSample)).Should(Succeed())
		}()

		By("waiting until Storage is ready...")
		waitUntilStorageReady(ctx, storageSample.Name, testobjects.YdbNamespace)

		By("checking that all the storage pods are running and ready...")
		checkPodsRunningAndReady(ctx, "ydb-cluster", "kind-storage", storageSample.Spec.Nodes)

		By("waiting until database is ready...")
		waitUntilDatabaseReady(ctx, databaseSample.Name, testobjects.YdbNamespace)

		By("checking that all the database pods are running and ready...")
		checkPodsRunningAndReady(ctx, "ydb-cluster", "kind-database", databaseSample.Spec.Nodes)

		By("delete nodeSetSpec inline to check inheritance...")
		database := v1alpha1.Database{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{
			Name:      databaseSample.Name,
			Namespace: testobjects.YdbNamespace,
		}, &database)).Should(Succeed())
		database.Spec.Nodes = 4
		database.Spec.NodeSets = []v1alpha1.DatabaseNodeSetSpecInline{
			{
				Name: testNodeSetName + "-" + strconv.Itoa(1),
				DatabaseNodeSpec: v1alpha1.DatabaseNodeSpec{
					Nodes: 4,
				},
			},
		}
		Expect(k8sClient.Update(ctx, &database)).Should(Succeed())

		By("check that ObservedDatabaseGeneration changed...")
		Eventually(func(g Gomega) bool {
			database := v1alpha1.Database{}
			g.Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name:      databaseSample.Name,
				Namespace: testobjects.YdbNamespace,
			}, &database)).Should(Succeed())

			databaseNodeSetList := v1alpha1.DatabaseNodeSetList{}
			g.Expect(k8sClient.List(ctx, &databaseNodeSetList,
				client.InNamespace(testobjects.YdbNamespace),
			)).Should(Succeed())
			for _, databaseNodeSet := range databaseNodeSetList.Items {
				if database.GetGeneration() != databaseNodeSet.Status.ObservedDatabaseGeneration {
					return false
				}
			}
			return true
		}, Timeout, Interval).Should(BeTrue())

		By("expecting databaseNodeSet pods deletion...")
		Eventually(func(g Gomega) bool {
			database := v1alpha1.Database{}
			g.Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name:      databaseSample.Name,
				Namespace: testobjects.YdbNamespace,
			}, &database)).Should(Succeed())

			databasePods := corev1.PodList{}
			g.Expect(k8sClient.List(ctx, &databasePods, client.InNamespace(testobjects.YdbNamespace), client.MatchingLabels{
				"ydb-cluster": "kind-database",
			})).Should(Succeed())
			return len(databasePods.Items) == int(database.Spec.Nodes)
		}, Timeout, Interval).Should(BeTrue())

		By("execute simple query inside ydb database pod...")
		databasePods := corev1.PodList{}
		err := k8sClient.List(ctx, &databasePods,
			client.InNamespace(testobjects.YdbNamespace),
			client.MatchingLabels{
				"ydb-cluster": "kind-database",
			})
		Expect(err).To(BeNil())
		firstDBPod := databasePods.Items[0].Name

		Expect(bringYdbCliToPod(testobjects.YdbNamespace, firstDBPod, testobjects.YdbHome)).To(Succeed())

		Eventually(func(g Gomega) {
			out, err := execInPod(testobjects.YdbNamespace, firstDBPod, []string{
				fmt.Sprintf("%v/ydb", testobjects.YdbHome),
				"-d",
				"/" + testobjects.DefaultDomain,
				"-e",
				"grpc://localhost:2135",
				"yql",
				"-s",
				"select 1",
			})

			g.Expect(err).To(BeNil())

			// `yql` gives output in the following format:
			// ┌─────────┐
			// | column0 |
			// ├─────────┤
			// | 1       |
			// └─────────┘
			g.Expect(strings.ReplaceAll(out, "\n", "")).
				To(MatchRegexp(".*column0.*1.*"))
		})
	})

	It("operatorConnection check, create storage with default staticCredentials", func() {
		By("issuing create commands...")
		storageSample = testobjects.DefaultStorage(filepath.Join(".", "data", "storage-block-4-2-config-staticCreds.yaml"))
		storageSample.Spec.OperatorConnection = &v1alpha1.ConnectionOptions{
			StaticCredentials: &v1alpha1.StaticCredentialsAuth{
				Username: "root",
			},
		}
		Expect(k8sClient.Create(ctx, storageSample)).Should(Succeed())
		defer func() {
			Expect(k8sClient.Delete(ctx, storageSample)).Should(Succeed())
		}()

		By("waiting until Storage is ready...")
		waitUntilStorageReady(ctx, storageSample.Name, testobjects.YdbNamespace)

		By("checking that all the storage pods are running and ready...")
		checkPodsRunningAndReady(ctx, "ydb-cluster", "kind-storage", storageSample.Spec.Nodes)
	})

	It("storage.State goes Pending -> Preparing -> Provisioning -> Initializing -> Ready", func() {
		Expect(k8sClient.Create(ctx, storageSample)).Should(Succeed())
		defer func() {
			Expect(k8sClient.Delete(ctx, storageSample)).Should(Succeed())
		}()

		By("waiting until Storage is ready...")
		waitUntilStorageReady(ctx, storageSample.Name, testobjects.YdbNamespace)

		By("tracking storage state changes...")
		events, err := clientset.CoreV1().Events(testobjects.YdbNamespace).List(context.Background(),
			metav1.ListOptions{TypeMeta: metav1.TypeMeta{Kind: "Storage"}})
		Expect(err).ToNot(HaveOccurred())

		allowedChanges := map[ClusterState]ClusterState{
			StoragePending:      StoragePreparing,
			StoragePreparing:    StorageProvisioning,
			StorageProvisioning: StorageInitializing,
			StorageInitializing: StorageReady,
		}

		re := regexp.MustCompile(`Storage moved from ([a-zA-Z]+) to ([a-zA-Z]+)`)
		for _, event := range events.Items {
			if event.Reason == "StatusChanged" {
				match := re.FindStringSubmatch(event.Message)
				Expect(allowedChanges[ClusterState(match[1])]).To(BeEquivalentTo(ClusterState(match[2])))
			}
		}
	})

	AfterEach(func() {
		Expect(uninstallOperatorWithHelm(testobjects.YdbNamespace)).Should(BeTrue())
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
