package oaktree

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

func TestOSRunnerOutputIncludesStderrAndLogPath(t *testing.T) {
	stateDir := t.TempDir()
	runner := NewOSRunner(stateDir)
	logPath := filepath.Join(stateDir, "logs", "oak-tree.log")
	stdout := strings.Repeat("é", 600)
	stderr := strings.Repeat("ß", 700)

	out, err := runner.Output(
		context.Background(),
		"sh",
		"-c",
		"printf '%s' \"$1\"; printf '%s' \"$2\" >&2; exit 13",
		"sh",
		stdout,
		stderr,
	)
	if string(out) != stdout {
		t.Fatalf("Output() stdout = %q, want %q", string(out), stdout)
	}
	if err == nil {
		t.Fatal("Output() error = nil, want command error")
	}
	errText := err.Error()
	if !strings.Contains(errText, "sh -c") {
		t.Fatalf("Output() error %q does not include command context", errText)
	}
	if !strings.Contains(errText, "stderr:") {
		t.Fatalf("Output() error %q does not include stderr", errText)
	}
	if !strings.Contains(errText, "exit status 13") {
		t.Fatalf("Output() error %q does not include exit status", errText)
	}
	if !strings.Contains(errText, "log: "+logPath) {
		t.Fatalf("Output() error %q does not include log path", errText)
	}

	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("ReadFile(%s) = %v", logPath, err)
	}
	if info, err := os.Stat(logPath); err != nil {
		t.Fatal(err)
	} else if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("log mode = %o, want 600", got)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")

	if len(lines) != 1 {
		t.Fatalf("log line count = %d, want 1", len(lines))
	}
	var entry commandLogEntry
	if err := json.Unmarshal([]byte(lines[0]), &entry); err != nil {
		t.Fatalf("json.Unmarshal() = %v", err)
	}
	if entry.Command != "sh" {
		t.Fatalf("log command = %q, want sh", entry.Command)
	}
	if entry.ExitStatus != "13" {
		t.Fatalf("log exit status = %q, want 13", entry.ExitStatus)
	}
	if entry.Cwd == "" {
		t.Fatal("log cwd is empty")
	}
	if entry.Timestamp.IsZero() {
		t.Fatal("log timestamp is zero")
	}
	if !strings.HasSuffix(entry.Stdout, "…") || !strings.HasSuffix(entry.Stderr, "…") {
		t.Fatalf("log snippets were not truncated: stdout=%q stderr=%q", entry.Stdout, entry.Stderr)
	}
	if !utf8.ValidString(entry.Stdout) || !utf8.ValidString(entry.Stderr) {
		t.Fatalf("log snippets are not valid UTF-8: stdout=%q stderr=%q", entry.Stdout, entry.Stderr)
	}
	if got := utf8.RuneCountInString(entry.Stdout); got >= utf8.RuneCountInString(stdout) {
		t.Fatalf("stdout snippet rune count = %d, want shorter than %d", got, utf8.RuneCountInString(stdout))
	}
	if got := utf8.RuneCountInString(entry.Stderr); got >= utf8.RuneCountInString(stderr) {
		t.Fatalf("stderr snippet rune count = %d, want shorter than %d", got, utf8.RuneCountInString(stderr))
	}
}

func TestOSRunnerRunIncludesStdoutStderrAndLogPath(t *testing.T) {
	stateDir := t.TempDir()
	runner := NewOSRunner(stateDir)
	logPath := filepath.Join(stateDir, "logs", "oak-tree.log")

	err := runner.Run(
		context.Background(),
		"sh",
		"-c",
		"printf '%s' \"$1\"; printf '%s' \"$2\" >&2; exit 4",
		"sh",
		"stdout-data",
		"stderr-data",
	)
	if err == nil {
		t.Fatal("Run() error = nil, want command error")
	}
	errText := err.Error()
	if !strings.Contains(errText, "sh -c") {
		t.Fatalf("Run() error %q does not include command context", errText)
	}
	if !strings.Contains(errText, "stdout: stdout-data") {
		t.Fatalf("Run() error %q does not include stdout", errText)
	}
	if !strings.Contains(errText, "stderr: stderr-data") {
		t.Fatalf("Run() error %q does not include stderr", errText)
	}
	if !strings.Contains(errText, "exit status 4") {
		t.Fatalf("Run() error %q does not include exit status", errText)
	}
	if !strings.Contains(errText, "log: "+logPath) {
		t.Fatalf("Run() error %q does not include log path", errText)
	}

	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("ReadFile(%s) = %v", logPath, err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 1 {
		t.Fatalf("log line count = %d, want 1", len(lines))
	}
	var entry commandLogEntry
	if err := json.Unmarshal([]byte(lines[0]), &entry); err != nil {
		t.Fatalf("json.Unmarshal() = %v", err)
	}
	if entry.Command != "sh" {
		t.Fatalf("log command = %q, want sh", entry.Command)
	}
	if entry.ExitStatus != "4" {
		t.Fatalf("log exit status = %q, want 4", entry.ExitStatus)
	}
	if entry.Stdout != "stdout-data" {
		t.Fatalf("log stdout = %q, want stdout-data", entry.Stdout)
	}
	if entry.Stderr != "stderr-data" {
		t.Fatalf("log stderr = %q, want stderr-data", entry.Stderr)
	}
	if entry.Timestamp.After(time.Now().Add(1 * time.Minute)) {
		t.Fatalf("log timestamp looks wrong: %s", entry.Timestamp)
	}
}
