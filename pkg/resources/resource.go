package resources

import (
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	grpcServiceNameFormat         = "%s-grpc"
	interconnectServiceNameFormat = "%s-interconnect"
	statusServiceNameFormat       = "%s-status"
)

type ResourceBuilder interface {
	Placeholder(cr client.Object) client.Object
	Build(client.Object) error
}
