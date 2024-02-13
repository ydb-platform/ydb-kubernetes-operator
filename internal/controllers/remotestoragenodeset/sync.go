package remotestoragenodeset

import (
	"context"
	"fmt"
	"reflect"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	ydbv1alpha1 "github.com/ydb-platform/ydb-kubernetes-operator/api/v1alpha1"
	. "github.com/ydb-platform/ydb-kubernetes-operator/internal/controllers/constants" //nolint:revive,stylecheck
	"github.com/ydb-platform/ydb-kubernetes-operator/internal/resources"
)

func (r *Reconciler) Sync(ctx context.Context, crRemoteStorageNodeSet *ydbv1alpha1.RemoteStorageNodeSet) (ctrl.Result, error) {
	var stop bool
	var result ctrl.Result
	var err error

	remoteStorageNodeSet := resources.NewRemoteStorageNodeSet(crRemoteStorageNodeSet)
	remoteSecrets := getRemoteSecrets(crRemoteStorageNodeSet)
	remoteServices := getRemoteServices(crRemoteStorageNodeSet)

	for _, secret := range remoteSecrets {
		stop, result, err = r.syncRemoteObject(ctx, &remoteStorageNodeSet, &secret)
		if stop {
			return result, err
		}
	}

	for _, service := range remoteServices {
		stop, result, err = r.syncRemoteObject(ctx, &remoteStorageNodeSet, &service)
		if stop {
			return result, err
		}
	}

	stop, result, err = r.handleResourcesSync(ctx, &remoteStorageNodeSet)
	if stop {
		return result, err
	}

	stop, result, err = r.updateRemoteStatus(ctx, &remoteStorageNodeSet)
	if stop {
		return result, err
	}

	return result, err
}

func (r *Reconciler) handleResourcesSync(
	ctx context.Context,
	remoteStorageNodeSet *resources.RemoteStorageNodeSetResource,
) (bool, ctrl.Result, error) {
	r.Log.Info("running step handleResourcesSync")

	for _, builder := range remoteStorageNodeSet.GetResourceBuilders() {
		newResource := builder.Placeholder(remoteStorageNodeSet)

		result, err := resources.CreateOrUpdateOrMaybeIgnore(ctx, r.Client, newResource, func() error {
			err := builder.Build(newResource)
			if err != nil {
				r.Recorder.Event(
					remoteStorageNodeSet,
					corev1.EventTypeWarning,
					"ProvisioningFailed",
					fmt.Sprintf("Failed building resources: %s", err),
				)
				return err
			}
			setPrimaryResourceAnnotations(newResource)

			return nil
		}, func(oldObj, newObj runtime.Object) bool {
			return false
		})

		eventMessage := fmt.Sprintf(
			"Resource: %s, Namespace: %s, Name: %s",
			reflect.TypeOf(newResource),
			newResource.GetNamespace(),
			newResource.GetName(),
		)
		if err != nil {
			r.Recorder.Event(
				remoteStorageNodeSet,
				corev1.EventTypeWarning,
				"ProvisioningFailed",
				eventMessage+fmt.Sprintf(", failed to sync, error: %s", err),
			)
			return Stop, ctrl.Result{RequeueAfter: DefaultRequeueDelay}, err
		} else if result == controllerutil.OperationResultCreated || result == controllerutil.OperationResultUpdated {
			r.Recorder.Event(
				remoteStorageNodeSet,
				corev1.EventTypeNormal,
				"Provisioning",
				eventMessage+fmt.Sprintf(", changed, result: %s", result),
			)
		}
	}
	r.Log.Info("resource sync complete")
	return Continue, ctrl.Result{Requeue: false}, nil
}

func (r *Reconciler) syncRemoteObject(
	ctx context.Context,
	remoteStorageNodeSet *resources.RemoteStorageNodeSetResource,
	remoteObj client.Object,
) (bool, ctrl.Result, error) {
	r.Log.Info("running step handleRemoteObjectSync")

	remoteObjGVK, err := apiutil.GVKForObject(remoteObj, r.Scheme)
	if err != nil {
		r.Log.Error(err, "does not recognize GVK for resource")
		return Stop, ctrl.Result{RequeueAfter: DefaultRequeueDelay}, nil
	}
	remoteObjKind := remoteObjGVK.Kind
	remoteObjName := remoteObj.GetName()
	remoteObjNamespace := remoteObj.GetNamespace()

	err = r.RemoteClient.Get(ctx, types.NamespacedName{
		Name:      remoteObjName,
		Namespace: remoteObjNamespace,
	}, remoteObj)
	if err != nil {
		if errors.IsNotFound(err) {
			r.Recorder.Event(
				remoteStorageNodeSet,
				corev1.EventTypeWarning,
				"ProvisioningFailed",
				fmt.Sprintf("Resource %s with name %s was not found on remote cluster: %s", remoteObjKind, remoteObjName, err),
			)
			r.RemoteRecorder.Event(
				remoteStorageNodeSet,
				corev1.EventTypeWarning,
				"ProvisioningFailed",
				fmt.Sprintf("Resource %s with name %s was not found: %s", remoteObjKind, remoteObjName, err),
			)
			return Stop, ctrl.Result{RequeueAfter: DefaultRequeueDelay}, nil
		}
		r.Recorder.Event(
			remoteStorageNodeSet,
			corev1.EventTypeWarning,
			"ControllerError",
			fmt.Sprintf("Failed to get resource %s with name %s on remote cluster: %s", remoteObjKind, remoteObjName, err),
		)
		r.RemoteRecorder.Event(
			remoteStorageNodeSet,
			corev1.EventTypeWarning,
			"ProvisioningFailed",
			fmt.Sprintf("Resource %s with name %s was not found: %s", remoteObjKind, remoteObjName, err),
		)
		return Stop, ctrl.Result{RequeueAfter: DefaultRequeueDelay}, err
	}
	// get current resource version from remoteObj
	remoteObjResourceVersion := remoteObj.GetResourceVersion()
	// update remoteObj with primary-resource annotations
	setPrimaryResourceAnnotations(remoteObj)
	// use unstrsuctured objects for client to use generic
	localObj := &unstructured.Unstructured{}
	localObj.SetGroupVersionKind(remoteObjGVK)
	err = r.Client.Get(ctx, types.NamespacedName{
		Name:      remoteObjName,
		Namespace: remoteObjNamespace,
	}, localObj)
	if err != nil {
		if errors.IsNotFound(err) {
			if err := r.Client.Create(ctx, remoteObj); err != nil {
				r.Recorder.Event(
					remoteStorageNodeSet,
					corev1.EventTypeWarning,
					"ControllerError",
					fmt.Sprintf("Failed to create resource %s with name %s: %s", remoteObjKind, remoteObjName, err),
				)
				return Stop, ctrl.Result{RequeueAfter: DefaultRequeueDelay}, nil
			}
			r.Recorder.Event(
				remoteStorageNodeSet,
				corev1.EventTypeNormal,
				"Provisioning",
				fmt.Sprintf("Initial sync resource %s with name %s resourceVersion %s", remoteObjKind, remoteObjName, remoteObjResourceVersion),
			)
			return Continue, ctrl.Result{Requeue: false}, nil
		}
		r.Recorder.Event(
			remoteStorageNodeSet,
			corev1.EventTypeWarning,
			"ControllerError",
			fmt.Sprintf("Failed to get resource %s with name %s on remote cluster: %s", remoteObjKind, remoteObjName, err),
		)
		return Stop, ctrl.Result{RequeueAfter: DefaultRequeueDelay}, err
	}

	observedResourceVersion, exist := localObj.GetAnnotations()[PrimaryResourceVersionAnnotation]
	if !exist {
		r.Recorder.Event(
			remoteStorageNodeSet,
			corev1.EventTypeWarning,
			"ProvisioningFailed",
			fmt.Sprintf("Failed to find annotation %s with primary resource for %s with name %s", PrimaryResourceVersionAnnotation, remoteObjKind, remoteObjName),
		)
		if err := r.Client.Create(ctx, remoteObj); err != nil {
			r.Recorder.Event(
				remoteStorageNodeSet,
				corev1.EventTypeWarning,
				"ControllerError",
				fmt.Sprintf("Failed to create resource %s with name %s resourceVersion %s: %s", remoteObjKind, remoteObjName, remoteObjResourceVersion, err),
			)
			return Stop, ctrl.Result{RequeueAfter: DefaultRequeueDelay}, nil
		}
		r.Recorder.Event(
			remoteStorageNodeSet,
			corev1.EventTypeNormal,
			"Provisioning",
			fmt.Sprintf("Initial sync resource %s with name %s resourceVersion %s", remoteObjKind, remoteObjName, remoteObjResourceVersion),
		)
		return Continue, ctrl.Result{Requeue: false}, nil
	}
	if remoteObjResourceVersion != observedResourceVersion {
		if err := r.Client.Update(ctx, remoteObj); err != nil {
			r.Recorder.Event(
				remoteStorageNodeSet,
				corev1.EventTypeWarning,
				"ControllerError",
				fmt.Sprintf("Failed to update resource %s with name %s to resourceVersion %s: %s", remoteObjKind, remoteObjName, remoteObjResourceVersion, err),
			)
			return Stop, ctrl.Result{RequeueAfter: DefaultRequeueDelay}, nil
		}
		r.Recorder.Event(
			remoteStorageNodeSet,
			corev1.EventTypeNormal,
			"Provisioning",
			fmt.Sprintf("Sync resource %s with name %s resourceVersion %s", remoteObjKind, remoteObjName, remoteObjResourceVersion),
		)
	}
	return Continue, ctrl.Result{Requeue: false}, nil
}

func (r *Reconciler) updateRemoteStatus(
	ctx context.Context,
	remoteStorageNodeSet *resources.RemoteStorageNodeSetResource,
) (bool, ctrl.Result, error) {
	r.Log.Info("running step updateStatus")

	storageNodeSet := &ydbv1alpha1.StorageNodeSet{}
	err := r.Client.Get(ctx, types.NamespacedName{
		Name:      remoteStorageNodeSet.Name,
		Namespace: remoteStorageNodeSet.Namespace,
	}, storageNodeSet)
	if err != nil {
		r.Recorder.Event(
			remoteStorageNodeSet,
			corev1.EventTypeWarning,
			"ControllerError",
			"Failed fetching StorageNodeSet before status update",
		)
		return Stop, ctrl.Result{RequeueAfter: DefaultRequeueDelay}, err
	}

	oldStatus := remoteStorageNodeSet.Status.State
	remoteStorageNodeSet.Status.State = storageNodeSet.Status.State
	remoteStorageNodeSet.Status.Conditions = storageNodeSet.Status.Conditions

	err = r.RemoteClient.Status().Update(ctx, remoteStorageNodeSet.RemoteStorageNodeSet)
	if err != nil {
		r.Recorder.Event(
			remoteStorageNodeSet,
			corev1.EventTypeWarning,
			"ControllerError",
			fmt.Sprintf("Failed setting status on remote cluster: %s", err),
		)
		r.RemoteRecorder.Event(
			remoteStorageNodeSet,
			corev1.EventTypeWarning,
			"ControllerError",
			fmt.Sprintf("Failed setting status: %s", err),
		)
		return Stop, ctrl.Result{RequeueAfter: DefaultRequeueDelay}, err
	} else if oldStatus != remoteStorageNodeSet.Status.State {
		r.Recorder.Event(
			remoteStorageNodeSet,
			corev1.EventTypeNormal,
			"StatusChanged",
			fmt.Sprintf("StorageNodeSet moved from %s to %s on remote cluster", oldStatus, remoteStorageNodeSet.Status.State),
		)
		r.RemoteRecorder.Event(
			remoteStorageNodeSet,
			corev1.EventTypeNormal,
			"StatusChanged",
			fmt.Sprintf("StorageNodeSet moved from %s to %s", oldStatus, remoteStorageNodeSet.Status.State),
		)
	}

	return Continue, ctrl.Result{Requeue: false}, nil
}

func setPrimaryResourceAnnotations(obj client.Object) {
	annotations := obj.GetAnnotations()
	if annotations == nil {
		annotations = make(map[string]string)
	}

	annotations[PrimaryResourceNameAnnotation] = obj.GetName()
	annotations[PrimaryResourceNamespaceAnnotation] = obj.GetNamespace()
	annotations[PrimaryResourceTypeAnnotation] = obj.GetObjectKind().GroupVersionKind().Kind
	annotations[PrimaryResourceVersionAnnotation] = obj.GetResourceVersion()

	obj.SetAnnotations(annotations)
}

func getRemoteSecrets(crRemoteStorageNodeSet *ydbv1alpha1.RemoteStorageNodeSet) []corev1.Secret {
	remoteSecrets := []corev1.Secret{}
	for _, secret := range crRemoteStorageNodeSet.Spec.Secrets {
		remoteSecrets = append(remoteSecrets,
			corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      secret.Name,
					Namespace: crRemoteStorageNodeSet.Namespace,
				},
			})
	}
	return remoteSecrets
}

func getRemoteServices(crRemoteStorageNodeSet *ydbv1alpha1.RemoteStorageNodeSet) []corev1.Service {
	remoteServices := []corev1.Service{}
	remoteServices = append(remoteServices,
		corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      fmt.Sprintf(resources.GRPCServiceNameFormat, crRemoteStorageNodeSet.Spec.StorageRef.Name),
				Namespace: crRemoteStorageNodeSet.Namespace,
			},
		},
		corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      fmt.Sprintf(resources.InterconnectServiceNameFormat, crRemoteStorageNodeSet.Spec.StorageRef.Name),
				Namespace: crRemoteStorageNodeSet.Namespace,
			},
		},
		corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      fmt.Sprintf(resources.StatusServiceNameFormat, crRemoteStorageNodeSet.Spec.StorageRef.Name),
				Namespace: crRemoteStorageNodeSet.Namespace,
			},
		},
	)
	return remoteServices
}
