//go:build integration

package main

import (
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

// TestJavaScriptSearchFunctions runs the JavaScript tests for log search functionality.
// This test requires Node.js to be installed.
func TestJavaScriptSearchFunctions(t *testing.T) {
	// Check if node is available
	nodePath, err := exec.LookPath("node")
	if err != nil {
		t.Skip("Node.js not found, skipping JavaScript tests")
	}

	// Get the directory of this test file
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("Could not determine test file location")
	}
	projectDir := filepath.Dir(filename)
	testFile := filepath.Join(projectDir, "frontend", "run-tests.mjs")

	// Run the JavaScript tests
	cmd := exec.Command(nodePath, testFile)
	cmd.Dir = projectDir

	output, err := cmd.CombinedOutput()
	t.Logf("JavaScript test output:\n%s", string(output))

	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			t.Fatalf("JavaScript tests failed with exit code %d", exitErr.ExitCode())
		}
		t.Fatalf("Failed to run JavaScript tests: %v", err)
	}
}
