package kubernetes

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestWaitForDeploymentReadyStopsWhenContextEnds(t *testing.T) {
	for _, withProblem := range []bool{false, true} {
		t.Run(map[bool]string{false: "without pod problem", true: "with pod problem"}[withProblem], func(t *testing.T) {
			client := fake.NewSimpleClientset(&appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name: "deployment", Namespace: "default",
					Labels: map[string]string{"deployment-id": "test"},
				},
			})

			if withProblem {
				_, err := client.CoreV1().Pods("default").Create(context.Background(), &corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Name: "pod", Labels: map[string]string{"deployment-id": "test"},
					},
					Status: corev1.PodStatus{ContainerStatuses: []corev1.ContainerStatus{{
						State: corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{
							Reason: "ImagePullBackOff", Message: "image unavailable",
						}},
					}}},
				}, metav1.CreateOptions{})
				require.NoError(t, err)
			}
			
			executor := NewExecutor(client, "default", nil)
			ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
			defer cancel()
			err := executor.waitForDeploymentReady(ctx, "deployment", 1)
			require.ErrorIs(t, err, context.DeadlineExceeded)
			if withProblem {
				require.ErrorContains(t, err, "ImagePullBackOff")
			}
		})
	}
}
