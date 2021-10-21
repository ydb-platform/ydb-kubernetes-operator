package healthcheck

import (
	"context"

	"github.com/golang/protobuf/proto"
	"github.com/ydb-platform/ydb-go-genproto/protos/Ydb_Monitoring"
	"github.com/ydb-platform/ydb-kubernetes-operator/internal/grpc"
	"github.com/ydb-platform/ydb-kubernetes-operator/internal/resources"
)

const (
	selfCheckEndpoint = "/Ydb.Monitoring.V1.MonitoringService/SelfCheck"
)

func GetSelfCheckResult(ctx context.Context, cluster *resources.StorageClusterBuilder) (*Ydb_Monitoring.SelfCheckResult, error) {
	client := grpc.InsecureGrpcClient{
		Context: ctx,
		Target:  cluster.GetEndpoint(),
	}

	response := Ydb_Monitoring.SelfCheckResponse{}
	err := client.Invoke(
		selfCheckEndpoint,
		&Ydb_Monitoring.SelfCheckRequest{},
		&response,
	)

	result := &Ydb_Monitoring.SelfCheckResult{}
	if err != nil {
		return result, err
	}

	if err := proto.Unmarshal(response.Operation.Result.GetValue(), result); err != nil {
		return result, err
	}

	return result, err
}
