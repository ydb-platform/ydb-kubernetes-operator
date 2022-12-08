package resources

import (
	"context"
	"errors"

	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

func checkSecretHasField(
	namespace string,
	secretName string,
	secretField string,
	config *rest.Config,
) (bool, error) {
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return false, errors.New("failed to create kubernetes clientset")
	}

	req, err := clientset.
		CoreV1().
		Secrets(namespace).
		Get(context.TODO(), secretName, v1.GetOptions{})
	if err != nil {
		return false, err
	}

	_, exists := req.Data[secretField]

	return exists, nil
}
