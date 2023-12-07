package storage

import (
	"context"
	"fmt"
	"regexp"
	"time"

	ydbCredentials "github.com/ydb-platform/ydb-go-sdk/v3/credentials"
	"google.golang.org/grpc/metadata"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/ydb-platform/ydb-kubernetes-operator/api/v1alpha1"
	"github.com/ydb-platform/ydb-kubernetes-operator/internal/exec"
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
			Reason:  StorageInitializedReasonInProgress,
			Message: "Storage is not ready yet",
		})
		changed = true
	}
	if storage.Status.State == string(Pending) {
		storage.Status.State = string(Preparing)
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
		Reason:  StorageInitializedReasonCompleted,
		Message: message,
	})

	storage.Status.State = string(Ready)
	return r.setState(ctx, storage)
}

func (r *Reconciler) initializeStorage(
	ctx context.Context,
	storage *resources.StorageClusterBuilder,
	creds ydbCredentials.Credentials,
) (bool, ctrl.Result, error) {
	r.Log.Info("running step runInitScripts")

	if storage.Status.State == string(Provisioning) {
		storage.Status.State = string(Initializing)
		return r.setState(ctx, storage)
	}

	podName := fmt.Sprintf("%s-0", storage.Name)

	cmd := []string{
		fmt.Sprintf("%s/%s", v1alpha1.BinariesDir, v1alpha1.DaemonBinaryName),
	}

	if storage.Spec.OperatorConnection != nil {
		ydbCtx, cancel := context.WithTimeout(ctx, time.Second)
		defer cancel()
		token, err := creds.Token(
			metadata.AppendToOutgoingContext(ydbCtx, "x-ydb-database", storage.Spec.Domain),
		)
		if err != nil {
			r.Log.Error(err, "initializeStorage error")
			return Stop, ctrl.Result{RequeueAfter: StorageInitializationRequeueDelay}, err
		}
		cmd = append(
			cmd,
			"--token",
			token,
		)
	}

	if resources.IsGrpcSecure(storage.Storage) {
		cmd = append(
			cmd,
			"-s",
			storage.GetGRPCEndpointWithProto(),
		)
	}

	cmd = append(
		cmd,
		"admin", "blobstorage", "config", "init",
		"--yaml-file",
		fmt.Sprintf("%s/%s", v1alpha1.ConfigDir, v1alpha1.ConfigFileName),
	)

	stdout, _, err := exec.InPod(r.Scheme, r.Config, storage.Namespace, podName, "ydb-storage", cmd)
	if err != nil {
		if mismatchItemConfigGenerationRegexp.MatchString(stdout) {
			r.Log.Info("Storage is already initialized, continuing...")
			r.Recorder.Event(
				storage,
				corev1.EventTypeNormal,
				"InitializingStorage",
				"Storage initialization attempted and skipped, storage already initialized",
			)
			return r.setInitStorageCompleted(
				ctx,
				storage,
				"Storage already initialized",
			)
		}

		return Stop, ctrl.Result{RequeueAfter: StorageInitializationRequeueDelay}, err
	}

	return r.setInitStorageCompleted(ctx, storage, "Storage initialized successfully")
}

func (r *Reconciler) handlePauseResume(
	ctx context.Context,
	storage *resources.StorageClusterBuilder,
	creds ydbCredentials.Credentials,
) (bool, ctrl.Result, error) {
	if storage.Status.Pause == string(PauseRunning) && storage.Spec.Pause == string(PausePaused) {
		r.Log.Info("I noticed that Running -> Paused")
	}

	if storage.Status.Pause == string(PausePaused) && storage.Spec.Pause == string(PauseRunning) {
		r.Log.Info("I noticed that Paused -> Running")
	}

	return Continue, ctrl.Result{}, nil
}
