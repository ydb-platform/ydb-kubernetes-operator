package cms

import (
	"context"
	"fmt"

	"github.com/ydb-platform/ydb-go-genproto/Ydb_Operation_V1"
	"github.com/ydb-platform/ydb-go-genproto/protos/Ydb"
	"github.com/ydb-platform/ydb-go-genproto/protos/Ydb_Operations"
	"github.com/ydb-platform/ydb-go-sdk/v3"
	"github.com/ydb-platform/ydb-go-sdk/v3/credentials"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/ydb-platform/ydb-kubernetes-operator/internal/connection"
	"github.com/ydb-platform/ydb-kubernetes-operator/internal/resources"
)

func GetOperation(
	ctx context.Context,
	storage *resources.StorageClusterBuilder,
	operationID string,
	creds credentials.Credentials,
	opts ...ydb.Option,
) (*Ydb_Operations.GetOperationResponse, error) {
	logger := log.FromContext(ctx)
	endpoint := fmt.Sprintf(
		"%s/%s",
		storage.GetStorageEndpointWithProto(),
		storage.Spec.Domain,
	)
	conn, err := connection.Open(ctx,
		endpoint,
		ydb.WithCredentials(creds),
		ydb.MergeOptions(opts...),
	)
	if err != nil {
		return nil, fmt.Errorf("error connecting to YDB: %w", err)
	}
	defer func() {
		connection.Close(ctx, conn)
	}()

	client := Ydb_Operation_V1.NewOperationServiceClient(ydb.GRPCConn(conn))
	request := &Ydb_Operations.GetOperationRequest{
		Id: operationID,
	}
	logger.Info("CMS GetOperation", "request", request)
	return client.GetOperation(ctx, request)
}

func CheckOperationSuccess(operation *Ydb_Operations.Operation) error {
	if operation.Status == Ydb.StatusIds_ALREADY_EXISTS || operation.Status == Ydb.StatusIds_SUCCESS {
		return nil
	}

	if operation.Status == Ydb.StatusIds_STATUS_CODE_UNSPECIFIED && len(operation.Issues) == 0 {
		return nil
	}

	return fmt.Errorf("operation status is %v: %v", operation.Status, operation.Issues)
}
