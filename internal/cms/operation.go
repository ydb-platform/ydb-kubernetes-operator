package cms

import (
	"context"
	"fmt"
	"time"

	"github.com/ydb-platform/ydb-go-genproto/Ydb_Operation_V1"
	"github.com/ydb-platform/ydb-go-genproto/protos/Ydb"
	"github.com/ydb-platform/ydb-go-genproto/protos/Ydb_Operations"
	"github.com/ydb-platform/ydb-go-sdk/v3"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/ydb-platform/ydb-kubernetes-operator/internal/connection"
)

const (
	GetOperationTimeoutSeconds = 10
)

type Operation struct {
	StorageEndpoint string
	Domain          string
	Id              string
}

func (op *Operation) GetOperation(
	ctx context.Context,
	opts ...ydb.Option,
) (*Ydb_Operations.GetOperationResponse, error) {
	logger := log.FromContext(ctx)

	endpoint := fmt.Sprintf("%s/%s", op.StorageEndpoint, op.Domain)
	ydbCtx, ydbCtxCancel := context.WithTimeout(ctx, time.Second)
	defer ydbCtxCancel()
	conn, err := connection.Open(ydbCtx, endpoint, ydb.MergeOptions(opts...))
	if err != nil {
		return nil, fmt.Errorf("error connecting to YDB: %w", err)
	}
	defer func() {
		connection.Close(ydbCtx, conn)
	}()

	cmsCtx, cmsCtxCancel := context.WithTimeout(ctx, GetOperationTimeoutSeconds*time.Second)
	defer cmsCtxCancel()
	client := Ydb_Operation_V1.NewOperationServiceClient(ydb.GRPCConn(conn))
	request := &Ydb_Operations.GetOperationRequest{Id: op.Id}

	logger.Info("CMS GetOperation request", "endpoint", endpoint, "request", request)
	return client.GetOperation(cmsCtx, request)
}

func (op *Operation) CheckOperationResponse(ctx context.Context, response *Ydb_Operations.GetOperationResponse) (bool, string, error) {
	logger := log.FromContext(ctx)

	logger.Info("CMS GetOperation response", "response", response)
	return CheckOperationStatus(response.GetOperation())
}

func CheckOperationStatus(operation *Ydb_Operations.Operation) (bool, string, error) {
	if operation == nil {
		return false, "", ErrEmptyReplyFromStorage
	}

	if !operation.GetReady() {
		return false, operation.Id, nil
	}

	if operation.Status == Ydb.StatusIds_ALREADY_EXISTS || operation.Status == Ydb.StatusIds_SUCCESS {
		return true, operation.Id, nil
	}

	return true, operation.Id, fmt.Errorf("YDB response error: %v %v", operation.Status, operation.Issues)
}
