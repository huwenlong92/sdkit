package fingerprint

import (
	"net/http/httptest"
	"testing"
)

func TestFromRequestAndHash(t *testing.T) {
	req := httptest.NewRequest("GET", "/x", nil)
	req.Header.Set("X-Forwarded-For", "1.1.1.1, 2.2.2.2")
	req.Header.Set("User-Agent", "ua")
	req.Header.Set("X-Device-ID", "dev")
	info := FromRequest(req)
	if info.IP != "1.1.1.1" || info.UA != "ua" || info.DeviceID != "dev" {
		t.Fatalf("bad info: %+v", info)
	}
	if Hash(info) == "" {
		t.Fatalf("empty hash")
	}
}
