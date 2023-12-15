package v1alpha1

import (
	"fmt"

	"github.com/go-logr/logr"
	kube "k8s.io/client-go/kubernetes"
	ctrl "sigs.k8s.io/controller-runtime"
)

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
