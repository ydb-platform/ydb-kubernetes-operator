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

type RemoteDatabaseNodeSetBuilder struct {
	client.Object

	Name   string
	Labels map[string]string

	DatabaseNodeSetSpec api.DatabaseNodeSetSpec
}

type RemoteDatabaseNodeSetResource struct {
	*api.RemoteDatabaseNodeSet
}

func (b *RemoteDatabaseNodeSetBuilder) Build(obj client.Object) error {
	dns, ok := obj.(*api.RemoteDatabaseNodeSet)
	if !ok {
		return errors.New("failed to cast to RemoteDatabaseNodeSet object")
	}

	if dns.ObjectMeta.Name == "" {
		dns.ObjectMeta.Name = b.Name
	}
	dns.ObjectMeta.Namespace = b.GetNamespace()

	dns.ObjectMeta.Labels = b.Labels
	dns.Spec = b.DatabaseNodeSetSpec

	return nil
}

func (b *RemoteDatabaseNodeSetBuilder) Placeholder(cr client.Object) client.Object {
	return &api.DatabaseNodeSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      b.Name,
			Namespace: cr.GetNamespace(),
		},
	}
}

func (b *RemoteDatabaseNodeSetResource) GetResourceBuilders(restConfig *rest.Config) []ResourceBuilder {
	var resourceBuilders []ResourceBuilder
	resourceBuilders = append(resourceBuilders,
		&DatabaseNodeSetBuilder{
			Object: b,

			Name:                b.Name,
			Labels:              b.Labels,
			DatabaseNodeSetSpec: b.Spec,
		},
	)
	return resourceBuilders
}

func NewRemoteDatabaseNodeSet(remoteDatabaseNodeSet *api.RemoteDatabaseNodeSet) RemoteDatabaseNodeSetResource {
	crRemoteDatabaseNodeSet := remoteDatabaseNodeSet.DeepCopy()

	return RemoteDatabaseNodeSetResource{RemoteDatabaseNodeSet: crRemoteDatabaseNodeSet}
}

func (b *RemoteDatabaseNodeSetResource) SetStatusOnFirstReconcile() (bool, ctrl.Result, error) {
	if b.Status.Conditions == nil {
		b.Status.Conditions = []metav1.Condition{}

		if b.Spec.Pause {
			meta.SetStatusCondition(&b.Status.Conditions, metav1.Condition{
				Type:    DatabasePausedCondition,
				Status:  "False",
				Reason:  ReasonInProgress,
				Message: "Transitioning RemoteDatabaseNodeSet to Paused state",
			})

			return Stop, ctrl.Result{RequeueAfter: StatusUpdateRequeueDelay}, nil
		}
	}

	return Continue, ctrl.Result{}, nil
}
