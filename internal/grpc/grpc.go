package grpc

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"os"

	"github.com/ydb-platform/ydb-go-sdk/v3"
	creds "github.com/ydb-platform/ydb-go-sdk/v3/credentials"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

type Client struct {
	Context      context.Context
	Target       string
	DatabaseName string
	TLS          bool
}

func (client Client) DatabaseEndpoint() string {
	s := ""
	if client.TLS {
		s = "s"
	}
	return fmt.Sprintf("grpc%s://%s/%s", s, client.Target, client.DatabaseName)
}

func buildSystemTLSStoreOption() grpc.DialOption {
	certPool, _ := x509.SystemCertPool()
	// TODO(shmel1k@): figure out min allowed TLS version?
	tlsCredentials := credentials.NewTLS(&tls.Config{ //nolint
		RootCAs: certPool,
	})
	return grpc.WithTransportCredentials(tlsCredentials)
}

func (client *Client) Invoke(method string, input interface{}, output interface{}, secure bool) error {
	var opts []grpc.DialOption

	if secure {
		opts = append(opts, buildSystemTLSStoreOption())
	} else {
		opts = append(opts, grpc.WithTransportCredentials(insecure.NewCredentials()))
	}

	var conn grpc.ClientConnInterface
	if len(os.Getenv("YDB_STATIC_USERNAME")) != 0 && len(os.Getenv("YDB_STATIC_PASSWORD")) != 0 {
		db, err := ydb.Open(
			client.Context,
			client.DatabaseEndpoint(),
			ydb.WithStaticCredentials(os.Getenv("YDB_STATIC_USERNAME"), os.Getenv("YDB_STATIC_PASSWORD")),
		)
		if err != nil {
			return err
		}
		defer func() {
			if e := db.Close(client.Context); e != nil {
				log.FromContext(client.Context).Error(err, "")
			}
		}()
		conn = ydb.GRPCConn(db)
		token, err := creds.NewStaticCredentials(os.Getenv("YDB_STATIC_USERNAME"), os.Getenv("YDB_STATIC_PASSWORD"), client.Target, opts...).Token(client.Context)
		if err != nil {
			return err
		}
		err = os.Setenv("YDB_TOKEN", token)
		if err != nil {
			return err
		}
	} else {
		conn, err := grpc.Dial(client.Target, opts...)
		if err != nil {
			return err
		}
		defer conn.Close()
	}

	err := conn.Invoke(client.Context, method, input, output)
	if err != nil {
		return err
	}
	return nil
}
