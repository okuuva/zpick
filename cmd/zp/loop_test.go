package main

import (
	"errors"
	"testing"
)

func TestNormalizeLoopCommand(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"empty", "  \n", ""},
		{"exec prefix", ` exec zmx attach "work" `, `zmx attach "work"`},
		{"cd then exec", ` cd "/tmp/work" && exec zmx attach "work" `, `cd "/tmp/work" && exec zmx attach "work"`},
		{"plain", `tmux new-session -A -s "work"`, `tmux new-session -A -s "work"`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := normalizeLoopCommand(tt.in); got != tt.want {
				t.Fatalf("normalizeLoopCommand(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestRunLoopCommandEmptyExitsLoop(t *testing.T) {
	err := runLoopCommand("  ")
	if !errors.Is(err, errLoopExit) {
		t.Fatalf("runLoopCommand(empty) = %v, want errLoopExit", err)
	}
}
