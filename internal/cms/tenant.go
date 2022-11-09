package cms

import (
	"context"
	"errors"
	"fmt"

	"github.com/ydb-platform/ydb-go-genproto/protos/Ydb"
	"github.com/ydb-platform/ydb-go-genproto/protos/Ydb_Cms"
	ydbv1alpha1 "github.com/ydb-platform/ydb-kubernetes-operator/api/v1alpha1"
	"github.com/ydb-platform/ydb-kubernetes-operator/internal/grpc"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

const (
	createDatabaseMethod = "/Ydb.Cms.V1.CmsService/CreateDatabase"
)

var ErrEmptyReplyFromStorage = errors.New("empty reply from storage")

type Tenant struct {
	StorageEndpoint      string
	Path                 string
	StorageUnits         []ydbv1alpha1.StorageUnit
	Shared               bool
	SharedDatabasePath   string
	UseGrpcSecureChannel bool
}

func (t *Tenant) Create(ctx context.Context) error {
	logger := log.FromContext(ctx)
	client := grpc.Client{
		Context: ctx,
		Target:  t.StorageEndpoint,
	}
	logger.Info(fmt.Sprintf("creating tenant, endpoint: %s, secure: %t, method: %s", t.StorageEndpoint, t.UseGrpcSecureChannel, createDatabaseMethod))
	request := t.makeCreateDatabaseRequest()
	logger.Info(fmt.Sprintf("creating tenant, request: %s", request))
	response := &Ydb_Cms.CreateDatabaseResponse{}
	grpcCallResult := client.Invoke(
		createDatabaseMethod,
		request,
		response,
		t.UseGrpcSecureChannel,
	)
	if _, err := processDatabaseCreationResponse(response); err != nil {
		return err
	}
	logger.Info(fmt.Sprintf("creating tenant, response: %s, err: %s", response, grpcCallResult))
	return grpcCallResult
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
