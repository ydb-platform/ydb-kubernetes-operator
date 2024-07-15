package cms

import (
	"context"
	"fmt"
	"time"

	"github.com/ydb-platform/ydb-go-genproto/draft/Ydb_DynamicConfig_V1"
	"github.com/ydb-platform/ydb-go-genproto/draft/protos/Ydb_DynamicConfig"
	"github.com/ydb-platform/ydb-go-genproto/protos/Ydb_Operations"
	"github.com/ydb-platform/ydb-go-sdk/v3"
	"google.golang.org/protobuf/types/known/durationpb"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/ydb-platform/ydb-kubernetes-operator/internal/connection"
)

const (
	GetConfigTimeoutSeconds     = 10
	ReplaceConfigTimeoutSeconds = 30
)

type Config struct {
	StorageEndpoint    string
	Domain             string
	Config             string
	Version            uint64
	DryRun             bool
	AllowUnknownFields bool
}

func (c *Config) GetConfig(
	ctx context.Context,
	opts ...ydb.Option,
) (*Ydb_DynamicConfig.GetConfigResponse, error) {
	logger := log.FromContext(ctx)

	endpoint := fmt.Sprintf("%s/%s", c.StorageEndpoint, c.Domain)
	ydbCtx, ydbCtxCancel := context.WithTimeout(ctx, time.Second)
	defer ydbCtxCancel()
	conn, err := connection.Open(ydbCtx, endpoint, ydb.MergeOptions(opts...))
	if err != nil {
		return nil, fmt.Errorf("error connecting to YDB: %w", err)
	}
	defer func() {
		connection.Close(ydbCtx, conn)
	}()

	cmsCtx, cmsCtxCancel := context.WithTimeout(ctx, GetConfigTimeoutSeconds*time.Second)
	defer cmsCtxCancel()
	client := Ydb_DynamicConfig_V1.NewDynamicConfigServiceClient(ydb.GRPCConn(conn))
	request := c.makeGetConfigRequest()

	logger.Info("CMS GetConfig", "endpoint", endpoint, "request", request)
	return client.GetConfig(cmsCtx, request)
}

func (c *Config) ProcessConfigResponse(response *Ydb_DynamicConfig.GetConfigResponse) error {
	configResult := &Ydb_DynamicConfig.GetConfigResult{}
	err := response.GetOperation().GetResult().UnmarshalTo(configResult)
	if err != nil {
		return err
	}

	c.Config = configResult.GetConfig()
	c.Version = configResult.GetIdentity().GetVersion()
	return nil
}

func (c *Config) ReplaceConfig(
	ctx context.Context,
	opts ...ydb.Option,
) (*Ydb_DynamicConfig.ReplaceConfigResponse, error) {
	logger := log.FromContext(ctx)

	endpoint := fmt.Sprintf("%s/%s", c.StorageEndpoint, c.Domain)
	ydbCtx, ydbCtxCancel := context.WithTimeout(ctx, time.Second)
	defer ydbCtxCancel()
	conn, err := connection.Open(ydbCtx, endpoint, ydb.MergeOptions(opts...))
	if err != nil {
		return nil, fmt.Errorf("error connecting to YDB: %w", err)
	}
	defer func() {
		connection.Close(ydbCtx, conn)
	}()

	cmsCtx, cmsCtxCancel := context.WithTimeout(ctx, ReplaceConfigTimeoutSeconds*time.Second)
	defer cmsCtxCancel()
	client := Ydb_DynamicConfig_V1.NewDynamicConfigServiceClient(ydb.GRPCConn(conn))
	request := &Ydb_DynamicConfig.ReplaceConfigRequest{
		Config:             c.Config,
		DryRun:             c.DryRun,
		AllowUnknownFields: c.AllowUnknownFields,
	}

	logger.Info("CMS ReplaceConfig", "endpoint", endpoint, "request", request)
	return client.ReplaceConfig(cmsCtx, request)
}

func (c *Config) CheckReplaceConfigResponse(ctx context.Context, response *Ydb_DynamicConfig.ReplaceConfigResponse) (bool, string, error) {
	logger := log.FromContext(ctx)

	logger.Info("CMS ReplaceConfig response", "response", response)
	return CheckOperationStatus(response.GetOperation())
}

func (c *Config) makeGetConfigRequest() *Ydb_DynamicConfig.GetConfigRequest {
	request := &Ydb_DynamicConfig.GetConfigRequest{}
	request.OperationParams = &Ydb_Operations.OperationParams{
		OperationTimeout: &durationpb.Duration{Seconds: GetConfigTimeoutSeconds},
	}

	return request
}
