package tests

import (
	"bufio"
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
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
	"github.com/ydb-platform/ydb-kubernetes-operator/internal/resources"
)

const (
	Timeout  = time.Second * 600
	Interval = time.Second * 5
)

func podIsReady(conditions []corev1.PodCondition) bool {
	for _, condition := range conditions {
		if condition.Type == testobjects.ReadyStatus && condition.Status == "True" {
			return true
		}
	}
	return false
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
			DatabaseInitializedCondition,
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
			g.Expect(pod.Status.Phase).Should(BeEquivalentTo("Running"))
			g.Expect(podIsReady(pod.Status.Conditions)).Should(BeTrue())
		}
		return true
	}, Timeout, Interval).Should(BeTrue())

	Consistently(func(g Gomega) bool {
		pods := corev1.PodList{}
		g.Expect(k8sClient.List(ctx, &pods, client.InNamespace(testobjects.YdbNamespace), client.MatchingLabels{
			podLabelKey: podLabelValue,
		})).Should(Succeed())
		g.Expect(len(pods.Items)).Should(BeEquivalentTo(nPods))
		for _, pod := range pods.Items {
			g.Expect(pod.Status.Phase).Should(BeEquivalentTo("Running"))
			g.Expect(podIsReady(pod.Status.Conditions)).Should(BeTrue())
		}
		return true
	}, 30*time.Second, Interval).Should(BeTrue())
}

func bringYdbCliToPod(podName, podNamespace string) {
	Eventually(func(g Gomega) error {
		args := []string{
			"-n",
			podNamespace,
			"cp",
			fmt.Sprintf("%v/ydb/bin/ydb", os.ExpandEnv("$HOME")),
			fmt.Sprintf("%v:/tmp/ydb", podName),
		}
		cmd := exec.Command("kubectl", args...)
		return cmd.Run()
	}, Timeout, Interval).Should(BeNil())
}

func executeSimpleQuery(ctx context.Context, podName, podNamespace, storageEndpoint string) {
	Eventually(func(g Gomega) string {
		args := []string{
			"-n",
			podNamespace,
			"exec",
			podName,
			"--",
			"/tmp/ydb",
			"-d",
			"/" + testobjects.DefaultDomain,
			"-e" + storageEndpoint,
			"yql",
			"-s",
			"select 1",
		}
		output, err := exec.Command("kubectl", args...).Output()
		g.Expect(err).ShouldNot(HaveOccurred())

		// `yql` gives output in the following format:
		// ┌─────────┐
		// | column0 |
		// ├─────────┤
		// | 1       |
		// └─────────┘
		return strings.ReplaceAll(string(output), "\n", "")
	}, Timeout, Interval).Should(MatchRegexp(".*column0.*1.*"))
}

func portForward(ctx context.Context, svcName string, svcNamespace string, port int, f func(int) error) {
	Eventually(func(g Gomega) error {
		args := []string{
			"-n", svcNamespace,
			"port-forward",
			fmt.Sprintf("svc/%s", svcName),
			fmt.Sprintf(":%d", port),
		}

		cmd := exec.CommandContext(ctx, "kubectl", args...)
		stdout, err := cmd.StdoutPipe()
		if err != nil {
			return err
		}

		stderr, err := cmd.StderrPipe()
		if err != nil {
			return err
		}

		if err = cmd.Start(); err != nil {
			return err
		}

		defer func() {
			err := cmd.Process.Kill()
			if err != nil {
				_, _ = fmt.Fprintf(GinkgoWriter, "Unable to kill process: %s", err)
			}
		}()

		localPort := 0

		scanner := bufio.NewScanner(stdout)
		portForwardRegex := regexp.MustCompile(`Forwarding from 127.0.0.1:(\d+) ->`)

		for scanner.Scan() {
			line := scanner.Text()

			matches := portForwardRegex.FindStringSubmatch(line)
			if matches != nil {
				localPort, err = strconv.Atoi(matches[1])
				if err != nil {
					return err
				}
				break
			}
		}

		if localPort != 0 {
			if err = f(localPort); err != nil {
				return err
			}
		} else {
			content, _ := io.ReadAll(stderr)

			return fmt.Errorf("kubectl port-forward stderr: %s", content)
		}
		return nil
	}, 60*time.Second, Interval).Should(BeNil())
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

	It("Check webhook defaulter", func() {
		storageSample.Spec.Image = nil
		storageSample.Spec.Resources = nil
		storageSample.Spec.Service = nil
		storageSample.Spec.Monitoring = nil
		Expect(k8sClient.Create(ctx, storageSample)).Should(Succeed())
		defer func() {
			Expect(k8sClient.Delete(ctx, storageSample)).Should(Succeed())
		}()

		databaseSample.Spec.StorageClusterRef.Namespace = ""
		databaseSample.Spec.Image = nil
		databaseSample.Spec.Service = nil
		databaseSample.Spec.Domain = ""
		databaseSample.Spec.Path = ""
		databaseSample.Spec.Encryption = nil
		databaseSample.Spec.Datastreams = nil
		databaseSample.Spec.Monitoring = nil
		databaseSample.Spec.StorageEndpoint = ""
		Expect(k8sClient.Create(ctx, databaseSample)).Should(Succeed())
		defer func() {
			Expect(k8sClient.Delete(ctx, databaseSample)).Should(Succeed())
		}()
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
		bringYdbCliToPod(podName, testobjects.YdbNamespace)

		By("execute simple query inside ydb database pod...")
		executeSimpleQuery(ctx, podName, testobjects.YdbNamespace, storageEndpoint)
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

		/*
			// This test suite attempts to create a database on uninitialised storage

			By("database can be healthily created after Frozen storage...")
			Expect(k8sClient.Create(ctx, databaseSample)).Should(Succeed())
			defer func() {
				Expect(k8sClient.Delete(ctx, databaseSample)).Should(Succeed())
			}()
			By("waiting until database is ready...")
			waitUntilDatabaseReady(ctx, databaseSample.Name, testobjects.YdbNamespace)
			By("checking that all the database pods are running and ready...")
			checkPodsRunningAndReady(ctx, "ydb-cluster", "kind-database", databaseSample.Spec.Nodes)
		*/
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

		database := v1alpha1.Database{}
		databasePods := corev1.PodList{}
		By("delete nodeSetSpec inline to check inheritance...")
		Eventually(func(g Gomega) error {
			g.Expect(k8sClient.Get(ctx, types.NamespacedName{
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
		bringYdbCliToPod(podName, testobjects.YdbNamespace)

		By("execute simple query inside ydb database pod...")
		executeSimpleQuery(ctx, podName, testobjects.YdbNamespace, storageEndpoint)
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

	It("storage.State goes Pending -> Preparing -> Initializing -> Provisioning -> Ready", func() {
		Expect(k8sClient.Create(ctx, storageSample)).Should(Succeed())
		defer func() {
			Expect(k8sClient.Delete(ctx, storageSample)).Should(Succeed())
		}()

		By("waiting until Storage is ready...")
		waitUntilStorageReady(ctx, storageSample.Name, testobjects.YdbNamespace)

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

	It("using grpcs for storage connection", func() {
		By("create secret...")
		cert := testobjects.DefaultCertificate(
			filepath.Join(".", "data", "tls.crt"),
			filepath.Join(".", "data", "tls.key"),
			filepath.Join(".", "data", "ca.crt"),
		)
		Expect(k8sClient.Create(ctx, cert)).Should(Succeed())
		defer func() {
			Expect(k8sClient.Delete(ctx, cert)).Should(Succeed())
		}()

		By("create storage...")
		storageSample = testobjects.DefaultStorage(filepath.Join(".", "data", "storage-block-4-2-config-tls.yaml"))
		storageSample.Spec.Service.GRPC.TLSConfiguration.Enabled = true
		storageSample.Spec.Service.GRPC.TLSConfiguration.Certificate = corev1.SecretKeySelector{
			LocalObjectReference: corev1.LocalObjectReference{Name: testobjects.CertificateSecretName},
			Key:                  "tls.crt",
		}
		storageSample.Spec.Service.GRPC.TLSConfiguration.Key = corev1.SecretKeySelector{
			LocalObjectReference: corev1.LocalObjectReference{Name: testobjects.CertificateSecretName},
			Key:                  "tls.key",
		}
		storageSample.Spec.Service.GRPC.TLSConfiguration.CertificateAuthority = corev1.SecretKeySelector{
			LocalObjectReference: corev1.LocalObjectReference{Name: testobjects.CertificateSecretName},
			Key:                  "ca.crt",
		}

		Expect(k8sClient.Create(ctx, storageSample)).Should(Succeed())
		defer func() {
			Expect(k8sClient.Delete(ctx, storageSample)).Should(Succeed())
		}()
		By("create database...")
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
	})

	It("TLS for status service", func() {
		tlsHTTPCheck := func(port int) error {
			url := fmt.Sprintf("https://localhost:%d/", port)
			cert, err := os.ReadFile(filepath.Join(".", "data", "ca.crt"))
			Expect(err).ShouldNot(HaveOccurred())

			certPool := x509.NewCertPool()
			ok := certPool.AppendCertsFromPEM(cert)
			Expect(ok).To(BeTrue())

			tlsConfig := &tls.Config{
				MinVersion: tls.VersionTLS12,
				RootCAs:    certPool,
				ServerName: "storage-grpc.ydb.svc.cluster.local",
			}

			transport := &http.Transport{TLSClientConfig: tlsConfig}
			client := &http.Client{
				Transport: transport,
				Timeout:   10 * time.Second,
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

		By("create secret...")
		cert := testobjects.DefaultCertificate(
			filepath.Join(".", "data", "tls.crt"),
			filepath.Join(".", "data", "tls.key"),
			filepath.Join(".", "data", "ca.crt"),
		)
		Expect(k8sClient.Create(ctx, cert)).Should(Succeed())

		By("create storage...")
		storageSample.Spec.Service.Status.TLSConfiguration = &v1alpha1.TLSConfiguration{
			Enabled: true,
			Certificate: corev1.SecretKeySelector{
				LocalObjectReference: corev1.LocalObjectReference{Name: testobjects.CertificateSecretName},
				Key:                  "tls.crt",
			},
			Key: corev1.SecretKeySelector{
				LocalObjectReference: corev1.LocalObjectReference{Name: testobjects.CertificateSecretName},
				Key:                  "tls.key",
			},
			CertificateAuthority: corev1.SecretKeySelector{
				LocalObjectReference: corev1.LocalObjectReference{Name: testobjects.CertificateSecretName},
				Key:                  "ca.crt",
			},
		}

		Expect(k8sClient.Create(ctx, storageSample)).Should(Succeed())

		By("create database...")
		databaseSample.Spec.Nodes = 1
		databaseSample.Spec.Service.Status = *storageSample.Spec.Service.Status.DeepCopy()
		Expect(k8sClient.Create(ctx, databaseSample)).Should(Succeed())

		By("waiting until Storage is ready...")
		waitUntilStorageReady(ctx, storageSample.Name, testobjects.YdbNamespace)

		By("checking that all the storage pods are running and ready...")
		checkPodsRunningAndReady(ctx, "ydb-cluster", "kind-storage", storageSample.Spec.Nodes)

		By("forward storage status port and check that we can check TLS response")
		portForward(ctx,
			fmt.Sprintf(resources.StatusServiceNameFormat, storageSample.Name), storageSample.Namespace,
			v1alpha1.StatusPort, tlsHTTPCheck,
		)

		By("waiting until database is ready...")
		waitUntilDatabaseReady(ctx, databaseSample.Name, testobjects.YdbNamespace)

		By("checking that all the database pods are running and ready...")
		checkPodsRunningAndReady(ctx, "ydb-cluster", "kind-database", databaseSample.Spec.Nodes)

		By("forward database status port and check that we can check TLS response")
		portForward(ctx,
			fmt.Sprintf(resources.StatusServiceNameFormat, databaseSample.Name), databaseSample.Namespace,
			v1alpha1.StatusPort, tlsHTTPCheck,
		)
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
