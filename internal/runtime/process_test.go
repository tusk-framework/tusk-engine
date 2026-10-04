package runtime

import (
	"context"
	"errors"
	"io"
	"os"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestExecProcessFactoryPropagatesSpecAndSupportsReload(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell process signal semantics differ on Windows")
	}
	factory := ExecProcessFactory{Stdout: io.Discard, Stderr: io.Discard}
	process, err := factory.Start(ProcessSpec{
		Binary:     "sh",
		Args:       []string{"-c", "sleep 1"},
		ReloadArgs: []string{"-c", "true"},
		Dir:        t.TempDir(),
	})
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if err := process.Reload(); err != nil {
		t.Fatalf("Reload() error = %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := process.GracefulStop(ctx); err != nil {
		t.Fatalf("GracefulStop() error = %v", err)
	}
	if err := process.Wait(); err != nil {
		t.Fatalf("Wait() error = %v", err)
	}
}

func TestExecProcessFactoryReportsMissingBinary(t *testing.T) {
	_, err := (ExecProcessFactory{}).Start(ProcessSpec{Binary: "definitely-missing-tusk-rr"})
	if err == nil || !strings.Contains(err.Error(), "start process") {
		t.Fatalf("Start() error = %v, want process diagnostic", err)
	}
}

func TestExecProcessGracefulStopHonorsDeadline(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell process signal semantics differ on Windows")
	}
	process, err := (ExecProcessFactory{Stdout: io.Discard, Stderr: io.Discard}).Start(ProcessSpec{
		Binary: "sh",
		Args:   []string{"-c", "sleep 2"},
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond)
	defer cancel()
	if err := process.GracefulStop(ctx); err == nil && !errors.Is(ctx.Err(), context.DeadlineExceeded) {
		t.Fatal("GracefulStop() unexpectedly completed for a long-running process")
	}
	_ = process.Kill()
	_ = process.Wait()
	_ = os.ErrProcessDone
}
