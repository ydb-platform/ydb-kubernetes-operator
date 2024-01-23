package resources

import (
	"errors"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	api "github.com/ydb-platform/ydb-kubernetes-operator/api/v1alpha1"
	. "github.com/ydb-platform/ydb-kubernetes-operator/internal/controllers/constants" //nolint:revive,stylecheck
)

type DatabaseNodeSetBuilder struct {
	client.Object

	Name   string
	Labels map[string]string

	DatabaseNodeSetSpec api.DatabaseNodeSetSpec
}

type DatabaseNodeSetResource struct {
	*api.DatabaseNodeSet
}

func (b *DatabaseNodeSetBuilder) Build(obj client.Object) error {
	dns, ok := obj.(*api.DatabaseNodeSet)
	if !ok {
		return errors.New("failed to cast to DatabaseNodeSet object")
	}

	if dns.ObjectMeta.Name == "" {
		dns.ObjectMeta.Name = b.Name
	}
	dns.ObjectMeta.Namespace = b.GetNamespace()

	dns.ObjectMeta.Labels = b.Labels
	dns.Spec = b.DatabaseNodeSetSpec

	return nil
}

func (b *DatabaseNodeSetBuilder) Placeholder(cr client.Object) client.Object {
	return &api.DatabaseNodeSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      b.Name,
			Namespace: cr.GetNamespace(),
		},
	}
}

func (b *DatabaseNodeSetResource) GetResourceBuilders(restConfig *rest.Config) []ResourceBuilder {
	database := b.recastDatabaseNodeSet()

	var resourceBuilders []ResourceBuilder
	resourceBuilders = append(resourceBuilders,
		&DatabaseStatefulSetBuilder{
			Database:   database.DeepCopy(),
			RestConfig: restConfig,

			Name:            b.Name,
			Labels:          b.Labels,
			StorageEndpoint: b.Spec.StorageEndpoint,
		},
		&ConfigMapBuilder{
			Object: b,

			Name: b.Name,
			Data: map[string]string{
				api.ConfigFileName: b.Spec.Configuration,
			},
			Labels: b.Labels,
		},
	)
	return resourceBuilders
}

func NewDatabaseNodeSet(databaseNodeSet *api.DatabaseNodeSet) DatabaseNodeSetResource {
	crDatabaseNodeSet := databaseNodeSet.DeepCopy()

	return DatabaseNodeSetResource{DatabaseNodeSet: crDatabaseNodeSet}
}

func (b *DatabaseNodeSetResource) SetStatusOnFirstReconcile() (bool, ctrl.Result, error) {
	if b.Status.Conditions == nil {
		b.Status.Conditions = []metav1.Condition{}

		if b.Spec.Pause {
			meta.SetStatusCondition(&b.Status.Conditions, metav1.Condition{
				Type:    DatabasePausedCondition,
				Status:  "False",
				Reason:  ReasonInProgress,
				Message: "Transitioning DatabaseNodeSet to Paused state",
			})

			return Stop, ctrl.Result{RequeueAfter: StatusUpdateRequeueDelay}, nil
		}
	}

	return Continue, ctrl.Result{}, nil
}

func (b *DatabaseNodeSetResource) Unwrap() *api.DatabaseNodeSet {
	return b.DeepCopy()
}

func (b *DatabaseNodeSetResource) recastDatabaseNodeSet() *api.Database {
	return &api.Database{
		ObjectMeta: metav1.ObjectMeta{
			Name:      b.DatabaseNodeSet.Spec.DatabaseRef.Name,
			Namespace: b.DatabaseNodeSet.Spec.DatabaseRef.Namespace,
			Labels:    b.DatabaseNodeSet.Labels,
		},
		Spec: api.DatabaseSpec{
			DatabaseClusterSpec: b.Spec.DatabaseClusterSpec,
			DatabaseNodeSpec:    b.Spec.DatabaseNodeSpec,
		},
	}
}
