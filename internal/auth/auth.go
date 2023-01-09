package auth

import (
	"context"
	"errors"
	"fmt"

	"github.com/ydb-platform/ydb-go-genproto/protos/Ydb_Auth"
	"github.com/ydb-platform/ydb-kubernetes-operator/internal/grpc"

	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/proto"
)

const (
	HeaderTicket = "x-ydb-auth-ticket"
)

func AddUserTokenToCtx(ctx context.Context, grpcEndpoint string, grpcsEnabled bool, log func(msg string, kv ...interface{})) (context.Context, error) {
	md, has := metadata.FromOutgoingContext(ctx)
	if !has {
		md = metadata.MD{}
	}

	token, err := GetAuthToken(ctx, grpcEndpoint, grpcsEnabled, log)

	if err != nil {
		return ctx, fmt.Errorf("Failed to get auth token: %w", err)
	}

  // log("success in receiving " + token)

	md.Set(HeaderTicket, token)

	return metadata.NewOutgoingContext(ctx, md), nil
}

func GetAuthTokenFromMetadata(ctx context.Context) string {
	md, _ := metadata.FromOutgoingContext(ctx)
	return md[HeaderTicket][0]
}

func GetAuthToken(ctx context.Context, grpcEndpoint string, grpcsEnabled bool, log func(msg string, kv ...interface{})) (string, error) {
	log("hey 1")
	client := grpc.Client{
		Context: ctx,
		Target:  grpcEndpoint,
	}

	log("hey 2")

	request := &Ydb_Auth.LoginRequest{User: "root", Password: ""}
	response := &Ydb_Auth.LoginResponse{}
	log("hey 3")
	err := client.Invoke(
		"/Ydb.Auth.V1.AuthService/Login",
		request,
		response,
		grpcsEnabled,
	)
	log("hey 4")

	if err != nil {
		return "", err
	}

	result := &Ydb_Auth.LoginResult{}
	log("hey 5")

	if response.Operation == nil {
		return "", errors.New("empty response from Storage")
	}
	log("hey 6")

	log(fmt.Sprintf("Result: %v", response.Operation.Result))
	log(fmt.Sprintf("Result get value: %v", response.Operation.Result.GetValue()))

	if err := proto.Unmarshal(response.Operation.Result.GetValue(), result); err == nil {
		log("hey 7")
		return result.Token, nil
	} else {
		return "", fmt.Errorf("Failed to unmarshal: %w", err)
	}
}
