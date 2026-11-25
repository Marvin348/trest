package worker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"time"

	"example.com/trest/services/deployment-service/internal/deployment"
	"example.com/trest/services/deployment-service/internal/eventstore"
)

func (s *Service) Run(ctx context.Context, interval time.Duration) error {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := s.RecoverRunningDeployments(ctx); err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			log.Printf("deployment recovery: %v", err)
		}

		select {
		case <-ctx.Done():
			return ctx.Err()

		case <-ticker.C:
		}
	}
}

func (s *Service) RecoverRunningDeployments(ctx context.Context) error {
	runningIDs, err := s.deploymentRepo.ListRunningIDs(ctx)
	if err != nil {
		return fmt.Errorf("list running deployments ids: %w", err)
	}

	eventsByID, err := s.eventRepo.GetByDeploymentIDs(ctx, runningIDs)
	if err != nil {
		return fmt.Errorf("load events for running deployments: %w", err)
	}

	var recoveryErrors []error

	for _, id := range runningIDs {
		events := eventsByID[id]

		state, err := deployment.Rehydrate(events)
		if err != nil {
			recoveryErrors = append(
				recoveryErrors,
				err,
			)
		}

		if state.Status != deployment.StatusRunning {
			continue
		}

		requestedMessage, err := requestedMessageFromEvents(events)
		if err != nil {
			recoveryErrors = append(
				recoveryErrors,
				fmt.Errorf("reconstruct deployment %s: %w", id, err),
			)
			continue
		}

		err = s.recoverDeployment(ctx, state, requestedMessage)
		if err != nil {
			recoveryErrors = append(
				recoveryErrors,
				fmt.Errorf("recover deployment %s: %w", id, err),
			)
			continue
		}

	}

	return errors.Join(recoveryErrors...)
}

func (s *Service) recoverDeployment(
	ctx context.Context,
	state deployment.State,
	message deployment.DeploymentRequestedMessage,
) error {
	executionErr := s.executor.Deploy(ctx, message)

	if executionErr == nil {
		return s.finishDeploymentSafely(ctx, state, nil)
	}

	if isPermanentDeploymentError(executionErr) {
		return s.finishDeploymentSafely(ctx, state, executionErr)
	}

	return fmt.Errorf("execute kubernetes deployment: %w", executionErr)
}

func requestedMessageFromEvents(
	events []eventstore.Event,
) (deployment.DeploymentRequestedMessage, error) {

	for _, event := range events {
		if event.Type != string(deployment.DeploymentCreated) {
			continue
		}

		var payload deployment.DeploymentCreatedPayload

		if err := json.Unmarshal(event.Payload, &payload); err != nil {
			return deployment.DeploymentRequestedMessage{},
				fmt.Errorf("decode deployment created payload: %w", err)
		}

		return deployment.DeploymentRequestedMessage{
			DeploymentID:  event.DeploymentID,
			ProjectID:     payload.ProjectID,
			ApplicationID: payload.ApplicationID,
			Image:         payload.Image,
			Replicas:      payload.Replicas,
			ContainerPort: payload.ContainerPort,

			CPURequestM:     payload.CPURequestM,
			CPULimitM:       payload.CPULimitM,
			MemoryRequestMi: payload.MemoryRequestMi,
			MemoryLimitMi:   payload.MemoryLimitMi,

			Environment: payload.Environment,
		}, nil
	}

	return deployment.DeploymentRequestedMessage{},
		fmt.Errorf("deployment created event not found")
}