// Package ratelimit implémente un rate limiter en mémoire
package ratelimit

import (
	"sync"
	"time"
)

// Config définit la configuration d'un rate limiter
type Config struct {
	MaxAttempts int
	WindowMs    int64
	BlockMs     int64
}

// Configs prédéfinies pour différentes actions
var Configs = map[string]Config{
	"login": {
		MaxAttempts: 5,
		WindowMs:    60000,  // 1 minute
		BlockMs:     300000, // 5 minutes
	},
	"loginAccount": {
		MaxAttempts: 10,
		WindowMs:    600000,  // 10 minutes
		BlockMs:     900000,  // 15 minutes
	},
	"register": {
		MaxAttempts: 3,
		WindowMs:    3600000, // 1 heure
		BlockMs:     3600000, // 1 heure
	},
	"forgotPassword": {
		MaxAttempts: 3,
		WindowMs:    3600000, // 1 heure
		BlockMs:     3600000, // 1 heure
	},
	"verifyEmail": {
		MaxAttempts: 10,
		WindowMs:    3600000, // 1 heure
		BlockMs:     3600000, // 1 heure
	},
	"verifyEmailResend": {
		MaxAttempts: 1,
		WindowMs:    300000, // 5 minutes
		BlockMs:     300000, // 5 minutes
	},
	"twoFactor": {
		MaxAttempts: 5,
		WindowMs:    300000, // 5 minutes
		BlockMs:     900000, // 15 minutes
	},
	"resetPassword": {
		MaxAttempts: 5,
		WindowMs:    900000,  // 15 minutes
		BlockMs:     1800000, // 30 minutes
	},
}

type attempt struct {
	count     int
	firstTry  int64
	blockedAt int64
}

// Limiter gère le rate limiting par clé avec blocage temporaire après dépassement
type Limiter struct {
	mu       sync.RWMutex
	attempts map[string]*attempt
	stop     chan struct{}
}

var (
	limiters = make(map[string]*Limiter)
	mu       sync.Mutex

	// Disabled désactive globalement le rate limiting (tests E2E).
	Disabled bool
)

// limiterCleanupInterval est la durée entre deux nettoyages ; injectable dans les tests.
var limiterCleanupInterval = 5 * time.Minute

// getLimiter retourne le limiter pour une action donnée
func getLimiter(action string) *Limiter {
	mu.Lock()
	defer mu.Unlock()

	if l, ok := limiters[action]; ok {
		return l
	}

	l := &Limiter{
		attempts: make(map[string]*attempt),
		stop:     make(chan struct{}),
	}
	limiters[action] = l

	// Nettoyage périodique — capturer la valeur avant la goroutine pour éviter la race
	interval := limiterCleanupInterval
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				l.cleanup()
			case <-l.stop:
				return
			}
		}
	}()

	return l
}

func (l *Limiter) cleanup() {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now().UnixMilli()
	for key, att := range l.attempts {
		// Supprimer les entrées expirées (plus de 2 heures)
		if now-att.firstTry > 7200000 {
			delete(l.attempts, key)
		}
	}
}

// Result contient le résultat d'une vérification de rate limit
type Result struct {
	Allowed      bool
	RetryAfterMs int64
	Remaining    int
}

// Check vérifie si une requête est autorisée
func Check(identifier, action string) Result {
	if Disabled {
		return Result{Allowed: true, Remaining: 999}
	}

	cfg, ok := Configs[action]
	if !ok {
		// Action inconnue, autoriser par défaut
		return Result{Allowed: true, Remaining: 999}
	}

	limiter := getLimiter(action)
	key := action + ":" + identifier

	limiter.mu.Lock()
	defer limiter.mu.Unlock()

	now := time.Now().UnixMilli()
	att, exists := limiter.attempts[key]

	if !exists {
		limiter.attempts[key] = &attempt{
			count:    1,
			firstTry: now,
		}
		return Result{
			Allowed:   true,
			Remaining: cfg.MaxAttempts - 1,
		}
	}

	// Vérifier si bloqué
	if att.blockedAt > 0 {
		blockEnd := att.blockedAt + cfg.BlockMs
		if now < blockEnd {
			return Result{
				Allowed:      false,
				RetryAfterMs: blockEnd - now,
				Remaining:    0,
			}
		}
		// Débloquer
		att.blockedAt = 0
		att.count = 0
		att.firstTry = now
	}

	// Vérifier si la fenêtre est expirée
	if now-att.firstTry > cfg.WindowMs {
		att.count = 1
		att.firstTry = now
		return Result{
			Allowed:   true,
			Remaining: cfg.MaxAttempts - 1,
		}
	}

	// Incrémenter le compteur
	att.count++

	if att.count > cfg.MaxAttempts {
		att.blockedAt = now
		return Result{
			Allowed:      false,
			RetryAfterMs: cfg.BlockMs,
			Remaining:    0,
		}
	}

	return Result{
		Allowed:   true,
		Remaining: cfg.MaxAttempts - att.count,
	}
}

// StopAll arrête tous les goroutines de nettoyage (graceful shutdown)
func StopAll() {
	mu.Lock()
	defer mu.Unlock()
	for _, l := range limiters {
		close(l.stop)
	}
	limiters = make(map[string]*Limiter)
}

// Reset réinitialise le compteur pour un identifiant
func Reset(identifier, action string) {
	limiter := getLimiter(action)
	key := action + ":" + identifier

	limiter.mu.Lock()
	defer limiter.mu.Unlock()

	delete(limiter.attempts, key)
}
