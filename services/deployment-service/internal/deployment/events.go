package deployment

type EventType string

const (
	DeploymentCreated   EventType = "DeploymentCreated"
	DeploymentStarted   EventType = "DeploymentStarted"
	DeploymentSucceeded EventType = "DeploymentSucceeded"
	DeploymentFailed    EventType = "DeploymentFailed"
)