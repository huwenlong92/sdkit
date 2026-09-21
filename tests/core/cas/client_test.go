package cas_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/huwenlong92/sdkit/core/cas"
)

const callback = "https://portal.example.edu/auth/cas/callback"
const success = `<cas:serviceResponse xmlns:cas="http://www.yale.edu/tp/cas"><cas:authenticationSuccess><cas:user>cas-user</cas:user><cas:attributes><cas:uid>school-user</cas:uid><cas:cn>测试用户</cas:cn><cas:mail>user@example.edu</cas:mail><cas:role>student</cas:role><cas:role>teacher</cas:role></cas:attributes></cas:authenticationSuccess></cas:serviceResponse>`

func TestURLs(t *testing.T) {
	client, err := cas.NewClient(cas.Config{Enabled: true, ServerURL: "https://cas.example.edu/authserver/", CallbackURL: callback})
	if err != nil {
		t.Fatal(err)
	}
	login, err := client.LoginURL("/teacher?tab=todo")
	if err != nil {
		t.Fatal(err)
	}
	u, err := url.Parse(login)
	if err != nil {
		t.Fatal(err)
	}
	service, err := client.ServiceURL("/teacher?tab=todo")
	if err != nil {
		t.Fatal(err)
	}
	if u.Path != "/authserver/login" || u.Query().Get("service") != service {
		t.Fatalf("unexpected login: %s", login)
	}
	logout, err := client.LogoutURL("https://portal.example.edu/login")
	if err != nil {
		t.Fatal(err)
	}
	u, err = url.Parse(logout)
	if err != nil {
		t.Fatal(err)
	}
	if u.Path != "/authserver/logout" || u.Query().Get("service") != "https://portal.example.edu/login" {
		t.Fatalf("unexpected logout: %s", logout)
	}
}

func TestValidationVersionsAndAttributes(t *testing.T) {
	for _, protocol := range []string{cas.ProtocolV2, cas.ProtocolV3} {
		for _, attribute := range []string{"", "UID"} {
			t.Run(protocol+"/"+attribute, func(t *testing.T) {
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					path := "/authserver/serviceValidate"
					if protocol == cas.ProtocolV3 {
						path = "/authserver/p3/serviceValidate"
					}
					if r.URL.Path != path || r.URL.Query().Get("ticket") != "ST-example" || r.URL.Query().Get("service") != callback {
						t.Errorf("incorrect validation request")
					}
					fmt.Fprint(w, success)
				}))
				defer server.Close()
				client, err := cas.NewClient(cas.Config{Enabled: true, ServerURL: server.URL + "/authserver", CallbackURL: callback, Protocol: protocol, SubjectAttribute: attribute})
				if err != nil {
					t.Fatal(err)
				}
				p, err := client.ValidateTicket(context.Background(), callback, "ST-example")
				if err != nil {
					t.Fatal(err)
				}
				want := "cas-user"
				if attribute != "" {
					want = "school-user"
				}
				if p.Subject != want || p.Name != "测试用户" || p.Email != "user@example.edu" || len(p.Attributes["role"]) != 2 {
					t.Fatalf("unexpected principal: %+v", p)
				}
				metadata := p.Metadata()
				metadata["role"].([]string)[0] = "changed"
				if p.Attributes["role"][0] != "student" {
					t.Fatal("metadata aliases attributes")
				}
			})
		}
	}
}

func TestRejectsInvalidResponses(t *testing.T) {
	for name, body := range map[string]string{
		"failure":         `<cas:serviceResponse xmlns:cas="http://www.yale.edu/tp/cas"><cas:authenticationFailure code="INVALID_TICKET">ST-secret</cas:authenticationFailure></cas:serviceResponse>`,
		"both":            strings.Replace(success, "</cas:serviceResponse>", "<cas:authenticationFailure>ST-secret</cas:authenticationFailure></cas:serviceResponse>", 1),
		"wrong-root":      strings.ReplaceAll(success, "cas:serviceResponse", "cas:other"),
		"wrong-namespace": strings.ReplaceAll(success, "http://www.yale.edu/tp/cas", "https://example.edu/other"),
		"missing-subject": strings.ReplaceAll(success, "<cas:user>cas-user</cas:user>", ""),
		"malformed":       "<xml>",
		"oversize":        strings.Repeat("x", (1<<20)+1),
	} {
		t.Run(name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { fmt.Fprint(w, body) }))
			defer server.Close()
			client, err := cas.NewClient(cas.Config{Enabled: true, ServerURL: server.URL, CallbackURL: callback})
			if err != nil {
				t.Fatal(err)
			}
			_, err = client.ValidateTicket(context.Background(), callback, "ST-secret")
			if err == nil || strings.Contains(err.Error(), "ST-secret") {
				t.Fatalf("unsafe validation result: %v", err)
			}
		})
	}
}

func TestRedirectIsNeverFollowed(t *testing.T) {
	var reached atomic.Bool
	destination := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { reached.Store(true); fmt.Fprint(w, success) }))
	defer destination.Close()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, destination.URL, http.StatusFound) }))
	defer server.Close()
	shared := server.Client()
	client, err := cas.NewClientWithHTTPClient(cas.Config{Enabled: true, ServerURL: server.URL, CallbackURL: callback}, shared)
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.ValidateTicket(context.Background(), callback, "ST-secret")
	if err == nil || reached.Load() {
		t.Fatalf("followed redirect: %v", err)
	}
	if shared.CheckRedirect != nil {
		t.Fatal("mutated shared client")
	}
}

func TestTimeoutAndCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) { <-r.Context().Done() }))
	defer server.Close()
	client, err := cas.NewClientWithHTTPClient(cas.Config{Enabled: true, ServerURL: server.URL, CallbackURL: callback, RequestTimeout: 30 * time.Millisecond}, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.ValidateTicket(context.Background(), callback, "ST-example")
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected timeout: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = client.ValidateTicket(ctx, callback, "ST-example")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected cancellation: %v", err)
	}
}

func TestConfigurationAndSafeRedirect(t *testing.T) {
	for _, raw := range []string{"", "//evil.example", "https://evil.example", "/%2fevil.example", "/%5cevil.example", "/%0aevil"} {
		if got := cas.SafeRedirect(raw); got != "/" {
			t.Errorf("unsafe redirect %q => %q", raw, got)
		}
	}
	if got := cas.SafeRedirect("/student?tab=todo"); got != "/student?tab=todo" {
		t.Errorf("valid redirect: %q", got)
	}
	for _, raw := range []string{"https://user:password@cas.example.edu", "https://cas.example.edu/#x", "https://cas.example.edu/?ticket=x", "file:///tmp/cas"} {
		if _, err := cas.NewClient(cas.Config{Enabled: true, ServerURL: raw, CallbackURL: callback}); err == nil {
			t.Errorf("accepted invalid server URL %q", raw)
		}
	}
	client, err := cas.NewClient(cas.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.LoginURL("/"); !errors.Is(err, cas.ErrDisabled) {
		t.Fatalf("expected disabled: %v", err)
	}
}
