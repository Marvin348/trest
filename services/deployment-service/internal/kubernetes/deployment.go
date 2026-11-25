package kubernetes

import (
	"example.com/trest/services/deployment-service/internal/deployment"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func newDeployment(
	message deployment.DeploymentRequestedMessage,
	secretName string,
) *appsv1.Deployment {
	name := "deployment-" + message.DeploymentID.String()

	labels := map[string]string{
		"app":            name,
		"deployment-id":  message.DeploymentID.String(),
		"project-id":     message.ProjectID.String(),
		"application-id": message.ApplicationID.String(),
	}

	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:   name,
			Labels: labels,
		},

		Spec: appsv1.DeploymentSpec{
			Replicas: &message.Replicas,

			Selector: &metav1.LabelSelector{
				MatchLabels: labels,
			},

			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels,
				},

				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "application",
							Image: message.Image,

							Ports: []corev1.ContainerPort{
								{
									ContainerPort: message.ContainerPort,
									Protocol:      corev1.ProtocolTCP,
								},
							},

							EnvFrom: []corev1.EnvFromSource{
								{
									SecretRef: &corev1.SecretEnvSource{
										LocalObjectReference: corev1.LocalObjectReference{
											Name: secretName,
										},
									},
								},
							},

							Resources: corev1.ResourceRequirements{
								Requests: corev1.ResourceList{
									corev1.ResourceCPU: *resource.NewMilliQuantity(int64(message.CPURequestM),
										resource.DecimalSI),

									corev1.ResourceMemory: *resource.NewQuantity(int64(message.MemoryRequestMi)*1024*1024,
										resource.BinarySI),
								},
								Limits: corev1.ResourceList{
									corev1.ResourceCPU: *resource.NewMilliQuantity(int64(message.CPULimitM), resource.DecimalSI),

									corev1.ResourceMemory: *resource.NewQuantity(int64(message.MemoryLimitMi)*1024*1024, resource.BinarySI),
								},
							},
						},
					},
				},
			},
		},
	}
}
