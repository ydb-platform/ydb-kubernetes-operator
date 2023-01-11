package cms

import (
	"context"
	"errors"
	"fmt"

	"github.com/ydb-platform/ydb-go-genproto/Ydb_Cms_V1"
	"github.com/ydb-platform/ydb-go-genproto/protos/Ydb"
	"github.com/ydb-platform/ydb-go-genproto/protos/Ydb_Cms"
	"github.com/ydb-platform/ydb-go-sdk/v3"
	"sigs.k8s.io/controller-runtime/pkg/log"

	ydbv1alpha1 "github.com/ydb-platform/ydb-kubernetes-operator/api/v1alpha1"
	"github.com/ydb-platform/ydb-kubernetes-operator/internal/connection"
	"github.com/ydb-platform/ydb-kubernetes-operator/internal/resources"
)

var ErrEmptyReplyFromStorage = errors.New("empty reply from storage")

type Tenant struct {
	StorageEndpoint    string
	Path               string
	StorageUnits       []ydbv1alpha1.StorageUnit
	Shared             bool
	SharedDatabasePath string
}

func (t *Tenant) Create(ctx context.Context, database *resources.DatabaseBuilder) error {
	createDatabaseURL := fmt.Sprintf("%s/%s", t.StorageEndpoint, database.Spec.Domain)
	db, err := connection.Build(ctx, createDatabaseURL)
	if err != nil {
		return err
	}

	logger := log.FromContext(ctx)

	defer func() {
		connection.Close(ctx, db)
	}()

	client := Ydb_Cms_V1.NewCmsServiceClient(ydb.GRPCConn(db))
	logger.Info(fmt.Sprintf("creating tenant, url: %s", createDatabaseURL))
	request := t.makeCreateDatabaseRequest()
	logger.Info(fmt.Sprintf("creating tenant, request: %s", request))
	response, err := client.CreateDatabase(ctx, request)
	if err != nil {
		return err
	}
	if _, err := processDatabaseCreationResponse(response); err != nil {
		return err
	}
	logger.Info(fmt.Sprintf("creating tenant, response: %s", response))
	return nil
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

func processDatabaseCreationResponse(response *Ydb_Cms.CreateDatabaseResponse) (bool, error) {
	if response.Operation == nil {
		return false, ErrEmptyReplyFromStorage
	}

	if response.Operation.Status == Ydb.StatusIds_ALREADY_EXISTS || response.Operation.Status == Ydb.StatusIds_SUCCESS {
		return true, nil
	}
	if response.Operation.Status == Ydb.StatusIds_STATUS_CODE_UNSPECIFIED && len(response.Operation.Issues) == 0 {
		return true, nil
	}

	return false, fmt.Errorf("YDB response error: %v %v", response.Operation.Status, response.Operation.Issues)
}
