package connection

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"

	"github.com/ydb-platform/ydb-go-sdk/v3"
	ydbCredentials "github.com/ydb-platform/ydb-go-sdk/v3/credentials"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

type YDBConnection struct {
	ctx         context.Context
	endpoint    string
	secure      bool
	credentials ydbCredentials.Credentials
}

func NewYDBConnection(ctx context.Context, endpoint string, secure bool, credentials ydbCredentials.Credentials) *YDBConnection {
	return &YDBConnection{
		ctx:         ctx,
		endpoint:    endpoint,
		secure:      secure,
		credentials: credentials,
	}
}

func (conn *YDBConnection) Open() (*ydb.Driver, error) {
	db, err := ydb.Open(
		conn.ctx,
		conn.endpoint,
		conn.getOptions()...,
	)
	if err != nil {
		log.FromContext(conn.ctx).Error(err,
			fmt.Sprintf(
				"Failed to open grpc connection to YDB, endpoint %s",
				conn.endpoint,
			))
		return nil, err
	}

	return db, nil
}

func (conn *YDBConnection) Close(db *ydb.Driver) {
	logger := log.FromContext(conn.ctx)
	if err := db.Close(conn.ctx); err != nil {
		logger.Error(err, "db close failed")
	}
}

func (conn *YDBConnection) getOptions() []ydb.Option {
	var opts []ydb.Option

	opts = append(opts, ydb.WithCredentials(conn.credentials))

	if conn.secure {
		certPool, _ := x509.SystemCertPool()
		// TODO(shmel1k@): figure out min allowed TLS version?
		opts = append(opts, ydb.WithTLSConfig(&tls.Config{ //nolint
			RootCAs: certPool,
		}))
	} else {
		opts = append(opts, ydb.WithTLSSInsecureSkipVerify())
	}

	return opts
}

func GetGRPCDialOptions(secure bool) []grpc.DialOption {
	var opts []grpc.DialOption
	if secure {
		certPool, _ := x509.SystemCertPool()
		// TODO(shmel1k@): figure out min allowed TLS version?
		tlsCredentials := credentials.NewTLS(&tls.Config{ //nolint
			RootCAs: certPool,
		})
		opts = append(opts, grpc.WithTransportCredentials(tlsCredentials))
	} else {
		opts = append(opts, grpc.WithTransportCredentials(insecure.NewCredentials()))
	}

	return opts
}
