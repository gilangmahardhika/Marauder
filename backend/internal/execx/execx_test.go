package execx_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"orchestrator/backend/internal/execx"
)

func TestNoShell(t *testing.T) {
	for _, argv0 := range []string{"/bin/sh", "/bin/bash"} {
		_, err := execx.Command(context.Background(), execx.Child{
			Argv:    []string{argv0, "-c", "echo hi"},
			Env:     []string{},
			Timeout: time.Second,
			MaxOut:  1024,
		})
		if !errors.Is(err, execx.ErrShell) {
			t.Fatalf("%s err = %v, want ErrShell", argv0, err)
		}
	}
}

func TestTimeout(t *testing.T) {
	res, err := execx.Command(context.Background(), execx.Child{
		Argv:    []string{"/bin/sleep", "30"},
		Env:     []string{},
		Timeout: 200 * time.Millisecond,
		MaxOut:  1024,
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Exit != 124 {
		t.Fatalf("exit = %d, want 124", res.Exit)
	}
}

func TestOutputCap(t *testing.T) {
	res, err := execx.Command(context.Background(), execx.Child{
		Argv:    []string{"/usr/bin/yes"},
		Env:     []string{},
		Timeout: 5 * time.Second,
		MaxOut:  64,
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Exit != 125 {
		t.Fatalf("exit = %d, want 125", res.Exit)
	}
	if len(res.Out) > 64 {
		t.Fatalf("output = %d bytes, cap 64", len(res.Out))
	}
}
