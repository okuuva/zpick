package picker

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/nerveband/zpick/internal/backend"
	"github.com/nerveband/zpick/internal/switcher"
)

// mockBackend implements backend.Backend for testing.
type mockBackend struct {
	name          string
	binaryName    string
	sessionEnvVar string
	inSession     bool
	available     bool
	sessions      []backend.Session
	detachCmd     string
}

func (m *mockBackend) Name() string                         { return m.name }
func (m *mockBackend) BinaryName() string                   { return m.binaryName }
func (m *mockBackend) SessionEnvVar() string                { return m.sessionEnvVar }
func (m *mockBackend) InSession() bool                      { return m.inSession }
func (m *mockBackend) CurrentSessionName() string           { return "" }
func (m *mockBackend) Available() (bool, error)             { return m.available, nil }
func (m *mockBackend) Version() (string, error)             { return "1.0.0", nil }
func (m *mockBackend) List() ([]backend.Session, error)     { return m.sessions, nil }
func (m *mockBackend) FastList() ([]backend.Session, error) { return m.sessions, nil }
func (m *mockBackend) Attach(name string) error             { return nil }
func (m *mockBackend) AttachCommand(name, dir string) string {
	if dir != "" {
		return m.binaryName + " attach " + name + " --dir " + dir
	}
	return m.binaryName + " attach " + name
}
func (m *mockBackend) DetachCommand() string  { return m.detachCmd }
func (m *mockBackend) Kill(name string) error { return nil }

func TestInSessionDetection(t *testing.T) {
	b := &mockBackend{
		name:          "tmux",
		sessionEnvVar: "TMUX",
		inSession:     true,
		detachCmd:     "tmux detach-client",
	}

	// When InSession() is true and ZPICK is not set, inSession should be true
	os.Unsetenv("ZPICK")
	inSession := b.InSession() && os.Getenv("ZPICK") == ""
	if !inSession {
		t.Error("expected inSession=true when InSession()=true and ZPICK unset")
	}

	// When ZPICK is set, inSession should be false (override)
	os.Setenv("ZPICK", "1")
	defer os.Unsetenv("ZPICK")
	inSession = b.InSession() && os.Getenv("ZPICK") == ""
	if inSession {
		t.Error("expected inSession=false when ZPICK is set")
	}
}

func TestInSessionAttachWritesSwitchTarget(t *testing.T) {
	// Set up a temp path for the switch target
	tmpDir := t.TempDir()
	switchPath := filepath.Join(tmpDir, "switch-target")
	switcher.SetPath(switchPath)
	defer switcher.SetPath("")

	b := &mockBackend{
		name:          "tmux",
		sessionEnvVar: "TMUX",
		inSession:     true,
		available:     true,
		detachCmd:     "tmux detach-client",
		sessions: []backend.Session{
			{Name: "dev", Active: true},
		},
	}

	// Simulate the attach action when inSession is true
	inSession := true
	actionName := "dev"

	if inSession {
		switcher.Write(switcher.Target{Action: "attach", Name: actionName})
		cmd := b.DetachCommand()

		// Verify the command is the detach command, not the attach command
		if cmd != "tmux detach-client" {
			t.Errorf("expected detach command, got %q", cmd)
		}

		// Verify the switch target was written
		target, err := switcher.Read()
		if err != nil {
			t.Fatalf("failed to read switch target: %v", err)
		}
		if target.Action != "attach" {
			t.Errorf("expected action=attach, got %q", target.Action)
		}
		if target.Name != "dev" {
			t.Errorf("expected name=dev, got %q", target.Name)
		}
	}
}

func TestInSessionNewWritesSwitchTarget(t *testing.T) {
	tmpDir := t.TempDir()
	switchPath := filepath.Join(tmpDir, "switch-target")
	switcher.SetPath(switchPath)
	defer switcher.SetPath("")

	b := &mockBackend{
		name:      "tmux",
		detachCmd: "tmux detach-client",
		inSession: true,
	}

	// Simulate the new action when inSession is true
	name := "my-project"
	switcher.Write(switcher.Target{Action: "new", Name: name})
	cmd := b.DetachCommand()

	if cmd != "tmux detach-client" {
		t.Errorf("expected detach command, got %q", cmd)
	}

	target, err := switcher.Read()
	if err != nil {
		t.Fatalf("failed to read switch target: %v", err)
	}
	if target.Action != "new" {
		t.Errorf("expected action=new, got %q", target.Action)
	}
	if target.Name != name {
		t.Errorf("expected name=%q, got %q", name, target.Name)
	}
}

func TestInSessionZoxideWritesSwitchTarget(t *testing.T) {
	tmpDir := t.TempDir()
	switchPath := filepath.Join(tmpDir, "switch-target")
	switcher.SetPath(switchPath)
	defer switcher.SetPath("")

	b := &mockBackend{
		name:      "tmux",
		detachCmd: "tmux detach-client",
		inSession: true,
	}

	// Simulate the zoxide action when inSession is true
	name := "my-project"
	dir := "/home/user/projects/my-project"
	switcher.Write(switcher.Target{Action: "new", Name: name, Dir: dir})
	cmd := b.DetachCommand()

	if cmd != "tmux detach-client" {
		t.Errorf("expected detach command, got %q", cmd)
	}

	target, err := switcher.Read()
	if err != nil {
		t.Fatalf("failed to read switch target: %v", err)
	}
	if target.Action != "new" {
		t.Errorf("expected action=new, got %q", target.Action)
	}
	if target.Name != name {
		t.Errorf("expected name=%q, got %q", name, target.Name)
	}
	if target.Dir != dir {
		t.Errorf("expected dir=%q, got %q", dir, target.Dir)
	}
}

func TestNotInSessionReturnsAttachCommand(t *testing.T) {
	b := &mockBackend{
		name:       "tmux",
		binaryName: "tmux",
		detachCmd:  "tmux detach-client",
		inSession:  false,
	}

	// When not in session, should return "exec <attach command>"
	inSession := false
	actionName := "dev"

	var cmd string
	if inSession {
		cmd = b.DetachCommand()
	} else {
		cmd = "exec " + b.AttachCommand(actionName, "")
	}

	expected := "exec tmux attach dev"
	if cmd != expected {
		t.Errorf("expected %q, got %q", expected, cmd)
	}
}

func TestCurrentSessionFromEnv(t *testing.T) {
	b := &mockBackend{
		name:          "tmux",
		sessionEnvVar: "TMUX",
		inSession:     true,
	}

	os.Setenv("TMUX", "/tmp/tmux-1000/default,12345,0")
	defer os.Unsetenv("TMUX")

	var currentSession string
	if b.InSession() {
		currentSession = os.Getenv(b.SessionEnvVar())
	}

	if currentSession == "" {
		t.Error("expected currentSession to be non-empty when in session")
	}
}

// TestShowPickerCurrentSessionParam tests that showPicker accepts currentSession.
// This is a compile-time verification test - it ensures the signature is correct.
func TestShowPickerAcceptsCurrentSession(t *testing.T) {
	// This test just verifies the function signature compiles.
	// The actual showPicker function reads from /dev/tty so we can't
	// fully test it in CI, but we verify it has the right signature.
	var _ func(*os.File, backend.Backend, []backend.Session, string) (Action, error) = showPicker
}
