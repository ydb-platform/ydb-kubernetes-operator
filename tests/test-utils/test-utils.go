package testutils

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	meta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	v1alpha1 "github.com/ydb-platform/ydb-kubernetes-operator/api/v1alpha1"
	. "github.com/ydb-platform/ydb-kubernetes-operator/internal/controllers/constants"
	testobjects "github.com/ydb-platform/ydb-kubernetes-operator/tests/test-k8s-objects"
)

const (
	ConsistentConditionTimeout = time.Second * 30
	Timeout                    = time.Second * 600
	Interval                   = time.Second * 2
	YdbOperatorRemoteChart     = "ydb/ydb-operator"
	YdbOperatorReleaseName     = "ydb-operator"
)

var (
	pathToHelmValuesInLocalInstall  = filepath.Join("..", "cfg", "operator-local-values.yaml")
	pathToHelmValuesInRemoteInstall = filepath.Join("..", "cfg", "operator-values.yaml")
)

func InstallLocalOperatorWithHelm(namespace string) {
	args := []string{
		"-n", namespace,
		"install",
		"--wait",
		"ydb-operator",
		filepath.Join("..", "..", "deploy", "ydb-operator"),
		"-f", pathToHelmValuesInLocalInstall,
	}

	result := exec.Command("helm", args...)
	stdout, err := result.Output()
	Expect(err).To(BeNil())
	Expect(stdout).To(ContainSubstring("deployed"))
}

func InstallOperatorWithHelm(namespace, version string) {
	args := []string{
		"-n", namespace,
		"install",
		"--wait",
		"ydb-operator",
		YdbOperatorRemoteChart,
		"-f", pathToHelmValuesInRemoteInstall,
		"--version", version,
	}

	Expect(exec.Command("helm", "repo", "add", "ydb", "https://charts.ydb.tech/").Run()).To(Succeed())
	Expect(exec.Command("helm", "repo", "update").Run()).To(Succeed())

	installCommand := exec.Command("helm", args...)
	output, err := installCommand.CombinedOutput()
	Expect(err).To(BeNil())
	Expect(string(output)).To(ContainSubstring("deployed"))
}

func UninstallOperatorWithHelm(namespace string) {
	args := []string{
		"-n", namespace,
		"uninstall",
		"--wait",
		"ydb-operator",
	}
	result := exec.Command("helm", args...)
	stdout, err := result.Output()
	Expect(err).To(BeNil())
	Expect(stdout).To(ContainSubstring("uninstalled"))
}

func UpgradeOperatorWithHelm(namespace, version string) {
	args := []string{
		"-n", namespace,
		"upgrade",
		"--wait",
		"ydb-operator",
		YdbOperatorRemoteChart,
		"--version", version,
		"-f", pathToHelmValuesInLocalInstall,
	}

	cmd := exec.Command("helm", args...)
	cmd.Stdout = GinkgoWriter
	cmd.Stderr = GinkgoWriter

	Expect(cmd.Run()).Should(Succeed())
}

func WaitUntilStorageReady(ctx context.Context, k8sClient client.Client, storageName, namespace string) {
	Eventually(func() bool {
		storage := &v1alpha1.Storage{}
		err := k8sClient.Get(ctx, types.NamespacedName{
			Name:      storageName,
			Namespace: namespace,
		}, storage)
		if err != nil {
			return false
		}

		return meta.IsStatusConditionPresentAndEqual(
			storage.Status.Conditions,
			StorageInitializedCondition,
			metav1.ConditionTrue,
		) && storage.Status.State == testobjects.ReadyStatus
	}, Timeout, Interval).Should(BeTrue())
}

func WaitUntilDatabaseReady(ctx context.Context, k8sClient client.Client, databaseName, namespace string) {
	Eventually(func() bool {
		database := &v1alpha1.Database{}
		err := k8sClient.Get(ctx, types.NamespacedName{
			Name:      databaseName,
			Namespace: namespace,
		}, database)
		if err != nil {
			return false
		}

		return meta.IsStatusConditionPresentAndEqual(
			database.Status.Conditions,
			DatabaseInitializedCondition,
			metav1.ConditionTrue,
		) && database.Status.State == testobjects.ReadyStatus
	}, Timeout, Interval).Should(BeTrue())
}

func CheckPodsRunningAndReady(ctx context.Context, k8sClient client.Client, podLabelKey, podLabelValue string, nPods int32) {
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
	}, ConsistentConditionTimeout, Interval).Should(BeTrue())
}

func podIsReady(conditions []corev1.PodCondition) bool {
	for _, condition := range conditions {
		if condition.Type == corev1.PodReady && condition.Status == corev1.ConditionTrue {
			return true
		}
	}
	return false
}

func BringYdbCliToPod(podName, podNamespace string) {
	expectedCliLocation := fmt.Sprintf("%v/ydb/bin/ydb", os.ExpandEnv("$HOME"))

	_, err := os.Stat(expectedCliLocation)
	Expect(err).NotTo(HaveOccurred(), fmt.Sprintf("Expected YDB CLI at path %s to exist", expectedCliLocation))

	Eventually(func(g Gomega) error {
		args := []string{
			"-n",
			podNamespace,
			"cp",
			expectedCliLocation,
			fmt.Sprintf("%v:/tmp/ydb", podName),
		}
		cmd := exec.Command("kubectl", args...)
		return cmd.Run()
	}, Timeout, Interval).Should(BeNil())
}

func ExecuteSimpleTableE2ETest(podName, podNamespace, storageEndpoint string, databasePath string) {
	tablePath := "testfolder/testtable"

	tableCreatingInterval := time.Second * 10

	Eventually(func(g Gomega) {
		args := []string{
			"-n", podNamespace,
			"exec", podName,
			"--",
			"/tmp/ydb",
			"-d", databasePath,
			"-e", storageEndpoint,
			"yql",
			"-s",
			fmt.Sprintf("CREATE TABLE `%s` (testColumnA Utf8, testColumnB Utf8, PRIMARY KEY (testColumnA));", tablePath),
		}
		output, _ := exec.Command("kubectl", args...).CombinedOutput()
		fmt.Println(string(output))
	}, Timeout, tableCreatingInterval).Should(Succeed())

	argsInsert := []string{
		"-n", podNamespace,
		"exec", podName,
		"--",
		"/tmp/ydb",
		"-d", databasePath,
		"-e", storageEndpoint,
		"yql",
		"-s",
		fmt.Sprintf("INSERT INTO `%s` (testColumnA, testColumnB) VALUES ('valueA', 'valueB');", tablePath),
	}
	output, err := exec.Command("kubectl", argsInsert...).CombinedOutput()
	Expect(err).ShouldNot(HaveOccurred(), string(output))

	argsSelect := []string{
		"-n", podNamespace,
		"exec", podName,
		"--",
		"/tmp/ydb",
		"-d", databasePath,
		"-e", storageEndpoint,
		"yql",
		"--format", "csv",
		"-s",
		fmt.Sprintf("SELECT * FROM `%s`;", tablePath),
	}
	output, err = exec.Command("kubectl", argsSelect...).CombinedOutput()
	Expect(err).ShouldNot(HaveOccurred(), string(output))
	Expect(strings.TrimSpace(string(output))).To(ContainSubstring("\"valueA\",\"valueB\""))

	argsDrop := []string{
		"-n", podNamespace,
		"exec", podName,
		"--",
		"/tmp/ydb",
		"-d", databasePath,
		"-e", storageEndpoint,
		"yql",
		"-s",
		fmt.Sprintf("DROP TABLE `%s`;", tablePath),
	}
	output, err = exec.Command("kubectl", argsDrop...).CombinedOutput()
	Expect(err).ShouldNot(HaveOccurred(), string(output))
}

func PortForward(
	ctx context.Context,
	svcName, svcNamespace, serverName string,
	port int,
	f func(int, string) error,
) {
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
			if err = f(localPort, serverName); err != nil {
				return err
			}
		} else {
			content, _ := io.ReadAll(stderr)
			return fmt.Errorf("kubectl port-forward stderr: %s", content)
		}
		return nil
	}, Timeout, Interval).Should(BeNil())
}

func DeleteStorageSafely(ctx context.Context, k8sClient client.Client, storage *v1alpha1.Storage) {
	// not checking that deletion completed successfully
	// because some tests delete storage themselves and
	// it may already be deleted.
	_ = k8sClient.Delete(ctx, storage)

	Eventually(func() bool {
		fetched := v1alpha1.Storage{}
		err := k8sClient.Get(ctx, types.NamespacedName{
			Name:      storage.Name,
			Namespace: testobjects.YdbNamespace,
		}, &fetched)
		return apierrors.IsNotFound(err)
	}, Timeout, Interval).Should(BeTrue())
}

func DeleteDatabase(ctx context.Context, k8sClient client.Client, database *v1alpha1.Database) {
	Expect(k8sClient.Delete(ctx, database)).To(Succeed())

	Eventually(func() bool {
		fetched := v1alpha1.Storage{}
		err := k8sClient.Get(ctx, types.NamespacedName{
			Name:      database.Name,
			Namespace: testobjects.YdbNamespace,
		}, &fetched)
		return apierrors.IsNotFound(err)
	}, Timeout, Interval).Should(BeTrue())
}

func DatabasePathWithDefaultDomain(database *v1alpha1.Database) string {
	return fmt.Sprintf("/%s/%s", testobjects.DefaultDomain, database.Name)
}
