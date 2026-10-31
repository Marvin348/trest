package app

import (
	"fmt"
	"net"

	deploymentv1 "example.com/trest/contracts/deployment/v1"
	deploymentgrpc "example.com/trest/services/deployment-service/internal/transport/grpc"

	"google.golang.org/grpc"
)

func newGRPCServer(adress string) (*grpc.Server, net.Listener, error) {
	listener, err := net.Listen("tcp", adress)
	if err != nil {
		return nil, nil, fmt.Errorf("listen on gRPC address: %w", err)
	}

	server := grpc.NewServer()

	deploymentv1.RegisterDeploymentServiceServer(server, deploymentgrpc.NewServer())

	return server, listener, nil
}
