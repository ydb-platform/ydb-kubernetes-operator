package connection

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"

	"github.com/ydb-platform/ydb-go-sdk/v3"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"sigs.k8s.io/controller-runtime/pkg/log"

	ydbCredentials "github.com/ydb-platform/ydb-go-sdk/v3/credentials"
)

const (
	DefaultRootUsername = "root"
	DefaultRootPassword = ""
)

func buildSystemTLSStoreOption() grpc.DialOption {
	certPool, _ := x509.SystemCertPool()
	// TODO(shmel1k@): figure out min allowed TLS version?
	tlsCredentials := credentials.NewTLS(&tls.Config{ //nolint
		RootCAs: certPool,
	})
	return grpc.WithTransportCredentials(tlsCredentials)
}

func buildConnectionOpts(secure bool) []grpc.DialOption {
	var opts []grpc.DialOption

	if secure {
		opts = append(opts, buildSystemTLSStoreOption())
	} else {
		opts = append(opts, grpc.WithTransportCredentials(insecure.NewCredentials()))
	}

	return opts
}

func GetAuthToken(ctx context.Context, grpcEndpoint string, secure bool) (string, error) {
	staticCredentials := ydbCredentials.NewStaticCredentials(
		DefaultRootUsername,
		DefaultRootPassword,
		grpcEndpoint,
		buildConnectionOpts(secure)...
	)

	token, err := staticCredentials.Token(ctx)

	if err != nil {
		return "", err
	}

	return token, nil
}

func Build(ctx context.Context, grpcEndpointWithProto string) (ydb.Connection, error) {
	db, err := ydb.Open(
		ctx,
		grpcEndpointWithProto,
		ydb.WithStaticCredentials(DefaultRootUsername, DefaultRootPassword),
	)

	if err != nil {
		log.FromContext(ctx).Error(err, 
		fmt.Sprintf(
			"Failed to open grpc connection to YDB, endpoint %s",
			grpcEndpointWithProto,
		))
		return nil, err
	}

	return db, nil
}
