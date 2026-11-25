package worker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"example.com/trest/services/deployment-service/internal/deployment"
	"example.com/trest/services/deployment-service/internal/eventstore"
	"example.com/trest/services/deployment-service/internal/kubernetes"
	"github.com/jackc/pgx/v5/pgxpool"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type DeploymentExecutor interface {
	Deploy(
		ctx context.Context,
		message deployment.DeploymentRequestedMessage,
	) error
}

type Service struct {
	eventRepo        *eventstore.Repository
	deploymentRepo   *deployment.Repository
	db               *pgxpool.Pool
	executor         DeploymentExecutor
}

func NewService(eventRepo *eventstore.Repository,
	deploymentRepo *deployment.Repository,
	db *pgxpool.Pool,
	executor DeploymentExecutor) *Service {
	return &Service{
		eventRepo:        eventRepo,
		deploymentRepo:   deploymentRepo,
		db:               db,
		executor:         executor,
	}
}

func (s *Service) Process(ctx context.Context,
	message deployment.DeploymentRequestedMessage) error {
	events, err := s.eventRepo.GetByDeploymentID(ctx,
		message.DeploymentID)
	if err != nil {
		return err
	}

	state, err := deployment.Rehydrate(events)
	if err != nil {
		return err
	}

	switch state.Status {
	case deployment.StatusQueued:
		state, err = s.startDeployment(ctx, state)
		if err != nil {
			return err
		}

	case deployment.StatusRunning:

	case deployment.StatusSucceeded:
		return nil

	case deployment.StatusFailed:
		return nil

	default:
		return fmt.Errorf("unexpcted deployment status: %s", state.Status)
	}

	executionErr := s.executor.Deploy(ctx, message)

	if executionErr == nil {
		return s.finishDeploymentSafely(ctx, state, nil)
	}

	if isPermanentDeploymentError(executionErr) {
		return s.finishDeploymentSafely(ctx, state, executionErr)
	}

	return fmt.Errorf("execute kubernetes deployment: %w", executionErr)
}

func (s *Service) startDeployment(ctx context.Context,
	state deployment.State) (deployment.State, error) {
	startedEvent := eventstore.Event{
		DeploymentID:  state.ID,
		Type:          string(deployment.DeploymentStarted),
		Version:       state.Version + 1,
		SchemaVersion: 1,
		Payload:       json.RawMessage(`{}`),
	}

	updatedState := state
	if err := updatedState.Apply(startedEvent); err != nil {
		return deployment.State{}, err
	}

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return deployment.State{},
			fmt.Errorf("begin worker transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	err = s.eventRepo.Append(ctx, tx, startedEvent)
	if err != nil {
		return deployment.State{},
			fmt.Errorf(
				"append deployment started event (deployment_id=%s): %w",
				state.ID, err)
	}

	err = s.deploymentRepo.UpdateStatus(ctx, tx,
		state.ID, string(deployment.StatusRunning))
	if err != nil {
		return deployment.State{},
			fmt.Errorf(
				"update deployment projection status (deployment_id=%s): %w",
				state.ID, err)
	}

	if err := tx.Commit(ctx); err != nil {
		return deployment.State{},
			fmt.Errorf("commit worker transaction: %w", err)
	}

	return updatedState, nil
}

func (s *Service) finishDeploymentSafely(
	ctx context.Context,
	state deployment.State,
	executionErr error,
) error {
	err := s.finishDeployment(ctx, state, executionErr)

	if !errors.Is(err, eventstore.ErrConcurrencyConflict) {
		return err
	}

	events, err := s.eventRepo.GetByDeploymentID(ctx, state.ID)
	if err != nil {
		return fmt.Errorf("reload deployment events: %w", err)
	}

	currentState, err := deployment.Rehydrate(events)
	if err != nil {
		return err
	}

	expectedStatus := deployment.StatusSucceeded
	if executionErr != nil {
		expectedStatus = deployment.StatusFailed
	}

	if currentState.Status == expectedStatus {
		return nil
	}

	return fmt.Errorf(
		"deployment %s has unexpected status %s after concurrency conflict",
		state.ID,
		currentState.Status,
	)
}

func (s *Service) finishDeployment(ctx context.Context,
	state deployment.State, executionErr error) error {
	eventType := deployment.DeploymentSucceeded
	payload := json.RawMessage(`{}`)

	if executionErr != nil {
		eventType = deployment.DeploymentFailed

		failedPayload, err := json.Marshal(deployment.DeploymentFailedPayload{
			Reason:  deploymentFailureReason(executionErr),
			Message: executionErr.Error(),
		})
		if err != nil {
			return fmt.Errorf("marshal deployment failed payload: %w", err)
		}

		payload = failedPayload
	}

	finishedEvent := eventstore.Event{
		DeploymentID:  state.ID,
		Type:          string(eventType),
		Version:       state.Version + 1,
		SchemaVersion: 1,
		Payload:       payload,
	}

	updatedState := state
	if err := updatedState.Apply(finishedEvent); err != nil {
		return err
	}

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin worker transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	err = s.eventRepo.Append(ctx, tx, finishedEvent)
	if err != nil {
		return fmt.Errorf("append deployment finished event (deployment_id=%s): %w",
			state.ID, err)
	}

	err = s.deploymentRepo.UpdateStatus(ctx, tx,
		updatedState.ID,
		string(updatedState.Status))
	if err != nil {
		return fmt.Errorf(
			"update deployment projection status (deployment_id=%s): %w",
			state.ID, err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit worker transaction: %w", err)
	}

	return nil
}

func deploymentFailureReason(err error) string {
	switch {
	case errors.Is(err, kubernetes.ErrDeploymentMismatch):
		return "DeploymentMismatch"
	case errors.Is(err, kubernetes.ErrSecretMismatch):
		return "SecretMismatch"
	}

	reason := apierrors.ReasonForError(err)
	if reason != metav1.StatusReasonUnknown {
		return string(reason)
	}

	return "DeploymentExecutionFailed"
}

func isPermanentDeploymentError(err error) bool {
	return errors.Is(err, kubernetes.ErrDeploymentMismatch) ||
		errors.Is(err, kubernetes.ErrSecretMismatch) ||
		apierrors.IsInvalid(err)
}
