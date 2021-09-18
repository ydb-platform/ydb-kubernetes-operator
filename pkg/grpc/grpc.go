package grpc

import (
	"context"

	"google.golang.org/grpc"
)

type InsecureGrpcClient struct {
	Context context.Context
	Target  string
}

func (client *InsecureGrpcClient) Invoke(method string, input interface{}, output interface{}) error {
	var opts []grpc.DialOption
	opts = append(opts, grpc.WithInsecure())

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
