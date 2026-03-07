package zmosh

import (
	"net"
	"os"
	"path/filepath"
	"testing"

	"github.com/nerveband/zpick/internal/backend"
)

// Verify Zmosh implements the Backend interface.
var _ backend.Backend = (*Zmosh)(nil)

func TestZmoshName(t *testing.T) {
	b := New()
	if b.Name() != "zmosh" {
		t.Errorf("Name() = %q, want %q", b.Name(), "zmosh")
	}
}

func TestZmoshBinaryName(t *testing.T) {
	b := New()
	if b.BinaryName() != "zmosh" {
		t.Errorf("BinaryName() = %q, want %q", b.BinaryName(), "zmosh")
	}
}

func TestZmoshSessionEnvVar(t *testing.T) {
	b := New()
	if b.SessionEnvVar() != "ZMX_SESSION" {
		t.Errorf("SessionEnvVar() = %q, want %q", b.SessionEnvVar(), "ZMX_SESSION")
	}
}

func TestZmoshInSessionTrue(t *testing.T) {
	t.Setenv("ZMX_SESSION", "test")
	b := New()
	if !b.InSession() {
		t.Error("InSession() should return true when ZMX_SESSION is set")
	}
}

func TestZmoshInSessionFalse(t *testing.T) {
	t.Setenv("ZMX_SESSION", "")
	b := New()
	if b.InSession() {
		t.Error("InSession() should return false when ZMX_SESSION is empty")
	}
}

func TestZmoshAttachCommand(t *testing.T) {
	b := New()
	cmd := b.AttachCommand("my-session", "")
	want := `zmosh attach "my-session"`
	if cmd != want {
		t.Errorf("AttachCommand() = %q, want %q", cmd, want)
	}
}

func TestZmoshAttachCommandWithDir(t *testing.T) {
	b := New()
	cmd := b.AttachCommand("my-session", "/tmp/foo")
	want := `cd "/tmp/foo" && zmosh attach "my-session"`
	if cmd != want {
		t.Errorf("AttachCommand() = %q, want %q", cmd, want)
	}
}

func TestZmoshAttachCommandUDP(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	backend.SetUDP(true, "myhost")

	b := New()
	cmd := b.AttachCommand("my-session", "")
	want := `zmosh attach -r myhost "my-session"`
	if cmd != want {
		t.Errorf("AttachCommand() = %q, want %q", cmd, want)
	}
}

func TestZmoshAttachCommandUDPNoHost(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	backend.SetUDP(true, "")

	b := New()
	cmd := b.AttachCommand("my-session", "")
	// No host set — no -r flag
	want := `zmosh attach "my-session"`
	if cmd != want {
		t.Errorf("AttachCommand() = %q, want %q", cmd, want)
	}
}

func TestZmoshDetachCommand(t *testing.T) {
	b := New()
	got := b.DetachCommand()
	want := "zmosh detach"
	if got != want {
		t.Errorf("DetachCommand() = %q, want %q", got, want)
	}
}

func TestParseSessions(t *testing.T) {
	input := "  session_name=apcsp-1\tpid=1234\tclients=1\tcreated_at=1771652262707138000\ttask_ended_at=0\ttask_exit_code=0\tstarted_in=~/GitHub/apcsp\n" +
		"  session_name=bbcli\tpid=5678\tclients=0\tcreated_at=1771642928511196000\ttask_ended_at=0\ttask_exit_code=0\tstarted_in=~/Documents/GitHub/agent-to-bricks\n"

	sessions := ParseSessions(input)

	if len(sessions) != 2 {
		t.Fatalf("expected 2 sessions, got %d", len(sessions))
	}
	if sessions[0].Name != "apcsp-1" {
		t.Errorf("expected name apcsp-1, got %s", sessions[0].Name)
	}
	if sessions[0].PID != 1234 {
		t.Errorf("expected pid 1234, got %d", sessions[0].PID)
	}
	if !sessions[0].Active {
		t.Error("expected session to be active")
	}
}

func TestParseEmpty(t *testing.T) {
	sessions := ParseSessions("")
	if len(sessions) != 0 {
		t.Fatalf("expected 0 sessions, got %d", len(sessions))
	}
}

func TestParseCurrentSessionArrow(t *testing.T) {
	input := "\u2192 session_name=zmosh-picker\tpid=78409\tclients=1\tstarted_in=~/GitHub/zmosh-picker\n"
	sessions := ParseSessions(input)
	if len(sessions) != 1 {
		t.Fatalf("expected 1 session, got %d", len(sessions))
	}
	if sessions[0].Name != "zmosh-picker" {
		t.Errorf("expected name zmosh-picker, got %s", sessions[0].Name)
	}
}

func TestFastListDir(t *testing.T) {
	dir := t.TempDir()

	for _, name := range []string{"work", "play"} {
		sock := filepath.Join(dir, name)
		l, err := net.Listen("unix", sock)
		if err != nil {
			t.Fatal(err)
		}
		defer l.Close()
	}

	// Subdirectory and regular file should be skipped
	os.Mkdir(filepath.Join(dir, "logs"), 0o755)
	os.WriteFile(filepath.Join(dir, "lock"), []byte("x"), 0o644)

	sessions, err := FastListDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 2 {
		t.Fatalf("expected 2 sessions, got %d", len(sessions))
	}

	names := map[string]bool{}
	for _, s := range sessions {
		names[s.Name] = true
		if s.StartedIn != "~" {
			t.Errorf("expected StartedIn=~, got %q", s.StartedIn)
		}
	}
	if !names["work"] || !names["play"] {
		t.Errorf("expected work and play sessions, got %v", names)
	}
}

func TestFastListDirActive(t *testing.T) {
	dir := t.TempDir()

	sock := filepath.Join(dir, "active-sess")
	l, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()

	t.Setenv("ZMX_SESSION", "active-sess")

	sessions, err := FastListDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 1 {
		t.Fatalf("expected 1 session, got %d", len(sessions))
	}
	if !sessions[0].Active {
		t.Error("expected session to be active")
	}
}

func TestResolveZmxDir(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("ZMX_DIR", dir)

	got, err := ResolveZmxDir()
	if err != nil {
		t.Fatal(err)
	}
	if got != dir {
		t.Errorf("expected %q, got %q", dir, got)
	}
}

func TestResolveZmxDirTMPDIR(t *testing.T) {
	t.Setenv("ZMX_DIR", "")
	t.Setenv("XDG_RUNTIME_DIR", "")

	tmpdir := t.TempDir()
	t.Setenv("TMPDIR", tmpdir)

	// Create the expected directory: zmx-{uid}
	candidate := filepath.Join(tmpdir, "zmx-"+itoa(os.Getuid()))
	os.MkdirAll(candidate, 0o755)

	got, err := ResolveZmxDir()
	if err != nil {
		t.Fatal(err)
	}
	if got != candidate {
		t.Errorf("expected %q, got %q", candidate, got)
	}
}
