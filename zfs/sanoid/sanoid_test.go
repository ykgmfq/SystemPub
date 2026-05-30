package sanoid

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/ykgmfq/SystemPub/models"
)

func makeExitError(code int) *exec.ExitError {
	err := exec.Command("sh", "-c", fmt.Sprintf("exit %d", code)).Run()
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr
	}
	return nil
}

func makeKilledError(t *testing.T) *exec.ExitError {
	t.Helper()
	cmd := exec.Command("sleep", "10")
	require.NoError(t, cmd.Start())
	cmd.Process.Kill()
	err := cmd.Wait()
	var exitErr *exec.ExitError
	require.ErrorAs(t, err, &exitErr)
	return exitErr
}

type MockCommandExecutor struct {
	err    error
	output []byte
}

func (m *MockCommandExecutor) Output() ([]byte, error) {
	return m.output, m.err
}

func TestGetPoolStateOK(t *testing.T) {
	mockRun := func(_ context.Context, _ string, _ ...string) commandExecutor { return &MockCommandExecutor{} }
	result, _, _, err := getPoolState(context.Background(), mockRun, models.Health)
	assert.NoError(t, err, "Expected no error on clean exit")
	assert.True(t, result, "Expected pool state to be true on clean exit")
}

func TestGetPoolStatePoolProblem(t *testing.T) {
	for exitcode := 1; exitcode <= 4; exitcode++ {
		sanoidErr := makeExitError(exitcode)
		assert.NotNil(t, sanoidErr, "Failed to create exit error for code %d", exitcode)
		mockRun := func(_ context.Context, _ string, _ ...string) commandExecutor { return &MockCommandExecutor{err: sanoidErr} }
		result, _, _, err := getPoolState(context.Background(), mockRun, models.Health)
		assert.NoError(t, err, "Expected no error on exit codes 1-4")
		assert.False(t, result, "Expected pool state to be false on exit codes 1-4")
	}
}

func TestGetPoolStateSanoidProblem(t *testing.T) {
	for _, testerr := range []error{makeExitError(255), makeExitError(5), exec.ErrNotFound} {
		mockRun := func(_ context.Context, _ string, _ ...string) commandExecutor { return &MockCommandExecutor{err: testerr} }
		result, _, _, err := getPoolState(context.Background(), mockRun, models.Health)
		assert.Equal(t, testerr, err, "Expected error to be escalated")
		assert.False(t, result, "Expected pool state to be false on sanoid problem")
	}
}

func TestGetPoolStateKilled(t *testing.T) {
	killedErr := makeKilledError(t)
	assert.Equal(t, -1, killedErr.ExitCode())
	mockRun := func(_ context.Context, _ string, _ ...string) commandExecutor { return &MockCommandExecutor{err: killedErr} }
	result, _, _, err := getPoolState(context.Background(), mockRun, models.Health)
	assert.Equal(t, killedErr, err, "Expected killed-process error to be escalated")
	assert.False(t, result)
}
