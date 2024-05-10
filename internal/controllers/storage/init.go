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

func (r *Reconciler) processSkipInitPipeline(
	ctx context.Context,
	storage *resources.StorageClusterBuilder,
) (bool, ctrl.Result, error) {
	r.Log.Info("running step processSkipInitPipeline")
	r.Log.Info("Storage initialization disabled (with annotation), proceed with caution")

	r.Recorder.Event(
		storage,
		corev1.EventTypeWarning,
		"SkippingInit",
		"Skipping initialization due to skip annotation present, be careful!",
	)

	meta.SetStatusCondition(&storage.Status.Conditions, metav1.Condition{
		Type:    StorageInitializedCondition,
		Status:  "True",
		Reason:  ReasonCompleted,
		Message: "Storage initialization not performed because initialization is skipped",
	})
	storage.Status.State = StorageReady
	return r.updateStatus(ctx, storage)
}

func (r *Reconciler) setInitialStatus(
	ctx context.Context,
	storage *resources.StorageClusterBuilder,
) (bool, ctrl.Result, error) {
	r.Log.Info("running step setInitialStatus")

	if meta.IsStatusConditionTrue(storage.Status.Conditions, OldStorageInitializedCondition) {
		meta.SetStatusCondition(&storage.Status.Conditions, metav1.Condition{
			Type:    StorageInitializedCondition,
			Status:  "True",
			Reason:  ReasonCompleted,
			Message: "Storage initialized successfully",
		})
		storage.Status.State = StorageReady
		return r.updateStatus(ctx, storage)
	}

	// This block is special internal logic that skips all Storage initialization.
	// It is needed when large clusters are migrated where `waitForStatefulSetToScale`
	// does not make sense, since some nodes can be down for a long time (and it is okay, since
	// database is healthy even with partial outage).
	if value, ok := storage.Annotations[v1alpha1.AnnotationSkipInitialization]; ok && value == v1alpha1.AnnotationValueTrue {
		if meta.FindStatusCondition(storage.Status.Conditions, StorageInitializedCondition) == nil ||
			meta.IsStatusConditionFalse(storage.Status.Conditions, StorageInitializedCondition) {
			return r.processSkipInitPipeline(ctx, storage)
		}
		return Stop, ctrl.Result{RequeueAfter: StorageInitializationRequeueDelay}, nil
	}

	if storage.Status.State == StoragePending ||
		meta.FindStatusCondition(storage.Status.Conditions, StorageInitializedCondition) == nil {
		meta.SetStatusCondition(&storage.Status.Conditions, metav1.Condition{
			Type:    StorageInitializedCondition,
			Status:  "False",
			Reason:  ReasonInProgress,
			Message: "Storage has not been initialized yet",
		})
		storage.Status.State = StoragePreparing
		return r.updateStatus(ctx, storage)
	}
	return Continue, ctrl.Result{Requeue: false}, nil
}

func (r *Reconciler) setInitStorageCompleted(
	ctx context.Context,
	storage *resources.StorageClusterBuilder,
	message string,
) (bool, ctrl.Result, error) {
	meta.SetStatusCondition(&storage.Status.Conditions, metav1.Condition{
		Type:    StorageInitializedCondition,
		Status:  "True",
		Reason:  ReasonCompleted,
		Message: message,
	})
	storage.Status.State = StorageProvisioning
	return r.updateStatus(ctx, storage)
}

func (r *Reconciler) initializeStorage(
	ctx context.Context,
	storage *resources.StorageClusterBuilder,
) (bool, ctrl.Result, error) {
	r.Log.Info("running step initializeStorage")

	if storage.Status.State == StoragePreparing {
		storage.Status.State = StorageInitializing
		return r.updateStatus(ctx, storage)
	}

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

	//nolint:nestif
	if initJob.Status.Failed > 0 {
		initialized, err := r.checkFailedJob(ctx, storage, initJob)
		if err != nil {
			r.Recorder.Event(
				storage,
				corev1.EventTypeWarning,
				"ControllerError",
				fmt.Sprintf("Failed to check logs from failed Pod for Job: %s", err),
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
			r.Log.Info("Init Job status failed")
			r.Recorder.Event(
				storage,
				corev1.EventTypeWarning,
				"InitializingStorage",
				"Failed to initializing Storage",
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
		}
	}

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
