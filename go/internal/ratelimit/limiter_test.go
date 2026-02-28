package ratelimit_test

import (
	"testing"
	"time"

	"pilot-finance/internal/ratelimit"
)

func TestCheck_FirstRequest_Allowed(t *testing.T) {
	ratelimit.StopAll()

	r := ratelimit.Check("192.168.1.1", "login")
	if !r.Allowed {
		t.Error("first request should be allowed")
	}
	want := ratelimit.Configs["login"].MaxAttempts - 1
	if r.Remaining != want {
		t.Errorf("Remaining: want %d, got %d", want, r.Remaining)
	}

	ratelimit.StopAll()
}

func TestCheck_UnknownAction_AlwaysAllowed(t *testing.T) {
	ratelimit.StopAll()

	r := ratelimit.Check("192.168.1.2", "nonexistent_action")
	if !r.Allowed {
		t.Error("unknown action should always be allowed")
	}
	if r.Remaining != 999 {
		t.Errorf("Remaining: want 999, got %d", r.Remaining)
	}

	ratelimit.StopAll()
}

func TestCheck_BlockAfterMaxAttempts(t *testing.T) {
	ratelimit.StopAll()

	ip := "10.0.0.100"
	cfg := ratelimit.Configs["login"]

	for i := 0; i < cfg.MaxAttempts; i++ {
		r := ratelimit.Check(ip, "login")
		if !r.Allowed {
			t.Fatalf("attempt %d should be allowed (max=%d)", i+1, cfg.MaxAttempts)
		}
	}

	// Next attempt exceeds limit
	r := ratelimit.Check(ip, "login")
	if r.Allowed {
		t.Error("should be blocked after max attempts")
	}
	if r.Remaining != 0 {
		t.Errorf("Remaining: want 0, got %d", r.Remaining)
	}
	if r.RetryAfterMs <= 0 {
		t.Error("RetryAfterMs should be positive when blocked")
	}

	ratelimit.StopAll()
}

func TestCheck_BlockPersists(t *testing.T) {
	ratelimit.StopAll()

	ip := "10.0.0.101"
	cfg := ratelimit.Configs["login"]

	for i := 0; i <= cfg.MaxAttempts; i++ {
		ratelimit.Check(ip, "login")
	}

	// Subsequent attempts while blocked
	for i := 0; i < 3; i++ {
		r := ratelimit.Check(ip, "login")
		if r.Allowed {
			t.Errorf("attempt %d while blocked should not be allowed", i+1)
		}
	}

	ratelimit.StopAll()
}

func TestReset_ClearsBlock(t *testing.T) {
	ratelimit.StopAll()

	ip := "10.0.0.102"
	cfg := ratelimit.Configs["login"]

	for i := 0; i <= cfg.MaxAttempts; i++ {
		ratelimit.Check(ip, "login")
	}

	// Verify blocked
	r := ratelimit.Check(ip, "login")
	if r.Allowed {
		t.Fatal("should be blocked before reset")
	}

	ratelimit.Reset(ip, "login")

	// Should be allowed again
	r = ratelimit.Check(ip, "login")
	if !r.Allowed {
		t.Error("should be allowed after reset")
	}

	ratelimit.StopAll()
}

func TestReset_NonExistentKey_NoOp(t *testing.T) {
	ratelimit.StopAll()

	// Reset on a key that never had attempts — should not panic
	ratelimit.Reset("192.0.2.1", "login")

	r := ratelimit.Check("192.0.2.1", "login")
	if !r.Allowed {
		t.Error("should be allowed after reset of non-existent key")
	}

	ratelimit.StopAll()
}

func TestCheck_WindowExpiry_ResetsCounter(t *testing.T) {
	ratelimit.StopAll()

	// Register a short-window action
	ratelimit.Configs["testWin"] = ratelimit.Config{
		MaxAttempts: 2,
		WindowMs:    50,
		BlockMs:     5000,
	}
	defer delete(ratelimit.Configs, "testWin")

	ip := "10.0.0.103"
	ratelimit.Check(ip, "testWin")
	ratelimit.Check(ip, "testWin")

	// Wait for window to expire
	time.Sleep(60 * time.Millisecond)

	r := ratelimit.Check(ip, "testWin")
	if !r.Allowed {
		t.Error("should be allowed after window expiry")
	}

	ratelimit.StopAll()
}

func TestCheck_DifferentIdentifiers_Independent(t *testing.T) {
	ratelimit.StopAll()

	cfg := ratelimit.Configs["login"]
	ip1 := "10.1.0.1"
	ip2 := "10.1.0.2"

	// Block ip1
	for i := 0; i <= cfg.MaxAttempts; i++ {
		ratelimit.Check(ip1, "login")
	}

	// ip2 should still be free
	r := ratelimit.Check(ip2, "login")
	if !r.Allowed {
		t.Error("ip2 should not be affected by ip1 being blocked")
	}

	ratelimit.StopAll()
}

func TestCheck_BlockExpired_Unblocks(t *testing.T) {
	ratelimit.StopAll()

	ratelimit.Configs["testUnblock"] = ratelimit.Config{
		MaxAttempts: 1,
		WindowMs:    60000,
		BlockMs:     50, // 50ms block
	}
	defer delete(ratelimit.Configs, "testUnblock")

	ip := "10.0.0.200"
	// Trigger block: 2 attempts (1 max + 1 over limit)
	ratelimit.Check(ip, "testUnblock")
	ratelimit.Check(ip, "testUnblock")

	// Verify blocked immediately
	r := ratelimit.Check(ip, "testUnblock")
	if r.Allowed {
		t.Fatal("should be blocked immediately after exceeding limit")
	}

	// Wait for block to expire
	time.Sleep(60 * time.Millisecond)

	// Next check should reset blockedAt and allow the request
	r = ratelimit.Check(ip, "testUnblock")
	if !r.Allowed {
		t.Error("should be unblocked after block period expires")
	}

	ratelimit.StopAll()
}

func TestStopAll_ResetsState(t *testing.T) {
	ip := "10.0.0.104"
	cfg := ratelimit.Configs["login"]

	// Block the IP
	for i := 0; i <= cfg.MaxAttempts; i++ {
		ratelimit.Check(ip, "login")
	}

	ratelimit.StopAll()

	// After StopAll, fresh limiter — should be allowed
	r := ratelimit.Check(ip, "login")
	if !r.Allowed {
		t.Error("should be allowed after StopAll resets state")
	}

	ratelimit.StopAll()
}
