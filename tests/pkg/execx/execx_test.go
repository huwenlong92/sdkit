package execx_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/huwenlong92/sdkit/pkg/execx"
)

func TestRunSuccess(t *testing.T) {
	name, args := helperCommand("ok")
	result, err := execx.Run(context.Background(), name, args)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.ExitCode != 0 {
		t.Fatalf("ExitCode = %d, want 0", result.ExitCode)
	}
	if result.PID == 0 {
		t.Fatal("PID should be set")
	}
}

func TestRunCommandNotFound(t *testing.T) {
	_, err := execx.Run(context.Background(), "sdkit-execx-command-not-found", nil)
	if err == nil {
		t.Fatal("Run() error = nil, want command start error")
	}
}

func TestRunOutputSeparatesStdoutAndStderr(t *testing.T) {
	name, args := helperCommand("stdout-stderr")
	output, err := execx.RunOutput(context.Background(), name, args)
	if err != nil {
		t.Fatalf("RunOutput() error = %v", err)
	}
	if !strings.Contains(string(output.Stdout), "stdout-line") {
		t.Fatalf("stdout = %q, want stdout-line", output.Stdout)
	}
	if !strings.Contains(string(output.Stderr), "stderr-line") {
		t.Fatalf("stderr = %q, want stderr-line", output.Stderr)
	}
}

func TestRunShellOutput(t *testing.T) {
	output, err := execx.RunShellOutput(context.Background(), shellPrint("shell-ok"))
	if err != nil {
		t.Fatalf("RunShellOutput() error = %v", err)
	}
	if string(output.Stdout) != "shell-ok" {
		t.Fatalf("stdout = %q, want shell-ok", output.Stdout)
	}
}

func TestRunShellWithShellOverride(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("custom shell override test uses sh")
	}
	output, err := execx.RunShellOutput(
		context.Background(),
		"printf custom-ok",
		execx.WithShell("sh", "-c"),
	)
	if err != nil {
		t.Fatalf("RunShellOutput() error = %v", err)
	}
	if string(output.Stdout) != "custom-ok" {
		t.Fatalf("stdout = %q, want custom-ok", output.Stdout)
	}
}

func TestRunShellStream(t *testing.T) {
	var text string
	_, err := execx.RunShellStream(
		context.Background(),
		shellPrint("stream-ok"),
		execx.SinkFunc(func(ctx context.Context, event execx.Event) error {
			text = event.Text
			return nil
		}),
	)
	if err != nil {
		t.Fatalf("RunShellStream() error = %v", err)
	}
	if text != "stream-ok" {
		t.Fatalf("stream text = %q, want stream-ok", text)
	}
}

func TestRunOutputMergeStderr(t *testing.T) {
	name, args := helperCommand("stdout-stderr")
	output, err := execx.RunOutput(context.Background(), name, args, execx.WithMergeStderr())
	if err != nil {
		t.Fatalf("RunOutput() error = %v", err)
	}
	if len(output.Stderr) != 0 {
		t.Fatalf("stderr = %q, want empty when merged", output.Stderr)
	}
	if !strings.Contains(string(output.Stdout), "stdout-line") || !strings.Contains(string(output.Stdout), "stderr-line") {
		t.Fatalf("stdout = %q, want merged stdout and stderr", output.Stdout)
	}
}

func TestRunNonZeroExit(t *testing.T) {
	name, args := helperCommand("exit", "7")
	result, err := execx.Run(context.Background(), name, args)
	if err == nil {
		t.Fatal("Run() error = nil, want exit error")
	}
	var exitErr *execx.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("Run() error = %T, want *execx.ExitError", err)
	}
	if result.ExitCode != 7 || exitErr.Result.ExitCode != 7 {
		t.Fatalf("ExitCode result=%d err=%d, want 7", result.ExitCode, exitErr.Result.ExitCode)
	}
}

func TestRunContextDeadline(t *testing.T) {
	name, args := helperCommand("sleep", "2s")
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	_, err := execx.Run(ctx, name, args)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Run() error = %v, want deadline exceeded", err)
	}
}

func TestRunStreamStdoutStderr(t *testing.T) {
	name, args := helperCommand("stdout-stderr")
	var mu sync.Mutex
	events := make([]execx.Event, 0, 2)
	sink := execx.SinkFunc(func(ctx context.Context, event execx.Event) error {
		mu.Lock()
		defer mu.Unlock()
		events = append(events, event)
		return nil
	})
	if _, err := execx.RunStream(context.Background(), name, args, sink); err != nil {
		t.Fatalf("RunStream() error = %v", err)
	}
	if !hasEvent(events, execx.StreamStdout, "stdout-line") {
		t.Fatalf("events = %+v, want stdout-line", events)
	}
	if !hasEvent(events, execx.StreamStderr, "stderr-line") {
		t.Fatalf("events = %+v, want stderr-line", events)
	}
}

func TestRunStreamCRLFSplit(t *testing.T) {
	name, args := helperCommand("crlf")
	var events []execx.Event
	sink := execx.SinkFunc(func(ctx context.Context, event execx.Event) error {
		events = append(events, event)
		return nil
	})
	if _, err := execx.RunStream(context.Background(), name, args, sink, execx.WithSplitMode(execx.SplitCRLF)); err != nil {
		t.Fatalf("RunStream() error = %v", err)
	}
	if !hasEvent(events, execx.StreamStdout, "10%") || !hasEvent(events, execx.StreamStdout, "20%") {
		t.Fatalf("events = %+v, want CRLF progress tokens", events)
	}
}

func TestRunOutputStdin(t *testing.T) {
	name, args := helperCommand("stdin")
	output, err := execx.RunOutput(context.Background(), name, args, execx.WithInputString("yes\n"))
	if err != nil {
		t.Fatalf("RunOutput() error = %v", err)
	}
	if string(output.Stdout) != "yes\n" {
		t.Fatalf("stdout = %q, want input echo", output.Stdout)
	}
}

func TestRunOutputCleanEnvAndEnv(t *testing.T) {
	name, args := helperCommand("env", "EXECX_TEST_ENV")
	output, err := execx.RunOutput(
		context.Background(),
		name,
		args,
		execx.WithCleanEnv(),
		execx.WithEnv([]string{"EXECX_TEST_ENV=ok"}),
	)
	if err != nil {
		t.Fatalf("RunOutput() error = %v", err)
	}
	if string(output.Stdout) != "ok" {
		t.Fatalf("stdout = %q, want env value", output.Stdout)
	}
}

func TestRunStreamDecodeFunc(t *testing.T) {
	name, args := helperCommand("decode")
	var text string
	sink := execx.SinkFunc(func(ctx context.Context, event execx.Event) error {
		text = event.Text
		return nil
	})
	_, err := execx.RunStream(context.Background(), name, args, sink, execx.WithDecodeFunc(func(data []byte) (string, error) {
		return "decoded:" + string(data), nil
	}))
	if err != nil {
		t.Fatalf("RunStream() error = %v", err)
	}
	if text != "decoded:abc" {
		t.Fatalf("decoded text = %q, want decoded:abc", text)
	}
}

func TestRunOutputLimit(t *testing.T) {
	name, args := helperCommand("big", "2048")
	_, err := execx.RunOutput(context.Background(), name, args, execx.WithOutputLimit(10))
	if !errors.Is(err, execx.ErrOutputLimitExceeded) {
		t.Fatalf("RunOutput() error = %v, want output limit", err)
	}
}

func TestRunStreamSinkErrorStopsCommand(t *testing.T) {
	name, args := helperCommand("loop")
	sinkErr := errors.New("sink closed")
	_, err := execx.RunStream(context.Background(), name, args, execx.SinkFunc(func(ctx context.Context, event execx.Event) error {
		return sinkErr
	}))
	if !errors.Is(err, sinkErr) {
		t.Fatalf("RunStream() error = %v, want sink error", err)
	}
}

func TestStartStopRecentEvents(t *testing.T) {
	name, args := helperCommand("loop")
	p, err := execx.Start(context.Background(), name, args, execx.WithRingBuffer(5))
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	time.Sleep(120 * time.Millisecond)
	if !p.Running() {
		t.Fatal("process should be running")
	}
	if len(p.RecentEvents()) == 0 {
		t.Fatal("RecentEvents() should contain output")
	}
	stopCtx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := p.Stop(stopCtx); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}
	result, _ := p.Wait()
	if result.PID == 0 {
		t.Fatal("Wait() result should include PID")
	}
	if p.Running() {
		t.Fatal("process should not be running after Stop")
	}
}

func TestHelperProcess(t *testing.T) {
	args := helperArgs()
	if len(args) == 0 {
		return
	}
	switch args[0] {
	case "ok":
		os.Exit(0)
	case "stdout-stderr":
		fmt.Fprintln(os.Stdout, "stdout-line")
		fmt.Fprintln(os.Stderr, "stderr-line")
	case "exit":
		code, _ := strconv.Atoi(args[1])
		os.Exit(code)
	case "sleep":
		d, _ := time.ParseDuration(args[1])
		time.Sleep(d)
	case "crlf":
		fmt.Fprint(os.Stdout, "10%\r20%\n")
	case "stdin":
		_, _ = io.Copy(os.Stdout, os.Stdin)
	case "env":
		fmt.Fprint(os.Stdout, os.Getenv(args[1]))
	case "decode":
		fmt.Fprint(os.Stdout, "abc")
	case "big":
		n, _ := strconv.Atoi(args[1])
		fmt.Fprint(os.Stdout, strings.Repeat("x", n))
	case "loop":
		for {
			fmt.Fprintln(os.Stdout, "tick")
			time.Sleep(20 * time.Millisecond)
		}
	}
	os.Exit(0)
}

func helperCommand(args ...string) (string, []string) {
	cmdArgs := []string{"-test.run=TestHelperProcess", "--"}
	cmdArgs = append(cmdArgs, args...)
	return os.Args[0], cmdArgs
}

func helperArgs() []string {
	for i, arg := range os.Args {
		if arg == "--" {
			return os.Args[i+1:]
		}
	}
	return nil
}

func hasEvent(events []execx.Event, stream execx.Stream, text string) bool {
	for _, event := range events {
		if event.Stream == stream && event.Text == text {
			return true
		}
	}
	return false
}

func shellPrint(text string) string {
	if runtime.GOOS == "windows" {
		return "echo|set /p=" + text
	}
	return "printf " + text
}
