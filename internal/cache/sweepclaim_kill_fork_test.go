//go:build unix

package cache

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// TestSweepClaimHolderProcess is the body of the sibling process that
// TestTryClaimThreadSweep_KilledHolderIsTakenOver spawns: it claims T1's
// sweep, reports "claimed" on stdout, then blocks until it is killed.
// It skips in a normal test run.
func TestSweepClaimHolderProcess(t *testing.T) {
	if os.Getenv("SLK_SWEEPCLAIM_HELPER") != "1" {
		t.Skip("helper body for TestTryClaimThreadSweep_KilledHolderIsTakenOver")
	}
	db, err := New(os.Getenv("SLK_SWEEPCLAIM_DB"))
	if err != nil {
		fmt.Fprintf(os.Stderr, "helper: New: %v\n", err)
		os.Exit(2)
	}
	claim, err := db.TryClaimThreadSweep("T1", time.Unix(1000, 0), 30*time.Minute)
	if err != nil || claim == nil {
		fmt.Fprintf(os.Stderr, "helper: TryClaimThreadSweep claim=%v err=%v\n", claim, err)
		os.Exit(2)
	}
	fmt.Println("claimed")
	io.Copy(io.Discard, os.Stdin) // the parent kills us before stdin closes
	claim.Close()
}

// A holder that dies mid-sweep (SIGKILL, crash, power loss) must not
// keep the workspace's claim for the window: the next sibling takes it
// over.
func TestTryClaimThreadSweep_KilledHolderIsTakenOver(t *testing.T) {
	path := filepath.Join(t.TempDir(), "c.db")

	holder := exec.Command(os.Args[0], "-test.run=^TestSweepClaimHolderProcess$")
	holder.Env = append(os.Environ(), "SLK_SWEEPCLAIM_HELPER=1", "SLK_SWEEPCLAIM_DB="+path)
	holder.Stderr = os.Stderr
	stdin, err := holder.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	defer stdin.Close()
	stdout, err := holder.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := holder.Start(); err != nil {
		t.Fatalf("starting holder process: %v", err)
	}
	line, err := bufio.NewReader(stdout).ReadString('\n')
	if err != nil || line != "claimed\n" {
		holder.Process.Kill()
		holder.Wait()
		t.Fatalf("holder process reported %q, %v; want \"claimed\"", line, err)
	}
	if err := holder.Process.Kill(); err != nil {
		t.Fatalf("killing holder process: %v", err)
	}
	holder.Wait()

	db := newClaimDB(t, path)
	claim, err := db.TryClaimThreadSweep("T1", time.Unix(1001, 0), 30*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if claim == nil {
		t.Fatal("claim refused after the holder was killed; a dead holder's claim must be taken over")
	}
	claim.Close()
}
