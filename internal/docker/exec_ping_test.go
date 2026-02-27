package docker

import (
	"context"
	"errors"
	"testing"
)

// TestExecPing_MockSuccess verifies the mock-based happy path where
// ExecPing returns nil (ping succeeded).
func TestExecPing_MockSuccess(t *testing.T) {
	mock := newTestMock()

	err := mock.ExecPing(context.Background(), "container-abc", "10.0.0.1")
	if err != nil {
		t.Fatalf("expected nil error, got: %v", err)
	}
}

// TestExecPing_MockFailure verifies that the testMockAPI can simulate
// a ping failure when configured with an error for a specific container.
func TestExecPing_MockFailure(t *testing.T) {
	mock := newTestMockWithExecPing()
	mock.execPingErr["container-abc"] = errors.New("ping exited with code 1")

	err := mock.ExecPing(context.Background(), "container-abc", "10.0.0.1")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if err.Error() != "ping exited with code 1" {
		t.Errorf("expected 'ping exited with code 1', got: %v", err)
	}
}

// TestExecPing_MockDifferentContainers verifies that errors are keyed
// per container — one failing doesn't affect another.
func TestExecPing_MockDifferentContainers(t *testing.T) {
	mock := newTestMockWithExecPing()
	mock.execPingErr["broken-container"] = errors.New("ping exited with code 1")

	// The broken one should fail.
	if err := mock.ExecPing(context.Background(), "broken-container", "10.0.0.1"); err == nil {
		t.Error("expected error for broken-container, got nil")
	}

	// A different container should succeed.
	if err := mock.ExecPing(context.Background(), "healthy-container", "10.0.0.1"); err != nil {
		t.Errorf("expected nil for healthy-container, got: %v", err)
	}
}

// testMockWithExecPing extends testMockAPI with ExecPing error tracking.
type testMockWithExecPing struct {
	testMockAPI
	execPingErr map[string]error
}

func newTestMockWithExecPing() *testMockWithExecPing {
	return &testMockWithExecPing{
		testMockAPI: *newTestMock(),
		execPingErr: make(map[string]error),
	}
}

func (m *testMockWithExecPing) ExecPing(_ context.Context, containerID string, _ string) error {
	if err, ok := m.execPingErr[containerID]; ok {
		return err
	}
	return nil
}

// Verify testMockWithExecPing satisfies the API interface.
var _ API = (*testMockWithExecPing)(nil)
