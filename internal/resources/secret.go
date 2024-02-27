package resources

import (
	"context"
	"errors"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func GetSecretKey(
	namespace string,
	config *rest.Config,
	secretKeyRef *corev1.SecretKeySelector,
) (string, error) {
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return "", errors.New("failed to create kubernetes clientset")
	}

	secret, err := clientset.CoreV1().Secrets(namespace).
		Get(context.Background(), secretKeyRef.Name, metav1.GetOptions{})
	if err != nil {
		return "", err
	}

	secretVal, exist := secret.Data[secretKeyRef.Key]
	if !exist {
		return "", fmt.Errorf(
			"key %s does not exist in secret %s",
			secretKeyRef.Key,
			secretKeyRef.Name,
		)
	}

	return string(secretVal), nil
}

type OperatorTokenSecretBuilder struct {
	client.Object

	Name  string
	Token string
}

func (b *OperatorTokenSecretBuilder) Build(obj client.Object) error {
	secret, ok := obj.(*corev1.Secret)
	if !ok {
		return errors.New("failed to cast to Job object")
	}

	if secret.ObjectMeta.Name == "" {
		secret.ObjectMeta.Name = b.Name
	}
	secret.ObjectMeta.Namespace = b.GetNamespace()

	secret.Data = map[string][]byte{
		wellKnownNameForOperatorToken: []byte(b.Token),
	}

	return nil
}

func (b *OperatorTokenSecretBuilder) Placeholder(obj client.Object) client.Object {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      b.Name,
			Namespace: obj.GetNamespace(),
		},
	}
}

func GetOperatorTokenSecretBuilder(obj client.Object, operatorToken string) ResourceBuilder {
	return &OperatorTokenSecretBuilder{
		Object: obj,

		Name:  fmt.Sprintf(OperatorTokenSecretNameFormat, obj.GetName()),
		Token: operatorToken,
	}
}
