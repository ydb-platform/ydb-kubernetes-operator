package v1alpha1

import (
	"context"
	"fmt"
	"strings"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	kube "k8s.io/client-go/kubernetes"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// generateValidatePath is a copy from controller-runtime
func generateValidatePath(gvk schema.GroupVersionKind) string {
	return "/validate-" + strings.ReplaceAll(gvk.Group, ".", "-") + "-" +
		gvk.Version + "-" + strings.ToLower(gvk.Kind)
}

func getStorageRef(ctx context.Context, client client.Client, namespace string, name string) (*Storage, error) {
	found := &Storage{}
	err := client.Get(ctx, types.NamespacedName{Namespace: namespace, Name: name}, found)
	fmt.Printf("err = %v, namespace=%s, name=%s\n", err, namespace, name)
	if err != nil {
		return nil, err
	}
	return found, nil
}

func getDatabaseRef(ctx context.Context, client client.Client, namespace string, name string) (*Database, error) {
	found := &Database{}
	err := client.Get(ctx, types.NamespacedName{Namespace: namespace, Name: name}, found)
	if err != nil {
		return nil, err
	}
	return found, nil
}

func checkMonitoringCRD(manager ctrl.Manager, logger logr.Logger, monitoringEnabled bool) error {
	if monitoringEnabled {
		return nil
	}

	config := manager.GetConfig()
	clientset, err := kube.NewForConfig(config)
	if err != nil {
		logger.Error(err, "unable to get clientset while checking monitoring CRD")
		return nil
	}
	_, resources, err := clientset.ServerGroupsAndResources()
	if err != nil {
		logger.Error(err, "unable to get ServerGroupsAndResources while checking monitoring CRD")
		return nil
	}

	foundMonitoring := false
	for _, resource := range resources {
		if resource.GroupVersion == "monitoring.coreos.com/v1" {
			for _, res := range resource.APIResources {
				if res.Kind == "ServiceMonitor" {
					foundMonitoring = true
				}
			}
		}
	}
	if foundMonitoring {
		return nil
	}
	crdError := fmt.Errorf("required Prometheus CRDs not found in the cluster: `monitoring.coreos.com/v1/servicemonitor`. Please make sure your Prometheus installation is healthy, `kubectl get servicemonitors` must be non-empty")
	return crdError
}
