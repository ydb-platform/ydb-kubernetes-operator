package test

import (
	"context"
	"fmt"

	. "github.com/onsi/ginkgo/v2" //nolint:revive,stylecheck
	. "github.com/onsi/gomega"    //nolint:revive,stylecheck
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func RunAdditionalLabelsTest(ctx context.Context, k8sClient client.Client, namespace string, uid types.UID, additionalLabels map[string]string, sts appsv1.StatefulSet) {
	By("Check that Services were created...")
	allFoundServices := corev1.ServiceList{}
	var foundServices []corev1.Service

	Eventually(func() error {
		err := k8sClient.List(ctx, &allFoundServices, client.InNamespace(namespace))
		if err != nil {
			return err
		}

		for _, svc := range allFoundServices.Items {
			for _, ownerRef := range svc.GetOwnerReferences() {
				if ownerRef.UID == uid {
					foundServices = append(foundServices, svc)
				}
			}
		}

		return nil
	}, Timeout, Interval).ShouldNot(HaveOccurred())

	Expect(foundServices).ShouldNot(BeEmpty())

	By("Check additionalLabels propagated to sts...", func() {
		for k, v := range additionalLabels {
			Expect(sts.Labels).Should(HaveKeyWithValue(k, v))
		}
	})
	By("Check additionalLabels propagated to services...", func() {
		for _, svc := range foundServices {
			for k, v := range additionalLabels {
				Expect(svc.Labels).Should(HaveKeyWithValue(k, v), fmt.Sprintf("svc %s", svc.Name))
			}
		}
	})

	By("Check additionalLabels are not propagated to selector field in the service...", func() {
		for _, svc := range foundServices {
			for k, v := range additionalLabels {
				Expect(svc.Spec.Selector).ShouldNot(HaveKeyWithValue(k, v), fmt.Sprintf("svc %s", svc.Name))
			}
		}
	})
}
