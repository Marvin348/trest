package app

import (
	"fmt"
	"net"

	deploymentv1 "example.com/trest/contracts/deployment/v1"
	"example.com/trest/services/deployment-service/internal/deployment"
	deploymentgrpc "example.com/trest/services/deployment-service/internal/transport/grpc"

	"google.golang.org/grpc"
)

func newGRPCServer(adress string, service *deployment.Service) (*grpc.Server, net.Listener, error) {
	listener, err := net.Listen("tcp", adress)
	if err != nil {
		return nil, nil, fmt.Errorf("listen on gRPC address: %w", err)
	}

	server := grpc.NewServer()

	deploymentv1.RegisterDeploymentServiceServer(server, deploymentgrpc.NewServer(service))

	return server, listener, nil
}
