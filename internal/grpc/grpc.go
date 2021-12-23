package grpc

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	"google.golang.org/grpc"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

type InsecureGrpcClient struct {
	Context context.Context
	Target  string
	Log logr.Logger
}

func NewGrpcClient(ctx context.Context, target string) InsecureGrpcClient {
	return InsecureGrpcClient{ctx, target, log.FromContext(ctx)}
}

func (client *InsecureGrpcClient) Invoke(method string, input interface{}, output interface{}, log bool) error {
	var opts []grpc.DialOption
	opts = append(opts, grpc.WithInsecure())

	if log {
		client.Log.Info(fmt.Sprintf("creating channel to endpoint: %s, options: %s", client.Target, opts))
	}
	conn, err := grpc.Dial(client.Target, opts...)
	if err != nil {
		client.Log.Error(err, "creating channel failed")
		return err
	}
	defer conn.Close()

	err = conn.Invoke(client.Context, method, input, output)
	if log {
		client.Log.Info(fmt.Sprintf("method call: %s, request: %s, response: %s, err: %s", method, input, output, err))
	}
	if err != nil {
		return err
	}
	return nil
}
