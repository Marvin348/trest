package deployment

import (
	"fmt"
	"uuid"

	"example.com/trest/services/deployment-service/internal/eventstore"
)

type State struct {
	ID      uuid.UUID
	Status  Status
	Version int64
}

func Rehydrate(events []eventstore.Event) (State, error) {
	var state State

	if len(events) == 0 {
		return state, ErrNoEvents
	}

	for _, event := range events {
		if err := state.Apply(event); err != nil {
			return State{}, err
		}

	}

	return state, nil
}

func (s *State) Apply(event eventstore.Event) error {
	if event.Version != s.Version+1 {
		return fmt.Errorf(
			"invalid event version: expected %d, got %d",
			s.Version+1,
			event.Version,
		)
	}

	switch EventType(event.Type) {
	case DeploymentCreated:
		if s.Version != 0 {
			return fmt.Errorf(
				"deployment already created: %w", ErrInvalidTransition)
		}

		s.ID = event.DeploymentID
		s.Status = StatusQueued

	case DeploymentStarted:
		if s.Status != StatusQueued {
			return fmt.Errorf(
				"cannot start deployment from status %s: %w",
				s.Status, ErrInvalidTransition)
		}

		s.Status = StatusRunning

	case DeploymentSucceeded:
		if s.Status != StatusRunning {
			return fmt.Errorf(
				"cannot succeed deployment from status %s: %w",
				s.Status,
				ErrInvalidTransition,
			)
		}

		s.Status = StatusSucceeded

	case DeploymentFailed:
		if s.Status != StatusRunning {
			return fmt.Errorf(
				"cannot fail deployment from status %s: %w",
				s.Status, ErrInvalidTransition)
		}

		s.Status = StatusFailed

	default:
		return fmt.Errorf("unknown event: %s", event.Type)
	}

	s.Version = event.Version
	return nil
}
