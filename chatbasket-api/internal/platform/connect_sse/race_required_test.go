//go:build !race

package connect_sse

import "testing"

// TestRaceDetectorRequired runs ONLY when -race is NOT passed.
// It reminds the developer that concurrent tests in this package require the race detector.
func TestRaceDetectorRequired(t *testing.T) {
	t.Log("⚠️  Re-run with -race for full concurrent safety verification:")
	t.Log("   go test ./internal/platform/connect_sse/... -v -race")
}
