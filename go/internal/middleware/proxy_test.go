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
// headers and returns (resolved client IP, RemoteAddr seen downstream).
func serveWith(t *testing.T, mw func(http.Handler) http.Handler, remoteAddr string, headers map[string]string) (string, string) {
	t.Helper()
	var seenIP, seenRemote string
	mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenIP = ClientIP(r)
		seenRemote = r.RemoteAddr
	})).ServeHTTP(httptest.NewRecorder(), func() *http.Request {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.RemoteAddr = remoteAddr
		for k, v := range headers {
			req.Header.Set(k, v)
		}
		return req
	}())
	return seenIP, seenRemote
}

func TestTrustedProxy_DevNoProxies_XFFAnySource(t *testing.T) {
	mw, err := TrustedProxy("", false)
	if err != nil || mw == nil {
		t.Fatalf("dev fallback should return a non-nil middleware and nil err; mwNil=%t err=%v", mw == nil, err)
	}

	cases := []struct {
		name       string
		remoteAddr string
		headers    map[string]string
		want       string
	}{
		{"XFF présent → entrée la plus à droite", "127.0.0.1:1234", map[string]string{"X-Forwarded-For": "1.2.3.4, 5.6.7.8"}, "5.6.7.8"},
		{"pas de XFF → RemoteAddr", "9.9.9.9:1234", nil, "9.9.9.9"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got, _ := serveWith(t, mw, tc.remoteAddr, tc.headers); got != tc.want {
				t.Errorf("ClientIP: want %q, got %q", tc.want, got)
			}
		})
	}
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
		// Parcours XFF droite→gauche (sémantique chi) : la dernière entrée non
		// approuvée est celle posée par notre proxy — l'entrée la plus à gauche
		// (forgeable par le client) est ignorée.
		{"trusted exact IP + XFF → entrée droite non approuvée", "192.168.1.10:5000", map[string]string{"X-Forwarded-For": "1.2.3.4, 9.9.9.9"}, "9.9.9.9"},
		{"trusted CIDR + XFF simple", "10.5.6.7:5000", map[string]string{"X-Forwarded-For": "1.2.3.4"}, "1.2.3.4"},
		{"XFF saute les hops de confiance", "192.168.1.10:5000", map[string]string{"X-Forwarded-For": "1.2.3.4, 10.9.9.9"}, "1.2.3.4"},
		{"trusted CIDR + X-Real-IP seul", "10.5.6.7:5000", map[string]string{"X-Real-IP": "5.6.7.8"}, "5.6.7.8"},
		{"XFF prioritaire sur X-Real-IP", "10.5.6.7:5000", map[string]string{"X-Forwarded-For": "5.5.5.5", "X-Real-IP": "6.6.6.6"}, "5.5.5.5"},
		{"XFF illisible → fail-closed, repli RemoteAddr", "10.5.6.7:5000", map[string]string{"X-Forwarded-For": "garbage"}, "10.5.6.7"},
		{"trusted mais aucun header proxy", "10.5.6.7:5000", nil, "10.5.6.7"},
		{"untrusted peer ignore XFF", "8.8.8.8:5000", map[string]string{"X-Forwarded-For": "1.2.3.4"}, "8.8.8.8"},
		{"RemoteAddr illisible", "garbage", map[string]string{"X-Forwarded-For": "1.2.3.4"}, "garbage"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gotIP, gotRemote := serveWith(t, mw, tc.remoteAddr, tc.headers)
			if gotIP != tc.want {
				t.Errorf("ClientIP: want %q, got %q", tc.want, gotIP)
			}
			// r.RemoteAddr n'est plus réécrit — l'IP résolue vit dans le contexte.
			if gotRemote != tc.remoteAddr {
				t.Errorf("RemoteAddr rewritten: want %q, got %q", tc.remoteAddr, gotRemote)
			}
		})
	}
}

func TestTrustedProxy_IPv6ExactProxy(t *testing.T) {
	// IP exacte IPv6 → branche /128 de la construction des préfixes.
	mw, err := TrustedProxy("::1", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got, _ := serveWith(t, mw, "[::1]:9000", map[string]string{"X-Forwarded-For": "2001:db8::5"})
	if got != "2001:db8::5" {
		t.Errorf("ClientIP: want 2001:db8::5, got %q", got)
	}
}

func TestClientIP_Fallbacks(t *testing.T) {
	// Sans middleware TrustedProxy : repli sur l'hôte de RemoteAddr.
	cases := []struct {
		name       string
		remoteAddr string
		want       string
	}{
		{"host:port", "1.2.3.4:5000", "1.2.3.4"},
		{"sans port", "1.2.3.4", "1.2.3.4"},
		{"IPv6 bracketée", "[::1]:9000", "::1"},
		{"vide", "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			req.RemoteAddr = tc.remoteAddr
			if got := ClientIP(req); got != tc.want {
				t.Errorf("ClientIP(%q): want %q, got %q", tc.remoteAddr, tc.want, got)
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
