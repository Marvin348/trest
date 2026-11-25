package kubernetes

import (
	"context"
	"fmt"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type PodProblem struct {
	PodName string
	Reason  string
	Message string
}

func (e *Executor) checkPodProblems(
	ctx context.Context,
	deployment *appsv1.Deployment,
) (*PodProblem, error) {
	pods, err := e.client.CoreV1().
		Pods(e.namespace).
		List(ctx, metav1.ListOptions{
			LabelSelector: "deployment-id=" + deployment.Labels["deployment-id"],
		})
	if err != nil {
		return nil, fmt.Errorf("list deployment pods: %w", err)
	}

	for _, pod := range pods.Items {
		for _, container := range pod.Status.ContainerStatuses {

			waiting := container.State.Waiting
			if waiting == nil {
				continue
			}

			switch waiting.Reason {
			case "ImagePullBackOff", "ErrImagePull":

				return &PodProblem{
					PodName: pod.Name,
					Reason:  waiting.Reason,
					Message: waiting.Message,
				}, nil
			}
		}
	}

	return nil, nil
}
