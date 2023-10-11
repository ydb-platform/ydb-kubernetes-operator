package storage

import (
	"context"
	"fmt"
	"reflect"
	"time"

	"github.com/ydb-platform/ydb-go-genproto/protos/Ydb_Monitoring"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	ydbCredentials "github.com/ydb-platform/ydb-go-sdk/v3/credentials"
	"github.com/ydb-platform/ydb-kubernetes-operator/api/v1alpha1"
	ydbv1alpha1 "github.com/ydb-platform/ydb-kubernetes-operator/api/v1alpha1"
	"github.com/ydb-platform/ydb-kubernetes-operator/internal/connection"
	"github.com/ydb-platform/ydb-kubernetes-operator/internal/healthcheck"
	"github.com/ydb-platform/ydb-kubernetes-operator/internal/labels"
	"github.com/ydb-platform/ydb-kubernetes-operator/internal/resources"
)

const (
	Pending      ClusterState = "Pending"
	Preparing    ClusterState = "Preparing"
	Provisioning ClusterState = "Provisioning"
	Initializing ClusterState = "Initializing"
	Ready        ClusterState = "Ready"

	DefaultRequeueDelay               = 10 * time.Second
	StatusUpdateRequeueDelay          = 1 * time.Second
	SelfCheckRequeueDelay             = 30 * time.Second
	StorageInitializationRequeueDelay = 5 * time.Second

	ReasonInProgress  = "InProgress"
	ReasonNotRequired = "NotRequired"
	ReasonCompleted   = "Completed"

	StorageInitializedCondition        = "StorageReady"
	StorageInitializedReasonInProgress = ReasonInProgress
	StorageInitializedReasonCompleted  = ReasonCompleted

	Stop     = true
	Continue = false
)

type ClusterState string

func (r *Reconciler) Sync(ctx context.Context, cr *ydbv1alpha1.Storage) (ctrl.Result, error) {
	var stop bool
	var result ctrl.Result
	var err error

	storage := resources.NewCluster(cr)
	storage.SetStatusOnFirstReconcile()

	stop, result, err = r.handleResourcesSync(ctx, &storage)
	if stop {
		return result, err
	}

	if !meta.IsStatusConditionTrue(storage.Status.Conditions, StorageInitializedCondition) {
		stop, result, err = r.setInitialStatus(ctx, &storage)
		if stop {
			return result, err
		}
		stop, result, err = r.waitForStatefulSetToScale(ctx, &storage)
		if stop {
			return result, err
		}
		stop, result, err = r.runSelfCheck(ctx, &storage, false)
		if stop {
			return result, err
		}
		stop, result, err = r.initializeStorage(ctx, &storage)
		if stop {
			return result, err
		}
	}

	_, result, err = r.runSelfCheck(ctx, &storage, false)
	return result, err
}

func (r *Reconciler) waitForStatefulSetToScale(
	ctx context.Context,
	storage *resources.StorageClusterBuilder,
) (bool, ctrl.Result, error) {
	r.Log.Info("running step waitForStatefulSetToScale for Storage")

	if storage.Status.State == string(Preparing) {
		msg := fmt.Sprintf("Starting to track number of running storage pods, expected: %d", storage.Spec.Nodes)
		r.Recorder.Event(storage, corev1.EventTypeNormal, string(Provisioning), msg)
		storage.Status.State = string(Provisioning)
		return r.setState(ctx, storage)
	}

	found := &appsv1.StatefulSet{}
	err := r.Get(ctx, types.NamespacedName{
		Name:      storage.Name,
		Namespace: storage.Namespace,
	}, found)
	if err != nil {
		if errors.IsNotFound(err) {
			return Stop, ctrl.Result{RequeueAfter: DefaultRequeueDelay}, nil
		}
		r.Recorder.Event(
			storage,
			corev1.EventTypeNormal,
			"Syncing",
			fmt.Sprintf("Failed to get StatefulSets: %s", err),
		)
		return Stop, ctrl.Result{RequeueAfter: DefaultRequeueDelay}, err
	}

	podLabels := labels.Common(storage.Name, make(map[string]string))
	podLabels.Merge(map[string]string{
		labels.ComponentKey: labels.StorageComponent,
	})

	matchingLabels := client.MatchingLabels{}
	for k, v := range podLabels {
		matchingLabels[k] = v
	}

	podList := &corev1.PodList{}
	opts := []client.ListOption{
		client.InNamespace(storage.Namespace),
		matchingLabels,
	}

	err = r.List(ctx, podList, opts...)
	if err != nil {
		r.Recorder.Event(
			storage,
			corev1.EventTypeNormal,
			"Syncing",
			fmt.Sprintf("Failed to list cluster pods: %s", err),
		)
		return Stop, ctrl.Result{RequeueAfter: DefaultRequeueDelay}, err
	}

	runningPods := 0
	for _, e := range podList.Items {
		if e.Status.Phase == "Running" {
			runningPods++
		}
	}

	if runningPods != int(storage.Spec.Nodes) {
		msg := fmt.Sprintf("Waiting for number of running storage pods to match expected: %d != %d", runningPods, storage.Spec.Nodes)
		r.Recorder.Event(storage, corev1.EventTypeNormal, string(Provisioning), msg)
		return Stop, ctrl.Result{RequeueAfter: DefaultRequeueDelay}, nil
	}

	return Continue, ctrl.Result{Requeue: false}, nil
}

func (r *Reconciler) handleResourcesSync(
	ctx context.Context,
	storage *resources.StorageClusterBuilder,
) (bool, ctrl.Result, error) {
	r.Log.Info("running step handleResourcesSync")

	for _, builder := range storage.GetResourceBuilders(r.Config) {
		newResource := builder.Placeholder(storage)

		result, err := resources.CreateOrUpdateIgnoreStatus(ctx, r.Client, newResource, func() error {
			var err error

			err = builder.Build(newResource)
			if err != nil {
				r.Recorder.Event(
					storage,
					corev1.EventTypeWarning,
					"ProvisioningFailed",
					fmt.Sprintf("Failed building resources: %s", err),
				)
				return err
			}
			err = ctrl.SetControllerReference(storage.Unwrap(), newResource, r.Scheme)
			if err != nil {
				r.Recorder.Event(
					storage,
					corev1.EventTypeWarning,
					"ProvisioningFailed",
					fmt.Sprintf("Error setting controller reference for resource: %s", err),
				)
				return err
			}

			return nil
		})

		eventMessage := fmt.Sprintf(
			"Resource: %s, Namespace: %s, Name: %s",
			reflect.TypeOf(newResource),
			newResource.GetNamespace(),
			newResource.GetName(),
		)
		if err != nil {
			r.Recorder.Event(
				storage,
				corev1.EventTypeWarning,
				"ProvisioningFailed",
				eventMessage+fmt.Sprintf(", failed to sync, error: %s", err),
			)
			return Stop, ctrl.Result{RequeueAfter: DefaultRequeueDelay}, err
		} else if result == controllerutil.OperationResultCreated || result == controllerutil.OperationResultUpdated {
			r.Recorder.Event(
				storage,
				corev1.EventTypeNormal,
				string(Provisioning),
				eventMessage+fmt.Sprintf(", changed, result: %s", result),
			)
		}
	}
	r.Log.Info("resource sync complete")
	return Continue, ctrl.Result{Requeue: false}, nil
}

func (r *Reconciler) runSelfCheck(
	ctx context.Context,
	storage *resources.StorageClusterBuilder,
	waitForGoodResultWithoutIssues bool,
) (bool, ctrl.Result, error) {
	r.Log.Info("running step runSelfCheck")
	credentials, err := r.getAuthCredentials(ctx, storage)
	if err != nil {
		r.Log.Error(err, "Error connecting to YDB storage %s", storage.Name)
		return Stop, ctrl.Result{RequeueAfter: SelfCheckRequeueDelay}, err
	}

	result, err := healthcheck.GetSelfCheckResult(ctx, storage, credentials)
	if err != nil {
		r.Log.Error(err, "GetSelfCheckResult error")
		return Stop, ctrl.Result{RequeueAfter: SelfCheckRequeueDelay}, err
	}

	eventType := corev1.EventTypeNormal
	if result.SelfCheckResult != Ydb_Monitoring.SelfCheck_GOOD {
		eventType = corev1.EventTypeWarning
	}

	r.Recorder.Event(
		storage,
		eventType,
		"SelfCheck",
		fmt.Sprintf("SelfCheck result: %s, issues found: %d", result.SelfCheckResult.String(), len(result.IssueLog)),
	)

	if waitForGoodResultWithoutIssues && result.SelfCheckResult.String() != "GOOD" {
		return Stop, ctrl.Result{RequeueAfter: SelfCheckRequeueDelay}, err
	}
	return Continue, ctrl.Result{Requeue: false}, nil
}

func (r *Reconciler) setState(
	ctx context.Context,
	storage *resources.StorageClusterBuilder,
) (bool, ctrl.Result, error) {
	storageCr := &ydbv1alpha1.Storage{}
	err := r.Get(ctx, client.ObjectKey{
		Namespace: storage.Namespace,
		Name:      storage.Name,
	}, storageCr)
	if err != nil {
		r.Recorder.Event(storageCr, corev1.EventTypeWarning, "ControllerError", "Failed fetching CR before status update")
		return Stop, ctrl.Result{RequeueAfter: DefaultRequeueDelay}, err
	}

	oldStatus := storageCr.Status.State
	storageCr.Status.State = storage.Status.State
	storageCr.Status.Conditions = storage.Status.Conditions

	err = r.Status().Update(ctx, storageCr)
	if err != nil {
		r.Recorder.Event(storageCr, corev1.EventTypeWarning, "ControllerError", fmt.Sprintf("Failed setting status: %s", err))
		return Stop, ctrl.Result{RequeueAfter: DefaultRequeueDelay}, err
	} else if oldStatus != storage.Status.State {
		r.Recorder.Event(
			storageCr,
			corev1.EventTypeNormal,
			"StatusChanged",
			fmt.Sprintf("Storage moved from %s to %s", oldStatus, storage.Status.State),
		)
	}

	return Stop, ctrl.Result{RequeueAfter: StatusUpdateRequeueDelay}, nil
}

func (r *Reconciler) getAuthCredentials(
	ctx context.Context,
	storage *resources.StorageClusterBuilder,
) (ydbCredentials.Credentials, error) {
	switch auth := storage.Spec.Auth; {
	case auth.AccessToken != nil:
		token, err := r.getSecretKey(
			ctx,
			storage.Namespace,
			auth.AccessToken.SecretKeyRef,
		)
		if err != nil {
			return nil, err
		}
		return ydbCredentials.NewAccessTokenCredentials(token), nil
	case auth.StaticCredentials != nil:
		endpoint := storage.GetGRPCEndpointWithProto()
		opts := connection.GetGRPCDialOptions(resources.IsGrpcSecure(storage.Storage))
		username := auth.StaticCredentials.Username
		password := v1alpha1.DefaultRootPassword
		if auth.StaticCredentials.SecretKeyRef != nil {
			var err error
			password, err = r.getSecretKey(
				ctx,
				storage.Namespace,
				auth.StaticCredentials.SecretKeyRef,
			)
			if err != nil {
				return nil, err
			}
		}
		return ydbCredentials.NewStaticCredentials(username, password, endpoint, opts...), nil
	default:
		return ydbCredentials.NewAnonymousCredentials(), nil
	}
}

func (r *Reconciler) getSecretKey(
	ctx context.Context,
	namespace string,
	secretKeyRef *corev1.SecretKeySelector,
) (string, error) {
	secret := &corev1.Secret{}
	err := r.Get(ctx, types.NamespacedName{
		Name:      secretKeyRef.Name,
		Namespace: namespace,
	}, secret)
	if err != nil {
		return "", err
	}
	secretVal, exist := secret.Data[secretKeyRef.Key]
	if !exist {
		return "", fmt.Errorf(
			"key %s does not exist in secretData %s",
			secretKeyRef.Key,
			secretKeyRef.Name,
		)
	}
	return string(secretVal), nil
}
