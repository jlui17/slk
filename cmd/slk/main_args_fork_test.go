package main

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// TestRunMainAsSlk is not a test on its own: TestUnknownFlagExitsWithUsageError
// re-runs the test binary on it so main() gets a process of its own to exit.
func TestRunMainAsSlk(t *testing.T) {
	args := os.Getenv("SLK_TEST_MAIN_ARGS")
	if args == "" {
		t.Skip("runs only as the subprocess of TestUnknownFlagExitsWithUsageError")
	}
	os.Args = append([]string{"slk"}, strings.Fields(args)...)
	main()
}

func TestUnknownFlagExitsWithUsageError(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestRunMainAsSlk$")
	home := t.TempDir()
	cmd.Env = append(os.Environ(),
		"SLK_TEST_MAIN_ARGS=--bogus",
		"HOME="+home,
		"XDG_CONFIG_HOME="+home+"/config",
		"XDG_DATA_HOME="+home+"/data",
		"XDG_CACHE_HOME="+home+"/cache",
	)
	var stderr strings.Builder
	cmd.Stderr = &stderr
	err := cmd.Run()

	if ctx.Err() != nil {
		t.Fatalf("slk --bogus still running after 10s; want a prompt usage error\nstderr: %s", stderr.String())
	}
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) || exitErr.ExitCode() != 2 {
		t.Fatalf("slk --bogus: err = %v, want exit status 2\nstderr: %s", err, stderr.String())
	}
	if want := `unknown flag "--bogus"`; !strings.Contains(stderr.String(), want) {
		t.Errorf("stderr = %q, want it to contain %q", stderr.String(), want)
	}
}
