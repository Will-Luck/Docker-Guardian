package docker

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/events"
)

// testMockAPI implements API for testing DependentsOf logic.
type testMockAPI struct {
	running    []container.Summary
	runningErr error

	inspects   map[string]container.InspectResponse
	inspectErr map[string]error
}

func newTestMock() *testMockAPI {
	return &testMockAPI{
		inspects:   make(map[string]container.InspectResponse),
		inspectErr: make(map[string]error),
	}
}

func (m *testMockAPI) UnhealthyContainers(_ context.Context, _ string, _ bool) ([]container.Summary, error) {
	return nil, nil
}
func (m *testMockAPI) ExitedContainers(_ context.Context) ([]container.Summary, error) {
	return nil, nil
}
func (m *testMockAPI) RunningContainers(_ context.Context) ([]container.Summary, error) {
	return m.running, m.runningErr
}
func (m *testMockAPI) InspectContainer(_ context.Context, id string) (container.InspectResponse, error) {
	if err, ok := m.inspectErr[id]; ok && err != nil {
		return container.InspectResponse{}, err
	}
	if resp, ok := m.inspects[id]; ok {
		return resp, nil
	}
	return container.InspectResponse{}, errors.New("not found: " + id)
}
func (m *testMockAPI) RestartContainer(_ context.Context, _ string, _ int) error { return nil }
func (m *testMockAPI) StartContainer(_ context.Context, _ string) error          { return nil }
func (m *testMockAPI) StopContainer(_ context.Context, _ string, _ int) error    { return nil }
func (m *testMockAPI) ContainerStatus(_ context.Context, _ string) (string, error) {
	return "", nil
}
func (m *testMockAPI) ContainerFinishedAt(_ context.Context, _ string) (time.Time, error) {
	return time.Time{}, nil
}
func (m *testMockAPI) ContainerHealthLog(_ context.Context, _ string) (string, error) {
	return "", nil
}
func (m *testMockAPI) ContainerEvents(_ context.Context, _, _ time.Time, _ bool) ([]events.Message, error) {
	return nil, nil
}
func (m *testMockAPI) DependentsOf(_ context.Context, _ string) ([]container.Summary, error) {
	return nil, nil
}
func (m *testMockAPI) Close() error { return nil }

// Verify testMockAPI satisfies the API interface.
var _ API = (*testMockAPI)(nil)

func TestDependentsOf_WithDependents(t *testing.T) {
	parentID := "vpn-parent-abc123"
	parentName := "wireguard-pia"

	mock := newTestMock()
	mock.running = []container.Summary{
		{ID: parentID},
		{ID: "dep-sonarr"},
		{ID: "dep-radarr"},
		{ID: "standalone-nginx"},
	}
	mock.inspects[parentID] = container.InspectResponse{
		Name: "/" + parentName,
		HostConfig: &container.HostConfig{
			NetworkMode: "bridge",
		},
	}
	mock.inspects["dep-sonarr"] = container.InspectResponse{
		Name: "/sonarr",
		HostConfig: &container.HostConfig{
			NetworkMode: container.NetworkMode("container:" + parentID),
		},
	}
	mock.inspects["dep-radarr"] = container.InspectResponse{
		Name: "/radarr",
		HostConfig: &container.HostConfig{
			NetworkMode: container.NetworkMode("container:" + parentName),
		},
	}
	mock.inspects["standalone-nginx"] = container.InspectResponse{
		Name: "/nginx",
		HostConfig: &container.HostConfig{
			NetworkMode: "bridge",
		},
	}

	deps, err := dependentsOf(context.Background(), mock, parentID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(deps) != 2 {
		t.Fatalf("expected 2 dependents, got %d", len(deps))
	}

	ids := map[string]bool{}
	for _, d := range deps {
		ids[d.ID] = true
	}
	if !ids["dep-sonarr"] {
		t.Error("expected dep-sonarr in dependents")
	}
	if !ids["dep-radarr"] {
		t.Error("expected dep-radarr in dependents")
	}
}

func TestDependentsOf_NoDependents(t *testing.T) {
	parentID := "vpn-parent-abc123"

	mock := newTestMock()
	mock.running = []container.Summary{
		{ID: parentID},
		{ID: "standalone-nginx"},
	}
	mock.inspects[parentID] = container.InspectResponse{
		Name: "/wireguard-pia",
		HostConfig: &container.HostConfig{
			NetworkMode: "bridge",
		},
	}
	mock.inspects["standalone-nginx"] = container.InspectResponse{
		Name: "/nginx",
		HostConfig: &container.HostConfig{
			NetworkMode: "bridge",
		},
	}

	deps, err := dependentsOf(context.Background(), mock, parentID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(deps) != 0 {
		t.Errorf("expected 0 dependents, got %d", len(deps))
	}
}

func TestDependentsOf_RunningContainersError(t *testing.T) {
	mock := newTestMock()
	mock.runningErr = errors.New("docker daemon unavailable")

	_, err := dependentsOf(context.Background(), mock, "some-parent")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "listing running containers") {
		t.Errorf("expected wrapped error, got: %v", err)
	}
}

func TestDependentsOf_ParentInspectError(t *testing.T) {
	parentID := "nonexistent-parent"

	mock := newTestMock()
	mock.running = []container.Summary{
		{ID: parentID},
	}
	mock.inspectErr[parentID] = errors.New("container not found")

	_, err := dependentsOf(context.Background(), mock, parentID)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "inspecting parent") {
		t.Errorf("expected parent inspect error, got: %v", err)
	}
}

func TestDependentsOf_InspectErrorSkipsContainer(t *testing.T) {
	parentID := "vpn-parent-abc123"

	mock := newTestMock()
	mock.running = []container.Summary{
		{ID: parentID},
		{ID: "dep-sonarr"},
		{ID: "vanished-container"},
	}
	mock.inspects[parentID] = container.InspectResponse{
		Name: "/wireguard-pia",
		HostConfig: &container.HostConfig{
			NetworkMode: "bridge",
		},
	}
	mock.inspects["dep-sonarr"] = container.InspectResponse{
		Name: "/sonarr",
		HostConfig: &container.HostConfig{
			NetworkMode: container.NetworkMode("container:" + parentID),
		},
	}
	mock.inspectErr["vanished-container"] = errors.New("no such container")

	deps, err := dependentsOf(context.Background(), mock, parentID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(deps) != 1 {
		t.Fatalf("expected 1 dependent (skipping vanished), got %d", len(deps))
	}
	if deps[0].ID != "dep-sonarr" {
		t.Errorf("expected dep-sonarr, got %s", deps[0].ID)
	}
}

func TestDependentsOf_MatchesByNameNotJustID(t *testing.T) {
	parentID := "vpn-parent-abc123"
	parentName := "wireguard-pia"

	mock := newTestMock()
	mock.running = []container.Summary{
		{ID: parentID},
		{ID: "dep-by-name"},
	}
	mock.inspects[parentID] = container.InspectResponse{
		Name: "/" + parentName,
		HostConfig: &container.HostConfig{
			NetworkMode: "bridge",
		},
	}
	mock.inspects["dep-by-name"] = container.InspectResponse{
		Name: "/prowlarr",
		HostConfig: &container.HostConfig{
			NetworkMode: container.NetworkMode("container:" + parentName),
		},
	}

	deps, err := dependentsOf(context.Background(), mock, parentID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(deps) != 1 {
		t.Fatalf("expected 1 dependent, got %d", len(deps))
	}
	if deps[0].ID != "dep-by-name" {
		t.Errorf("expected dep-by-name, got %s", deps[0].ID)
	}
}
