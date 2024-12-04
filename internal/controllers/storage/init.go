package storage

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	ydbCredentials "github.com/ydb-platform/ydb-go-sdk/v3/credentials"
	"google.golang.org/grpc/metadata"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/ydb-platform/ydb-kubernetes-operator/api/v1alpha1"
	. "github.com/ydb-platform/ydb-kubernetes-operator/internal/controllers/constants" //nolint:revive,stylecheck
	"github.com/ydb-platform/ydb-kubernetes-operator/internal/resources"
)

var mismatchItemConfigGenerationRegexp = regexp.MustCompile(".*mismatch.*ItemConfigGenerationProvided# " +
	"0.*ItemConfigGenerationExpected# 1.*")

func (r *Reconciler) setInitPipelineStatus(
	ctx context.Context,
	storage *resources.StorageClusterBuilder,
) (bool, ctrl.Result, error) {
	if storage.Status.State == StoragePreparing {
		meta.SetStatusCondition(&storage.Status.Conditions, metav1.Condition{
			Type:               StorageInitializedCondition,
			Status:             metav1.ConditionUnknown,
			Reason:             ReasonInProgress,
			ObservedGeneration: storage.Generation,
			Message:            "Storage has not been initialized yet",
		})
		storage.Status.State = StorageInitializing
		return r.updateStatus(ctx, storage, StatusUpdateRequeueDelay)
	}

	// This block is special internal logic that skips all Storage initialization.
	if value, ok := storage.Annotations[v1alpha1.AnnotationSkipInitialization]; ok && value == v1alpha1.AnnotationValueTrue {
		r.Log.Info("Storage initialization disabled (with annotation), proceed with caution")
		r.Recorder.Event(
			storage,
			corev1.EventTypeWarning,
			"SkippingInit",
			"Skipping initialization due to skip annotation present, be careful!",
		)
		return r.setInitStorageCompleted(ctx, storage, "Storage initialization not performed because initialization is skipped")
	}

	if meta.IsStatusConditionTrue(storage.Status.Conditions, OldStorageInitializedCondition) {
		return r.setInitStorageCompleted(ctx, storage, "Storage initialized successfully")
	}
	return Continue, ctrl.Result{Requeue: false}, nil
}

func (r *Reconciler) setInitStorageCompleted(
	ctx context.Context,
	storage *resources.StorageClusterBuilder,
	message string,
) (bool, ctrl.Result, error) {
	meta.SetStatusCondition(&storage.Status.Conditions, metav1.Condition{
		Type:               StorageInitializedCondition,
		Status:             metav1.ConditionTrue,
		Reason:             ReasonCompleted,
		ObservedGeneration: storage.Generation,
		Message:            message,
	})
	return r.updateStatus(ctx, storage, StatusUpdateRequeueDelay)
}

func (r *Reconciler) initializeBlobstorage(
	ctx context.Context,
	storage *resources.StorageClusterBuilder,
) (bool, ctrl.Result, error) {
	initJob := &batchv1.Job{}
	err := r.Get(ctx, types.NamespacedName{
		Name:      fmt.Sprintf(resources.InitJobNameFormat, storage.Name),
		Namespace: storage.Namespace,
	}, initJob)

	//nolint:nestif
	if apierrors.IsNotFound(err) {
		if storage.Spec.OperatorConnection != nil {
			creds, err := resources.GetYDBCredentials(ctx, storage.Unwrap(), r.Config)
			if err != nil {
				r.Recorder.Event(
					storage,
					corev1.EventTypeWarning,
					"ControllerError",
					fmt.Sprintf("Failed to get YDB credentials: %s", err),
				)
				return Stop, ctrl.Result{RequeueAfter: DefaultRequeueDelay}, err
			}
			if err := r.createOrUpdateOperatorTokenSecret(ctx, storage, creds); err != nil {
				r.Recorder.Event(
					storage,
					corev1.EventTypeWarning,
					"InitializingStorage",
					fmt.Sprintf("Failed to create operator token Secret, error: %s", err),
				)
				return Stop, ctrl.Result{RequeueAfter: DefaultRequeueDelay}, err
			}
		}

		if err := r.createInitBlobstorageJob(ctx, storage); err != nil {
			r.Recorder.Event(
				storage,
				corev1.EventTypeWarning,
				"InitializingStorage",
				fmt.Sprintf("Failed to create init blobstorage Job, error: %s", err),
			)
			return Stop, ctrl.Result{RequeueAfter: DefaultRequeueDelay}, err
		}

		r.Recorder.Event(
			storage,
			corev1.EventTypeNormal,
			"InitializingStorage",
			fmt.Sprintf("Successfully created Job %s", fmt.Sprintf(resources.InitJobNameFormat, storage.Name)),
		)
		return Stop, ctrl.Result{RequeueAfter: StorageInitializationRequeueDelay}, nil
	}

	if err != nil {
		r.Recorder.Event(
			storage,
			corev1.EventTypeWarning,
			"ControllerError",
			fmt.Sprintf("Failed to get Job: %s", err),
		)
		return Stop, ctrl.Result{RequeueAfter: DefaultRequeueDelay}, err
	}

	if initJob.Status.Succeeded > 0 {
		r.Log.Info("Init Job status succeeded")
		r.Recorder.Event(
			storage,
			corev1.EventTypeNormal,
			"InitializingStorage",
			"Storage initialized successfully",
		)
		return r.setInitStorageCompleted(ctx, storage, "Storage initialized successfully")
	}

	var conditionFailed bool
	for _, condition := range initJob.Status.Conditions {
		if condition.Type == batchv1.JobFailed {
			conditionFailed = true
			break
		}
	}

	initialized, err := r.checkFailedJob(ctx, storage, initJob)
	if err != nil {
		r.Recorder.Event(
			storage,
			corev1.EventTypeWarning,
			"ControllerError",
			fmt.Sprintf("Failed to check logs for initBlobstorage Job: %s", err),
		)
		return Stop, ctrl.Result{RequeueAfter: DefaultRequeueDelay}, err
	}

	if initialized {
		r.Log.Info("Storage is already initialized, continuing...")
		r.Recorder.Event(
			storage,
			corev1.EventTypeNormal,
			"InitializingStorage",
			"Storage initialization attempted and skipped, storage already initialized",
		)
		if err := r.Delete(ctx, initJob, client.PropagationPolicy(metav1.DeletePropagationForeground)); err != nil {
			r.Recorder.Event(
				storage,
				corev1.EventTypeWarning,
				"ControllerError",
				fmt.Sprintf("Failed to delete Job: %s", err),
			)
			return Stop, ctrl.Result{RequeueAfter: DefaultRequeueDelay}, err
		}
		return r.setInitStorageCompleted(ctx, storage, "Storage already initialized")
	}

	if initJob.Status.Failed == *initJob.Spec.BackoffLimit || conditionFailed {
		r.Recorder.Event(
			storage,
			corev1.EventTypeWarning,
			"InitializingStorage",
			"Failed initBlobstorage Job, check Pod logs for addditional info",
		)
		meta.SetStatusCondition(&storage.Status.Conditions, metav1.Condition{
			Type:               StorageInitializedCondition,
			Status:             metav1.ConditionFalse,
			Reason:             ReasonInProgress,
			ObservedGeneration: storage.Generation,
		})
		if err := r.Delete(ctx, initJob, client.PropagationPolicy(metav1.DeletePropagationForeground)); err != nil {
			r.Recorder.Event(
				storage,
				corev1.EventTypeWarning,
				"ControllerError",
				fmt.Sprintf("Failed to delete initBlobstorage Job: %s", err),
			)
			return Stop, ctrl.Result{RequeueAfter: DefaultRequeueDelay}, err
		}

		return r.updateStatus(ctx, storage, StatusUpdateRequeueDelay)
	}

	r.Recorder.Event(
		storage,
		corev1.EventTypeNormal,
		"InitializingStorage",
		fmt.Sprintf("Waiting for Job %s status update", initJob.Name),
	)
	return Stop, ctrl.Result{RequeueAfter: StorageInitializationRequeueDelay}, nil
}

func (r *Reconciler) checkFailedJob(
	ctx context.Context,
	storage *resources.StorageClusterBuilder,
	job *batchv1.Job,
) (bool, error) {
	podList := &corev1.PodList{}
	opts := []client.ListOption{
		client.InNamespace(storage.Namespace),
		client.MatchingLabels{
			"job-name": job.Name,
		},
	}
	if err := r.List(ctx, podList, opts...); err != nil {
		r.Recorder.Event(
			storage,
			corev1.EventTypeWarning,
			"ControllerError",
			fmt.Sprintf("Failed to list pods for Job: %s", err),
		)
		return false, fmt.Errorf("failed to list pods for checkFailedJob, error: %w", err)
	}

	for _, pod := range podList.Items {
		if pod.Status.Phase == corev1.PodFailed {
			clientset, err := kubernetes.NewForConfig(r.Config)
			if err != nil {
				return false, fmt.Errorf("failed to initialize clientset for checkFailedJob, error: %w", err)
			}

			podLogs, err := getPodLogs(ctx, clientset, storage.Namespace, pod.Name)
			if err != nil {
				return false, fmt.Errorf("failed to get pod logs for checkFailedJob, error: %w", err)
			}

			if mismatchItemConfigGenerationRegexp.MatchString(podLogs) {
				return true, nil
			}
		}
	}
	return false, nil
}

func getPodLogs(ctx context.Context, clientset *kubernetes.Clientset, namespace, name string) (string, error) {
	var logsBuilder strings.Builder

	streamCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	podLogs, err := clientset.CoreV1().
		Pods(namespace).
		GetLogs(name, &corev1.PodLogOptions{}).
		Stream(streamCtx)
	if err != nil {
		return "", fmt.Errorf("failed to stream GetLogs from pod %s/%s, error: %w", namespace, name, err)
	}
	defer podLogs.Close()

	buf := make([]byte, 4096)
	for {
		numBytes, err := podLogs.Read(buf)
		if numBytes == 0 && err != nil {
			break
		}
		logsBuilder.Write(buf[:numBytes])
	}

	return logsBuilder.String(), nil
}

func shouldIgnoreJobUpdate() resources.IgnoreChangesFunction {
	return func(oldObj, newObj runtime.Object) bool {
		if _, ok := oldObj.(*batchv1.Job); ok {
			return true
		}
		return false
	}
}

func (r *Reconciler) createInitBlobstorageJob(
	ctx context.Context,
	storage *resources.StorageClusterBuilder,
) error {
	builder := resources.GetInitJobBuilder(storage.DeepCopy())
	newResource := builder.Placeholder(storage)
	_, err := resources.CreateOrUpdateOrMaybeIgnore(ctx, r.Client, newResource, func() error {
		var err error

		err = builder.Build(newResource)
		if err != nil {
			return err
		}
		err = ctrl.SetControllerReference(storage.Unwrap(), newResource, r.Scheme)
		if err != nil {
			return err
		}

		return nil
	}, shouldIgnoreJobUpdate())

	return err
}

func (r *Reconciler) createOrUpdateOperatorTokenSecret(
	ctx context.Context,
	storage *resources.StorageClusterBuilder,
	creds ydbCredentials.Credentials,
) error {
	ydbCtx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	token, err := creds.Token(
		metadata.AppendToOutgoingContext(ydbCtx, "x-ydb-database", storage.Spec.Domain),
	)
	if err != nil {
		return fmt.Errorf("failed to get token from ydb credentials for createOrUpdateOperatorTokenSecret, error: %w", err)
	}

	builder := resources.GetOperatorTokenSecretBuilder(storage, token)
	newResource := builder.Placeholder(storage)
	_, err = resources.CreateOrUpdateOrMaybeIgnore(ctx, r.Client, newResource, func() error {
		var err error

		err = builder.Build(newResource)
		if err != nil {
			return err
		}
		err = ctrl.SetControllerReference(storage.Unwrap(), newResource, r.Scheme)
		if err != nil {
			return err
		}

		return nil
	}, resources.DoNotIgnoreChanges())

	return err
}
