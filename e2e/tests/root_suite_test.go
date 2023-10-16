package tests

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/client-go/kubernetes"
	"k8s.io/kubectl/pkg/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	v1alpha1 "github.com/ydb-platform/ydb-kubernetes-operator/api/v1alpha1"
)

var (
	k8sClient client.Client
	clientset *kubernetes.Clientset
	testEnv   *envtest.Environment
)

func TestStorage(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Operator Suite")
}

var _ = BeforeSuite(func(ctx SpecContext) {
	logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))

	By("using existing test environment...")
	useExistingCluster := true
	testEnv = &envtest.Environment{
		CRDDirectoryPaths:     []string{filepath.Join("..", "..", "deploy", "ydb-operator", "crds")},
		ErrorIfCRDPathMissing: true,
		UseExistingCluster:    &useExistingCluster,
	}

	cfg, err := testEnv.Start()
	Expect(err).NotTo(HaveOccurred())
	Expect(cfg).NotTo(BeNil())

	if useExistingCluster && !(strings.Contains(cfg.Host, "127.0.0.1") || strings.Contains(cfg.Host, "localhost")) {
		Fail("You are trying to run e2e tests against some real cluster, not the local `kind` cluster!")
	}

	err = v1alpha1.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())

	//+kubebuilder:scaffold:scheme

	k8sClient, err = client.NewWithWatch(cfg, client.Options{Scheme: scheme.Scheme})
	Expect(err).NotTo(HaveOccurred())
	Expect(k8sClient).NotTo(BeNil())

	clientset, err = kubernetes.NewForConfig(cfg)
	Expect(err).NotTo(HaveOccurred())
	Expect(clientset).NotTo(BeNil())
}, NodeTimeout(60*time.Second))

var _ = AfterSuite(func() {
	By("cleaning up the test environment")
	err := testEnv.Stop()
	Expect(err).NotTo(HaveOccurred())
})
