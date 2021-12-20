package grpc

import (
	"context"
	"crypto/tls"
	"crypto/x509"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

type GrpcClient struct {
	Context context.Context
	Target  string
}

func buildSystemTLSStoreOption() grpc.DialOption {
	certPool, _ := x509.SystemCertPool()
	tlsCredentials := credentials.NewTLS(&tls.Config{
		RootCAs: certPool,
	})
	return grpc.WithTransportCredentials(tlsCredentials)
}

func (client *GrpcClient) Invoke(method string, input interface{}, output interface{}, insecure bool) error {
	var opts []grpc.DialOption

	if insecure {
		opts = append(opts, grpc.WithInsecure())
	} else {
		opts = append(opts, buildSystemTLSStoreOption())
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
