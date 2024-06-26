package healthcheck

import (
	"context"
	"fmt"

	"github.com/ydb-platform/ydb-go-genproto/Ydb_Monitoring_V1"
	"github.com/ydb-platform/ydb-go-genproto/protos/Ydb_Monitoring"
	ydb "github.com/ydb-platform/ydb-go-sdk/v3"
	ydbCredentials "github.com/ydb-platform/ydb-go-sdk/v3/credentials"
	"google.golang.org/protobuf/proto"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/ydb-platform/ydb-kubernetes-operator/internal/connection"
	"github.com/ydb-platform/ydb-kubernetes-operator/internal/resources"
)

func GetSelfCheckResult(
	ctx context.Context,
	cluster *resources.StorageClusterBuilder,
	creds ydbCredentials.Credentials,
	opts ...ydb.Option,
) (*Ydb_Monitoring.SelfCheckResult, error) {
	logger := log.FromContext(ctx)
	getSelfCheckURL := fmt.Sprintf(
		"%s/%s",
		cluster.GetStorageEndpointWithProto(),
		cluster.Storage.Spec.Domain,
	)

	db, err := connection.Open(ctx,
		getSelfCheckURL,
		ydb.WithCredentials(creds),
		ydb.MergeOptions(opts...),
	)
	if err != nil {
		return nil, err
	}
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
