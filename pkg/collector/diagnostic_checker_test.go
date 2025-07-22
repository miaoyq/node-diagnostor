package collector

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCheckKernelParameters(t *testing.T) {
	result := checkKernelParameters()
	assert.NotEmpty(t, result.Name)
	assert.Contains(t, []string{"OK", "WARN", "ERROR"}, result.Status)
}

func TestCheckDStateProcesses(t *testing.T) {
	result := checkDStateProcesses()
	assert.NotEmpty(t, result.Name)
	assert.Contains(t, []string{"OK", "WARN", "ERROR"}, result.Status)
}

func TestCheckFileDescriptors(t *testing.T) {
	result := checkFileDescriptors()
	assert.NotEmpty(t, result.Name)
	assert.Contains(t, []string{"OK", "WARN", "ERROR"}, result.Status)
}

func TestNewDiagnosticChecker(t *testing.T) {
	checker := NewDiagnosticChecker()
	assert.NotNil(t, checker)
	assert.NotEmpty(t, checker.checks)
}

func TestDiagnosticChecker_RunChecks(t *testing.T) {
	// 测试运行所有检查
	checker := NewDiagnosticChecker()
	results := checker.RunChecks()

	assert.NotEmpty(t, results)
	for _, result := range results {
		assert.NotEmpty(t, result.Name)
		assert.Contains(t, []string{"OK", "WARN", "ERROR"}, result.Status)
		assert.NotEmpty(t, result.Message)
		assert.NotNil(t, result.Details)
	}
}
