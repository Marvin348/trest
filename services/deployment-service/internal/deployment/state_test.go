package deployment

import (
	"errors"
	"testing"

	"example.com/trest/services/deployment-service/internal/eventstore"
	"github.com/stretchr/testify/require"

	"uuid"
)

func TestStateApplyValidTransitions(t *testing.T) {
	deploymentID := uuid.New()

	testCases := []struct {
		name     string
		initial  State
		event    eventstore.Event
		expected State
	}{
		{
			name:    "create deployment",
			initial: State{},
			event: stateTestEvent(
				deploymentID,
				DeploymentCreated,
				1,
			),
			expected: State{
				ID:      deploymentID,
				Status:  StatusQueued,
				Version: 1,
			},
		},
		{
			name: "start queued deployment",
			initial: State{
				ID:      deploymentID,
				Status:  StatusQueued,
				Version: 1,
			},
			event: stateTestEvent(
				deploymentID,
				DeploymentStarted,
				2,
			),
			expected: State{
				ID:      deploymentID,
				Status:  StatusRunning,
				Version: 2,
			},
		},
		{
			name: "succeed running deployment",
			initial: State{
				ID:      deploymentID,
				Status:  StatusRunning,
				Version: 2,
			},
			event: stateTestEvent(
				deploymentID,
				DeploymentSucceeded,
				3,
			),
			expected: State{
				ID:      deploymentID,
				Status:  StatusSucceeded,
				Version: 3,
			},
		},
		{
			name: "fail running deployment",
			initial: State{
				ID:      deploymentID,
				Status:  StatusRunning,
				Version: 2,
			},
			event: stateTestEvent(
				deploymentID,
				DeploymentFailed,
				3,
			),
			expected: State{
				ID:      deploymentID,
				Status:  StatusFailed,
				Version: 3,
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			state := testCase.initial

			err := state.Apply(testCase.event)

			require.NoError(t, err)
			require.Equal(t, testCase.expected, state)
		})
	}
}

func TestStateApplyRejectsInvalidVersions(t *testing.T) {
	deploymentID := uuid.New()

	testCases := []struct {
		name        string
		initial     State
		event       eventstore.Event
		errContains string
	}{
		{
			name:    "zero event version",
			initial: State{},
			event: stateTestEvent(
				deploymentID,
				DeploymentCreated,
				0,
			),
			errContains: "invalid event version: expected 1, got 0",
		},
		{
			name:    "first event skips version one",
			initial: State{},
			event: stateTestEvent(
				deploymentID,
				DeploymentCreated,
				2,
			),
			errContains: "invalid event version: expected 1, got 2",
		},
		{
			name: "duplicate version",
			initial: State{
				ID:      deploymentID,
				Status:  StatusQueued,
				Version: 1,
			},
			event: stateTestEvent(
				deploymentID,
				DeploymentStarted,
				1,
			),
			errContains: "invalid event version: expected 2, got 1",
		},
		{
			name: "version gap",
			initial: State{
				ID:      deploymentID,
				Status:  StatusQueued,
				Version: 1,
			},
			event: stateTestEvent(
				deploymentID,
				DeploymentStarted,
				3,
			),
			errContains: "invalid event version: expected 2, got 3",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			state := testCase.initial

			err := state.Apply(testCase.event)

			require.ErrorContains(t, err, testCase.errContains)
			require.Equal(t, testCase.initial, state)
		})
	}
}

func TestStateApplyRejectsInvalidTransitions(t *testing.T) {
	deploymentID := uuid.New()

	testCases := []struct {
		name    string
		initial State
		event   eventstore.Event
	}{
		{
			name: "create deployment twice",
			initial: State{
				ID:      deploymentID,
				Status:  StatusQueued,
				Version: 1,
			},
			event: stateTestEvent(deploymentID, DeploymentCreated, 2),
		},
		{
			name:    "start deployment before creation",
			initial: State{},
			event:   stateTestEvent(deploymentID, DeploymentStarted, 1),
		},
		{
			name: "start running deployment",
			initial: State{
				ID:      deploymentID,
				Status:  StatusRunning,
				Version: 2,
			},
			event: stateTestEvent(deploymentID, DeploymentStarted, 3),
		},
		{
			name: "start succeeded deployment",
			initial: State{
				ID:      deploymentID,
				Status:  StatusSucceeded,
				Version: 3,
			},
			event: stateTestEvent(deploymentID, DeploymentStarted, 4),
		},
		{
			name: "start failed deployment",
			initial: State{
				ID:      deploymentID,
				Status:  StatusFailed,
				Version: 3,
			},
			event: stateTestEvent(deploymentID, DeploymentStarted, 4),
		},
		{
			name:    "succeed deployment before creation",
			initial: State{},
			event:   stateTestEvent(deploymentID, DeploymentSucceeded, 1),
		},
		{
			name: "succeed queued deployment",
			initial: State{
				ID:      deploymentID,
				Status:  StatusQueued,
				Version: 1,
			},
			event: stateTestEvent(deploymentID, DeploymentSucceeded, 2),
		},
		{
			name: "succeed succeeded deployment",
			initial: State{
				ID:      deploymentID,
				Status:  StatusSucceeded,
				Version: 3,
			},
			event: stateTestEvent(deploymentID, DeploymentSucceeded, 4),
		},
		{
			name: "succeed failed deployment",
			initial: State{
				ID:      deploymentID,
				Status:  StatusFailed,
				Version: 3,
			},
			event: stateTestEvent(deploymentID, DeploymentSucceeded, 4),
		},
		{
			name:    "fail deployment before creation",
			initial: State{},
			event:   stateTestEvent(deploymentID, DeploymentFailed, 1),
		},
		{
			name: "fail queued deployment",
			initial: State{
				ID:      deploymentID,
				Status:  StatusQueued,
				Version: 1,
			},
			event: stateTestEvent(deploymentID, DeploymentFailed, 2),
		},
		{
			name: "fail succeeded deployment",
			initial: State{
				ID:      deploymentID,
				Status:  StatusSucceeded,
				Version: 3,
			},
			event: stateTestEvent(deploymentID, DeploymentFailed, 4),
		},
		{
			name: "fail failed deployment",
			initial: State{
				ID:      deploymentID,
				Status:  StatusFailed,
				Version: 3,
			},
			event: stateTestEvent(deploymentID, DeploymentFailed, 4),
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			state := testCase.initial

			err := state.Apply(testCase.event)

			require.Error(t, err)
			require.ErrorIs(t, err, ErrInvalidTransition)
			require.Equal(t, testCase.initial, state)
		})
	}
}

func TestStateApplyRejectsUnknownEvent(t *testing.T) {
	initial := State{}
	state := initial

	err := state.Apply(eventstore.Event{
		DeploymentID: uuid.New(),
		Type:         "DeploymentPaused",
		Version:      1,
	})

	require.ErrorContains(t, err, "unknown event: DeploymentPaused")
	require.Equal(t, initial, state)
}

func TestRehydrateValidEventStreams(t *testing.T) {
	deploymentID := uuid.New()
	created := stateTestEvent(deploymentID, DeploymentCreated, 1)
	started := stateTestEvent(deploymentID, DeploymentStarted, 2)

	testCases := []struct {
		name     string
		events   []eventstore.Event
		expected State
	}{
		{
			name:   "queued deployment",
			events: []eventstore.Event{created},
			expected: State{
				ID:      deploymentID,
				Status:  StatusQueued,
				Version: 1,
			},
		},
		{
			name:   "running deployment",
			events: []eventstore.Event{created, started},
			expected: State{
				ID:      deploymentID,
				Status:  StatusRunning,
				Version: 2,
			},
		},
		{
			name: "succeeded deployment",
			events: []eventstore.Event{
				created,
				started,
				stateTestEvent(deploymentID, DeploymentSucceeded, 3),
			},
			expected: State{
				ID:      deploymentID,
				Status:  StatusSucceeded,
				Version: 3,
			},
		},
		{
			name: "failed deployment",
			events: []eventstore.Event{
				created,
				started,
				stateTestEvent(deploymentID, DeploymentFailed, 3),
			},
			expected: State{
				ID:      deploymentID,
				Status:  StatusFailed,
				Version: 3,
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			state, err := Rehydrate(testCase.events)

			require.NoError(t, err)
			require.Equal(t, testCase.expected, state)
		})
	}
}

func TestRehydrateRejectsEmptyEventStream(t *testing.T) {
	testCases := []struct {
		name   string
		events []eventstore.Event
	}{
		{name: "nil events", events: nil},
		{name: "empty events", events: []eventstore.Event{}},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			state, err := Rehydrate(testCase.events)

			require.ErrorIs(t, err, ErrNoEvents)
			require.Equal(t, State{}, state)
		})
	}
}

func TestRehydrateRejectsInvalidEventStreams(t *testing.T) {
	deploymentID := uuid.New()
	created := stateTestEvent(deploymentID, DeploymentCreated, 1)

	testCases := []struct {
		name        string
		events      []eventstore.Event
		targetError error
		errContains string
	}{
		{
			name: "stream starts without creation",
			events: []eventstore.Event{
				stateTestEvent(deploymentID, DeploymentStarted, 1),
			},
			targetError: ErrInvalidTransition,
		},
		{
			name: "duplicate creation",
			events: []eventstore.Event{
				created,
				stateTestEvent(deploymentID, DeploymentCreated, 2),
			},
			targetError: ErrInvalidTransition,
		},
		{
			name: "duplicate version",
			events: []eventstore.Event{
				created,
				stateTestEvent(deploymentID, DeploymentStarted, 1),
			},
			errContains: "invalid event version: expected 2, got 1",
		},
		{
			name: "version gap",
			events: []eventstore.Event{
				created,
				stateTestEvent(deploymentID, DeploymentStarted, 3),
			},
			errContains: "invalid event version: expected 2, got 3",
		},
		{
			name: "invalid transition after valid events",
			events: []eventstore.Event{
				created,
				stateTestEvent(deploymentID, DeploymentSucceeded, 2),
			},
			targetError: ErrInvalidTransition,
		},
		{
			name: "unknown event after valid event",
			events: []eventstore.Event{
				created,
				{
					DeploymentID: deploymentID,
					Type:         "DeploymentPaused",
					Version:      2,
				},
			},
			errContains: "unknown event: DeploymentPaused",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			state, err := Rehydrate(testCase.events)

			require.Error(t, err)
			if testCase.targetError != nil {
				require.True(t, errors.Is(err, testCase.targetError))
			}
			if testCase.errContains != "" {
				require.ErrorContains(t, err, testCase.errContains)
			}
			require.Equal(t, State{}, state)
		})
	}
}

func stateTestEvent(
	deploymentID uuid.UUID,
	eventType EventType,
	version int64,
) eventstore.Event {
	return eventstore.Event{
		DeploymentID:  deploymentID,
		Type:          string(eventType),
		Version:       version,
		SchemaVersion: 1,
	}
}
