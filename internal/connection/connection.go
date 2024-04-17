package connection

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
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

func LoadTLSCredentials(secure bool, caBundle []byte) (grpc.DialOption, error) {
	if !secure {
		return grpc.WithTransportCredentials(insecure.NewCredentials()), nil
	}
	var certPool *x509.CertPool
	if len(caBundle) > 0 {
		certPool = x509.NewCertPool()
		if ok := certPool.AppendCertsFromPEM(caBundle); !ok {
			return nil, errors.New("failed to parse CA bundle")
		}
	} else {
		var err error
		certPool, err = x509.SystemCertPool()
		if err != nil {
			return nil, fmt.Errorf("failed to get system cert pool, error: %w", err)
		}
	}
	tlsConfig := &tls.Config{
		MinVersion: tls.VersionTLS12,
		RootCAs:    certPool,
	}
	return grpc.WithTransportCredentials(credentials.NewTLS(tlsConfig)), nil
}
