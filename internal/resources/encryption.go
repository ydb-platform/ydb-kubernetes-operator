package resources

import (
	"errors"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/ydb-platform/ydb-kubernetes-operator/internal/encryption"
)

type EncryptionSecretBuilder struct {
	client.Object

	Labels map[string]string
	Pin    string
}

func (b *EncryptionSecretBuilder) Build(obj client.Object) error {
	sec, ok := obj.(*corev1.Secret)
	if !ok {
		return errors.New("failed to cast to Secret object")
	}

	if sec.ObjectMeta.Name == "" {
		sec.ObjectMeta.Name = b.GetName()
	}
	sec.ObjectMeta.Namespace = b.GetNamespace()

	sec.Labels = b.Labels

	key, err := encryption.GenerateRSAKey(b.Pin)
	if err != nil {
		return fmt.Errorf("failed to generate key for encryption: %w", err)
	}

	sec.StringData = map[string]string{
		wellKnownNameForEncryptionKeySecret: key,
	}

	sec.Type = corev1.SecretTypeOpaque

	return nil
}

func (b *EncryptionSecretBuilder) Placeholder(cr client.Object) client.Object {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      b.GetName(),
			Namespace: cr.GetNamespace(),
		},
	}
}
