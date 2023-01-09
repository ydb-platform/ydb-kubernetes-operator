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

func AddUserTokenToCtx(ctx context.Context, grpcEndpoint string, grpcsEnabled bool) (context.Context, error) {
	md, has := metadata.FromOutgoingContext(ctx)
	if !has {
		md = metadata.MD{}
	}

	token, err := GetAuthToken(ctx, grpcEndpoint, grpcsEnabled)

	if err != nil {
		return ctx, fmt.Errorf("Failed to get auth token: %w", err)
	}

	md.Set(HeaderTicket, token)

	return metadata.NewOutgoingContext(ctx, md), nil
}

func GetAuthTokenFromMetadata(ctx context.Context) string {
	md, _ := metadata.FromOutgoingContext(ctx)
	return md[HeaderTicket][0]
}

func GetAuthToken(ctx context.Context, grpcEndpoint string, grpcsEnabled bool) (string, error) {
	client := grpc.Client{
		Context: ctx,
		Target:  grpcEndpoint,
	}

	request := &Ydb_Auth.LoginRequest{User: "root", Password: ""}
	response := &Ydb_Auth.LoginResponse{}
	err := client.Invoke(
		"/Ydb.Auth.V1.AuthService/Login",
		request,
		response,
		grpcsEnabled,
	)

	if err != nil {
		return "", err
	}

	result := &Ydb_Auth.LoginResult{}

	if response.Operation == nil {
		return "", errors.New("empty response from Storage")
	}

	if err := proto.Unmarshal(response.Operation.Result.GetValue(), result); err == nil {
		return result.Token, nil
	} else {
		return "", fmt.Errorf("Failed to unmarshal: %w", err)
	}
}
