package resources

import (
	"errors"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type ServiceBuilder struct {
	client.Object

	NameFormat string

	Ports    []corev1.ServicePort
	Headless bool

	IPFamilies     []corev1.IPFamily
	IPFamilyPolicy corev1.IPFamilyPolicyType

	Labels map[string]string
}

func (b *ServiceBuilder) Build(obj client.Object) error {
	service, ok := obj.(*corev1.Service)
	if !ok {
		return errors.New("failed to cast to Service object")
	}

	if b.NameFormat == "" {
		b.NameFormat = "%s"
	}

	if service.ObjectMeta.Name == "" {
		service.ObjectMeta.Name = fmt.Sprintf(b.NameFormat, b.GetName())
	}
	service.ObjectMeta.Namespace = b.GetNamespace()
	service.ObjectMeta.Labels = b.Labels

	service.Spec.Ports = b.Ports
	service.Spec.Selector = b.Labels

	if len(b.IPFamilies) > 0 {
		service.Spec.IPFamilies = b.IPFamilies
	}

	if b.IPFamilyPolicy != "" {
		service.Spec.IPFamilyPolicy = &b.IPFamilyPolicy
	}

	if b.Headless && service.Spec.ClusterIP == "" {
		service.Spec.ClusterIP = "None"
	}

	return nil
}

func (b *ServiceBuilder) Placeholder(cr client.Object) client.Object {
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf(b.NameFormat, b.GetName()),
			Namespace: cr.GetNamespace(),
		},
	}
}
