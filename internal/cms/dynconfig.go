package cms

import (
	"context"
	"fmt"
	"time"

	"github.com/ydb-platform/ydb-go-genproto/draft/Ydb_DynamicConfig_V1"
	"github.com/ydb-platform/ydb-go-genproto/draft/protos/Ydb_DynamicConfig"
	"github.com/ydb-platform/ydb-go-genproto/protos/Ydb_Operations"
	"github.com/ydb-platform/ydb-go-sdk/v3"
	"github.com/ydb-platform/ydb-go-sdk/v3/credentials"
	"google.golang.org/protobuf/types/known/durationpb"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/ydb-platform/ydb-kubernetes-operator/api/v1alpha1"
	"github.com/ydb-platform/ydb-kubernetes-operator/internal/connection"
	"github.com/ydb-platform/ydb-kubernetes-operator/internal/resources"
)

const (
	GetConfigTimeoutSeconds     = 10
	ReplaceConfigTimeoutSeconds = 30
)

func GetConfig(
	ctx context.Context,
	storage *resources.StorageClusterBuilder,
	creds credentials.Credentials,
	opts ...ydb.Option,
) (*Ydb_DynamicConfig.GetConfigResponse, error) {
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
		return nil, fmt.Errorf("Error connecting to YDB: %w", err)
	}
	defer func() {
		connection.Close(ctx, conn)
	}()

	cmsCtx, cancel := context.WithTimeout(ctx, GetConfigTimeoutSeconds*time.Second)
	defer cancel()
	client := Ydb_DynamicConfig_V1.NewDynamicConfigServiceClient(ydb.GRPCConn(conn))
	request := &Ydb_DynamicConfig.GetConfigRequest{
		OperationParams: &Ydb_Operations.OperationParams{
			OperationMode:    Ydb_Operations.OperationParams_SYNC,
			OperationTimeout: &durationpb.Duration{Seconds: GetConfigTimeoutSeconds},
		},
	}
	return client.GetConfig(cmsCtx, request)
}

func GetConfigResult(
	response *Ydb_DynamicConfig.GetConfigResponse,
) (*Ydb_DynamicConfig.GetConfigResult, error) {
	configResult := &Ydb_DynamicConfig.GetConfigResult{}
	err := response.GetOperation().GetResult().UnmarshalTo(configResult)
	if err != nil {
		return nil, err
	}
	return configResult, nil
}

func ReplaceConfig(
	ctx context.Context,
	storage *resources.StorageClusterBuilder,
	dryRun bool,
	creds credentials.Credentials,
	opts ...ydb.Option,
) (*Ydb_DynamicConfig.ReplaceConfigResponse, error) {
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
		return nil, fmt.Errorf("Error connecting to YDB: %w", err)
	}
	defer func() {
		connection.Close(ctx, conn)
	}()

	config, err := v1alpha1.GetConfigForCMS(storage.Spec.Configuration)
	if err != nil {
		return nil, err
	}

	cmsCtx, cancel := context.WithTimeout(ctx, ReplaceConfigTimeoutSeconds*time.Second)
	defer cancel()
	client := Ydb_DynamicConfig_V1.NewDynamicConfigServiceClient(ydb.GRPCConn(conn))
	request := &Ydb_DynamicConfig.ReplaceConfigRequest{
		Config:             string(config),
		DryRun:             dryRun,
		AllowUnknownFields: true,
	}
	logger.Info(fmt.Sprintf("Sending CMS ReplaceConfigRequest: %s", request))
	return client.ReplaceConfig(cmsCtx, request)
}
