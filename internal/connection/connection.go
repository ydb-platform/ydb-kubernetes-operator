package connection

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"time"

	"github.com/ydb-platform/ydb-go-sdk/v3"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

func Open(ctx context.Context, endpoint string, opts ...ydb.Option) (*ydb.Driver, error) {
	ctx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()

	db, err := ydb.Open(
		ctx,
		endpoint,
		opts...,
	)
	if err != nil {
		return nil, fmt.Errorf(
			"failed to open grpc connection to YDB, endpoint %s. %w",
			endpoint,
			err,
		)
	}

	return db, nil
}

func Close(ctx context.Context, db *ydb.Driver) {
	logger := log.FromContext(ctx)
	if err := db.Close(ctx); err != nil {
		logger.Error(err, "db close failed")
	}
}

func LoadTLSCredentials(secure bool) grpc.DialOption {
	if secure {
		certPool, _ := x509.SystemCertPool()
		tlsConfig := &tls.Config{
			MinVersion: tls.VersionTLS12,
			RootCAs:    certPool,
		}
		return grpc.WithTransportCredentials(credentials.NewTLS(tlsConfig))
	}
	return grpc.WithTransportCredentials(insecure.NewCredentials())
}
