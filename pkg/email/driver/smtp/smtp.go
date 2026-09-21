package smtp

import (
	"context"
	"crypto/tls"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	"net"
	"net/mail"
	"net/smtp"
	"net/textproto"
	"path"
	"strings"
	"time"

	"github.com/huwenlong92/sdkit/pkg/email"
)

func init() {
	email.RegisterDriver("smtp", New)
}

type Provider struct {
	name   string
	config email.ProviderConfig
}

func New(name string, cfg email.ProviderConfig) (email.Provider, error) {
	if cfg.Port == 0 {
		cfg.Port = 25
	}
	if cfg.Timeout == 0 {
		cfg.Timeout = 10 * time.Second
	}
	return &Provider{name: name, config: cfg}, nil
}

func (p *Provider) Send(ctx context.Context, payload email.Payload) (*email.ProviderResult, error) {
	recipients, err := messageRecipients(payload)
	if err != nil {
		return nil, err
	}
	attachments, err := openAttachments(ctx, payload.Attachments)
	if err != nil {
		return nil, err
	}
	defer closeAttachments(attachments)

	if err := p.send(ctx, recipients, func(writer io.Writer) error {
		return p.writeMessage(ctx, writer, payload, attachments)
	}); err != nil {
		return nil, err
	}
	return &email.ProviderResult{}, nil
}

func (p *Provider) Close() error {
	return nil
}

func (p *Provider) send(ctx context.Context, recipients []string, writeMessage func(io.Writer) error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	address := fmt.Sprintf("%s:%d", p.config.Host, p.config.Port)
	dialer := net.Dialer{Timeout: p.config.Timeout}
	var conn net.Conn
	var err error
	switch strings.ToLower(strings.TrimSpace(p.config.Encryption)) {
	case "ssl", "tls":
		conn, err = tls.DialWithDialer(&dialer, "tcp", address, &tls.Config{ServerName: p.config.Host})
	default:
		conn, err = dialer.DialContext(ctx, "tcp", address)
	}
	if err != nil {
		return err
	}
	defer conn.Close()

	client, err := smtp.NewClient(conn, p.config.Host)
	if err != nil {
		return err
	}
	defer client.Close()

	switch strings.ToLower(strings.TrimSpace(p.config.Encryption)) {
	case "starttls", "tls_mandatory", "mandatory":
		if ok, _ := client.Extension("STARTTLS"); !ok {
			return errors.New("smtp: starttls is not supported")
		}
		if err := client.StartTLS(&tls.Config{ServerName: p.config.Host}); err != nil {
			return err
		}
	default:
		if ok, _ := client.Extension("STARTTLS"); ok {
			if err := client.StartTLS(&tls.Config{ServerName: p.config.Host}); err != nil {
				return err
			}
		}
	}

	if p.config.Username != "" {
		auth := smtp.PlainAuth("", p.config.Username, p.config.Password, p.config.Host)
		if err := client.Auth(auth); err != nil {
			return err
		}
	}
	if err := client.Mail(p.config.FromAddress); err != nil {
		return err
	}
	for _, recipient := range recipients {
		if err := client.Rcpt(recipient); err != nil {
			return err
		}
	}
	writer, err := client.Data()
	if err != nil {
		return err
	}
	if err := writeMessage(writer); err != nil {
		_ = writer.Close()
		return err
	}
	if err := writer.Close(); err != nil {
		return err
	}
	return client.Quit()
}

func messageRecipients(payload email.Payload) ([]string, error) {
	to, err := parseAddresses(payload.To)
	if err != nil {
		return nil, err
	}
	cc, err := parseAddresses(payload.Cc)
	if err != nil {
		return nil, err
	}
	bcc, err := parseAddresses(payload.Bcc)
	if err != nil {
		return nil, err
	}
	recipients := make([]string, 0, len(to)+len(cc)+len(bcc))
	recipients = appendAddressValues(recipients, to)
	recipients = appendAddressValues(recipients, cc)
	recipients = appendAddressValues(recipients, bcc)
	if len(recipients) == 0 {
		return nil, errors.New("smtp: recipient is required")
	}
	return recipients, nil
}

type openedAttachment struct {
	name        string
	contentType string
	size        int64
	reader      io.ReadCloser
}

func openAttachments(ctx context.Context, attachments []email.Attachment) ([]openedAttachment, error) {
	opened := make([]openedAttachment, 0, len(attachments))
	for _, attachment := range attachments {
		name := attachmentName(attachment.Name)
		if name == "" || attachment.Size < 0 || attachment.Source == nil {
			closeAttachments(opened)
			return nil, fmt.Errorf("%w: %s", email.ErrAttachmentInvalid, name)
		}
		reader, err := attachment.Source.Open(ctx)
		if err != nil {
			closeAttachments(opened)
			return nil, &email.AttachmentOpenError{Name: name, Err: err}
		}
		if reader == nil {
			closeAttachments(opened)
			return nil, &email.AttachmentOpenError{Name: name, Err: email.ErrAttachmentInvalid}
		}
		opened = append(opened, openedAttachment{
			name: name, contentType: attachmentContentType(attachment.ContentType), size: attachment.Size, reader: reader,
		})
	}
	return opened, nil
}

func closeAttachments(attachments []openedAttachment) {
	for _, attachment := range attachments {
		if attachment.reader != nil {
			_ = attachment.reader.Close()
		}
	}
}

func attachmentName(value string) string {
	value = strings.TrimSpace(strings.ReplaceAll(value, "\\", "/"))
	value = path.Base(value)
	if value == "." || value == "/" {
		return ""
	}
	return value
}

func attachmentContentType(value string) string {
	value = strings.TrimSpace(value)
	mediaType, _, err := mime.ParseMediaType(value)
	if err != nil || mediaType == "" {
		return "application/octet-stream"
	}
	return mediaType
}

func (p *Provider) writeMessage(ctx context.Context, output io.Writer, payload email.Payload, attachments []openedAttachment) error {
	from := mail.Address{Name: p.config.FromName, Address: p.config.FromAddress}
	to, err := parseAddresses(payload.To)
	if err != nil {
		return err
	}
	cc, err := parseAddresses(payload.Cc)
	if err != nil {
		return err
	}

	headers := map[string]string{
		"From":         from.String(),
		"To":           joinAddresses(to),
		"Subject":      mime.QEncoding.Encode("UTF-8", payload.Subject),
		"Date":         time.Now().Format(time.RFC1123Z),
		"MIME-Version": "1.0",
	}
	if len(cc) > 0 {
		headers["Cc"] = joinAddresses(cc)
	}
	if p.config.ReplyTo != "" {
		headers["Reply-To"] = p.config.ReplyTo
	}
	for key, value := range payload.Headers {
		headers[key] = value
	}
	if len(attachments) > 0 {
		mixed := multipart.NewWriter(output)
		headers["Content-Type"] = `multipart/mixed; boundary="` + mixed.Boundary() + `"`
		delete(headers, "Content-Transfer-Encoding")
		if err := writeHeaders(output, headers); err != nil {
			return err
		}
		if err := writeMixedBody(mixed, payload); err != nil {
			return err
		}
		for _, attachment := range attachments {
			if err := writeAttachment(ctx, mixed, attachment); err != nil {
				return err
			}
		}
		return mixed.Close()
	}

	return writeBody(output, headers, payload)
}

func writeHeaders(writer io.Writer, headers map[string]string) error {
	for _, key := range headerOrder(headers) {
		if _, err := io.WriteString(writer, key+": "+headers[key]+"\r\n"); err != nil {
			return err
		}
	}
	_, err := io.WriteString(writer, "\r\n")
	return err
}

func writeBody(output io.Writer, headers map[string]string, payload email.Payload) error {
	if payload.HTML != "" && payload.Text != "" {
		alternative := multipart.NewWriter(output)
		headers["Content-Type"] = `multipart/alternative; boundary="` + alternative.Boundary() + `"`
		delete(headers, "Content-Transfer-Encoding")
		if err := writeHeaders(output, headers); err != nil {
			return err
		}
		if err := writePart(alternative, "text/plain; charset=UTF-8", payload.Text); err != nil {
			return err
		}
		if err := writePart(alternative, "text/html; charset=UTF-8", payload.HTML); err != nil {
			return err
		}
		return alternative.Close()
	}
	contentType, body := "text/plain; charset=UTF-8", payload.Text
	if payload.HTML != "" {
		contentType, body = "text/html; charset=UTF-8", payload.HTML
	}
	headers["Content-Type"] = contentType
	headers["Content-Transfer-Encoding"] = "quoted-printable"
	if err := writeHeaders(output, headers); err != nil {
		return err
	}
	return writeQuotedPrintable(output, body)
}

func writeMixedBody(mixed *multipart.Writer, payload email.Payload) error {
	if payload.HTML != "" && payload.Text != "" {
		boundary := multipart.NewWriter(io.Discard).Boundary()
		part, err := mixed.CreatePart(textproto.MIMEHeader{
			"Content-Type": {`multipart/alternative; boundary="` + boundary + `"`},
		})
		if err != nil {
			return err
		}
		alternative := multipart.NewWriter(part)
		if err := alternative.SetBoundary(boundary); err != nil {
			return err
		}
		if err := writePart(alternative, "text/plain; charset=UTF-8", payload.Text); err != nil {
			return err
		}
		if err := writePart(alternative, "text/html; charset=UTF-8", payload.HTML); err != nil {
			return err
		}
		return alternative.Close()
	}
	contentType, body := "text/plain; charset=UTF-8", payload.Text
	if payload.HTML != "" {
		contentType, body = "text/html; charset=UTF-8", payload.HTML
	}
	part, err := mixed.CreatePart(textproto.MIMEHeader{
		"Content-Type":              {contentType},
		"Content-Transfer-Encoding": {"quoted-printable"},
	})
	if err != nil {
		return err
	}
	return writeQuotedPrintable(part, body)
}

func writeAttachment(ctx context.Context, mixed *multipart.Writer, attachment openedAttachment) error {
	part, err := mixed.CreatePart(textproto.MIMEHeader{
		"Content-Type":              {mime.FormatMediaType(attachment.contentType, map[string]string{"name": attachment.name})},
		"Content-Disposition":       {mime.FormatMediaType("attachment", map[string]string{"filename": attachment.name})},
		"Content-Transfer-Encoding": {"base64"},
	})
	if err != nil {
		return err
	}
	lineWriter := &base64LineWriter{writer: part}
	encoder := base64.NewEncoder(base64.StdEncoding, lineWriter)
	written, copyErr := io.Copy(encoder, contextReader{ctx: ctx, reader: attachment.reader})
	closeErr := encoder.Close()
	lineErr := lineWriter.Close()
	if copyErr != nil {
		return copyErr
	}
	if closeErr != nil {
		return closeErr
	}
	if lineErr != nil {
		return lineErr
	}
	if attachment.size > 0 && written != attachment.size {
		return fmt.Errorf("smtp: attachment %s size changed: got %d, want %d", attachment.name, written, attachment.size)
	}
	return nil
}

type contextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (r contextReader) Read(buffer []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.reader.Read(buffer)
}

type base64LineWriter struct {
	writer io.Writer
	column int
}

func (w *base64LineWriter) Write(value []byte) (int, error) {
	written := 0
	for len(value) > 0 {
		if w.column == 76 {
			if _, err := io.WriteString(w.writer, "\r\n"); err != nil {
				return written, err
			}
			w.column = 0
		}
		count := min(76-w.column, len(value))
		n, err := w.writer.Write(value[:count])
		written += n
		w.column += n
		value = value[n:]
		if err != nil {
			return written, err
		}
		if n != count {
			return written, io.ErrShortWrite
		}
	}
	return written, nil
}

func (w *base64LineWriter) Close() error {
	if w.column == 0 {
		return nil
	}
	_, err := io.WriteString(w.writer, "\r\n")
	w.column = 0
	return err
}

func writePart(writer *multipart.Writer, contentType string, body string) error {
	part, err := writer.CreatePart(map[string][]string{
		"Content-Type":              {contentType},
		"Content-Transfer-Encoding": {"quoted-printable"},
	})
	if err != nil {
		return err
	}
	return writeQuotedPrintable(part, body)
}

func writeQuotedPrintable(writer io.Writer, value string) error {
	qp := quotedprintable.NewWriter(writer)
	if _, err := qp.Write([]byte(value)); err != nil {
		_ = qp.Close()
		return err
	}
	return qp.Close()
}

func parseAddresses(values []string) ([]mail.Address, error) {
	addresses := make([]mail.Address, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		address, err := mail.ParseAddress(value)
		if err != nil {
			return nil, err
		}
		addresses = append(addresses, *address)
	}
	return addresses, nil
}

func appendAddressValues(values []string, addresses []mail.Address) []string {
	for _, address := range addresses {
		values = append(values, address.Address)
	}
	return values
}

func joinAddresses(addresses []mail.Address) string {
	values := make([]string, 0, len(addresses))
	for _, address := range addresses {
		values = append(values, address.String())
	}
	return strings.Join(values, ", ")
}

func headerOrder(headers map[string]string) []string {
	preferred := []string{"From", "To", "Cc", "Reply-To", "Subject", "Date", "MIME-Version", "Content-Type", "Content-Transfer-Encoding"}
	keys := make([]string, 0, len(headers))
	seen := make(map[string]struct{}, len(headers))
	for _, key := range preferred {
		if _, ok := headers[key]; ok {
			keys = append(keys, key)
			seen[key] = struct{}{}
		}
	}
	for key := range headers {
		if _, ok := seen[key]; !ok {
			keys = append(keys, key)
		}
	}
	return keys
}
