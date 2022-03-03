package resources

import (
	"errors"

	"github.com/ydb-platform/ydb-kubernetes-operator/internal/encryption"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type EncryptionSecretBuilder struct {
	client.Object

	Pin    string
	Labels map[string]string
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

	// Do not update already existing secret data
	if (sec.StringData == nil || len(sec.StringData) == 0) && (sec.Data == nil || len(sec.Data) == 0) {
		key, err := encryption.GenerateRSAKey(b.Pin)
		if err != nil {
			return err
		}
		sec.StringData = map[string]string{
			defaultEncryptionSecretKey: key,
		}
	}
	sec.Labels = b.Labels
	sec.Type = corev1.SecretTypeOpaque

	return nil
}

func (b *EncryptionSecretBuilder) Placeholder(cr client.Object) client.Object {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cr.GetName(),
			Namespace: cr.GetNamespace(),
		},
	}
}
