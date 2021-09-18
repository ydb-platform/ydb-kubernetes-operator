package healthcheck

import (
	"context"
	"errors"
	"fmt"

	"github.com/golang/protobuf/proto"
	"github.com/ydb-platform/ydb-go-genproto/protos/Ydb_Monitoring"
	"github.com/ydb-platform/ydb-kubernetes-operator/pkg/grpc"
	"github.com/ydb-platform/ydb-kubernetes-operator/pkg/resources"
)

const (
	selfCheckEndpoint = "/Ydb.Monitoring.V1.MonitoringService/SelfCheck"
)

func CheckBootstrapHealth(ctx context.Context, cluster *resources.StorageClusterBuilder) error {
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

	if err != nil {
		return err
	}

	result := &Ydb_Monitoring.SelfCheckResult{}
	if err := proto.Unmarshal(response.Operation.Result.GetValue(), result); err != nil {
		return err
	}

	if len(result.IssueLog) > 0 {
		return errors.New(
			fmt.Sprintf("healthcheck API status code %s", result.SelfCheckResult.String()),
		)
	}

	return nil
}
