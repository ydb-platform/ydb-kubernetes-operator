package storage

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	ydbCredentials "github.com/ydb-platform/ydb-go-sdk/v3/credentials"
	"google.golang.org/grpc/metadata"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	"github.com/ydb-platform/ydb-kubernetes-operator/api/v1alpha1"
	. "github.com/ydb-platform/ydb-kubernetes-operator/internal/controllers/constants" //nolint:revive,stylecheck
	"github.com/ydb-platform/ydb-kubernetes-operator/internal/ptr"
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

	return r.setInitStorageCompleted(
		ctx,
		storage,
		"Storage initialization not performed because initialization is skipped",
	)
}

func (r *Reconciler) setInitialStatus(
	ctx context.Context,
	storage *resources.StorageClusterBuilder,
) (bool, ctrl.Result, error) {
	r.Log.Info("running step setInitialStatus")

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

	changed := false
	if meta.FindStatusCondition(storage.Status.Conditions, StorageInitializedCondition) == nil {
		meta.SetStatusCondition(&storage.Status.Conditions, metav1.Condition{
			Type:    StorageInitializedCondition,
			Status:  "False",
			Reason:  ReasonInProgress,
			Message: "Storage is not ready yet",
		})
		changed = true
	}
	if storage.Status.State == StoragePending {
		storage.Status.State = StoragePreparing
		changed = true
	}
	if changed {
		return r.setState(ctx, storage)
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

	storage.Status.State = StorageReady
	return r.setState(ctx, storage)
}

func (r *Reconciler) initializeStorage(
	ctx context.Context,
	storage *resources.StorageClusterBuilder,
	creds ydbCredentials.Credentials,
) (bool, ctrl.Result, error) {
	r.Log.Info("running step initializeStorage")

	if storage.Status.State == StorageProvisioning {
		storage.Status.State = StorageInitializing
		return r.setState(ctx, storage)
	}

	if result, err := r.createInitBlobstorageJob(ctx, storage); err != nil {
		r.Recorder.Event(
			storage,
			corev1.EventTypeWarning,
			"ProvisioningFailed",
			fmt.Sprintf("Failed to create init blobstorage Job, error: %s", err),
		)
		return Stop, ctrl.Result{RequeueAfter: DefaultRequeueDelay}, err
	} else if result == controllerutil.OperationResultCreated {
		r.Recorder.Event(
			storage,
			corev1.EventTypeNormal,
			"Provisioning",
			"Init bobstorage Job was created successfully",
		)
		return Stop, ctrl.Result{RequeueAfter: StorageInitializationRequeueDelay}, nil
	}

	initJob := &batchv1.Job{}
	if err := r.Get(ctx, types.NamespacedName{
		Name:      fmt.Sprintf(resources.InitJobNameFormat, storage.Name),
		Namespace: storage.Namespace,
	}, initJob); err != nil {
		r.Recorder.Event(
			storage,
			corev1.EventTypeWarning,
			"ControllerError",
			fmt.Sprintf("Failed to get Job: %s", err),
		)
		return Stop, ctrl.Result{RequeueAfter: DefaultRequeueDelay}, err
	}

	if initJob.Spec.Suspend != nil && *initJob.Spec.Suspend {
		if storage.Spec.OperatorConnection != nil {
			if _, err := r.createOrUpdateOperatorTokenSecret(ctx, storage, creds); err != nil {
				r.Recorder.Event(
					storage,
					corev1.EventTypeWarning,
					"ProvisioningFailed",
					fmt.Sprintf("Failed to create/update operator token Secret, error: %s", err),
				)
				return Stop, ctrl.Result{RequeueAfter: DefaultRequeueDelay}, err
			}
		}

		initJob.Spec.Suspend = ptr.Bool(false)
		if err := r.Update(ctx, initJob); err != nil {
			r.Recorder.Event(
				storage,
				corev1.EventTypeWarning,
				"ControllerError",
				fmt.Sprintf("Failed to update Job: %s", err),
			)
			return Stop, ctrl.Result{RequeueAfter: DefaultRequeueDelay}, err
		}

		return Stop, ctrl.Result{RequeueAfter: StorageInitializationRequeueDelay}, nil
	}

	if initJob.Status.Succeeded > 0 {
		r.Log.Info("Init Job status succeeded")
		podLogs, err := r.getSucceededJobLogs(ctx, storage, initJob)
		if err != nil {
			r.Recorder.Event(
				storage,
				corev1.EventTypeWarning,
				"ControllerError",
				fmt.Sprintf("Failed to get succeeded Pod for Job: %s", err),
			)
			return Stop, ctrl.Result{RequeueAfter: DefaultRequeueDelay}, err
		}

		if mismatchItemConfigGenerationRegexp.MatchString(podLogs) {
			r.Log.Info("Storage is already initialized, continuing...")
			r.Recorder.Event(
				storage,
				corev1.EventTypeNormal,
				"InitializingStorage",
				"Storage initialization attempted and skipped, storage already initialized",
			)
			return r.setInitStorageCompleted(ctx, storage, "Storage already initialized")
		}

		r.Recorder.Event(
			storage,
			corev1.EventTypeNormal,
			"InitializingStorage",
			"Storage initialized successfully",
		)
		return r.setInitStorageCompleted(ctx, storage, "Storage initialized successfully")
	}

	if initJob.Status.Failed > 0 {
		r.Log.Info("Init Job status failed")
		r.Recorder.Event(
			storage,
			corev1.EventTypeWarning,
			"InitializingStorage",
			"Failed to initializing Storage",
		)
		if err := r.Delete(ctx, initJob); err != nil {
			r.Recorder.Event(
				storage,
				corev1.EventTypeWarning,
				"ControllerError",
				fmt.Sprintf("Failed to delete Job: %s", err),
			)
			return Stop, ctrl.Result{RequeueAfter: DefaultRequeueDelay}, err
		}
		return Stop, ctrl.Result{RequeueAfter: StorageInitializationRequeueDelay}, nil
	}

	return Stop, ctrl.Result{RequeueAfter: StorageInitializationRequeueDelay}, nil
}

func (r *Reconciler) getSucceededJobLogs(
	ctx context.Context,
	storage *resources.StorageClusterBuilder,
	job *batchv1.Job,
) (string, error) {
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
		return "", err
	}

	// Assuming there is only one succeeded pod, you can adjust the logic if needed
	for _, pod := range podList.Items {
		if pod.Status.Phase == corev1.PodSucceeded {
			clientset, err := kubernetes.NewForConfig(r.Config)
			if err != nil {
				return "", err
			}

			podLogs, err := clientset.CoreV1().
				Pods(storage.Namespace).
				GetLogs(pod.Name, &corev1.PodLogOptions{}).
				Stream(context.TODO())
			if err != nil {
				return "", err
			}
			defer podLogs.Close()

			var logsBuilder strings.Builder
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
	}

	return "", errors.New("failed to get succeeded Pod for Job")
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
) (controllerutil.OperationResult, error) {
	builder := resources.GetInitJobBuilder(storage.DeepCopy())
	newResource := builder.Placeholder(storage)
	return resources.CreateOrUpdateOrMaybeIgnore(ctx, r.Client, newResource, func() error {
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
}

func (r *Reconciler) createOrUpdateOperatorTokenSecret(
	ctx context.Context,
	storage *resources.StorageClusterBuilder,
	creds ydbCredentials.Credentials,
) (controllerutil.OperationResult, error) {
	ydbCtx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	token, err := creds.Token(
		metadata.AppendToOutgoingContext(ydbCtx, "x-ydb-database", storage.Spec.Domain),
	)
	if err != nil {
		return controllerutil.OperationResultNone, err
	}

	builder := resources.GetOperatorTokenSecretBuilder(storage, token)
	newResource := builder.Placeholder(storage)
	return resources.CreateOrUpdateOrMaybeIgnore(ctx, r.Client, newResource, func() error {
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
	}, func(oldObj, newObj runtime.Object) bool {
		return false
	})
}
