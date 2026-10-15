package auth

import (
	"slices"
	"testing"
)

// InitWebAuthnForHost : rpId sans port, http:// accepté pour localhost seulement.
func TestInitWebAuthnForHost(t *testing.T) {
	cases := []struct {
		host        string
		wantRPID    string
		wantOrigins []string
	}{
		{"pilot.example.com", "pilot.example.com", []string{"https://pilot.example.com"}},
		{"pilot.example.com:8443", "pilot.example.com", []string{"https://pilot.example.com:8443"}},
		{"localhost:3000", "localhost", []string{"https://localhost:3000", "http://localhost:3000"}},
		{"localhost", "localhost", []string{"https://localhost", "http://localhost"}},
	}
	for _, tc := range cases {
		t.Run(tc.host, func(t *testing.T) {
			if err := InitWebAuthnForHost(tc.host, "Test"); err != nil {
				t.Fatalf("InitWebAuthnForHost(%q): %v", tc.host, err)
			}
			if got := webAuthn.Config.RPID; got != tc.wantRPID {
				t.Errorf("RPID: got %q, want %q", got, tc.wantRPID)
			}
			if got := webAuthn.Config.RPOrigins; !slices.Equal(got, tc.wantOrigins) {
				t.Errorf("RPOrigins: got %v, want %v", got, tc.wantOrigins)
			}
		})
	}
}
