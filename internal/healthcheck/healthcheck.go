package healthcheck

import (
	"context"

	"github.com/ydb-platform/ydb-go-genproto/protos/Ydb_Monitoring"
	"google.golang.org/protobuf/proto"

	"github.com/ydb-platform/ydb-kubernetes-operator/internal/grpc"
	"github.com/ydb-platform/ydb-kubernetes-operator/internal/resources"
)

const (
	selfCheckEndpoint = "/Ydb.Monitoring.V1.MonitoringService/SelfCheck"
)

func GetSelfCheckResult(ctx context.Context, cluster *resources.StorageClusterBuilder) (*Ydb_Monitoring.SelfCheckResult, error) {
	client := grpc.Client{
		Context:      ctx,
		Target:       cluster.GetGRPCEndpoint(),
		TLS:          cluster.Spec.Service.GRPC.TLSConfiguration.Enabled,
		DatabaseName: cluster.Name,
	}

	response := Ydb_Monitoring.SelfCheckResponse{}
	err := client.Invoke(
		selfCheckEndpoint,
		&Ydb_Monitoring.SelfCheckRequest{},
		&response,
		cluster.Spec.Service.GRPC.TLSConfiguration.Enabled,
	)

	result := &Ydb_Monitoring.SelfCheckResult{}
	if err != nil {
		return result, err
	}

	if err = proto.Unmarshal(response.Operation.Result.GetValue(), result); err != nil {
		return result, err
	}

	return result, err
}
