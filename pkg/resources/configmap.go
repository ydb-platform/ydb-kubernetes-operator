package resources

import (
	"errors"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type ConfigMapBuilder struct {
	client.Object

	Data   map[string]string
	Labels map[string]string
}

func (b *ConfigMapBuilder) Build(obj client.Object) error {
	cm, ok := obj.(*v1.ConfigMap)
	if !ok {
		return errors.New("failed to cast to ConfigMap object")
	}

	if cm.ObjectMeta.Name == "" {
		cm.ObjectMeta.Name = b.GetName()
	}
	cm.ObjectMeta.Namespace = b.GetNamespace() // fixme should we really slap namespace on any object?

	cm.Data = b.Data
	cm.Labels = b.Labels

	return nil
}

func (b *ConfigMapBuilder) Placeholder(cr client.Object) client.Object {
	return &v1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cr.GetName(),
			Namespace: cr.GetNamespace(),
		},
	}
}
