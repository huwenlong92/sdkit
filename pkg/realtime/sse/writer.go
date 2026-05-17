package sse

import (
	"bytes"
	"fmt"
	"io"
)

func WriteEvent(w io.Writer, event string, data []byte) error {
	if event != "" {
		if _, err := fmt.Fprintf(w, "event: %s\n", event); err != nil {
			return err
		}
	}
	lines := bytes.Split(data, []byte("\n"))
	for _, line := range lines {
		if _, err := fmt.Fprintf(w, "data: %s\n", line); err != nil {
			return err
		}
	}
	_, err := io.WriteString(w, "\n")
	return err
}

func WriteComment(w io.Writer, comment string) error {
	_, err := fmt.Fprintf(w, ": %s\n\n", comment)
	return err
}
