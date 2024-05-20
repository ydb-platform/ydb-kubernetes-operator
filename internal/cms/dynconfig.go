package cms

import (
	"context"
	"fmt"

	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/ydb-platform/ydb-go-genproto/draft/Ydb_DynamicConfig_V1"
	"github.com/ydb-platform/ydb-go-genproto/draft/protos/Ydb_DynamicConfig"
	"github.com/ydb-platform/ydb-go-genproto/protos/Ydb_Operations"
	"github.com/ydb-platform/ydb-go-sdk/v3"
	"github.com/ydb-platform/ydb-go-sdk/v3/credentials"

	"github.com/ydb-platform/ydb-kubernetes-operator/internal/connection"
	"github.com/ydb-platform/ydb-kubernetes-operator/internal/resources"
)

func GetConfig(
	ctx context.Context,
	storage *resources.StorageClusterBuilder,
	creds credentials.Credentials,
	opts ...ydb.Option,
) (*Ydb_DynamicConfig.GetConfigResponse, error) {
	logger := log.FromContext(ctx)
	conn, err := connection.Open(ctx,
		storage.GetStorageEndpointWithProto(),
		ydb.WithCredentials(creds),
		ydb.MergeOptions(opts...),
	)
	if err != nil {
		logger.Error(err, "Error connecting to YDB storage")
		return nil, err
	}
	defer func() {
		connection.Close(ctx, conn)
	}()

	client := Ydb_DynamicConfig_V1.NewDynamicConfigServiceClient(ydb.GRPCConn(conn))
	request := &Ydb_DynamicConfig.GetConfigRequest{}
	return client.GetConfig(ctx, request)
}

func GetConfigResult(
	response *Ydb_Operations.GetOperationResponse,
) (*Ydb_DynamicConfig.GetConfigResult, error) {
	configResult := Ydb_DynamicConfig.GetConfigResult{}
	err := response.GetOperation().GetResult().UnmarshalTo(&configResult)
	if err != nil {
		return nil, err
	}

	return &configResult, nil
}

func ReplaceConfig(
	ctx context.Context,
	storage *resources.StorageClusterBuilder,
	creds credentials.Credentials,
	opts ...ydb.Option,
) (*Ydb_DynamicConfig.ReplaceConfigResponse, error) {
	logger := log.FromContext(ctx)
	conn, err := connection.Open(ctx,
		storage.GetStorageEndpointWithProto(),
		ydb.WithCredentials(creds),
		ydb.MergeOptions(opts...),
	)
	if err != nil {
		logger.Error(err, "Error connecting to YDB storage")
		return nil, err
	}
	defer func() {
		connection.Close(ctx, conn)
	}()

	client := Ydb_DynamicConfig_V1.NewDynamicConfigServiceClient(ydb.GRPCConn(conn))
	request := &Ydb_DynamicConfig.ReplaceConfigRequest{
		Config:             storage.Spec.Configuration,
		DryRun:             false,
		AllowUnknownFields: false,
	}
	logger.Info(fmt.Sprintf("Sending CMS ReplaceConfigRequest: %s", request))
	return client.ReplaceConfig(ctx, request)
}
