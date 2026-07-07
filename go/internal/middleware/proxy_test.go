package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestTrustedProxy_ProdNoProxies_Error(t *testing.T) {
	mw, err := TrustedProxy("", true)
	if err == nil || mw != nil {
		t.Fatalf("expected (nil mw, error) in prod with empty TRUSTED_PROXIES; mwNil=%t err=%v", mw == nil, err)
	}
}

func TestTrustedProxy_DevNoProxies_FallsBackToRealIP(t *testing.T) {
	mw, err := TrustedProxy("", false)
	if err != nil || mw == nil {
		t.Fatalf("dev fallback should return a non-nil middleware and nil err; mwNil=%t err=%v", mw == nil, err)
	}
}

func TestTrustedProxy_InvalidCIDR_Error(t *testing.T) {
	if _, err := TrustedProxy("10.0.0.0/99", false); err == nil {
		t.Fatal("expected error for invalid CIDR")
	}
}

func TestTrustedProxy_InvalidIP_Error(t *testing.T) {
	if _, err := TrustedProxy("not-an-ip", false); err == nil {
		t.Fatal("expected error for invalid IP")
	}
}

// serveWith runs the middleware against a request with the given RemoteAddr +
// headers and returns the (possibly rewritten) RemoteAddr seen downstream.
func serveWith(t *testing.T, mw func(http.Handler) http.Handler, remoteAddr string, headers map[string]string) string {
	t.Helper()
	var seen string
	mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = r.RemoteAddr
	})).ServeHTTP(httptest.NewRecorder(), func() *http.Request {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.RemoteAddr = remoteAddr
		for k, v := range headers {
			req.Header.Set(k, v)
		}
		return req
	}())
	return seen
}

func TestTrustedProxy_Forwarding(t *testing.T) {
	// Exact IP (192.168.1.10) + CIDR (10.0.0.0/8) trusted; the empty token is skipped.
	mw, err := TrustedProxy("192.168.1.10, 10.0.0.0/8, ", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	cases := []struct {
		name       string
		remoteAddr string
		headers    map[string]string
		want       string
	}{
		{"trusted exact IP + XFF", "192.168.1.10:5000", map[string]string{"X-Forwarded-For": "1.2.3.4, 9.9.9.9"}, "1.2.3.4:0"},
		{"trusted CIDR + XFF", "10.5.6.7:5000", map[string]string{"X-Forwarded-For": "1.2.3.4"}, "1.2.3.4:0"},
		{"trusted CIDR + X-Real-IP only", "10.5.6.7:5000", map[string]string{"X-Real-IP": "5.6.7.8"}, "5.6.7.8:0"},
		{"trusted but no proxy headers", "10.5.6.7:5000", nil, "10.5.6.7:5000"},
		{"untrusted peer ignores XFF", "8.8.8.8:5000", map[string]string{"X-Forwarded-For": "1.2.3.4"}, "8.8.8.8:5000"},
		{"unparseable remote addr (host empty)", "garbage", map[string]string{"X-Forwarded-For": "1.2.3.4"}, "garbage"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := serveWith(t, mw, tc.remoteAddr, tc.headers); got != tc.want {
				t.Errorf("RemoteAddr: want %q, got %q", tc.want, got)
			}
		})
	}
}

func TestClientIPKey(t *testing.T) {
	cases := []struct {
		name       string
		remoteAddr string
		want       string
	}{
		{"IPv4 host:port", "1.2.3.4:5000", "1.2.3.4"},
		{"IPv4 sans port", "1.2.3.4", "1.2.3.4"},
		{"IPv6 bracketé regroupé par /64", "[2001:db8:1:2:3:4:5:6]:5000", "2001:db8:1:2::"},
		{"vide", "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			req.RemoteAddr = tc.remoteAddr
			got, err := ClientIPKey(req)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Errorf("ClientIPKey(%q): want %q, got %q", tc.remoteAddr, tc.want, got)
			}
		})
	}
}

func TestFormatRemoteAddr(t *testing.T) {
	cases := map[string]string{
		"":            ":0",
		"1.2.3.4":     "1.2.3.4:0",
		"::1":         "[::1]:0",
		"[2001:db8::1]": "[2001:db8::1]:0",
	}
	for in, want := range cases {
		if got := formatRemoteAddr(in); got != want {
			t.Errorf("formatRemoteAddr(%q): want %q, got %q", in, want, got)
		}
	}
}
