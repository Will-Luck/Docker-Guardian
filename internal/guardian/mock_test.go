package guardian

import (
	"context"
	"sync"
	"time"

	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/events"
)

// mockDocker implements docker.API for testing.
type mockDocker struct {
	mu sync.Mutex

	unhealthyContainers []container.Summary
	unhealthyErr        error

	exitedContainers []container.Summary
	exitedErr        error

	runningContainers []container.Summary
	runningErr        error

	inspectResults map[string]container.InspectResponse
	inspectErr     map[string]error

	restartCalls []string
	restartErr   map[string]error

	startCalls []string
	startErr   map[string]error

	stopCalls []string
	stopErr   map[string]error

	statusResults map[string]string
	statusErr     map[string]error

	finishedAtResults map[string]time.Time
	finishedAtErr     map[string]error

	containerEvents    []events.Message
	containerEventsErr error
}

func newMockDocker() *mockDocker {
	return &mockDocker{
		inspectResults:    make(map[string]container.InspectResponse),
		inspectErr:        make(map[string]error),
		restartErr:        make(map[string]error),
		startErr:          make(map[string]error),
		stopErr:           make(map[string]error),
		statusResults:     make(map[string]string),
		statusErr:         make(map[string]error),
		finishedAtResults: make(map[string]time.Time),
		finishedAtErr:     make(map[string]error),
	}
}

func (m *mockDocker) UnhealthyContainers(_ context.Context, _ string, _ bool) ([]container.Summary, error) {
	return m.unhealthyContainers, m.unhealthyErr
}

func (m *mockDocker) ExitedContainers(_ context.Context) ([]container.Summary, error) {
	return m.exitedContainers, m.exitedErr
}

func (m *mockDocker) RunningContainers(_ context.Context) ([]container.Summary, error) {
	return m.runningContainers, m.runningErr
}

func (m *mockDocker) InspectContainer(_ context.Context, id string) (container.InspectResponse, error) {
	if err, ok := m.inspectErr[id]; ok && err != nil {
		return container.InspectResponse{}, err
	}
	return m.inspectResults[id], nil
}

func (m *mockDocker) RestartContainer(_ context.Context, id string, _ int) error {
	m.mu.Lock()
	m.restartCalls = append(m.restartCalls, id)
	m.mu.Unlock()
	if err, ok := m.restartErr[id]; ok {
		return err
	}
	return nil
}

func (m *mockDocker) StartContainer(_ context.Context, id string) error {
	m.mu.Lock()
	m.startCalls = append(m.startCalls, id)
	m.mu.Unlock()
	if err, ok := m.startErr[id]; ok {
		return err
	}
	return nil
}

func (m *mockDocker) StopContainer(_ context.Context, id string, _ int) error {
	m.mu.Lock()
	m.stopCalls = append(m.stopCalls, id)
	m.mu.Unlock()
	if err, ok := m.stopErr[id]; ok {
		return err
	}
	return nil
}

func (m *mockDocker) ContainerStatus(_ context.Context, id string) (string, error) {
	if err, ok := m.statusErr[id]; ok && err != nil {
		return "", err
	}
	return m.statusResults[id], nil
}

func (m *mockDocker) ContainerFinishedAt(_ context.Context, id string) (time.Time, error) {
	if err, ok := m.finishedAtErr[id]; ok && err != nil {
		return time.Time{}, err
	}
	return m.finishedAtResults[id], nil
}

func (m *mockDocker) ContainerEvents(_ context.Context, _, _ time.Time, _ bool) ([]events.Message, error) {
	return m.containerEvents, m.containerEventsErr
}

func (m *mockDocker) Close() error { return nil }

// mockNotifier implements notify.Notifier for testing.
type mockNotifier struct {
	mu       sync.Mutex
	startups []string
	actions  []string
	skips    []string
	closed   bool
}

func (m *mockNotifier) Startup(text string) {
	m.mu.Lock()
	m.startups = append(m.startups, text)
	m.mu.Unlock()
}

func (m *mockNotifier) Action(text string) {
	m.mu.Lock()
	m.actions = append(m.actions, text)
	m.mu.Unlock()
}

func (m *mockNotifier) Skip(text string) {
	m.mu.Lock()
	m.skips = append(m.skips, text)
	m.mu.Unlock()
}

func (m *mockNotifier) Close() {
	m.mu.Lock()
	m.closed = true
	m.mu.Unlock()
}

// mockClock implements clock.Clock for testing.
type mockClock struct {
	now time.Time
}

func newMockClock(t time.Time) *mockClock {
	return &mockClock{now: t}
}

func (c *mockClock) Now() time.Time { return c.now }
func (c *mockClock) After(d time.Duration) <-chan time.Time {
	ch := make(chan time.Time, 1)
	ch <- c.now.Add(d)
	return ch
}
func (c *mockClock) Since(t time.Time) time.Duration { return c.now.Sub(t) }
func (c *mockClock) Advance(d time.Duration)         { c.now = c.now.Add(d) }
