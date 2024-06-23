package cms

import (
	"context"
	"errors"
	"fmt"

	"github.com/ydb-platform/ydb-go-genproto/Ydb_Cms_V1"
	"github.com/ydb-platform/ydb-go-genproto/Ydb_Operation_V1"
	"github.com/ydb-platform/ydb-go-genproto/protos/Ydb"
	"github.com/ydb-platform/ydb-go-genproto/protos/Ydb_Cms"
	"github.com/ydb-platform/ydb-go-genproto/protos/Ydb_Operations"
	ydb "github.com/ydb-platform/ydb-go-sdk/v3"
	"sigs.k8s.io/controller-runtime/pkg/log"

	ydbv1alpha1 "github.com/ydb-platform/ydb-kubernetes-operator/api/v1alpha1"
	"github.com/ydb-platform/ydb-kubernetes-operator/internal/connection"
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
) (string, error) {
	logger := log.FromContext(ctx)
	url := fmt.Sprintf("%s/%s", t.StorageEndpoint, t.Domain)
	conn, err := connection.Open(ctx, url, opts...)
	if err != nil {
		logger.Error(err, "Error connecting to YDB storage")
		return "", err
	}
	defer func() {
		connection.Close(ctx, conn)
	}()

	client := Ydb_Cms_V1.NewCmsServiceClient(ydb.GRPCConn(conn))
	logger.Info(fmt.Sprintf("creating tenant, url: %s", url))
	request := t.makeCreateDatabaseRequest()
	logger.Info(fmt.Sprintf("creating tenant, request: %s", request))
	response, err := client.CreateDatabase(ctx, request)
	if err != nil {
		return "", err
	}
	logger.Info(fmt.Sprintf("creating tenant, response: %s", response))
	return processDatabaseCreationOperation(response.Operation)
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

func processDatabaseCreationOperation(operation *Ydb_Operations.Operation) (string, error) {
	if operation == nil {
		return "", ErrEmptyReplyFromStorage
	}
	if !operation.Ready {
		return operation.Id, nil
	}
	if operation.Status == Ydb.StatusIds_ALREADY_EXISTS || operation.Status == Ydb.StatusIds_SUCCESS {
		return "", nil
	}
	return "", fmt.Errorf("YDB response error: %v %v", operation.Status, operation.Issues)
}

func (t *Tenant) CheckCreateOperation(
	ctx context.Context,
	operationID string,
	opts ...ydb.Option,
) (bool, error, error) {
	logger := log.FromContext(ctx)
	url := fmt.Sprintf("%s/%s", t.StorageEndpoint, t.Domain)
	conn, err := connection.Open(ctx, url, opts...)
	if err != nil {
		logger.Error(err, "Error connecting to YDB storage")
		return false, nil, err
	}
	defer func() {
		connection.Close(ctx, conn)
	}()

	client := Ydb_Operation_V1.NewOperationServiceClient(ydb.GRPCConn(conn))
	request := &Ydb_Operations.GetOperationRequest{Id: operationID}
	logger.Info(fmt.Sprintf("checking operation, url: %s, operationId: %s, request: %s", url, operationID, request))
	response, err := client.GetOperation(ctx, request)
	if err != nil {
		return false, nil, err
	}
	logger.Info(fmt.Sprintf("checking operation, response: %s", response))
	if response.Operation == nil {
		return false, nil, ErrEmptyReplyFromStorage
	}
	oid, err := processDatabaseCreationOperation(response.Operation)
	return len(oid) == 0, err, nil
}
