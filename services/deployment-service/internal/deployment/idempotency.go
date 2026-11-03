package deployment

// add envs later
func sameDeploymentRequest(
	existing Deployment, params CreateDeploymentParams,
) bool {
	return existing.ProjectID == params.ProjectID &&
		existing.ApplicationID == params.ApplicationID &&
		existing.Image == params.Image &&
		existing.ContainerPort == params.ContainerPort &&
		existing.Replicas == params.Replicas &&
		existing.CPURequestM == params.CPURequestM &&
		existing.CPULimitM == params.CPULimitM &&
		existing.MemoryRequestMi == params.MemoryRequestMi &&
		existing.MemoryLimitMi == params.MemoryLimitMi
}
