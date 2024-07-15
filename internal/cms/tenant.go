package cms

import (
	"context"
	"errors"
	"fmt"

	"github.com/ydb-platform/ydb-go-genproto/Ydb_Cms_V1"
	"github.com/ydb-platform/ydb-go-genproto/protos/Ydb_Cms"
	ydb "github.com/ydb-platform/ydb-go-sdk/v3"
	"sigs.k8s.io/controller-runtime/pkg/log"

	ydbv1alpha1 "github.com/ydb-platform/ydb-kubernetes-operator/api/v1alpha1"
	"github.com/ydb-platform/ydb-kubernetes-operator/internal/connection"
)

const (
	CreateDatabaseTimeoutSeconds = 10
)

var ErrEmptyReplyFromStorage = errors.New("empty reply from storage")

type Tenant struct {
	StorageEndpoint    string
	Domain             string
	Path               string
	StorageUnits       []ydbv1alpha1.StorageUnit
	Shared             bool
	SharedDatabasePath string
}

func (t *Tenant) Create(
	ctx context.Context,
	opts ...ydb.Option,
) (*Ydb_Cms.CreateDatabaseResponse, error) {
	logger := log.FromContext(ctx)

	endpoint := fmt.Sprintf("%s/%s", t.StorageEndpoint, t.Domain)
	ydbCtx, ydbCtxCancel := context.WithTimeout(ctx, time.Second)
	defer ydbCtxCancel()
	conn, err := connection.Open(ydbCtx, endpoint, opts...)
	if err != nil {
		logger.Error(err, "Error connecting to YDB")
		return nil, err
	}
	defer func() {
		connection.Close(ydbCtx, conn)
	}()

	cmsCtx, cmsCtxCancel := context.WithTimeout(ctx, CreateDatabaseTimeoutSeconds*time.Second)
	defer cmsCtxCancel()
	client := Ydb_Cms_V1.NewCmsServiceClient(ydb.GRPCConn(conn))
	request := t.makeCreateDatabaseRequest()
	logger.Info("CMS CreateDatabase request", "endpoint", endpoint, "request", request)
	return client.CreateDatabase(cmsCtx, request)
}

func (t *Tenant) CheckCreateDatabaseResponse(ctx context.Context, response *Ydb_Cms.CreateDatabaseResponse) (bool, string, error) {
	logger := log.FromContext(ctx)

	logger.Info("CMS GetOperation response", "response", response)
	return CheckOperationStatus(response.GetOperation())
}

func (t *Tenant) makeCreateDatabaseRequest() *Ydb_Cms.CreateDatabaseRequest {
	request := &Ydb_Cms.CreateDatabaseRequest{Path: t.Path}
	if t.SharedDatabasePath != "" {
		request.ResourcesKind = &Ydb_Cms.CreateDatabaseRequest_ServerlessResources{
			ServerlessResources: &Ydb_Cms.ServerlessResources{
				SharedDatabasePath: t.SharedDatabasePath,
			},
		}
	} else {
		storageUnitsPb := []*Ydb_Cms.StorageUnits{}
		for _, i := range t.StorageUnits {
			storageUnitsPb = append(
				storageUnitsPb,
				&Ydb_Cms.StorageUnits{UnitKind: i.UnitKind, Count: i.Count},
			)
		}
		if t.Shared {
			request.ResourcesKind = &Ydb_Cms.CreateDatabaseRequest_SharedResources{
				SharedResources: &Ydb_Cms.Resources{
					StorageUnits: storageUnitsPb,
				},
			}
		} else {
			request.ResourcesKind = &Ydb_Cms.CreateDatabaseRequest_Resources{
				Resources: &Ydb_Cms.Resources{
					StorageUnits: storageUnitsPb,
				},
			}
		}
	}
	return request
}
