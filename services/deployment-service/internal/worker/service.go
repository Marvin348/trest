package worker

import (
	"context"
	"encoding/json"
	"fmt"

	"example.com/trest/services/deployment-service/internal/deployment"
	"example.com/trest/services/deployment-service/internal/eventstore"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Service struct {
	eventRepo      *eventstore.Repository
	deploymentRepo *deployment.Repository
	db             *pgxpool.Pool
}

func NewService(eventRepo *eventstore.Repository,
	deploymentRepo *deployment.Repository,
	db *pgxpool.Pool) *Service {
	return &Service{
		eventRepo:      eventRepo,
		deploymentRepo: deploymentRepo,
		db:             db,
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

		// gets fixed in the next branch
	case deployment.StatusRunning:
		return fmt.Errorf("deployment already running")

	case deployment.StatusSucceeded:
		return nil

	case deployment.StatusFailed:
		return nil

	default:
		return fmt.Errorf("unexpcted deployment status: %s", state.Status)
	}

	// Kubernetes

	if err := s.finishDeployment(ctx, state, err); err != nil {
		return err
	}

	return nil
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

func (s *Service) finishDeployment(ctx context.Context,
	state deployment.State, executionErr error) error {
	eventType := deployment.DeploymentSucceeded

	if executionErr != nil {
		eventType = deployment.DeploymentFailed
	}

	finishedEvent := eventstore.Event{
		DeploymentID:  state.ID,
		Type:          string(eventType),
		Version:       state.Version + 1,
		SchemaVersion: 1,
		Payload:       json.RawMessage(`{}`),
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
