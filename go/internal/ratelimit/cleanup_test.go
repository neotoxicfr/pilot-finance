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

	l.cleanup("login")

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

// TestGetLimiter_TickerCleanup couvre limiter.go — case <-ticker.C: l.cleanup(action).
// Oracle réel : un hook signale sur un canal que cleanup a vraiment tourné ; le test
// échoue si le hook n'est jamais appelé (select avec timeout).
func TestGetLimiter_TickerCleanup(t *testing.T) {
	StopAll()
	Start() // réautoriser le démarrage de la goroutine après StopAll

	orig := limiterCleanupInterval
	limiterCleanupInterval = 1 * time.Millisecond

	ran := make(chan struct{}, 1)
	hookCleanupRan = func() {
		select {
		case ran <- struct{}{}:
		default:
		}
	}
	defer func() {
		limiterCleanupInterval = orig
		hookCleanupRan = nil
		StopAll()
	}()

	// Créer un limiter — goroutine démarre avec intervalle 1ms
	getLimiter("login")

	select {
	case <-ran:
		// cleanup a réellement été exécuté par la goroutine périodique
	case <-time.After(2 * time.Second):
		t.Fatal("ticker cleanup goroutine n'a jamais exécuté cleanup()")
	}
}

// TestGetLimiter_StoppedNoGoroutine couvre la branche `if stopped { return l }` :
// après StopAll, getLimiter retourne un limiter fonctionnel SANS démarrer de goroutine.
// Oracle : avec un intervalle de cleanup minuscule et un hook armé, le hook ne doit
// JAMAIS être appelé (aucune goroutine n'a été lancée).
func TestGetLimiter_StoppedNoGoroutine(t *testing.T) {
	StopAll() // stopped = true

	orig := limiterCleanupInterval
	limiterCleanupInterval = 1 * time.Millisecond

	ran := make(chan struct{}, 1)
	hookCleanupRan = func() {
		select {
		case ran <- struct{}{}:
		default:
		}
	}
	defer func() {
		limiterCleanupInterval = orig
		hookCleanupRan = nil
		StopAll()
	}()

	// stopped == true : aucun goroutine ne doit être lancé.
	l := getLimiter("login")
	if l == nil {
		t.Fatal("getLimiter doit retourner un limiter utilisable même après StopAll")
	}

	select {
	case <-ran:
		t.Fatal("aucune goroutine de cleanup ne doit tourner après StopAll")
	case <-time.After(50 * time.Millisecond):
		// Attendu : le hook n'a jamais été appelé.
	}
}

func TestCleanup_EmptyMap_NoOp(t *testing.T) {
	StopAll()

	l := getLimiter("register")

	// cleanup on empty map should not panic
	l.cleanup("register")

	l.mu.RLock()
	count := len(l.attempts)
	l.mu.RUnlock()

	if count != 0 {
		t.Errorf("want 0 entries after cleanup of empty map, got %d", count)
	}

	StopAll()
}

// TestLimiter_GoroutineStop couvre la branche `case <-l.stop: return` de la goroutine.
// Oracle réel : un hook signale sur un canal au moment du return ; le test échoue si la
// goroutine ne s'arrête jamais (select avec timeout).
func TestLimiter_GoroutineStop(t *testing.T) {
	StopAll()
	Start() // réautoriser le démarrage de la goroutine

	stoppedCh := make(chan struct{}, 1)
	hookGoroutineStopped = func() {
		select {
		case stoppedCh <- struct{}{}:
		default:
		}
	}
	defer func() {
		hookGoroutineStopped = nil
		StopAll()
	}()

	getLimiter("login") // crée le limiter et démarre la goroutine cleanup
	StopAll()           // close(l.stop) → la goroutine doit recevoir et return

	select {
	case <-stoppedCh:
		// la goroutine a réellement exécuté la branche de sortie
	case <-time.After(2 * time.Second):
		t.Fatal("la goroutine de cleanup ne s'est jamais arrêtée après StopAll")
	}
}

// TestCleanup_BlockedEntryNotEvictedEarly couvre la branche de garde du blocage actif
// dans cleanup (finding 2) : une entrée dont la fenêtre a expiré mais dont le blocage
// court encore ne doit PAS être évincée ; une fois le blocage expiré, elle l'est.
func TestCleanup_BlockedEntryNotEvictedEarly(t *testing.T) {
	StopAll()

	// Config déterministe injectée pour le calcul de blockedAt+BlockMs.
	origConfigs := cleanupConfigs
	cleanupConfigs = func() map[string]Config {
		return map[string]Config{
			"blk": {MaxAttempts: 1, WindowMs: 1000, BlockMs: 10_000_000_000},
		}
	}
	defer func() {
		cleanupConfigs = origConfigs
		StopAll()
	}()

	l := getLimiter("blk")
	now := time.Now().UnixMilli()

	l.mu.Lock()
	// Fenêtre largement expirée (firstTry il y a > 2h) mais blocage encore actif.
	l.attempts["blk:stillBlocked"] = &attempt{
		count:     5,
		firstTry:  now - 7_200_001,
		blockedAt: now, // blocage actif (blockedAt + BlockMs très loin dans le futur)
	}
	// Fenêtre expirée et blocage déjà terminé → doit être évincée.
	l.attempts["blk:unblockedOld"] = &attempt{
		count:     5,
		firstTry:  now - 7_200_001,
		blockedAt: now - 20_000_000_000, // blockedAt + BlockMs déjà dans le passé
	}
	// Fenêtre expirée, jamais bloquée → doit être évincée.
	l.attempts["blk:neverBlocked"] = &attempt{
		count:    1,
		firstTry: now - 7_200_001,
	}
	l.mu.Unlock()

	l.cleanup("blk")

	l.mu.RLock()
	_, hasBlocked := l.attempts["blk:stillBlocked"]
	_, hasUnblocked := l.attempts["blk:unblockedOld"]
	_, hasNever := l.attempts["blk:neverBlocked"]
	l.mu.RUnlock()

	if !hasBlocked {
		t.Error("une entrée encore bloquée ne doit pas être évincée même si la fenêtre a expiré")
	}
	if hasUnblocked {
		t.Error("une entrée dont le blocage est terminé et la fenêtre expirée doit être évincée")
	}
	if hasNever {
		t.Error("une entrée jamais bloquée avec fenêtre expirée doit être évincée")
	}
}

// TestCleanup_BlockedEntry_NoConfig couvre la branche hasCfg == false : si l'action n'a
// pas de config, on ne peut pas calculer l'expiration du blocage, donc on évince dès que
// la fenêtre a expiré (comportement de repli sûr).
func TestCleanup_BlockedEntry_NoConfig(t *testing.T) {
	StopAll()
	defer StopAll()

	l := getLimiter("login")
	now := time.Now().UnixMilli()

	l.mu.Lock()
	l.attempts["x:blocked"] = &attempt{
		count:     5,
		firstTry:  now - 7_200_001,
		blockedAt: now,
	}
	l.mu.Unlock()

	// "unknownAction" n'existe pas dans Configs → hasCfg == false.
	l.cleanup("unknownAction")

	l.mu.RLock()
	_, has := l.attempts["x:blocked"]
	l.mu.RUnlock()

	if has {
		t.Error("sans config, une entrée à fenêtre expirée doit être évincée même si blockedAt > 0")
	}
}
