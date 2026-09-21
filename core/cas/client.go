package cas

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	ProtocolV2 = "2"
	ProtocolV3 = "3"

	defaultRequestTimeout = 10 * time.Second
	maxResponseBytes      = 1 << 20
)

var (
	ErrDisabled             = errors.New("cas login is disabled")
	ErrAuthenticationFailed = errors.New("cas authentication failed")
)

type Config struct {
	Enabled          bool          `mapstructure:"enabled" yaml:"enabled"`
	ServerURL        string        `mapstructure:"server_url" yaml:"server_url"`
	CallbackURL      string        `mapstructure:"callback_url" yaml:"callback_url"`
	RequestTimeout   time.Duration `mapstructure:"request_timeout" yaml:"request_timeout"`
	Protocol         string        `mapstructure:"protocol" yaml:"protocol"`
	SubjectAttribute string        `mapstructure:"subject_attribute" yaml:"subject_attribute"`
}

func (c Config) Normalized() Config {
	c.ServerURL = strings.TrimRight(strings.TrimSpace(c.ServerURL), "/")
	c.CallbackURL = strings.TrimSpace(c.CallbackURL)
	c.Protocol = strings.TrimSpace(c.Protocol)
	c.SubjectAttribute = strings.TrimSpace(c.SubjectAttribute)
	if c.Protocol == "" {
		c.Protocol = ProtocolV2
	}
	if c.RequestTimeout <= 0 {
		c.RequestTimeout = defaultRequestTimeout
	}
	return c
}

func (c Config) Validate() error {
	c = c.Normalized()
	if !c.Enabled {
		return nil
	}
	if c.Protocol != ProtocolV2 && c.Protocol != ProtocolV3 {
		return errors.New("cas protocol must be 2 or 3")
	}
	if err := validateHTTPURL("server_url", c.ServerURL); err != nil {
		return err
	}
	if err := validateHTTPURL("callback_url", c.CallbackURL); err != nil {
		return err
	}
	return nil
}

func validateHTTPURL(name string, raw string) error {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Hostname() == "" || parsed.User != nil || parsed.Fragment != "" || parsed.Opaque != "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return fmt.Errorf("cas %s must be an absolute http(s) URL", name)
	}
	if name == "server_url" && parsed.RawQuery != "" {
		return errors.New("cas server_url must not contain query parameters")
	}
	return nil
}

type Principal struct {
	Subject    string
	Username   string
	Name       string
	Email      string
	Attributes map[string][]string
}

func (p Principal) Metadata() map[string]any {
	metadata := make(map[string]any, len(p.Attributes)+2)
	for key, values := range p.Attributes {
		cloned := append([]string(nil), values...)
		if len(cloned) == 1 {
			metadata[key] = cloned[0]
		} else {
			metadata[key] = cloned
		}
	}
	metadata["subject"] = p.Subject
	if p.Name != "" {
		metadata["name"] = p.Name
	}
	return metadata
}

type Client struct {
	config     Config
	httpClient *http.Client
}

func NewClient(config Config) (*Client, error) {
	config = config.Normalized()
	if err := config.Validate(); err != nil {
		return nil, err
	}
	return &Client{
		config:     config,
		httpClient: &http.Client{Timeout: config.RequestTimeout, CheckRedirect: rejectRedirect},
	}, nil
}

func NewClientWithHTTPClient(config Config, httpClient *http.Client) (*Client, error) {
	client, err := NewClient(config)
	if err != nil {
		return nil, err
	}
	if httpClient != nil {
		// Do not mutate a shared HTTP client; validation must never forward tickets to redirects.
		cloned := *httpClient
		cloned.CheckRedirect = rejectRedirect
		client.httpClient = &cloned
	}
	return client, nil
}

func rejectRedirect(_ *http.Request, _ []*http.Request) error {
	return http.ErrUseLastResponse
}

// LogoutURL builds the CAS logout address. The caller must clear its local session separately.
// returnURL must come from trusted application configuration, never unchecked request input.
func (c *Client) LogoutURL(returnURL string) (string, error) {
	endpoint, err := c.endpoint("logout")
	if err != nil {
		return "", err
	}
	if returnURL != "" {
		if err := validateHTTPURL("logout_return_url", returnURL); err != nil {
			return "", err
		}
		query := endpoint.Query()
		query.Set("service", returnURL)
		endpoint.RawQuery = query.Encode()
	}
	return endpoint.String(), nil
}

func (c *Client) ServiceURL(redirect string) (string, error) {
	if c == nil || !c.config.Enabled {
		return "", ErrDisabled
	}
	service, err := url.Parse(c.config.CallbackURL)
	if err != nil {
		return "", err
	}
	query := service.Query()
	query.Set("redirect", SafeRedirect(redirect))
	service.RawQuery = query.Encode()
	return service.String(), nil
}

func (c *Client) LoginURL(redirect string) (string, error) {
	serviceURL, err := c.ServiceURL(redirect)
	if err != nil {
		return "", err
	}
	endpoint, err := c.endpoint("login")
	if err != nil {
		return "", err
	}
	query := endpoint.Query()
	query.Set("service", serviceURL)
	endpoint.RawQuery = query.Encode()
	return endpoint.String(), nil
}

func (c *Client) ValidateTicket(ctx context.Context, serviceURL string, ticket string) (Principal, error) {
	principal := Principal{}
	if c == nil || !c.config.Enabled {
		return principal, ErrDisabled
	}
	ticket = strings.TrimSpace(ticket)
	if ticket == "" {
		return principal, fmt.Errorf("%w: ticket is empty", ErrAuthenticationFailed)
	}
	if err := validateHTTPURL("service_url", serviceURL); err != nil {
		return principal, err
	}
	action := "serviceValidate"
	if c.config.Protocol == ProtocolV3 {
		action = "p3/serviceValidate"
	}
	endpoint, err := c.endpoint(action)
	if err != nil {
		return principal, err
	}
	query := endpoint.Query()
	query.Set("service", serviceURL)
	query.Set("ticket", ticket)
	endpoint.RawQuery = query.Encode()

	ctx, cancel := context.WithTimeout(ctx, c.config.RequestTimeout)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return principal, errors.New("create cas validation request failed")
	}
	response, err := c.httpClient.Do(request)
	if err != nil {
		if ctx.Err() != nil {
			return principal, ctx.Err()
		}
		return principal, errors.New("cas validation request failed")
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return principal, fmt.Errorf("validate cas ticket: unexpected status %d", response.StatusCode)
	}

	limited := io.LimitReader(response.Body, maxResponseBytes+1)
	body, err := io.ReadAll(limited)
	if err != nil {
		return principal, fmt.Errorf("read cas validation response: %w", err)
	}
	if len(body) > maxResponseBytes {
		return principal, errors.New("cas validation response is too large")
	}
	var payload serviceResponse
	if err := xml.Unmarshal(body, &payload); err != nil {
		return principal, fmt.Errorf("decode cas validation response: %w", err)
	}
	if payload.Success == nil || payload.Failure != nil {
		// Server messages may echo credentials or personal data; keep errors safe to log.
		return principal, ErrAuthenticationFailed
	}

	attributes := map[string][]string(payload.Success.Attributes)
	subject := strings.TrimSpace(payload.Success.User)
	if c.config.SubjectAttribute != "" {
		subject = firstAttribute(attributes, c.config.SubjectAttribute)
	}
	if subject == "" {
		return principal, fmt.Errorf("%w: subject is empty", ErrAuthenticationFailed)
	}
	name := firstAttribute(attributes, "cn", "displayName", "name")
	email := firstAttribute(attributes, "mail", "email")
	return Principal{
		Subject:    subject,
		Username:   subject,
		Name:       name,
		Email:      email,
		Attributes: attributes,
	}, nil
}

func (c *Client) endpoint(action string) (*url.URL, error) {
	if c == nil || !c.config.Enabled {
		return nil, ErrDisabled
	}
	endpoint, err := url.Parse(c.config.ServerURL)
	if err != nil {
		return nil, err
	}
	endpoint.Path = strings.TrimRight(endpoint.Path, "/") + "/" + strings.TrimLeft(action, "/")
	endpoint.RawQuery = ""
	endpoint.Fragment = ""
	return endpoint, nil
}

func SafeRedirect(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" || !strings.HasPrefix(raw, "/") || strings.HasPrefix(raw, "//") || strings.Contains(raw, "\\") {
		return "/"
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.IsAbs() || parsed.Host != "" || strings.HasPrefix(parsed.Path, "//") || strings.Contains(parsed.Path, "\\") || strings.ContainsAny(parsed.Path, "\r\n\x00") {
		return "/"
	}
	return parsed.String()
}

func firstAttribute(attributes map[string][]string, names ...string) string {
	for _, name := range names {
		for key, values := range attributes {
			if !strings.EqualFold(key, name) {
				continue
			}
			for _, value := range values {
				if value = strings.TrimSpace(value); value != "" {
					return value
				}
			}
		}
	}
	return ""
}

type serviceResponse struct {
	XMLName xml.Name               `xml:"http://www.yale.edu/tp/cas serviceResponse"`
	Success *authenticationSuccess `xml:"authenticationSuccess"`
	Failure *authenticationFailure `xml:"authenticationFailure"`
}

type authenticationSuccess struct {
	User       string        `xml:"user"`
	Attributes attributeList `xml:"attributes"`
}

type authenticationFailure struct {
	Code    string `xml:"code,attr"`
	Message string `xml:",chardata"`
}

type attributeList map[string][]string

func (a *attributeList) UnmarshalXML(decoder *xml.Decoder, start xml.StartElement) error {
	values := make(map[string][]string)
	for {
		token, err := decoder.Token()
		if err != nil {
			return err
		}
		switch item := token.(type) {
		case xml.StartElement:
			var value string
			if err := decoder.DecodeElement(&value, &item); err != nil {
				return err
			}
			value = strings.TrimSpace(value)
			if value != "" {
				values[item.Name.Local] = append(values[item.Name.Local], value)
			}
		case xml.EndElement:
			if item.Name == start.Name {
				*a = values
				return nil
			}
		}
	}
}
