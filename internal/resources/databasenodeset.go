package resources

import (
	"errors"

	api "github.com/ydb-platform/ydb-kubernetes-operator/api/v1alpha1"
	"github.com/ydb-platform/ydb-kubernetes-operator/internal/labels"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type DatabaseNodeSetBuilder struct {
	*api.DatabaseNodeSet

	Database *api.Database
	Labels   labels.Labels

	NodeSetSpecInline *api.NodeSetSpecInline
}

func NewDatabaseNodeSet(databaseNodeSet *api.DatabaseNodeSet, database *api.Database) DatabaseNodeSetBuilder {
	crDatabase := database.DeepCopy()
	crDatabaseNodeSet := databaseNodeSet.DeepCopy()

	return DatabaseNodeSetBuilder{
		DatabaseNodeSet: crDatabaseNodeSet,
		Database:        crDatabase,
		Labels:          labels.DatabaseLabels(database),
	}
}

func (b *DatabaseNodeSetBuilder) SetStatusOnFirstReconcile() {
	if b.Status.Conditions == nil {
		b.Status.Conditions = []metav1.Condition{}
	}
}

func (b *DatabaseNodeSetBuilder) Unwrap() *api.DatabaseNodeSet {
	return b.DeepCopy()
}

func (b *DatabaseNodeSetBuilder) Build(obj client.Object) error {
	dns, ok := obj.(*api.DatabaseNodeSet)
	if !ok {
		return errors.New("failed to cast to DatabaseNodeSet object")
	}

	dns.ObjectMeta.Name = b.NodeSetSpecInline.Name
	dns.ObjectMeta.Namespace = b.Database.GetNamespace()

	var databaseLabels labels.Labels = b.Database.Labels
	dns.ObjectMeta.Labels = databaseLabels.Merge(b.NodeSetSpecInline.AdditionalLabels)

	dns.Spec = api.DatabaseNodeSetSpec{
		DatabaseRef: b.Database.GetName(),
		NodeSetSpec: b.NodeSetSpecInline.NodeSetSpec,
	}

	return nil

}

func (b *DatabaseNodeSetBuilder) GetResourceBuilders(restConfig *rest.Config) []ResourceBuilder {

	var optionalBuilders []ResourceBuilder

	stsBuilder := &DatabaseStatefulSetBuilder{
		Database:   b.Database.DeepCopy(),
		RestConfig: restConfig,
		Labels:     b.Labels.AsMap(),
	}

	return append(optionalBuilders,
		&DatabaseNodeSetStatefulSetBuilder{
			DatabaseStatefulSetBuilder: stsBuilder,
			DatabaseNodeSet:            b.DatabaseNodeSet,
		},
	)
}
