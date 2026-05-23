package execx

import (
	"errors"
	"fmt"
	"time"
)

type Stream string

const (
	StreamStdout Stream = "stdout"
	StreamStderr Stream = "stderr"
)

type SplitMode int

const (
	SplitLine SplitMode = iota
	SplitCRLF
	SplitChunk
)

var (
	ErrOutputLimitExceeded = errors.New("execx: output limit exceeded")
	ErrProcessNotRunning   = errors.New("execx: process is not running")
	ErrProcessRunning      = errors.New("execx: process is already running")
)

type Event struct {
	Stream    Stream
	Data      []byte
	Text      string
	Time      time.Time
	DecodeErr error
}

type Result struct {
	Command    string
	Args       []string
	Dir        string
	PID        int
	ExitCode   int
	StartedAt  time.Time
	FinishedAt time.Time
}

type OutputResult struct {
	Result
	Stdout   []byte
	Stderr   []byte
	Combined []byte
}

type ExitError struct {
	Result Result
	Err    error
}

func (e *ExitError) Error() string {
	if e == nil {
		return ""
	}
	if e.Err == nil {
		return fmt.Sprintf("execx: command exited with code %d", e.Result.ExitCode)
	}
	return fmt.Sprintf("execx: command exited with code %d: %v", e.Result.ExitCode, e.Err)
}

func (e *ExitError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}
