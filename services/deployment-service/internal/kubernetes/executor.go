package kubernetes

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"time"
	"uuid"

	"example.com/trest/services/deployment-service/internal/deployment"
	"github.com/Marvin348/trest/packages/secretcrypto"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

type Executor struct {
	client      kubernetes.Interface
	namespace   string
	keyProvider secretcrypto.KeyProvider
}

func NewExecutor(
	client kubernetes.Interface,
	namespace string,
	keyProvider secretcrypto.KeyProvider,
) *Executor {
	return &Executor{
		client:      client,
		namespace:   namespace,
		keyProvider: keyProvider,
	}
}

func (e *Executor) Deploy(ctx context.Context,
	message deployment.DeploymentRequestedMessage) error {
	environment, err := prepareEnvironment(
		message.Environment,
		e.keyProvider,
		message.ApplicationID,
	)
	if err != nil {
		return fmt.Errorf("prepare deployment environment: %w", err)
	}

	secret := createSecret(environment, message.ApplicationID)

	secretClient := e.client.CoreV1().Secrets(e.namespace)

	_, err = secretClient.
		Create(ctx, secret, metav1.CreateOptions{})

	if apierrors.IsAlreadyExists(err) {
		existing, getErr := secretClient.Get(
			ctx,
			secret.Name,
			metav1.GetOptions{},
		)
		if getErr != nil {
			return fmt.Errorf("get existing secret: %w", getErr)
		}

		if !reflect.DeepEqual(existing.Data, secret.Data) {
			return ErrSecretMismatch
		}
	} else if err != nil {
		return fmt.Errorf("create kubernetes secret: %w", err)
	}

	deployment := newDeployment(message, secret.Name)

	deploymentClient := e.client.AppsV1().Deployments(e.namespace)

	_, err = deploymentClient.
		Create(ctx, deployment, metav1.CreateOptions{})

	if apierrors.IsAlreadyExists(err) {
		existing, getErr := deploymentClient.Get(
			ctx,
			deployment.Name,
			metav1.GetOptions{},
		)
		if getErr != nil {
			return fmt.Errorf("get existing deployment: %w", getErr)
		}

		if !sameDeploymentConfig(existing, deployment) {
			return ErrDeploymentMismatch
		}

	} else if err != nil {
		return fmt.Errorf("create kubernetes deployment: %w", err)
	}

	readyCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()

	if err := e.waitForDeploymentReady(
		readyCtx,
		deployment.Name,
		message.Replicas,
	); err != nil {
		return fmt.Errorf("wait for deployment readiness: %w", err)
	}

	return nil
}

func prepareEnvironment(
	environment map[string]secretcrypto.EncryptedValue,
	keyProvider secretcrypto.KeyProvider,
	applicationID uuid.UUID,
) (map[string][]byte, error) {
	values := make(map[string][]byte, len(environment))
	if len(environment) > 0 && keyProvider == nil {
		return nil, fmt.Errorf("key provider is required")
	}

	for name, encrypted := range environment {
		key, err := keyProvider.GetKey(encrypted.KeyID)
		if err != nil {
			return nil, fmt.Errorf("get encryption key %q: %w",
				encrypted.KeyID, err)
		}

		plaintext, err := secretcrypto.Decrypt(
			encrypted,
			key,
			applicationID.String(),
			name,
		)
		if err != nil {
			return nil, fmt.Errorf(
				"decrypt environment variable %q: %w",
				name,
				err,
			)
		}

		values[name] = []byte(plaintext)
	}

	return values, nil
}

func createSecret(environment map[string][]byte,
	deploymentID uuid.UUID) *corev1.Secret {
	secretName := "deployment-" + deploymentID.String() + "-env"

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name: secretName,
		},
		Type: corev1.SecretTypeOpaque,
		Data: environment,
	}

	return secret
}

func sameDeploymentConfig(
	existing *appsv1.Deployment,
	desired *appsv1.Deployment,
) bool {
	if existing == nil || desired == nil {
		return false
	}

	if existing.Name != desired.Name ||
		existing.Labels["deployment-id"] != desired.Labels["deployment-id"] ||
		existing.Labels["project-id"] != desired.Labels["project-id"] ||
		existing.Labels["application-id"] != desired.Labels["application-id"] {
		return false
	}

	if existing.Spec.Replicas == nil || desired.Spec.Replicas == nil {
		return false
	}

	if *existing.Spec.Replicas != *desired.Spec.Replicas {
		return false
	}

	if len(existing.Spec.Template.Spec.Containers) != 1 ||
		len(desired.Spec.Template.Spec.Containers) != 1 {
		return false
	}

	a := existing.Spec.Template.Spec.Containers[0]
	b := desired.Spec.Template.Spec.Containers[0]

	if a.Name != b.Name || a.Image != b.Image {
		return false
	}

	if len(a.Ports) != 1 || len(b.Ports) != 1 {
		return false
	}

	if a.Ports[0].ContainerPort != b.Ports[0].ContainerPort ||
		a.Ports[0].Protocol != b.Ports[0].Protocol {
		return false
	}

	for _, resourceName := range []corev1.ResourceName{
		corev1.ResourceCPU,
		corev1.ResourceMemory,
	} {
		aRequest, aOK := a.Resources.Requests[resourceName]
		bRequest, bOK := b.Resources.Requests[resourceName]

		if !aOK || !bOK || aRequest.Cmp(bRequest) != 0 {
			return false
		}

		aLimit, aOK := a.Resources.Limits[resourceName]
		bLimit, bOK := b.Resources.Limits[resourceName]

		if !aOK || !bOK || aLimit.Cmp(bLimit) != 0 {
			return false
		}
	}

	if !reflect.DeepEqual(a.EnvFrom, b.EnvFrom) {
		return false
	}

	return true
}

func (e *Executor) waitForDeploymentReady(
	ctx context.Context,
	name string,
	replicas int32,
) error {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	deploymentClient := e.client.AppsV1().Deployments(e.namespace)

	var lastProblem *PodProblem
	for {
		deployment, err := deploymentClient.Get(
			ctx,
			name,
			metav1.GetOptions{},
		)
		if err != nil {
			return fmt.Errorf("get kubernetes deployment: %w", err)
		}

		if deployment.Status.ObservedGeneration >= deployment.Generation &&
			deployment.Status.AvailableReplicas >= replicas &&
			deployment.Status.UpdatedReplicas >= replicas {
			return nil
		}

		problem, err := e.checkPodProblems(ctx, deployment)
		if err != nil {
			return err
		}

		if problem != nil {
			lastProblem = problem
		} else {
			lastProblem = nil
		}

		select {
		case <-ctx.Done():
			if errors.Is(ctx.Err(), context.DeadlineExceeded) &&
				lastProblem != nil {

				return fmt.Errorf(
					"deployment readiness timed out: pod %s: %s: %s: %w",
					lastProblem.PodName,
					lastProblem.Reason,
					lastProblem.Message,
					ctx.Err(),
				)
			}
			return ctx.Err()

		case <-ticker.C:

		}

	}
}
