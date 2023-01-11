package healthcheck

import (
	"context"
	"fmt"

	"github.com/ydb-platform/ydb-go-genproto/Ydb_Monitoring_V1"
	"github.com/ydb-platform/ydb-go-genproto/protos/Ydb_Monitoring"
	"github.com/ydb-platform/ydb-go-sdk/v3"
	"github.com/ydb-platform/ydb-kubernetes-operator/internal/connection"
	"github.com/ydb-platform/ydb-kubernetes-operator/internal/resources"
	"google.golang.org/protobuf/proto"
)

func GetSelfCheckResult(ctx context.Context, cluster *resources.StorageClusterBuilder) (*Ydb_Monitoring.SelfCheckResult, error) {
	getSelfCheckUrl := fmt.Sprintf(
		"%s/%s",
		cluster.GetGRPCEndpointWithProto(),
		cluster.Storage.Spec.Domain,
	)

	db, err := connection.Build(ctx, getSelfCheckUrl)

	if err != nil {
		return nil, err
	}

	defer func() {
		connection.Close(ctx, db)
	}()

	client := Ydb_Monitoring_V1.NewMonitoringServiceClient(ydb.GRPCConn(db))
	response, err := client.SelfCheck(ctx, &Ydb_Monitoring.SelfCheckRequest{})

	result := &Ydb_Monitoring.SelfCheckResult{}
	if err = proto.Unmarshal(response.Operation.Result.GetValue(), result); err != nil {
		return result, err
	}

	return result, nil
}
