package deploymentgrpc

import (
	deploymentv1 "example.com/trest/contracts/deployment/v1"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func NewClient(
	adress string,
) (deploymentv1.DeploymentServiceClient, *grpc.ClientConn, error) {

	conn, err := grpc.NewClient(
		adress,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		return nil, nil, err
	}

	client := deploymentv1.NewDeploymentServiceClient(conn)

	return client, conn, nil
}
