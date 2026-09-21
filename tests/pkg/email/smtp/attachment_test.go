package smtp_test

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net"
	"net/mail"
	"net/textproto"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/huwenlong92/sdkit/pkg/email"
	smtpdriver "github.com/huwenlong92/sdkit/pkg/email/driver/smtp"
)

func TestProviderStreamsAttachmentAndClosesSource(t *testing.T) {
	t.Parallel()

	host, port, messages, serverErrors := startSMTPServer(t)
	provider, err := smtpdriver.New("test", email.ProviderConfig{
		Host: host, Port: port, FromAddress: "sender@example.com", FromName: "测试发件人", Timeout: 3 * time.Second,
	})
	if err != nil {
		t.Fatalf("new provider: %v", err)
	}
	defer provider.Close()

	content := bytes.Repeat([]byte("attachment-stream-"), 128)
	var openCount atomic.Int64
	var closed atomic.Bool
	result, err := provider.Send(context.Background(), email.Payload{
		To: []string{"receiver@example.com"}, Subject: "带附件测试", HTML: "<p>正文</p>",
		Attachments: []email.Attachment{{
			Name: "测试报告.txt", ContentType: "text/plain; charset=utf-8", Size: int64(len(content)),
			Source: email.AttachmentSourceFunc(func(context.Context) (io.ReadCloser, error) {
				openCount.Add(1)
				return &trackingReadCloser{Reader: bytes.NewReader(content), closed: &closed}, nil
			}),
		}},
	})
	if err != nil {
		t.Fatalf("send: %v", err)
	}
	if result == nil || result.Raw != nil {
		t.Fatalf("result = %#v, raw MIME must not be retained", result)
	}
	if openCount.Load() != 1 || !closed.Load() {
		t.Fatalf("attachment source opens=%d closed=%v", openCount.Load(), closed.Load())
	}

	select {
	case raw := <-messages:
		name, body := readAttachment(t, raw)
		if name != "测试报告.txt" {
			t.Fatalf("attachment name = %q", name)
		}
		if !bytes.Equal(body, content) {
			t.Fatalf("attachment content mismatch: got %d bytes, want %d", len(body), len(content))
		}
	case err := <-serverErrors:
		t.Fatalf("smtp server: %v", err)
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for SMTP message")
	}
}

func TestProviderReturnsKnownNotSentErrorWhenAttachmentCannotOpen(t *testing.T) {
	t.Parallel()

	provider, err := smtpdriver.New("test", email.ProviderConfig{
		Host: "127.0.0.1", Port: 1, FromAddress: "sender@example.com", Timeout: time.Second,
	})
	if err != nil {
		t.Fatalf("new provider: %v", err)
	}
	sourceErr := errors.New("object not found")
	_, err = provider.Send(context.Background(), email.Payload{
		To: []string{"receiver@example.com"}, Subject: "test", Text: "body",
		Attachments: []email.Attachment{{
			Name: "missing.pdf", Size: 10,
			Source: email.AttachmentSourceFunc(func(context.Context) (io.ReadCloser, error) {
				return nil, sourceErr
			}),
		}},
	})
	if !errors.Is(err, email.ErrAttachmentOpen) || !errors.Is(err, sourceErr) {
		t.Fatalf("send error = %v", err)
	}
}

type trackingReadCloser struct {
	io.Reader
	closed *atomic.Bool
}

func (r *trackingReadCloser) Close() error {
	r.closed.Store(true)
	return nil
}

func startSMTPServer(t *testing.T) (string, int, <-chan []byte, <-chan error) {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	messages := make(chan []byte, 1)
	errorsChannel := make(chan error, 1)
	go func() {
		conn, acceptErr := listener.Accept()
		if acceptErr != nil {
			if !errors.Is(acceptErr, net.ErrClosed) {
				errorsChannel <- acceptErr
			}
			return
		}
		defer conn.Close()
		if serveErr := serveSMTP(conn, messages); serveErr != nil {
			errorsChannel <- serveErr
		}
	}()
	host, portText, err := net.SplitHostPort(listener.Addr().String())
	if err != nil {
		t.Fatalf("split address: %v", err)
	}
	port, err := strconv.Atoi(portText)
	if err != nil {
		t.Fatalf("parse port: %v", err)
	}
	return host, port, messages, errorsChannel
}

func serveSMTP(conn net.Conn, messages chan<- []byte) error {
	reader := bufio.NewReader(conn)
	writer := bufio.NewWriter(conn)
	writeLine := func(value string) error {
		if _, err := writer.WriteString(value + "\r\n"); err != nil {
			return err
		}
		return writer.Flush()
	}
	if err := writeLine("220 localhost ESMTP test"); err != nil {
		return err
	}
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return err
		}
		command := strings.ToUpper(strings.TrimSpace(line))
		switch {
		case strings.HasPrefix(command, "EHLO"), strings.HasPrefix(command, "HELO"):
			if _, err := writer.WriteString("250-localhost\r\n250 OK\r\n"); err != nil {
				return err
			}
			if err := writer.Flush(); err != nil {
				return err
			}
		case strings.HasPrefix(command, "MAIL FROM:"), strings.HasPrefix(command, "RCPT TO:"):
			if err := writeLine("250 OK"); err != nil {
				return err
			}
		case command == "DATA":
			if err := writeLine("354 End data with <CR><LF>.<CR><LF>"); err != nil {
				return err
			}
			raw, err := textproto.NewReader(reader).ReadDotBytes()
			if err != nil {
				return err
			}
			messages <- raw
			if err := writeLine("250 queued"); err != nil {
				return err
			}
		case command == "QUIT":
			return writeLine("221 bye")
		default:
			return fmt.Errorf("unexpected SMTP command %q", command)
		}
	}
}

func readAttachment(t *testing.T, raw []byte) (string, []byte) {
	t.Helper()
	message, err := mail.ReadMessage(bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("read message: %v", err)
	}
	mediaType, params, err := mime.ParseMediaType(message.Header.Get("Content-Type"))
	if err != nil || mediaType != "multipart/mixed" {
		t.Fatalf("content type = %q, params=%v, error=%v", mediaType, params, err)
	}
	parts := multipart.NewReader(message.Body, params["boundary"])
	for {
		part, err := parts.NextPart()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatalf("next MIME part: %v", err)
		}
		if part.FileName() == "" {
			continue
		}
		reader := io.Reader(part)
		if strings.EqualFold(part.Header.Get("Content-Transfer-Encoding"), "base64") {
			reader = base64.NewDecoder(base64.StdEncoding, part)
		}
		body, err := io.ReadAll(reader)
		if err != nil {
			t.Fatalf("read attachment: %v", err)
		}
		return part.FileName(), body
	}
	t.Fatal("attachment MIME part not found")
	return "", nil
}
