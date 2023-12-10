package tests

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v1alpha1 "github.com/ydb-platform/ydb-kubernetes-operator/api/v1alpha1"
	testobjects "github.com/ydb-platform/ydb-kubernetes-operator/e2e/tests/test-objects"
	. "github.com/ydb-platform/ydb-kubernetes-operator/internal/controllers/constants" //nolint:revive
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
		Expect(installOperatorWithHelm(testobjects.YdbNamespace)).Should(BeTrue())
		time.Sleep(time.Second * 10)
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

		storage.Spec.Pause = PausedState
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

		storage.Spec.Pause = RunningState
		Expect(k8sClient.Update(ctx, &storage)).Should(Succeed())

		By("expecting storage to become ready again...")
		waitUntilStorageReady(ctx, storageSample.Name, testobjects.YdbNamespace)

		By("checking that all the storage pods are running and ready...")
		checkPodsRunningAndReady(ctx, "ydb-cluster", "kind-storage", storageSample.Spec.Nodes)
	})

	It("pause and un-pause Database, should destroy and bring up Pods", func() {
		By("issuing create commands...")
		Expect(k8sClient.Create(ctx, storageSample)).Should(Succeed())
		defer func() {
			Expect(k8sClient.Delete(ctx, storageSample)).Should(Succeed())
		}()
		Expect(k8sClient.Create(ctx, databaseSample)).Should(Succeed())
		defer func() {
			Expect(k8sClient.Delete(ctx, databaseSample)).Should(Succeed())
		}()

		By("waiting until database is ready...")
		waitUntilDatabaseReady(ctx, databaseSample.Name, testobjects.YdbNamespace)

		By("checking that all the database pods are running and ready...")
		checkPodsRunningAndReady(ctx, "ydb-cluster", "kind-database", databaseSample.Spec.Nodes)

		By("setting database pause to Paused...")
		database := v1alpha1.Database{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{
			Name:      databaseSample.Name,
			Namespace: testobjects.YdbNamespace,
		}, &database)).Should(Succeed())
		database.Spec.Pause = PausedState
		Expect(k8sClient.Update(ctx, &database)).Should(Succeed())

		By("expecting all Pods to die...")
		databasePods := corev1.PodList{}
		Eventually(func(g Gomega) bool {
			g.Expect(k8sClient.List(ctx, &databasePods, client.InNamespace(testobjects.YdbNamespace), client.MatchingLabels{
				"ydb-cluster": "kind-database",
			})).Should(Succeed())

			return len(databasePods.Items) == 0
		}, Timeout, Interval).Should(BeTrue())

		By("setting database pause back to Running...")
		database = v1alpha1.Database{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{
			Name:      databaseSample.Name,
			Namespace: testobjects.YdbNamespace,
		}, &database)).Should(Succeed())
		database.Spec.Pause = RunningState
		Expect(k8sClient.Update(ctx, &database)).Should(Succeed())

		By("expecting database to become ready again...")
		waitUntilDatabaseReady(ctx, databaseSample.Name, testobjects.YdbNamespace)

		By("checking that all the database pods are running and ready...")
		checkPodsRunningAndReady(ctx, "ydb-cluster", "kind-database", databaseSample.Spec.Nodes)
	})

	It("operatorConnection check, create storage with default staticCredentials", func() {
		By("issuing create commands...")
		storageSample = testobjects.StorageWithStaticCredentials(filepath.Join(".", "data", "storage-block-4-2-config-staticCreds.yaml"))
		Expect(k8sClient.Create(ctx, storageSample)).Should(Succeed())
		defer func() {
			Expect(k8sClient.Delete(ctx, storageSample)).Should(Succeed())
		}()

		By("waiting until Storage is ready...")
		waitUntilStorageReady(ctx, storageSample.Name, testobjects.YdbNamespace)

		By("checking that all the storage pods are running and ready...")
		checkPodsRunningAndReady(ctx, "ydb-cluster", "kind-database", databaseSample.Spec.Nodes)
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
		Eventually(func() metav1.StatusReason {
			key := client.ObjectKeyFromObject(&namespace)
			if err := k8sClient.Get(ctx, key, &namespace); err != nil {
				return apierrors.ReasonForError(err)
			}
			return ""
		}, Timeout, Interval).Should(Equal(metav1.StatusReasonNotFound))
	})
})
