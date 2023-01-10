package healthcheck

import (
	"context"
	"fmt"

	"github.com/ydb-platform/ydb-go-genproto/Ydb_Monitoring_V1"
	"github.com/ydb-platform/ydb-go-genproto/protos/Ydb_Monitoring"
	"github.com/ydb-platform/ydb-go-sdk/v3"
	"github.com/ydb-platform/ydb-kubernetes-operator/internal/resources"
	"google.golang.org/protobuf/proto"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

func GetSelfCheckResult(oldCtx context.Context, cluster *resources.StorageClusterBuilder) (*Ydb_Monitoring.SelfCheckResult, error) {
	ctx := context.Background()
	db, err := ydb.Open(
		ctx,
		fmt.Sprintf("%s/%s", cluster.GetGRPCEndpointWithProto(), cluster.Storage.Spec.Domain),
		ydb.WithStaticCredentials("root", ""),
	)
	if err != nil {
		return nil, err
	}

	logger := log.FromContext(oldCtx)

	defer func() {
		if e := db.Close(ctx); e != nil {
			logger.Error(e, "db close failed")
		}
	}()

	client := Ydb_Monitoring_V1.NewMonitoringServiceClient(ydb.GRPCConn(db))
	response, err := client.SelfCheck(ctx, &Ydb_Monitoring.SelfCheckRequest{})

	result := &Ydb_Monitoring.SelfCheckResult{}
	if err = proto.Unmarshal(response.Operation.Result.GetValue(), result); err != nil {
		return result, err
	}

	return result, nil
}
