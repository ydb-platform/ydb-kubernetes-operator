package compatibility

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/kubectl/pkg/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	logf "sigs.k8s.io/controller-runtime/pkg/log"

	v1alpha1 "github.com/ydb-platform/ydb-kubernetes-operator/api/v1alpha1"
	testobjects "github.com/ydb-platform/ydb-kubernetes-operator/tests/test-k8s-objects"
	. "github.com/ydb-platform/ydb-kubernetes-operator/tests/test-utils"
)

var (
	k8sClient  client.Client
	restConfig *rest.Config

	testEnv *envtest.Environment

	oldVersion string
	newVersion string
)

func TestCompatibility(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Compatibility Suite")
}

var _ = BeforeSuite(func() {
	logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))

	useExistingCluster := true
	testEnv = &envtest.Environment{
		CRDDirectoryPaths:     []string{filepath.Join("..", "..", "deploy", "ydb-operator", "crds")},
		ErrorIfCRDPathMissing: true,
		UseExistingCluster:    &useExistingCluster,
	}

	cfg, err := testEnv.Start()
	Expect(err).NotTo(HaveOccurred())
	Expect(cfg).NotTo(BeNil())

	if useExistingCluster && !(strings.Contains(cfg.Host, "127.0.0.1") || strings.Contains(cfg.Host, "::1") || strings.Contains(cfg.Host, "localhost")) {
		Fail("You are trying to run e2e tests against some real cluster, not the local `kind` cluster!")
	}

	err = v1alpha1.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())

	k8sClient, err = client.New(cfg, client.Options{Scheme: scheme.Scheme})
	Expect(err).NotTo(HaveOccurred())
	Expect(k8sClient).NotTo(BeNil())

	clientset, err := kubernetes.NewForConfig(cfg)
	Expect(err).NotTo(HaveOccurred())
	Expect(clientset).NotTo(BeNil())

	oldVersion = os.Getenv("PREVIOUS_VERSION")
	newVersion = os.Getenv("NEW_VERSION")
	Expect(oldVersion).NotTo(BeEmpty(), "PREVIOUS_VERSION environment variable is required")
	Expect(newVersion).NotTo(BeEmpty(), "NEW_VERSION environment variable is required")
})

var _ = Describe("Operator Compatibility Test", func() {
	var (
		ctx            context.Context
		namespace      corev1.Namespace
		storageSample  *v1alpha1.Storage
		databaseSample *v1alpha1.Database
	)

	BeforeEach(func() {
		ctx = context.Background()

		namespace = corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: testobjects.YdbNamespace,
			},
		}
		Expect(k8sClient.Create(ctx, &namespace)).Should(Succeed())

		Eventually(func() bool {
			ns := &corev1.Namespace{}
			err := k8sClient.Get(ctx, client.ObjectKey{Name: namespace.Name}, ns)
			return err == nil
		}, Timeout, Interval).Should(BeTrue())

		By(fmt.Sprintf("Installing previous operator version %s", oldVersion))
		InstallOperatorWithHelm(testobjects.YdbNamespace, oldVersion)

		storageSample = testobjects.DefaultStorage(filepath.Join("..", "data", "storage-mirror-3-dc-config.yaml"))
		databaseSample = testobjects.DefaultDatabase()
	})

	It("Upgrades from old operator to new operator, objects persist, YQL works", func() {
		By("Creating Storage resource")
		Expect(k8sClient.Create(ctx, storageSample)).Should(Succeed())
		defer DeleteStorageSafely(ctx, k8sClient, storageSample)
		Expect(k8sClient.Get(ctx, types.NamespacedName{
			Name:      storageSample.Name,
			Namespace: testobjects.YdbNamespace,
		}, storageSample)).Should(Succeed())

		By("Waiting for Storage to be ready")
		WaitUntilStorageReady(ctx, k8sClient, storageSample.Name, testobjects.YdbNamespace)

		By("Creating Database resource")
		Expect(k8sClient.Create(ctx, databaseSample)).Should(Succeed())
		Expect(k8sClient.Get(ctx, types.NamespacedName{
			Name:      databaseSample.Name,
			Namespace: testobjects.YdbNamespace,
		}, databaseSample)).Should(Succeed())
		defer DeleteDatabase(ctx, k8sClient, databaseSample)

		By("Waiting for Database to be ready")
		WaitUntilDatabaseReady(ctx, k8sClient, databaseSample.Name, testobjects.YdbNamespace)

		By(fmt.Sprintf("Upgrading CRDs from %s to %s", oldVersion, newVersion))
		UpdateCRDsTo(YdbOperatorReleaseName, namespace.Name, newVersion)

		By(fmt.Sprintf("Upgrading operator from %s to %s, with uninstalling, just to cause more chaos", oldVersion, newVersion))
		UninstallOperatorWithHelm(testobjects.YdbNamespace)
		InstallOperatorWithHelm(testobjects.YdbNamespace, newVersion)

		By("Verifying Storage + Database are the same objects after upgrade")
		Consistently(func() error {
			storageAfterUpgrade := v1alpha1.Storage{}
			err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      storageSample.Name,
				Namespace: testobjects.YdbNamespace,
			}, &storageAfterUpgrade)
			if err != nil {
				return err
			}
			if storageAfterUpgrade.UID != storageSample.UID {
				return fmt.Errorf("storage UID has changed")
			}

			databaseAfterUpgrade := v1alpha1.Database{}
			err = k8sClient.Get(ctx, types.NamespacedName{
				Name:      databaseSample.Name,
				Namespace: testobjects.YdbNamespace,
			}, &databaseAfterUpgrade)
			if err != nil {
				return err
			}
			if databaseAfterUpgrade.UID != databaseSample.UID {
				return fmt.Errorf("database UID has changed")
			}
			return nil
		}, ConsistentConditionTimeout, Interval).Should(Succeed())

		By("Restarting storage pods (one by one, no rolling restart, for simplicity)")
		RestartPodsNoRollingRestart(ctx, k8sClient, testobjects.YdbNamespace, "ydb-cluster", "kind-storage")

		By("Restarting database pods (one by one, no rolling restart, for simplicity)")
		RestartPodsNoRollingRestart(ctx, k8sClient, testobjects.YdbNamespace, "ydb-cluster", "kind-database")

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

		Expect(databasePods.Items).ToNot(BeEmpty())
		podName := databasePods.Items[0].Name

		By("bring YDB CLI inside ydb database pod...")
		BringYdbCliToPod(podName, testobjects.YdbNamespace)

		By("execute simple query inside ydb database pod...")
		databasePath := "/" + testobjects.DefaultDomain + "/" + databaseSample.Name
		ExecuteSimpleTableE2ETest(podName, testobjects.YdbNamespace, storageEndpoint, databasePath)
	})

	It("Upgrades from old operator to new operator, applying objects later succeeds", func() {
		By("Creating Storage resource")
		Expect(k8sClient.Create(ctx, storageSample)).Should(Succeed())
		defer DeleteStorageSafely(ctx, k8sClient, storageSample)
		Expect(k8sClient.Get(ctx, types.NamespacedName{
			Name:      storageSample.Name,
			Namespace: testobjects.YdbNamespace,
		}, storageSample)).Should(Succeed())

		By("Waiting for Storage to be ready")
		WaitUntilStorageReady(ctx, k8sClient, storageSample.Name, testobjects.YdbNamespace)

		By("Creating Database resource")
		Expect(k8sClient.Create(ctx, databaseSample)).Should(Succeed())
		Expect(k8sClient.Get(ctx, types.NamespacedName{
			Name:      databaseSample.Name,
			Namespace: testobjects.YdbNamespace,
		}, databaseSample)).Should(Succeed())
		defer DeleteDatabase(ctx, k8sClient, databaseSample)

		By("Waiting for Database to be ready")
		WaitUntilDatabaseReady(ctx, k8sClient, databaseSample.Name, testobjects.YdbNamespace)

		By(fmt.Sprintf("Upgrading CRDs from %s to %s", oldVersion, newVersion))
		UpdateCRDsTo(YdbOperatorReleaseName, namespace.Name, newVersion)

		By(fmt.Sprintf("Upgrading operator from %s to %s, with uninstalling, just to cause more chaos", oldVersion, newVersion))
		UninstallOperatorWithHelm(testobjects.YdbNamespace)
		InstallOperatorWithHelm(testobjects.YdbNamespace, newVersion)

		By("Verifying Storage + Database are the same objects after upgrade")
		Consistently(func() error {
			storageAfterUpgrade := v1alpha1.Storage{}
			err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      storageSample.Name,
				Namespace: testobjects.YdbNamespace,
			}, &storageAfterUpgrade)
			if err != nil {
				return err
			}
			if storageAfterUpgrade.UID != storageSample.UID {
				return fmt.Errorf("storage UID has changed")
			}

			databaseAfterUpgrade := v1alpha1.Database{}
			err = k8sClient.Get(ctx, types.NamespacedName{
				Name:      databaseSample.Name,
				Namespace: testobjects.YdbNamespace,
			}, &databaseAfterUpgrade)
			if err != nil {
				return err
			}
			if databaseAfterUpgrade.UID != databaseSample.UID {
				return fmt.Errorf("database UID has changed")
			}
			return nil
		}, ConsistentConditionTimeout, Interval).Should(Succeed())

		By("Restarting storage pods (one by one, no rolling restart, for simplicity)")
		RestartPodsNoRollingRestart(ctx, k8sClient, testobjects.YdbNamespace, "ydb-cluster", "kind-storage")

		By("Restarting database pods (one by one, no rolling restart, for simplicity)")
		RestartPodsNoRollingRestart(ctx, k8sClient, testobjects.YdbNamespace, "ydb-cluster", "kind-database")

		// This is probably the most important check.
		// If any major fields moved or got deleted, updating a resource will fail
		// For this to work even better, TODO @jorres make this storage object as full as possible
		// (utilizing as many fields), as it will help catching an error
		By("applying the storage again must NOT fail because of CRD issues...")
		Expect(k8sClient.Get(ctx, client.ObjectKey{
			Name:      storageSample.Name,
			Namespace: storageSample.Namespace,
		}, storageSample)).Should(Succeed())
		Expect(k8sClient.Update(ctx, storageSample)).Should(Succeed())

		By("applying the database again must NOT fail because of CRD issues...")
		Expect(k8sClient.Get(ctx, client.ObjectKey{
			Name:      databaseSample.Name,
			Namespace: databaseSample.Namespace,
		}, databaseSample)).Should(Succeed())
		Expect(k8sClient.Update(ctx, databaseSample)).Should(Succeed())
	})

	AfterEach(func() {
		By("Uninstalling operator")
		UninstallOperatorWithHelm(testobjects.YdbNamespace)

		By("Deleting namespace")
		Expect(k8sClient.Delete(ctx, &namespace)).Should(Succeed())
		Eventually(func() bool {
			ns := &corev1.Namespace{}
			err := k8sClient.Get(ctx, client.ObjectKey{Name: namespace.Name}, ns)
			return apierrors.IsNotFound(err)
		}, Timeout, Interval).Should(BeTrue())
	})
})

func UpdateCRDsTo(releaseName, namespace, version string) {
	tempDir, err := os.MkdirTemp("", "helm-chart-*")
	Expect(err).ShouldNot(HaveOccurred())
	defer os.RemoveAll(tempDir)

	cmd := exec.Command("helm", "pull", YdbOperatorRemoteChart, "--version", version, "--untar", "--untardir", tempDir)
	output, err := cmd.CombinedOutput()
	Expect(err).ShouldNot(HaveOccurred(), string(output))

	crdDir := filepath.Join(tempDir, YdbOperatorRemoteChart, "crds")
	crdFiles, err := filepath.Glob(filepath.Join(crdDir, "*.yaml"))
	Expect(err).ShouldNot(HaveOccurred())
	for _, crdFile := range crdFiles {
		cmd := exec.Command("kubectl", "apply", "-f", crdFile)
		output, err := cmd.CombinedOutput()
		Expect(err).ShouldNot(HaveOccurred(), fmt.Sprintf("failed to apply CRD %s: %s", crdFile, string(output)))
	}
}

var _ = AfterSuite(func() {
	By("cleaning up the test environment")
	err := testEnv.Stop()
	Expect(err).NotTo(HaveOccurred())
})

// This was the fastest way to implement restart without bringing rolling 
// restart to operator itself or using ydbops. If you read this and 
// operator already can do rolling restart natively, please rewrite
// this function!
func RestartPodsNoRollingRestart(
	ctx context.Context,
	k8sClient client.Client,
	namespace string,
	labelKey, labelValue string,
) {
	podList := corev1.PodList{}

	Expect(k8sClient.List(ctx, &podList, client.InNamespace(namespace), client.MatchingLabels{
		labelKey: labelValue,
	})).Should(Succeed())

	for _, pod := range podList.Items {
		originalUID := pod.UID
		Expect(k8sClient.Delete(ctx, &pod)).Should(Succeed())

		Eventually(func() bool {
			newPod := corev1.Pod{}
			err := k8sClient.Get(ctx, types.NamespacedName{Name: pod.Name, Namespace: namespace}, &newPod)
			if err != nil {
				return apierrors.IsNotFound(err)
			}
			return newPod.UID != originalUID
		}, Timeout, Interval).Should(BeTrue(), fmt.Sprintf("Pod %s should be recreated with a new UID", pod.Name))

		Eventually(func() bool {
			newPod := corev1.Pod{}
			err := k8sClient.Get(ctx, types.NamespacedName{Name: pod.Name, Namespace: namespace}, &newPod)
			if err != nil {
				return false
			}
			return newPod.Status.Phase == corev1.PodRunning
		}, Timeout, Interval).Should(BeTrue(), fmt.Sprintf("Pod %s should be running", pod.Name))

		time.Sleep(90 * time.Second)
	}
}
