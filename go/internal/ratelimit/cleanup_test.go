package ratelimit

import (
	"testing"
	"time"
)

func TestCleanup_RemovesExpiredEntries(t *testing.T) {
	StopAll()

	l := getLimiter("login")
	now := time.Now().UnixMilli()

	l.mu.Lock()
	// Entry older than 2 hours — should be removed
	l.attempts["cleanup:old"] = &attempt{
		count:    1,
		firstTry: now - 7_200_001,
	}
	// Entry from 1 minute ago — should be kept
	l.attempts["cleanup:recent"] = &attempt{
		count:    1,
		firstTry: now - 60_000,
	}
	l.mu.Unlock()

	l.cleanup()

	l.mu.RLock()
	_, hasOld := l.attempts["cleanup:old"]
	_, hasRecent := l.attempts["cleanup:recent"]
	l.mu.RUnlock()

	if hasOld {
		t.Error("entry older than 2 hours should be removed by cleanup")
	}
	if !hasRecent {
		t.Error("recent entry should not be removed by cleanup")
	}

	StopAll()
}

// TestGetLimiter_TickerCleanup couvre limiter.go:83-84 — case <-ticker.C: l.cleanup().
// On utilise un intervalle de 1ms pour que le ticker se déclenche pendant le test.
func TestGetLimiter_TickerCleanup(t *testing.T) {
	StopAll()

	orig := limiterCleanupInterval
	limiterCleanupInterval = 1 * time.Millisecond
	defer func() {
		limiterCleanupInterval = orig
		StopAll()
	}()

	// Créer un limiter — goroutine démarre avec intervalle 1ms
	getLimiter("login")

	// Attendre que le ticker se déclenche → case <-ticker.C: l.cleanup() exécuté
	time.Sleep(20 * time.Millisecond)
}

func TestCleanup_EmptyMap_NoOp(t *testing.T) {
	StopAll()

	l := getLimiter("register")

	// cleanup on empty map should not panic
	l.cleanup()

	l.mu.RLock()
	count := len(l.attempts)
	l.mu.RUnlock()

	if count != 0 {
		t.Errorf("want 0 entries after cleanup of empty map, got %d", count)
	}

	StopAll()
}
