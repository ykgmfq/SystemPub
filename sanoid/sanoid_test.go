package sanoid

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/ykgmfq/SystemPub/models"
)

// Used to stub the return of the Output method
type MockCommandExecutor struct {
	output string
}

// Implements the commandExecutor interface
func (m *MockCommandExecutor) Output() ([]byte, error) {
	return []byte(m.output), nil
}

// Tests sanoid output parsing for checking pool health
func TestGetPoolState(t *testing.T) {
	// OK
	shellCommandFunc = func(_ string, _ ...string) commandExecutor {
		return &MockCommandExecutor{output: "OK \n"}
	}
	prop := models.Health
	result := getPoolState(prop)
	assert.True(t, result, "Expected pool state to be true when 'OK' is returned")
	// FAILED
	shellCommandFunc = func(_ string, _ ...string) commandExecutor {
		return &MockCommandExecutor{output: "FAILED \n"}
	}
	prop = models.Health
	result = getPoolState(prop)
	assert.False(t, result, "Expected pool state to be false when 'FAILED' is returned")
}
