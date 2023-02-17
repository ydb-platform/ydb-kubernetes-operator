package storage

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	. "github.com/onsi/ginkgo" //nolint:all
	. "github.com/onsi/gomega" //nolint:all
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"
	"k8s.io/kubectl/pkg/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	"github.com/ydb-platform/ydb-kubernetes-operator/api/v1alpha1"
	testobjects "github.com/ydb-platform/ydb-kubernetes-operator/e2e/tests/test-objects"
)

var (
	cfg       *rest.Config
	k8sClient client.Client
	testEnv   *envtest.Environment
	ctx       context.Context
	cancel    context.CancelFunc
)

const (
	Timeout  = time.Second * 600
	Interval = time.Second * 5
)

func TestAPIs(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Storage controller medium tests suite")
}

var _ = BeforeSuite(func() {
	logf.SetLogger(zap.New(zap.WriteTo(os.Stdout), zap.UseDevMode(true)))
	ctx, cancel = context.WithCancel(context.TODO())

	By("bootstrapping test environment")
	useExistingCluster := false
	testEnv = &envtest.Environment{
		CRDDirectoryPaths: []string{filepath.Join(
			"..",
			"..",
			"..",
			"deploy",
			"ydb-operator",
			"crds",
		)},
		ErrorIfCRDPathMissing: true,
		UseExistingCluster:    &useExistingCluster,
	}

	var err error
	cfg, err = testEnv.Start()
	Expect(err).NotTo(HaveOccurred())
	Expect(cfg).NotTo(BeNil())

	err = v1alpha1.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())

	//+kubebuilder:scaffold:scheme
	k8sClient, err = client.New(cfg, client.Options{Scheme: scheme.Scheme})
	Expect(err).NotTo(HaveOccurred())
	Expect(k8sClient).NotTo(BeNil())
	k8sManager, err := ctrl.NewManager(cfg, ctrl.Options{
		Scheme: scheme.Scheme,
	})
	Expect(err).ToNot(HaveOccurred())

	err = (&Reconciler{
		Client: k8sManager.GetClient(),
		Scheme: k8sManager.GetScheme(),
	}).SetupWithManager(k8sManager)

	Expect(err).ToNot(HaveOccurred())

	go func() {
		defer GinkgoRecover()
		err = k8sManager.Start(ctx)
		Expect(err).ToNot(HaveOccurred(), "failed to run manager")
	}()
})

var _ = Describe("Storage controller medium tests", func() {
	var namespace corev1.Namespace

	BeforeEach(func() {
		namespace = corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: testobjects.YdbNamespace,
			},
		}
		Expect(k8sClient.Create(ctx, &namespace)).Should(Succeed())
	})

	AfterEach(func() {
		Expect(k8sClient.Delete(ctx, &namespace)).Should(Succeed())
	})

	It("Check volume has been propagated to pods", func() {
		storageSample := testobjects.DefaultStorage(filepath.Join("..", "..", "..", "e2e", "tests", "data", "storage-block-4-2-config.yaml"))

		tmpFilesDir := "/tmp/mounted_volume"
		testVolumeName := "sample-volume"
		testVolumeMountPath := fmt.Sprintf("%v/volume", tmpFilesDir)

		HostPathDirectoryType := corev1.HostPathDirectory

		storageSample.Spec.Volumes = append(storageSample.Spec.Volumes, &corev1.Volume{
			Name: testVolumeName,
			VolumeSource: corev1.VolumeSource{
				HostPath: &corev1.HostPathVolumeSource{
					Path: testVolumeMountPath,
					Type: &HostPathDirectoryType,
				},
			},
		})

		Expect(k8sClient.Create(ctx, storageSample)).Should(Succeed())

		storageStatefulSets := appsv1.StatefulSetList{}
		Eventually(func() bool {
			Expect(k8sClient.List(ctx, &storageStatefulSets, client.InNamespace(
				testobjects.YdbNamespace,
			))).Should(Succeed())
			foundStatefulSet := false
			for _, statefulSet := range storageStatefulSets.Items {
				if statefulSet.Name == testobjects.StorageName {
					foundStatefulSet = true
					break
				}
			}
			return foundStatefulSet
		}, Timeout, Interval).Should(BeTrue())

		storageSS := storageStatefulSets.Items[0]
		volumes := storageSS.Spec.Template.Spec.Volumes
		fmt.Printf("%#v\n", volumes)
		// Pod Template always has `ydb-config` mounted as a volume, plus in
		// this test it also has our test volume. So two in total:
		Expect(len(volumes)).To(Equal(1 + 1))

		foundVolume := false
		for _, volume := range volumes {
			if volume.Name == testVolumeName {
				foundVolume = true
				Expect(volume.VolumeSource.HostPath.Path).To(Equal(testVolumeMountPath))
			}
		}
		Expect(foundVolume).To(BeTrue())
	})
})

var _ = AfterSuite(func() {
	cancel()
	By("tearing down the test environment")
	err := testEnv.Stop()
	Expect(err).NotTo(HaveOccurred())
})
