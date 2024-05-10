package resources

import (
	"bytes"
	"errors"
	"fmt"
	"html/template"

	api "github.com/ydb-platform/ydb-kubernetes-operator/api/v1alpha1"
	"github.com/ydb-platform/ydb-kubernetes-operator/internal/configuration/schema"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const keyConfigTmpl = `Keys {
  ContainerPath: "{{ .ContainerPath }}"
  Pin: "{{ .Pin }}"
  Id: "{{ .ID }}"
  Version: "{{ .Version }}"
}`

type ConfigMapBuilder struct {
	client.Object

	Name   string
	Data   map[string]string
	Labels map[string]string
}

type EncryptionConfigBuilder struct {
	client.Object

	Name      string
	KeyConfig schema.KeyConfig
	Labels    map[string]string
}

func (b *ConfigMapBuilder) Build(obj client.Object) error {
	cm, ok := obj.(*v1.ConfigMap)
	if !ok {
		return errors.New("failed to cast to ConfigMap object")
	}

	if cm.ObjectMeta.Name == "" {
		cm.ObjectMeta.Name = b.Name
	}
	cm.ObjectMeta.Namespace = b.GetNamespace()

	cm.Data = b.Data
	cm.Labels = b.Labels

	return nil
}

func (b *EncryptionConfigBuilder) Build(obj client.Object) error {
	cm, ok := obj.(*v1.ConfigMap)
	if !ok {
		return errors.New("failed to cast to ConfigMap object")
	}

	if cm.ObjectMeta.Name == "" {
		cm.ObjectMeta.Name = b.Name
	}
	cm.ObjectMeta.Namespace = b.GetNamespace()

	t, err := template.New("keyConfig").Parse(keyConfigTmpl)
	if err != nil {
		return fmt.Errorf("failed to parse keyConfig template: %s", err)
	}

	var buf bytes.Buffer
	err = t.Execute(&buf, b.KeyConfig.Keys[0])
	if err != nil {
		return fmt.Errorf("failed to execute keyConfig template: %s", err)
	}

	cm.Data = map[string]string{
		api.DatabaseEncryptionKeyConfigFileName: buf.String(),
	}
	cm.Labels = b.Labels

	return nil
}

func (b *ConfigMapBuilder) Placeholder(cr client.Object) client.Object {
	return &v1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      b.Name,
			Namespace: cr.GetNamespace(),
		},
	}
}

func (b *EncryptionConfigBuilder) Placeholder(cr client.Object) client.Object {
	return &v1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      b.Name,
			Namespace: cr.GetNamespace(),
		},
	}
}
