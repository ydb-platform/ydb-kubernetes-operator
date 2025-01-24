package resources

import (
	"context"
	"errors"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func GetSecretKey(
	ctx context.Context,
	namespace string,
	config *rest.Config,
	secretKeyRef *corev1.SecretKeySelector,
) (string, error) {
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return "", fmt.Errorf("failed to create kubernetes clientset, error: %w", err)
	}

	getCtx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	secret, err := clientset.CoreV1().Secrets(namespace).Get(getCtx, secretKeyRef.Name, metav1.GetOptions{})
	if err != nil {
		return "", fmt.Errorf("failed to get secret %s, error: %w", secretKeyRef.Name, err)
	}

	secretVal, exist := secret.Data[secretKeyRef.Key]
	if !exist {
		errMsg := fmt.Sprintf("key %s does not exist in secret %s", secretKeyRef.Key, secretKeyRef.Name)
		return "", errors.New(errMsg)
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
