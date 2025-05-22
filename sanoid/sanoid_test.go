package sanoid

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/ykgmfq/SystemPub/models"
)

// Used to stub the return of the Output method
type MockCommandExecutor struct {
	output string
	error  error
}

// Implements the commandExecutor interface
func (m *MockCommandExecutor) Output() ([]byte, error) {
	return []byte(m.output), m.error
}

// Tests sanoid output parsing for checking pool health
func TestGetPoolState(t *testing.T) {
	// OK
	shellCommandFunc = func(_ string, _ ...string) commandExecutor {
		return &MockCommandExecutor{output: "OK \n"}
	}
	prop := models.Health
	result, err := getPoolState(prop)
	assert.NoError(t, err, "Expected no error when 'OK' is returned")
	assert.True(t, result, "Expected pool state to be true when 'OK' is returned")
	// FAILED
	shellCommandFunc = func(_ string, _ ...string) commandExecutor {
		return &MockCommandExecutor{output: "FAILED \n"}
	}
	result, err = getPoolState(prop)
	assert.NoError(t, err, "Expected no error when 'FAILED' is returned")
	assert.False(t, result, "Expected pool state to be false when 'FAILED' is returned")
	// ERROR
	shellCommandFunc = func(_ string, _ ...string) commandExecutor {
		return &MockCommandExecutor{error: fmt.Errorf("error")}
	}
	_, err = getPoolState(prop)
	assert.Error(t, err, "Expected an error when 'error' is returned")
}
