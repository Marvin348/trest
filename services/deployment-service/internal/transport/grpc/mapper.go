package grpc

import (
	"uuid"

	deploymentv1 "example.com/trest/contracts/deployment/v1"
	"example.com/trest/services/deployment-service/internal/deployment"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func parseCreateDeploymentRequest(
	req *deploymentv1.CreateDeploymentRequest,
) (deployment.CreateDeploymentParams, error) {

	if req == nil {
		return deployment.CreateDeploymentParams{},
			status.Error(codes.InvalidArgument, "request is required")
	}

	requestID, err := uuid.Parse(req.GetRequestId())
	if err != nil || requestID == uuid.Nil() {
		return deployment.CreateDeploymentParams{},
			status.Error(codes.InvalidArgument, "invalid request_id")
	}

	applicationID, err := uuid.Parse(req.GetApplicationId())
	if err != nil || applicationID == uuid.Nil() {
		return deployment.CreateDeploymentParams{},
			status.Error(codes.InvalidArgument, "invalid application_id")
	}

	projectID, err := uuid.Parse(req.GetProjectId())
	if err != nil || projectID == uuid.Nil() {
		return deployment.CreateDeploymentParams{},
			status.Error(codes.InvalidArgument, "invalid project_id")
	}

	params := deployment.CreateDeploymentParams{
		RequestID:     requestID,
		ApplicationID: applicationID,
		ProjectID:     projectID,
		Image:         req.GetImage(),
		Replicas:      req.GetReplicas(),
		ContainerPort: req.GetContainerPort(),

		CPURequestM:     req.GetCpuRequestM(),
		CPULimitM:       req.GetCpuLimitM(),
		MemoryRequestMi: req.GetMemoryRequestMi(),
		MemoryLimitMi:   req.GetMemoryLimitMi(),

		// later env vars
	}

	return params, nil
}
