package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/huwenlong92/sdkit/core/sandbox"
)

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	runner, err := sandbox.New(sandbox.Options{
		AllowedImages: []string{"python:3.12-slim"},
	})
	if err != nil {
		log.Fatal(err)
	}

	result, err := runner.Run(ctx, &sandbox.RunRequest{
		SubmissionID: "demo-python-1",
		Language:     sandbox.LanguagePython,
		Files: []sandbox.File{
			{
				Path: "main.py",
				Content: []byte(`import sys
name = sys.stdin.read().strip() or "sandbox"
print(f"hello {name}")
`),
			},
		},
		Stdin:       []byte("sdkit\n"),
		Timeout:     2 * time.Minute,
		RunTimeout:  5 * time.Second,
		MemoryBytes: 128 << 20,
		CPUNano:     500_000_000,
	})
	if err != nil {
		log.Fatalf("sandbox run failed: %v\nstderr: %s", err, result.Stderr)
	}

	fmt.Printf("container=%s exit=%d timed_out=%v\n", result.ContainerID, result.ExitCode, result.TimedOut)
	fmt.Printf("stdout=%s", result.Stdout)
}
