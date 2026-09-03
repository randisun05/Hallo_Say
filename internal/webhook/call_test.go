package webhook

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCallRejectsPrivateHosts(t *testing.T) {
	cases := []string{
		"http://localhost:8080/",
		"http://127.0.0.1/",
		"http://169.254.169.254/latest/meta-data/",
		"ftp://example.com",
		"not-a-url",
	}
	for _, target := range cases {
		if _, err := Call(target, "GET", ""); err == nil {
			t.Errorf("Call(%q) succeeded, want error", target)
		}
	}
}

func TestCallRejectsBadMethod(t *testing.T) {
	server := httptest.NewServer(nil)
	defer server.Close()

	if _, err := Call(server.URL, "TRACE", ""); err == nil {
		t.Errorf("Call with method TRACE succeeded, want error")
	} else if !strings.Contains(err.Error(), "not allowed") {
		t.Errorf("unexpected error: %v", err)
	}
}
