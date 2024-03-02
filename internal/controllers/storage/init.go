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
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/ydb-platform/ydb-kubernetes-operator/api/v1alpha1"
	. "github.com/ydb-platform/ydb-kubernetes-operator/internal/controllers/constants" //nolint:revive,stylecheck
	"github.com/ydb-platform/ydb-kubernetes-operator/internal/exec"
	"github.com/ydb-platform/ydb-kubernetes-operator/internal/labels"
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
	r.Log.Info("running step runInitScripts")

	if storage.Status.State == StorageProvisioning {
		storage.Status.State = StorageInitializing
		return r.setState(ctx, storage)
	}

	// List Pods by label Selector
	podList := &corev1.PodList{}
	matchingLabels := client.MatchingLabels{}
	storageLabels := labels.StorageLabels(storage.Unwrap())
	for k, v := range storageLabels {
		matchingLabels[k] = v
	}
	opts := []client.ListOption{
		client.InNamespace(storage.Namespace),
		matchingLabels,
	}
	if err := r.List(ctx, podList, opts...); err != nil || len(podList.Items) == 0 {
		r.Recorder.Event(
			storage,
			corev1.EventTypeWarning,
			"Syncing",
			fmt.Sprintf("Failed to list storage pods: %s", err),
		)
		return Stop, ctrl.Result{RequeueAfter: DefaultRequeueDelay}, err
	}

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

	cmd = append(
		cmd,
		"-s",
		storage.GetStorageEndpointWithProto(),
	)

	cmd = append(
		cmd,
		"admin", "blobstorage", "config", "init",
		"--yaml-file",
		fmt.Sprintf("%s/%s", v1alpha1.ConfigDir, v1alpha1.ConfigFileName),
	)

	stdout, _, err := exec.InPod(r.Scheme, r.Config, storage.Namespace, podList.Items[0].Name, "ydb-storage", cmd)
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
