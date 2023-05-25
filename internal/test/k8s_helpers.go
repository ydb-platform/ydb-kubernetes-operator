package test

import (
	"context"
	"path/filepath"
	"runtime"

	. "github.com/onsi/ginkgo/v2" //nolint:revive,stylecheck
	. "github.com/onsi/gomega"    //nolint:revive,stylecheck
	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	"k8s.io/kubectl/pkg/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	"github.com/ydb-platform/ydb-kubernetes-operator/api/v1alpha1"
)

type Reconciler interface {
	SetupWithManager(manager ctrl.Manager) error
}

func SetupK8STestManager(testCtx *context.Context, k8sClient *client.Client, controllers func(mgr *manager.Manager) []Reconciler) {
	ctx, cancel := context.WithCancel(context.TODO())
	*testCtx = ctx

	useExistingCluster := false

	// FIXME: find a better way?
	_, curfile, _, _ := runtime.Caller(0) //nolint:dogsled
	testEnv := &envtest.Environment{
		CRDDirectoryPaths: []string{
			filepath.Join(curfile, "..", "..", "..", "deploy", "ydb-operator", "crds"),
		},
		ErrorIfCRDPathMissing: true,
		UseExistingCluster:    &useExistingCluster,
	}

	BeforeSuite(func() {
		By("bootstrapping test environment")

		logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))

		Expect(monitoringv1.AddToScheme(scheme.Scheme)).To(Succeed())
		Expect(v1alpha1.AddToScheme(scheme.Scheme)).To(Succeed())

		cfg, err := testEnv.Start()
		Expect(err).NotTo(HaveOccurred())
		Expect(cfg).NotTo(BeNil())

		//+kubebuilder:scaffold:scheme
		mgr, err := ctrl.NewManager(cfg, ctrl.Options{
			MetricsBindAddress: "0",
			Scheme:             scheme.Scheme,
		})
		Expect(err).ToNot(HaveOccurred())

		*k8sClient = mgr.GetClient()

		for _, c := range controllers(&mgr) {
			Expect(c.SetupWithManager(mgr)).To(Succeed())
		}

		go func() {
			defer GinkgoRecover()
			err = mgr.Start(ctx)
			Expect(err).ToNot(HaveOccurred(), "failed to run manager")
		}()
	})
	AfterSuite(func() {
		cancel()
		By("tearing down the test environment")
		err := testEnv.Stop()
		Expect(err).NotTo(HaveOccurred())
	})
}
