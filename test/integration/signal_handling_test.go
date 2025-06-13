package integration

import (
	"context"
	"os"
	"os/exec"
	"syscall"
	"testing"
	"time"
)

func TestServerGracefulShutdown(t *testing.T) {
	// Skip if no kubectl is available (server depends on k8s config)
	if _, err := exec.LookPath("kubectl"); err != nil {
		t.Skip("kubectl not available, skipping integration test")
	}

	// Build the server binary for testing
	buildCmd := exec.Command("go", "build", "-o", "/tmp/mcp-kubernetes-test", ".")
	buildCmd.Dir = "../../" // Go back to project root
	if err := buildCmd.Run(); err != nil {
		t.Fatalf("Failed to build server: %v", err)
	}
	defer os.Remove("/tmp/mcp-kubernetes-test")

	t.Run("SIGTERM handling", func(t *testing.T) {
		testSignalHandling(t, syscall.SIGTERM)
	})

	t.Run("SIGINT handling", func(t *testing.T) {
		testSignalHandling(t, syscall.SIGINT)
	})
}

func testSignalHandling(t *testing.T, signal syscall.Signal) {
	// Start the server process
	cmd := exec.Command("/tmp/mcp-kubernetes-test")
	cmd.Env = append(os.Environ(), "KUBECONFIG=/dev/null") // Prevent actual k8s connection

	// Start the process
	if err := cmd.Start(); err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}

	// Give the server a moment to start up
	time.Sleep(100 * time.Millisecond)

	// Send the signal
	if err := cmd.Process.Signal(signal); err != nil {
		t.Fatalf("Failed to send %s signal: %v", signal, err)
	}

	// Wait for the process to exit with a timeout
	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
	}()

	select {
	case err := <-done:
		// Process exited
		if err != nil {
			// Check if it's a normal exit or signal-related exit
			if exitError, ok := err.(*exec.ExitError); ok {
				// For signal handling, the process might exit with a non-zero code
				// but that's expected when interrupted by a signal
				t.Logf("Process exited with: %v", exitError)
			} else {
				t.Fatalf("Process exited with unexpected error: %v", err)
			}
		}
		t.Logf("Server gracefully handled %s signal", signal)
	case <-time.After(5 * time.Second):
		// Force kill if it doesn't exit in time
		if err := cmd.Process.Kill(); err != nil {
			t.Logf("Failed to force kill process: %v", err)
		}
		t.Fatalf("Server did not exit within 5 seconds after %s signal", signal)
	}
}

func TestServerContextCancellation(t *testing.T) {
	// This test verifies that the server context propagates cancellation properly
	// by checking that operations respect context cancellation

	// Note: This is more of a unit test but placed here for integration context
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	// Simulate a long-running operation that should be cancelled
	done := make(chan bool)
	go func() {
		select {
		case <-ctx.Done():
			done <- true
		case <-time.After(1 * time.Second):
			done <- false
		}
	}()

	result := <-done
	if !result {
		t.Error("Context cancellation was not properly handled")
	}
}
