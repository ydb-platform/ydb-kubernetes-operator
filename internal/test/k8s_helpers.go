package test

import (
	"context"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2" //nolint:revive,stylecheck
	. "github.com/onsi/gomega"    //nolint:revive,stylecheck
	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes"
	"k8s.io/kubectl/pkg/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	"github.com/ydb-platform/ydb-kubernetes-operator/api/v1alpha1"
)

const (
	Timeout  = 30 * time.Second
	Interval = 1 * time.Second
)

type Reconciler interface {
	SetupWithManager(manager ctrl.Manager) error
}

func DeleteAllObjects(env *envtest.Environment, k8sClient client.Client, objs ...client.Object) {
	for _, obj := range objs {
		ctx := context.Background()
		clientGo, err := kubernetes.NewForConfig(env.Config)
		Expect(err).ShouldNot(HaveOccurred())
		Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, obj))).Should(Succeed())

		//nolint:nestif
		if ns, ok := obj.(*corev1.Namespace); ok {
			// Normally the kube-controller-manager would handle finalization
			// and garbage collection of namespaces, but with envtest, we aren't
			// running a kube-controller-manager. Instead we're gonna approximate
			// (poorly) the kube-controller-manager by explicitly deleting some
			// resources within the namespace and then removing the `kubernetes`
			// finalizer from the namespace resource so it can finish deleting.
			// Note that any resources within the namespace that we don't
			// successfully delete could reappear if the namespace is ever
			// recreated with the same name.

			// Look up all namespaced resources under the discovery API
			_, apiResources, err := clientGo.Discovery().ServerGroupsAndResources()
			Expect(err).ShouldNot(HaveOccurred())
			namespacedGVKs := make(map[string]schema.GroupVersionKind)
			for _, apiResourceList := range apiResources {
				defaultGV, err := schema.ParseGroupVersion(apiResourceList.GroupVersion)
				Expect(err).ShouldNot(HaveOccurred())
				for _, r := range apiResourceList.APIResources {
					if !r.Namespaced || strings.Contains(r.Name, "/") {
						// skip non-namespaced and subresources
						continue
					}
					gvk := schema.GroupVersionKind{
						Group:   defaultGV.Group,
						Version: defaultGV.Version,
						Kind:    r.Kind,
					}
					if r.Group != "" {
						gvk.Group = r.Group
					}
					if r.Version != "" {
						gvk.Version = r.Version
					}
					namespacedGVKs[gvk.String()] = gvk
				}
			}

			// Delete all namespaced resources in this namespace
			for _, gvk := range namespacedGVKs {
				var u unstructured.Unstructured
				u.SetGroupVersionKind(gvk)
				err := k8sClient.DeleteAllOf(ctx, &u, client.InNamespace(ns.Name))
				Expect(client.IgnoreNotFound(ignoreMethodNotAllowed(err))).ShouldNot(HaveOccurred())
			}

			// Delete all Services in this namespace
			serviceList := corev1.ServiceList{}
			err = k8sClient.List(ctx, &serviceList, client.InNamespace(ns.Name))
			Expect(err).ShouldNot(HaveOccurred())
			for idx := range serviceList.Items {
				policy := metav1.DeletePropagationForeground
				err = k8sClient.Delete(ctx, &serviceList.Items[idx], &client.DeleteOptions{PropagationPolicy: &policy})
				Expect(err).ShouldNot(HaveOccurred())
			}

			Eventually(func() error {
				key := client.ObjectKeyFromObject(ns)
				if err := k8sClient.Get(ctx, key, ns); err != nil {
					return client.IgnoreNotFound(err)
				}
				// remove `kubernetes` finalizer
				const kubernetes = "kubernetes"
				finalizers := []corev1.FinalizerName{}
				for _, f := range ns.Spec.Finalizers {
					if f != kubernetes {
						finalizers = append(finalizers, f)
					}
				}
				ns.Spec.Finalizers = finalizers

				// We have to use the k8s.io/client-go library here to expose
				// ability to patch the /finalize subresource on the namespace
				_, err = clientGo.CoreV1().Namespaces().Finalize(ctx, ns, metav1.UpdateOptions{})
				return err
			}, Timeout, Interval).Should(Succeed())
		}

		Eventually(func() metav1.StatusReason {
			key := client.ObjectKeyFromObject(obj)
			if err := k8sClient.Get(ctx, key, obj); err != nil {
				return apierrors.ReasonForError(err)
			}
			return ""
		}, Timeout, Interval).Should(Equal(metav1.StatusReasonNotFound))
	}
}

func ignoreMethodNotAllowed(err error) error {
	if err != nil {
		if apierrors.ReasonForError(err) == metav1.StatusReasonMethodNotAllowed {
			return nil
		}
	}
	return err
}

func SetupK8STestManager(testCtx *context.Context, k8sClient *client.Client, controllers func(mgr *manager.Manager) []Reconciler) *envtest.Environment {
	ctx, cancel := context.WithCancel(context.TODO())
	*testCtx = ctx

	useExistingCluster := false

	// FIXME: find a better way?
	_, curfile, _, _ := runtime.Caller(0) //nolint:dogsled
	webhookDir := filepath.Join(curfile, "..", "..", "..", "config", "webhook")
	testEnv := &envtest.Environment{
		CRDDirectoryPaths: []string{
			filepath.Join(curfile, "..", "..", "..", "deploy", "ydb-operator", "crds"),
			filepath.Join(filepath.Dir(curfile), "extra_crds"),
		},
		ErrorIfCRDPathMissing: true,
		UseExistingCluster:    &useExistingCluster,
		WebhookInstallOptions: envtest.WebhookInstallOptions{
			Paths:               []string{webhookDir},
			LocalServingHost:    "127.0.0.1",
			LocalServingPort:    9443,
			LocalServingCertDir: "",
		},
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
			Host:               testEnv.WebhookInstallOptions.LocalServingHost,
			Port:               testEnv.WebhookInstallOptions.LocalServingPort,
			CertDir:            testEnv.WebhookInstallOptions.LocalServingCertDir,
		})
		Expect(err).ToNot(HaveOccurred())

		*k8sClient = mgr.GetClient()

		for _, c := range controllers(&mgr) {
			Expect(c.SetupWithManager(mgr)).To(Succeed())
		}

		// Setup webhooks
		Expect((&v1alpha1.Storage{}).SetupWebhookWithManager(mgr)).To(Succeed())
		Expect((&v1alpha1.Database{}).SetupWebhookWithManager(mgr)).To(Succeed())
		Expect(v1alpha1.RegisterMonitoringValidatingWebhook(mgr, true)).To(Succeed())

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

	return testEnv
}
