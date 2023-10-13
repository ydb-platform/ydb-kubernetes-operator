package connection

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"strings"
	"time"

	"github.com/ydb-platform/ydb-go-sdk/v3"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"sigs.k8s.io/controller-runtime/pkg/log"

	ydbv1alpha1 "github.com/ydb-platform/ydb-kubernetes-operator/api/v1alpha1"
)

func Open(ctx context.Context, endpoint string, opts ...ydb.Option) (*ydb.Driver, error) {
	logger := log.FromContext(ctx)
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	opts = append(
		opts,
		buildYDBTLSOption(endpoint),
	)
	db, err := ydb.Open(
		ctx,
		endpoint,
		opts...,
	)
	if err != nil {
		logger.Error(err,
			fmt.Sprintf(
				"Failed to open grpc connection to YDB, endpoint %s",
				endpoint,
			))
		return nil, err
	}

	return db, nil
}

func Close(ctx context.Context, db *ydb.Driver) {
	logger := log.FromContext(ctx)
	if err := db.Close(ctx); err != nil {
		logger.Error(err, "db close failed")
	}
}

func buildYDBTLSOption(endpoint string) ydb.Option {
	certPool, _ := x509.SystemCertPool()
	// TODO(shmel1k@): figure out min allowed TLS version?
	tlsConfig := &tls.Config{ //nolint
		RootCAs: certPool,
	}
	if strings.HasPrefix(endpoint, ydbv1alpha1.GRPCSProto) {
		return ydb.WithTLSConfig(tlsConfig)
	}
	return ydb.WithTLSSInsecureSkipVerify()
}

func BuildGRPCTLSOption(endpoint string) grpc.DialOption {
	certPool, _ := x509.SystemCertPool()
	// TODO(shmel1k@): figure out min allowed TLS version?
	tlsConfig := &tls.Config{ //nolint
		RootCAs: certPool,
	}
	if strings.HasPrefix(endpoint, ydbv1alpha1.GRPCSProto) {
		return grpc.WithTransportCredentials(credentials.NewTLS(tlsConfig))
	}
	return grpc.WithTransportCredentials(insecure.NewCredentials())
}
