package resources

import (
	"errors"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	DefaultNameFormat = "%s"
)

type ServiceBuilder struct {
	client.Object

	NameFormat string

	Ports    []corev1.ServicePort
	Headless bool

	IPFamilies     []corev1.IPFamily
	IPFamilyPolicy *corev1.IPFamilyPolicyType

	Labels         map[string]string
	SelectorLabels map[string]string

	Annotations map[string]string
}

func (b *ServiceBuilder) Build(obj client.Object) error {
	service, ok := obj.(*corev1.Service)
	if !ok {
		return errors.New("failed to cast to Service object")
	}

	if b.NameFormat == "" {
		b.NameFormat = DefaultNameFormat
	}

	if service.ObjectMeta.Name == "" {
		service.ObjectMeta.Name = fmt.Sprintf(b.NameFormat, b.GetName())
	}
	service.ObjectMeta.Namespace = b.GetNamespace()
	service.ObjectMeta.Labels = b.Labels
	service.ObjectMeta.Annotations = b.Annotations

	service.Spec.Ports = b.Ports
	service.Spec.Selector = b.SelectorLabels

	if len(b.IPFamilies) > 0 {
		service.Spec.IPFamilies = b.IPFamilies
	}

	if b.IPFamilyPolicy != nil {
		service.Spec.IPFamilyPolicy = b.IPFamilyPolicy
	}

	if b.Headless && service.Spec.ClusterIP == "" {
		service.Spec.ClusterIP = "None"
	}

	for _, port := range service.Spec.Ports {
		if port.NodePort != 0 {
			service.Spec.Type = corev1.ServiceTypeNodePort
		}
	}

	return nil
}

func (b *ServiceBuilder) Placeholder(cr client.Object) client.Object {
	if b.NameFormat == "" {
		b.NameFormat = "%s"
	}

	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf(b.NameFormat, b.GetName()),
			Namespace: cr.GetNamespace(),
		},
	}
}
