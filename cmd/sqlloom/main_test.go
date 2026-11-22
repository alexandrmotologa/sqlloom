package main

import (
	"bytes"
	"os/exec"
	"strings"
	"testing"
)

func TestCliVersionFlags(t *testing.T) {
	flags := []string{"--version", "-v", "-V", "version"}
	for _, flag := range flags {
		t.Run(flag, func(t *testing.T) {
			cmd := exec.Command("go", "run", ".", flag)
			var stdout, stderr bytes.Buffer
			cmd.Stdout = &stdout
			cmd.Stderr = &stderr
			err := cmd.Run()
			if err != nil {
				t.Fatalf("command failed with %v: %s", err, stderr.String())
			}
			out := stdout.String()
			if !strings.Contains(out, "sqlloom v1.0.0") {
				t.Errorf("expected output to contain 'sqlloom v1.0.0', got: %s", out)
			}
		})
	}
}

func TestCliHelpFlags(t *testing.T) {
	flags := []string{"--help", "-h", "help"}
	for _, flag := range flags {
		t.Run(flag, func(t *testing.T) {
			cmd := exec.Command("go", "run", ".", flag)
			var stdout, stderr bytes.Buffer
			cmd.Stdout = &stdout
			cmd.Stderr = &stderr
			err := cmd.Run()
			if err != nil {
				t.Fatalf("command failed with %v: %s", err, stderr.String())
			}
			out := stdout.String()
			if !strings.Contains(out, "sqlloom v1.0.0") || !strings.Contains(out, "Usage:") {
				t.Errorf("expected usage output, got: %s", out)
			}
		})
	}
}
