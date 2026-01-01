package sanoid

import (
	"errors"
	"fmt"
	"os/exec"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/ykgmfq/SystemPub/models"
)

// Helper function to create a real ExitError with the specified exit code
func makeExitError(code int) *exec.ExitError {
	err := exec.Command("sh", "-c", fmt.Sprintf("exit %d", code)).Run()
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr
	}
	return nil
}

// Used to stub the return of the Run method
type MockCommandExecutor struct {
	err error
}

// Implements the commandExecutor interface
func (m *MockCommandExecutor) Run() error {
	return m.err
}

// Tests for clean sanoid exit indicating healthy pool
func TestGetPoolStateOK(t *testing.T) {
	prop := models.Health
	shellCommandFunc = func(_ string, _ ...string) commandExecutor {
		return &MockCommandExecutor{}
	}
	result, err := getPoolState(prop)
	assert.NoError(t, err, "Expected no error on clean exit")
	assert.True(t, result, "Expected pool state to be true on clean exit")
}

// Tests for known sanoid exit conditions indicating pool problems
func TestGetPoolStatePoolProblem(t *testing.T) {
	prop := models.Health
	// Exit codes 1-4 - Warning, Critical, Error
	for exitcode := 1; exitcode <= 4; exitcode++ {
		sanoidErr := makeExitError(exitcode)
		assert.NotNil(t, sanoidErr, "Failed to create exit error for code %d", exitcode)
		shellCommandFunc = func(_ string, _ ...string) commandExecutor {
			return &MockCommandExecutor{err: sanoidErr}
		}
		result, err := getPoolState(prop)
		assert.NoError(t, err, "Expected no error on exit codes 1-4")
		assert.False(t, result, "Expected pool state to be false on exit codes 1-4")
	}
}

// Tests for unexpected sanoid exit
func TestGetPoolStateSanoidProblem(t *testing.T) {
	prop := models.Health
	for _, testerr := range []error{makeExitError(255), makeExitError(5), exec.ErrNotFound} {
		shellCommandFunc = func(_ string, _ ...string) commandExecutor {
			return &MockCommandExecutor{err: testerr}
		}
		result, err := getPoolState(prop)
		assert.Equal(t, testerr, err, "Expected error to be escalated")
		assert.False(t, result, "Expected pool state to be false on sanoid problem")
	}
}
