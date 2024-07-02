package resources

import (
	"bytes"
	"errors"
	"fmt"
	"html/template"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	api "github.com/ydb-platform/ydb-kubernetes-operator/api/v1alpha1"
	"github.com/ydb-platform/ydb-kubernetes-operator/internal/configuration/schema"
)

const keyConfigTmpl = `Keys {
    ContainerPath: "{{ .ContainerPath }}"
    Pin: "{{ .Pin }}"
    Id: "{{ .ID }}"
    Version: {{ .Version }}
}`

type ConfigMapBuilder struct {
	client.Object

	Name   string
	Labels map[string]string

	Data map[string]string
}

type EncryptionConfigBuilder struct {
	client.Object

	Name   string
	Labels map[string]string

	KeyConfig schema.KeyConfig
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

	cm.Labels = b.Labels

	cm.Data = b.Data

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

	cm.Labels = b.Labels

	t, err := template.New("keyConfig").Parse(keyConfigTmpl)
	if err != nil {
		return fmt.Errorf("failed to parse keyConfig template: %w", err)
	}

	var buf bytes.Buffer
	err = t.Execute(&buf, b.KeyConfig.Keys[0])
	if err != nil {
		return fmt.Errorf("failed to execute keyConfig template: %w", err)
	}

	cm.Data = map[string]string{api.DatabaseEncryptionKeyConfigFile: buf.String()}

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
