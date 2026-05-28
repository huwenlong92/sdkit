package smtp

import (
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	"net"
	"net/mail"
	"net/smtp"
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
	raw, recipients, err := p.message(payload)
	if err != nil {
		return nil, err
	}
	if err := p.send(ctx, recipients, raw); err != nil {
		return nil, err
	}
	return &email.ProviderResult{Raw: raw}, nil
}

func (p *Provider) Close() error {
	return nil
}

func (p *Provider) send(ctx context.Context, recipients []string, raw []byte) error {
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
	if _, err := writer.Write(raw); err != nil {
		_ = writer.Close()
		return err
	}
	if err := writer.Close(); err != nil {
		return err
	}
	return client.Quit()
}

func (p *Provider) message(payload email.Payload) ([]byte, []string, error) {
	from := mail.Address{Name: p.config.FromName, Address: p.config.FromAddress}
	to, err := parseAddresses(payload.To)
	if err != nil {
		return nil, nil, err
	}
	cc, err := parseAddresses(payload.Cc)
	if err != nil {
		return nil, nil, err
	}
	bcc, err := parseAddresses(payload.Bcc)
	if err != nil {
		return nil, nil, err
	}
	recipients := make([]string, 0, len(to)+len(cc)+len(bcc))
	recipients = appendAddressValues(recipients, to)
	recipients = appendAddressValues(recipients, cc)
	recipients = appendAddressValues(recipients, bcc)
	if len(recipients) == 0 {
		return nil, nil, errors.New("smtp: recipient is required")
	}

	var body bytes.Buffer
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
	if payload.HTML != "" && payload.Text != "" {
		writer := multipart.NewWriter(&body)
		headers["Content-Type"] = `multipart/alternative; boundary="` + writer.Boundary() + `"`
		if err := writePart(writer, "text/plain; charset=UTF-8", payload.Text); err != nil {
			return nil, nil, err
		}
		if err := writePart(writer, "text/html; charset=UTF-8", payload.HTML); err != nil {
			return nil, nil, err
		}
		if err := writer.Close(); err != nil {
			return nil, nil, err
		}
	} else if payload.HTML != "" {
		headers["Content-Type"] = "text/html; charset=UTF-8"
		headers["Content-Transfer-Encoding"] = "quoted-printable"
		if err := writeQuotedPrintable(&body, payload.HTML); err != nil {
			return nil, nil, err
		}
	} else {
		headers["Content-Type"] = "text/plain; charset=UTF-8"
		headers["Content-Transfer-Encoding"] = "quoted-printable"
		if err := writeQuotedPrintable(&body, payload.Text); err != nil {
			return nil, nil, err
		}
	}

	var raw bytes.Buffer
	for _, key := range headerOrder(headers) {
		raw.WriteString(key)
		raw.WriteString(": ")
		raw.WriteString(headers[key])
		raw.WriteString("\r\n")
	}
	raw.WriteString("\r\n")
	raw.Write(body.Bytes())
	return raw.Bytes(), recipients, nil
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
