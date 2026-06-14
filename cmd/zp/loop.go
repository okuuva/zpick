package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"

	"github.com/nerveband/zpick/internal/picker"
	"github.com/nerveband/zpick/internal/switcher"
)

var errLoopExit = errors.New("loop exit")

// runLoop keeps a tiny zp launcher process alive: picker -> foreground attach -> repeat.
func runLoop() error {
	for {
		b, err := loadBackend(true)
		if err != nil {
			return err
		}

		result, err := picker.RunResult(b, version)
		if err != nil {
			return err
		}
		if err := runLoopResult(result); err != nil {
			if errors.Is(err, errLoopExit) {
				return nil
			}
			return err
		}

		if err := runPendingSwitches(); err != nil {
			return err
		}
	}
}

func runPendingSwitches() error {
	for {
		target, ok := nextSwitchTarget()
		if !ok {
			return nil
		}

		if err := runAttachTarget(target); err != nil {
			if errors.Is(err, errLoopExit) {
				return nil
			}
			return err
		}
	}
}

func nextSwitchTarget() (switcher.Target, bool) {
	target, err := switcher.Read()
	if err != nil {
		return switcher.Target{}, false
	}
	return target, true
}

func runLoopResult(result picker.Result) error {
	if result.Target != nil {
		return runAttachTarget(*result.Target)
	}
	return runLoopCommand(result.Command)
}

func runAttachTarget(target switcher.Target) error {
	if target.Name == "" {
		return errLoopExit
	}
	if target.Action != "attach" && target.Action != "new" {
		return errLoopExit
	}

	exe, err := os.Executable()
	if err != nil {
		exe = os.Args[0]
	}

	args := []string{"attach", target.Name}
	if target.Dir != "" {
		args = append(args, "--dir", target.Dir)
	}

	if err := runForegroundCommand(exe, args...); err != nil {
		if isInteractiveSignalExit(err) {
			return nil
		}
		return fmt.Errorf("attach failed: %w", err)
	}
	return nil
}

func runLoopCommand(cmd string) error {
	cmd = normalizeLoopCommand(cmd)
	if cmd == "" {
		return errLoopExit
	}
	if err := runForegroundShell(cmd); err != nil {
		if isInteractiveSignalExit(err) {
			return nil
		}
		return fmt.Errorf("command failed: %w", err)
	}
	return nil
}

func normalizeLoopCommand(cmd string) string {
	cmd = strings.TrimSpace(cmd)
	cmd = strings.TrimPrefix(cmd, "exec ")
	return strings.TrimSpace(cmd)
}

func isInteractiveSignalExit(err error) bool {
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		return false
	}

	if code := exitErr.ExitCode(); code == 130 || code == 131 {
		return true
	}

	status, ok := exitErr.ProcessState.Sys().(syscall.WaitStatus)
	if !ok || !status.Signaled() {
		return false
	}
	return status.Signal() == syscall.SIGINT || status.Signal() == syscall.SIGQUIT
}

func runForegroundCommand(name string, args ...string) error {
	child := exec.Command(name, args...)
	return runForeground(child)
}

func runForegroundShell(cmd string) error {
	child := exec.Command("sh", "-lc", cmd)
	return runForeground(child)
}

func runForeground(child *exec.Cmd) error {
	child.Stdin = os.Stdin
	child.Stdout = os.Stdout
	child.Stderr = os.Stderr
	child.Env = append(os.Environ(), "ZPICK_SESSION=1")

	// Match shell behavior while a foreground child owns the terminal: Ctrl-C/Ctrl-\\
	// should affect the child without killing the launcher loop itself.
	sigs := make(chan os.Signal, 2)
	signal.Notify(sigs, os.Interrupt, syscall.SIGQUIT)
	defer signal.Stop(sigs)
	done := make(chan struct{})
	go func() {
		for {
			select {
			case <-sigs:
			case <-done:
				return
			}
		}
	}()

	err := child.Run()
	close(done)
	return err
}
