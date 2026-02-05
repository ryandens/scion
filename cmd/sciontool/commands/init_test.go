/*
Copyright 2025 The Scion Authors.
*/

package commands

import (
	"bytes"
	"os/exec"
	"strings"
	"testing"
)

func TestExtractChildCommand(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		expected []string
	}{
		{
			name:     "single command",
			args:     []string{"bash"},
			expected: []string{"bash"},
		},
		{
			name:     "command with args",
			args:     []string{"tmux", "new-session", "-A"},
			expected: []string{"tmux", "new-session", "-A"},
		},
		{
			name:     "empty args",
			args:     []string{},
			expected: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractChildCommand(tt.args)
			if len(result) != len(tt.expected) {
				t.Errorf("expected %d args, got %d", len(tt.expected), len(result))
				return
			}
			for i, v := range result {
				if v != tt.expected[i] {
					t.Errorf("arg[%d]: expected %q, got %q", i, tt.expected[i], v)
				}
			}
		})
	}
}

func TestInitCommand_Help(t *testing.T) {
	var buf bytes.Buffer
	rootCmd.SetOut(&buf)
	rootCmd.SetArgs([]string{"init", "--help"})

	err := rootCmd.Execute()
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "init") {
		t.Error("help output should mention 'init'")
	}
	if !strings.Contains(output, "grace-period") {
		t.Error("help output should mention 'grace-period' flag")
	}
}

func TestInitCommand_GracePeriodFlag(t *testing.T) {
	// Verify the flag exists and has the expected default
	flag := initCmd.Flags().Lookup("grace-period")
	if flag == nil {
		t.Fatal("grace-period flag not found")
	}
	if flag.DefValue != "10s" {
		t.Errorf("expected default grace-period 10s, got %s", flag.DefValue)
	}
}

// TestInitCommand_Integration performs an integration test with a real subprocess.
// This is skipped in short mode as it involves actual process execution.
func TestInitCommand_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	// Build sciontool if needed for integration testing
	cmd := exec.Command("go", "build", "-buildvcs=false", "-o", "/tmp/sciontool-test", "../")
	if err := cmd.Run(); err != nil {
		t.Skipf("failed to build sciontool for integration test: %v", err)
	}

	// Test running a simple command
	testCmd := exec.Command("/tmp/sciontool-test", "init", "--", "echo", "hello")
	output, err := testCmd.CombinedOutput()
	if err != nil {
		t.Errorf("init command failed: %v\nOutput: %s", err, output)
	}
	if !strings.Contains(string(output), "hello") {
		t.Errorf("expected output to contain 'hello', got: %s", output)
	}
}
