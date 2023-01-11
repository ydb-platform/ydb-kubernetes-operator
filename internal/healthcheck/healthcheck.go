package healthcheck

import (
	"context"
	"fmt"

	"github.com/ydb-platform/ydb-go-genproto/Ydb_Monitoring_V1"
	"github.com/ydb-platform/ydb-go-genproto/protos/Ydb_Monitoring"
	"github.com/ydb-platform/ydb-go-sdk/v3"
	"google.golang.org/protobuf/proto"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/ydb-platform/ydb-kubernetes-operator/internal/connection"
	"github.com/ydb-platform/ydb-kubernetes-operator/internal/resources"
)

func GetSelfCheckResult(ctx context.Context, cluster *resources.StorageClusterBuilder) (*Ydb_Monitoring.SelfCheckResult, error) {
	getSelfCheckURL := fmt.Sprintf(
		"%s/%s",
		cluster.GetGRPCEndpointWithProto(),
		cluster.Storage.Spec.Domain,
	)

	db, err := connection.Build(ctx, getSelfCheckURL)

	if err != nil {
		return nil, err
	}

	logger := log.FromContext(ctx)

	defer func() {
		connection.Close(ctx, db)
	}()

	client := Ydb_Monitoring_V1.NewMonitoringServiceClient(ydb.GRPCConn(db))
	response, err := client.SelfCheck(ctx, &Ydb_Monitoring.SelfCheckRequest{})

	if err != nil {
		logger.Error(err, "Failed to call SelfCheck")
		return nil, err
	}

	result := &Ydb_Monitoring.SelfCheckResult{}
	if err = proto.Unmarshal(response.Operation.Result.GetValue(), result); err != nil {
		logger.Error(err, "Failed to unmarshal SelfCheck response")
		return result, err
	}

	return result, nil
}
