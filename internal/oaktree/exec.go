package oaktree

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

type Runner interface {
	Output(ctx context.Context, name string, args ...string) ([]byte, error)
	CombinedOutput(ctx context.Context, name string, args ...string) ([]byte, error)
	Run(ctx context.Context, name string, args ...string) error
}

type OSRunner struct {
	stateDir string
	mu       sync.Mutex
}

func NewOSRunner(stateDir string) *OSRunner {
	runner := &OSRunner{stateDir: stateDir}
	if path := runner.logPath(); path != "" {
		_ = os.Chmod(path, 0o600)
	}
	return runner
}

func (r *OSRunner) Output(ctx context.Context, name string, args ...string) ([]byte, error) {
	start := time.Now()
	cmd := exec.CommandContext(ctx, name, args...)
	stdout, err := cmd.Output()
	stderr := commandStderr(err)
	r.logInvocation(start, name, args, stdout, stderr, err)
	if err != nil {
		return stdout, newCommandError(r.logPath(), name, args, stdout, stderr, err)
	}
	return stdout, nil
}

func (r *OSRunner) CombinedOutput(ctx context.Context, name string, args ...string) ([]byte, error) {
	start := time.Now()
	cmd := exec.CommandContext(ctx, name, args...)
	out, err := cmd.CombinedOutput()
	r.logInvocation(start, name, args, out, nil, err)
	if err != nil {
		return out, newCommandError(r.logPath(), name, args, out, nil, err)
	}
	return out, nil
}

func (r *OSRunner) Run(ctx context.Context, name string, args ...string) error {
	start := time.Now()
	cmd := exec.CommandContext(ctx, name, args...)
	var stdout limitedBuffer
	var stderr limitedBuffer
	stdout.limit = commandCaptureLimit
	stderr.limit = commandCaptureLimit
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	r.logInvocation(start, name, args, stdout.Bytes(), stderr.Bytes(), err)
	if err != nil {
		return newCommandError(r.logPath(), name, args, stdout.Bytes(), stderr.Bytes(), err)
	}
	return nil
}

func (r *OSRunner) logPath() string {
	if r == nil || r.stateDir == "" {
		return ""
	}
	return filepath.Join(r.stateDir, "logs", "oak-tree.log")
}

const commandCaptureLimit = 8 << 10
const commandSnippetLimit = 512

type limitedBuffer struct {
	limit int
	buf   bytes.Buffer
}

func (b *limitedBuffer) Write(p []byte) (int, error) {
	originalLen := len(p)
	if b.limit <= 0 {
		return originalLen, nil
	}
	remaining := b.limit - b.buf.Len()
	if remaining > 0 {
		if len(p) > remaining {
			p = p[:remaining]
		}
		_, _ = b.buf.Write(p)
	}
	return originalLen, nil
}

func (b *limitedBuffer) Bytes() []byte {
	return b.buf.Bytes()
}

type commandLogEntry struct {
	Timestamp  time.Time `json:"timestamp"`
	Cwd        string    `json:"cwd,omitempty"`
	Command    string    `json:"command"`
	Args       []string  `json:"args,omitempty"`
	Duration   string    `json:"duration"`
	ExitStatus string    `json:"exit_status"`
	Stdout     string    `json:"stdout,omitempty"`
	Stderr     string    `json:"stderr,omitempty"`
	Error      string    `json:"error,omitempty"`
}

type commandError struct {
	command string
	args    []string
	stdout  string
	stderr  string
	logPath string
	err     error
}

func (e *commandError) Error() string {
	parts := []string{formatCommand(e.command, e.args)}
	if e.err != nil {
		parts = append(parts, e.err.Error())
	}
	if e.stderr != "" {
		parts = append(parts, "stderr: "+e.stderr)
	}
	if e.stdout != "" {
		parts = append(parts, "stdout: "+e.stdout)
	}
	if e.logPath != "" {
		parts = append(parts, "log: "+e.logPath)
	}
	return strings.Join(parts, ": ")
}

func (e *commandError) Unwrap() error {
	return e.err
}

func newCommandError(logPath, name string, args []string, stdout, stderr []byte, err error) error {
	return &commandError{
		command: name,
		args:    append([]string(nil), args...),
		stdout:  commandSnippet(stdout),
		stderr:  commandSnippet(stderr),
		logPath: logPath,
		err:     err,
	}
}

func commandStderr(err error) []byte {
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.Stderr
	}
	return nil
}

func (r *OSRunner) logInvocation(start time.Time, name string, args []string, stdout, stderr []byte, err error) {
	if r == nil || r.logPath() == "" {
		return
	}
	entry := commandLogEntry{
		Timestamp:  start.UTC(),
		Cwd:        commandCwd(name, args),
		Command:    name,
		Args:       append([]string(nil), args...),
		Duration:   time.Since(start).Round(time.Millisecond).String(),
		ExitStatus: commandExitStatus(err),
		Stdout:     commandSnippet(stdout),
		Stderr:     commandSnippet(stderr),
	}
	if err != nil {
		entry.Error = err.Error()
	}
	data, marshalErr := json.Marshal(entry)
	if marshalErr != nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := ensurePrivateDir(filepath.Dir(r.logPath())); err != nil {
		return
	}
	f, err := os.OpenFile(r.logPath(), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return
	}
	defer f.Close()
	if err := f.Chmod(0o600); err != nil {
		return
	}
	_, _ = f.Write(append(data, '\n'))
}

func commandExitStatus(err error) string {
	if err == nil {
		return "0"
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		if code := exitErr.ExitCode(); code >= 0 {
			return strconv.Itoa(code)
		}
	}
	if errors.Is(err, context.Canceled) {
		return "canceled"
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "timeout"
	}
	return "error"
}

func commandCwd(name string, args []string) string {
	if cwd := cwdFromArgs(name, args); cwd != "" {
		return cwd
	}
	if cwd, err := os.Getwd(); err == nil {
		return cwd
	}
	return ""
}

func cwdFromArgs(name string, args []string) string {
	switch name {
	case "git":
		for i := 0; i < len(args)-1; i++ {
			if args[i] == "-C" {
				return args[i+1]
			}
		}
	case "tmux":
		for i := 0; i < len(args)-1; i++ {
			if args[i] == "-c" {
				return args[i+1]
			}
		}
	}
	return ""
}

func commandSnippet(data []byte) string {
	if len(data) == 0 {
		return ""
	}
	if len(data) <= commandSnippetLimit {
		return string(data)
	}
	limit := commandSnippetLimit
	if limit <= 0 {
		return ""
	}
	end := 0
	for end < len(data) && end < limit {
		_, size := utf8.DecodeRune(data[end:])
		if size <= 0 {
			break
		}
		if end+size > limit {
			break
		}
		end += size
	}
	if end == 0 {
		return "…"
	}
	return string(data[:end]) + "…"
}

func formatArgs(args []string) []string {
	formatted := make([]string, 0, len(args))
	for _, arg := range args {
		formatted = append(formatted, quoteArg(arg))
	}
	return formatted
}

func formatCommand(name string, args []string) string {
	if len(args) == 0 {
		return name
	}
	parts := make([]string, 0, len(args)+1)
	parts = append(parts, name)
	parts = append(parts, formatArgs(args)...)
	return strings.Join(parts, " ")
}

func quoteArg(value string) string {
	if value == "" {
		return `""`
	}
	if strings.IndexFunc(value, func(r rune) bool {
		return r == ' ' || r == '\t' || r == '\n' || r == '\r' || r == '\'' || r == '"' || r == '\\'
	}) == -1 {
		return value
	}
	return strconv.Quote(value)
}
