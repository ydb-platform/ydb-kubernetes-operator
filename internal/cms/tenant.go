package cms

import (
	"context"
	"errors"

	"github.com/ydb-platform/ydb-go-genproto/protos/Ydb"
	"github.com/ydb-platform/ydb-go-genproto/protos/Ydb_Cms"
	"github.com/ydb-platform/ydb-kubernetes-operator/internal/grpc"
	"github.com/ydb-platform/ydb-kubernetes-operator/internal/resources"
)

const (
	defaultUnitKind          = "hdd"
	defaultStorageUnitsCount = 2

	createDatabaseMethod = "/Ydb.Cms.V1.CmsService/CreateDatabase"
)

type Tenant struct {
	Name              string
	UnitKind          string
	StorageUnitsCount uint64
}

func NewTenant(name string) Tenant {
	return Tenant{
		Name:              name,
		UnitKind:          defaultUnitKind,
		StorageUnitsCount: defaultStorageUnitsCount,
	}
}

func (t *Tenant) Create(ctx context.Context, database *resources.DatabaseBuilder) error {
	client := grpc.InsecureGrpcClient{
		Context: ctx,
		Target:  database.GetStorageEndpoint(),
	}

	response := &Ydb_Cms.CreateDatabaseResponse{}
	grpcCallResult := client.Invoke(
		createDatabaseMethod,
		t.makeCreateDatabaseRequest(),
		response,
	)

	if _, err := parseDatabaseCreationResponse(response); err != nil {
		return err
	}

	return grpcCallResult
}

func (t *Tenant) makeCreateDatabaseRequest() *Ydb_Cms.CreateDatabaseRequest {
	return &Ydb_Cms.CreateDatabaseRequest{
		Path: t.Name,
		ResourcesKind: &Ydb_Cms.CreateDatabaseRequest_Resources{
			Resources: &Ydb_Cms.Resources{
				StorageUnits: []*Ydb_Cms.StorageUnits{
					{
						UnitKind: t.UnitKind,
						Count:    t.StorageUnitsCount,
					},
				},
			},
		},
	}
}

func parseDatabaseCreationResponse(response *Ydb_Cms.CreateDatabaseResponse) (bool, error) {
	if response.Operation == nil {
		return false, errors.New("empty reply from storage")
	}

	if response.Operation.Status == Ydb.StatusIds_ALREADY_EXISTS || response.Operation.Status == Ydb.StatusIds_SUCCESS {
		return true, nil
	}
	if response.Operation.Status == Ydb.StatusIds_STATUS_CODE_UNSPECIFIED && len(response.Operation.Issues) == 0 {
		return true, nil
	}

	return false, errors.New("unknown error")
}
