package sanoid

import (
	"errors"
	"fmt"
	"os/exec"
	"testing"

	"github.com/stretchr/testify/assert"
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

type MockCommandExecutor struct {
	err error
}

func (m *MockCommandExecutor) Run() error {
	return m.err
}

func TestGetPoolStateOK(t *testing.T) {
	mockRun := func(_ string, _ ...string) commandExecutor { return &MockCommandExecutor{} }
	result, _, err := getPoolState(mockRun, models.Health)
	assert.NoError(t, err, "Expected no error on clean exit")
	assert.True(t, result, "Expected pool state to be true on clean exit")
}

func TestGetPoolStatePoolProblem(t *testing.T) {
	for exitcode := 1; exitcode <= 4; exitcode++ {
		sanoidErr := makeExitError(exitcode)
		assert.NotNil(t, sanoidErr, "Failed to create exit error for code %d", exitcode)
		mockRun := func(_ string, _ ...string) commandExecutor { return &MockCommandExecutor{err: sanoidErr} }
		result, _, err := getPoolState(mockRun, models.Health)
		assert.NoError(t, err, "Expected no error on exit codes 1-4")
		assert.False(t, result, "Expected pool state to be false on exit codes 1-4")
	}
}

func TestGetPoolStateSanoidProblem(t *testing.T) {
	for _, testerr := range []error{makeExitError(255), makeExitError(5), exec.ErrNotFound} {
		mockRun := func(_ string, _ ...string) commandExecutor { return &MockCommandExecutor{err: testerr} }
		result, _, err := getPoolState(mockRun, models.Health)
		assert.Equal(t, testerr, err, "Expected error to be escalated")
		assert.False(t, result, "Expected pool state to be false on sanoid problem")
	}
}
