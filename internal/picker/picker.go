package picker

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/nerveband/zpick/internal/backend"
	"github.com/nerveband/zpick/internal/switcher"
	"golang.org/x/term"
)

// ANSI color codes
const (
	reset    = "\033[0m"
	dim      = "\033[2m"
	red      = "\033[31m"
	cyan     = "\033[36m"
	green    = "\033[32m"
	yellow   = "\033[33m"
	magenta  = "\033[35m"
	boldRed  = "\033[1;31m"
	boldCyan = "\033[1;36m"
	boldGrn  = "\033[1;32m"
	boldYel  = "\033[1;33m"
	boldWht  = "\033[1;97m"
)

type ActionType int

const (
	ActionAttach ActionType = iota
	ActionNew
	ActionNewDate
	ActionCustom
	ActionZoxide
	ActionKill
	ActionKillAll
	ActionHelp
	ActionRetry
	ActionEscape
)

type Action struct {
	Type ActionType
	Name string
}

type Result struct {
	Command string
	Target  *switcher.Target
}

func (r Result) Empty() bool {
	return strings.TrimSpace(r.Command) == "" && r.Target == nil
}

func attachResult(b backend.Backend, action, name, dir string) Result {
	var prefix string
	if dir != "" {
		prefix = fmt.Sprintf("cd %q", dir)
	}
	return Result{
		Command: sessionExec(b, name, prefix),
		Target:  &switcher.Target{Action: action, Name: name, Dir: dir},
	}
}

// Run is the main interactive picker loop.
// Returns a shell command string to be eval'd by the caller, or empty string.
func Run(b backend.Backend, version string) (string, error) {
	result, err := RunResult(b, version)
	if err != nil {
		return "", err
	}
	return result.Command, nil
}

// RunResult is the main interactive picker loop.
// Returns both the shell command for legacy hooks and a structured target for launchers.
func RunResult(b backend.Backend, version string) (Result, error) {
	// Detect in-session mode
	inSession := b.InSession() && os.Getenv("ZPICK") == ""
	var currentSession string
	if inSession {
		currentSession = b.CurrentSessionName()
	}

	// Load key mode preference (letters-first or numbers-first)
	LoadKeyMode(backend.ReadKeyMode())

	tty, err := os.OpenFile("/dev/tty", os.O_RDWR, 0)
	if err != nil {
		return Result{}, nil
	}
	defer tty.Close()

	// Guard: if backend binary not found, show install guidance
	if ok, _ := b.Available(); !ok {
		fmt.Fprintf(tty, "\n  %szp:%s %s not found\n", boldCyan, reset, b.BinaryName())
		fmt.Fprintf(tty, "  %sRun 'zp check' for full dependency status%s\n\n", dim, reset)
		return Result{}, nil
	}

	for {
		sessions, err := b.FastList()
		if err != nil {
			return Result{}, fmt.Errorf("failed to list sessions: %w", err)
		}

		action, err := showPicker(tty, b, sessions, currentSession)
		if err != nil {
			return Result{}, err
		}

		switch action.Type {
		case ActionAttach:
			if inSession {
				switcher.Write(switcher.Target{Action: "attach", Name: action.Name})
				return Result{Command: b.DetachCommand()}, nil
			}
			return attachResult(b, "attach", action.Name, ""), nil
		case ActionNew:
			cwd, _ := os.Getwd()
			name := CounterName(cwd, sessions)
			fmt.Fprintf(tty, "\n  %s>%s %s%s%s\n\n", boldGrn, reset, boldWht, name, reset)
			if inSession {
				switcher.Write(switcher.Target{Action: "new", Name: name})
				return Result{Command: b.DetachCommand()}, nil
			}
			return attachResult(b, "new", name, ""), nil
		case ActionNewDate:
			cwd, _ := os.Getwd()
			name := DateName(cwd)
			fmt.Fprintf(tty, "\n  %s>%s %s%s%s\n\n", boldGrn, reset, boldWht, name, reset)
			if inSession {
				switcher.Write(switcher.Target{Action: "new", Name: name})
				return Result{Command: b.DetachCommand()}, nil
			}
			return attachResult(b, "new", name, ""), nil
		case ActionCustom:
			result, err := handleCustom(tty, b, sessions, inSession)
			if err != nil {
				return Result{}, err
			}
			if !result.Empty() {
				return result, nil
			}
			continue
		case ActionZoxide:
			dir, err := runZoxide(tty)
			if err != nil || dir == "" {
				continue
			}
			name := CounterName(dir, sessions)
			fmt.Fprintf(tty, "\n  %s>%s %s%s%s %s%s%s\n\n", boldGrn, reset, boldWht, name, reset, dim, dir, reset)
			if inSession {
				switcher.Write(switcher.Target{Action: "new", Name: name, Dir: dir})
				return Result{Command: b.DetachCommand()}, nil
			}
			return attachResult(b, "new", name, dir), nil
		case ActionKill:
			if action.Name == "" {
				continue // no session selected, redraw
			}
			if err := confirmAndKill(tty, b, action.Name); err != nil {
				fmt.Fprintf(tty, "  %sfailed: %v%s\n", dim, err, reset)
			} else {
				fmt.Fprintf(tty, "  %skilled%s %s%s%s\n", boldRed, reset, boldWht, action.Name, reset)
			}
			continue
		case ActionKillAll:
			confirmAndKillAll(tty, b, sessions)
			continue
		case ActionHelp:
			b = showHelpConfig(tty, b, version)
			continue
		case ActionRetry:
			continue
		case ActionEscape:
			return Result{}, nil
		}
	}
}

func showPicker(tty *os.File, b backend.Backend, sessions []backend.Session, currentSession string) (Action, error) {
	fmt.Fprint(tty, "\033[H\033[2J") // clear screen
	fmt.Fprintln(tty)

	if len(sessions) > 0 {
		plural := ""
		if len(sessions) > 1 {
			plural = "s"
		}
		if currentSession != "" {
			fmt.Fprintf(tty, "  %s%s%s %s%d session%s%s  %s(in: %s ←)%s\n\n",
				boldCyan, b.Name(), reset, dim, len(sessions), plural, reset,
				dim, currentSession, reset)
		} else {
			fmt.Fprintf(tty, "  %s%s%s %s%d session%s%s\n\n", boldCyan, b.Name(), reset, dim, len(sessions), plural, reset)
		}

		for i, s := range sessions {
			if i >= MaxSessions {
				break
			}
			indicator := fmt.Sprintf("%s.%s", dim, reset)
			if s.Name == currentSession {
				indicator = fmt.Sprintf("%s←%s", boldCyan, reset)
			} else if s.Active {
				indicator = fmt.Sprintf("%s*%s", boldGrn, reset)
			}
			dir := truncatePath(s.StartedIn, 40)
			fmt.Fprintf(tty, "  %s%c%s  %s%s%s %s %s%s%s\n",
				boldYel, KeyForIndex(i), reset,
				boldWht, s.Name, reset,
				indicator,
				dim, dir, reset)
		}
		fmt.Fprintln(tty)
	} else {
		if currentSession != "" {
			fmt.Fprintf(tty, "  %s%s%s %sno sessions%s  %s(in: %s ←)%s\n\n",
				boldCyan, b.Name(), reset, dim, reset,
				dim, currentSession, reset)
		} else {
			fmt.Fprintf(tty, "  %s%s%s %sno sessions%s\n\n", boldCyan, b.Name(), reset, dim, reset)
		}
	}

	cwd, _ := os.Getwd()
	defaultName := CounterName(cwd, sessions)
	fmt.Fprintf(tty, "  %senter%s %snew%s %s%s%s\n", boldGrn, reset, dim, reset, boldWht, defaultName, reset)
	fmt.Fprintf(tty, "  %sc%s %scustom%s  %sz%s %spick dir%s  %sd%s %s+date%s\n",
		magenta, reset, dim, reset,
		magenta, reset, dim, reset,
		cyan, reset, dim, reset)
	fmt.Fprintf(tty, "  %sk%s %skill%s  %sh%s %shelp%s  %sesc%s %sskip%s\n",
		red, reset, dim, reset,
		cyan, reset, dim, reset,
		yellow, reset, dim, reset)
	fmt.Fprintln(tty)

	fmt.Fprintf(tty, "  %s>%s ", boldCyan, reset)

	oldState, err := term.MakeRaw(int(tty.Fd()))
	if err != nil {
		return Action{}, fmt.Errorf("failed to set raw mode: %w", err)
	}
	defer term.Restore(int(tty.Fd()), oldState)

	buf := make([]byte, 3)
	n, err := tty.Read(buf)
	term.Restore(int(tty.Fd()), oldState)
	fmt.Fprintln(tty)

	if err != nil {
		return Action{}, err
	}

	input := buf[:n]
	if len(input) == 1 && input[0] == 'k' {
		return enterKillMode(tty, sessions)
	}

	action := pickerActionForInput(input, sessions)
	if action.Type == ActionAttach {
		fmt.Fprintf(tty, "\n  %s>%s %s%s%s\n\n", boldGrn, reset, boldWht, action.Name, reset)
	}
	return action, nil
}

func enterKillMode(tty *os.File, sessions []backend.Session) (Action, error) {
	if len(sessions) == 0 {
		fmt.Fprintf(tty, "\n  %sno sessions to kill%s\n", dim, reset)
		time.Sleep(800 * time.Millisecond)
		return Action{Type: ActionKill}, nil // redraw picker
	}

	fmt.Fprintf(tty, "\n  %skill%s %swhich session? %sc%s %sclear all%s ", boldRed, reset, dim, boldRed, reset, dim, reset)

	oldState, err := term.MakeRaw(int(tty.Fd()))
	if err != nil {
		return Action{}, err
	}
	defer term.Restore(int(tty.Fd()), oldState)

	buf := make([]byte, 3)
	n, _ := tty.Read(buf)
	term.Restore(int(tty.Fd()), oldState)
	fmt.Fprintln(tty)

	if n == 1 && buf[0] == 27 {
		return Action{Type: ActionKill}, nil // cancelled, redraw picker
	}

	if buf[0] == 'c' || buf[0] == 'C' {
		return Action{Type: ActionKillAll}, nil
	}

	if idx, ok := IndexForKey(buf[0]); ok && idx < len(sessions) {
		return Action{Type: ActionKill, Name: sessions[idx].Name}, nil
	}

	return Action{Type: ActionKill}, nil // invalid key, redraw picker
}

func pickerActionForInput(input []byte, sessions []backend.Session) Action {
	if len(input) == 0 {
		return Action{Type: ActionRetry}
	}

	key := input[0]
	if key == 3 || (len(input) == 1 && key == 27) {
		return Action{Type: ActionEscape}
	}
	if key == 13 || key == 10 {
		return Action{Type: ActionNew}
	}

	switch key {
	case 'z':
		return Action{Type: ActionZoxide}
	case 'd':
		return Action{Type: ActionNewDate}
	case 'c':
		return Action{Type: ActionCustom}
	case 'h':
		return Action{Type: ActionHelp}
	}

	idx, ok := IndexForKey(key)
	if !ok || idx >= len(sessions) {
		return Action{Type: ActionRetry}
	}

	return Action{Type: ActionAttach, Name: sessions[idx].Name}
}

func confirmAndKill(tty *os.File, b backend.Backend, name string) error {
	if os.Getenv("ZPICK_NO_CONFIRM") == "1" {
		return b.Kill(name)
	}

	fmt.Fprintf(tty, "  %skill %s%s%s?%s %sy/n%s ", boldRed, boldWht, name, boldRed, reset, dim, reset)

	oldState, err := term.MakeRaw(int(tty.Fd()))
	if err != nil {
		return err
	}
	defer term.Restore(int(tty.Fd()), oldState)

	buf := make([]byte, 1)
	tty.Read(buf)
	term.Restore(int(tty.Fd()), oldState)
	fmt.Fprintln(tty)

	if buf[0] == 'y' || buf[0] == 'Y' {
		return b.Kill(name)
	}
	fmt.Fprintf(tty, "  %scancelled%s\n", dim, reset)
	return nil
}

func confirmAndKillAll(tty *os.File, b backend.Backend, sessions []backend.Session) {
	fmt.Fprintf(tty, "  %skill all %d sessions?%s %sy/n%s ", boldRed, len(sessions), reset, dim, reset)

	oldState, err := term.MakeRaw(int(tty.Fd()))
	if err != nil {
		return
	}
	defer term.Restore(int(tty.Fd()), oldState)

	buf := make([]byte, 1)
	tty.Read(buf)
	term.Restore(int(tty.Fd()), oldState)
	fmt.Fprintln(tty)

	if buf[0] != 'y' && buf[0] != 'Y' {
		fmt.Fprintf(tty, "  %scancelled%s\n", dim, reset)
		return
	}

	for _, s := range sessions {
		if err := b.Kill(s.Name); err != nil {
			fmt.Fprintf(tty, "  %sfailed: %s — %v%s\n", dim, s.Name, err, reset)
		} else {
			fmt.Fprintf(tty, "  %skilled%s %s%s%s\n", boldRed, reset, boldWht, s.Name, reset)
		}
	}
}

func handleCustom(tty *os.File, b backend.Backend, sessions []backend.Session, inSession bool) (Result, error) {
	fmt.Fprintf(tty, "\n  %sname:%s ", magenta, reset)

	customName, ok := readLineRaw(tty)
	if !ok || customName == "" {
		return Result{}, nil
	}

	fmt.Fprintf(tty, "\n  %s>%s %s%s%s\n\n", boldGrn, reset, boldWht, customName, reset)
	if inSession {
		switcher.Write(switcher.Target{Action: "new", Name: customName})
		return Result{Command: b.DetachCommand()}, nil
	}
	return attachResult(b, "new", customName, ""), nil
}

// readLineRaw reads a line in raw mode, supporting escape to cancel and backspace.
// Returns the entered string and true, or empty string and false if cancelled.
func readLineRaw(tty *os.File) (string, bool) {
	oldState, err := term.MakeRaw(int(tty.Fd()))
	if err != nil {
		return "", false
	}
	defer term.Restore(int(tty.Fd()), oldState)

	var buf []byte
	b := make([]byte, 3)
	for {
		n, err := tty.Read(b)
		if err != nil || n == 0 {
			return "", false
		}
		key := b[0]

		switch {
		case key == 27: // Escape
			fmt.Fprint(tty, "\r\n")
			term.Restore(int(tty.Fd()), oldState)
			return "", false
		case key == 3: // Ctrl-C
			fmt.Fprint(tty, "\r\n")
			term.Restore(int(tty.Fd()), oldState)
			return "", false
		case key == 13 || key == 10: // Enter
			fmt.Fprint(tty, "\r\n")
			term.Restore(int(tty.Fd()), oldState)
			return strings.TrimSpace(string(buf)), true
		case key == 127 || key == 8: // Backspace
			if len(buf) > 0 {
				buf = buf[:len(buf)-1]
				fmt.Fprint(tty, "\b \b")
			}
		case key >= 32 && key < 127: // Printable
			buf = append(buf, key)
			fmt.Fprintf(tty, "%c", key)
		}
	}
}

// sessionExec builds an exec command that sets ZPICK_SESSION so the inner shell's
// guard check won't re-trigger the picker. Some backends (e.g. zmosh) don't set
// their session env var in the child shell, so ZPICK_SESSION acts as a universal
// "already inside a session" marker.
func sessionExec(b backend.Backend, name, prefix string) string {
	cmd := fmt.Sprintf("ZPICK_SESSION=%q %s", name, b.AttachCommand(name, ""))
	if prefix != "" {
		return prefix + " && " + cmd
	}
	return cmd
}

func runZoxide(tty *os.File) (string, error) {
	if _, err := exec.LookPath("zoxide"); err != nil {
		fmt.Fprintf(tty, "  %szoxide not installed%s\n", yellow, reset)
		return "", nil
	}
	fmt.Fprintln(tty)
	cmd := exec.Command("zoxide", "query", "-i")
	cmd.Stdin = tty
	cmd.Stderr = tty
	out, err := cmd.Output()
	if err != nil {
		return "", nil
	}
	return strings.TrimSpace(string(out)), nil
}

func truncatePath(path string, maxLen int) string {
	home := os.Getenv("HOME")
	if home != "" {
		path = strings.Replace(path, home, "~", 1)
	}
	if len(path) <= maxLen {
		return path
	}
	parts := strings.Split(path, "/")
	if len(parts) > 4 {
		if strings.HasPrefix(path, "~") {
			path = "~/" + strings.Join(parts[len(parts)-3:], "/")
		} else {
			path = ".../" + strings.Join(parts[len(parts)-3:], "/")
		}
	}
	return path
}
