package grpc

import (
	"context"
	"crypto/tls"
	"crypto/x509"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

type Client struct {
	Context context.Context
	Target  string
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
		opts = append(opts, grpc.WithInsecure())
	}

	conn, err := grpc.Dial(client.Target, opts...)
	if err != nil {
		return err
	}
	defer conn.Close()

	err = conn.Invoke(client.Context, method, input, output)
	if err != nil {
		return err
	}
	return nil
}
