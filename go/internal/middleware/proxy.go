package middleware

import (
	"errors"
	"log/slog"
	"net"
	"net/http"
	"strings"

	chimw "github.com/go-chi/chi/v5/middleware"
)

// TrustedProxy construit le middleware de résolution d'IP client.
//
// proxyEnv est la valeur de TRUSTED_PROXIES (liste d'IPs exactes et/ou de ranges
// CIDR séparés par des virgules). isProd indique si l'on tourne en production.
//
// Comportement :
//   - proxyEnv vide + production → erreur (refus de démarrer : chimw.RealIP fait
//     confiance inconditionnellement à X-Forwarded-For, donc spoofable).
//   - proxyEnv vide + dev → fallback chi RealIP (usage dev uniquement).
//   - proxyEnv renseigné → middleware custom qui ne réécrit r.RemoteAddr depuis
//     X-Forwarded-For / X-Real-IP que si le pair direct est dans l'allowlist.
//
// Retourne une erreur (au lieu de os.Exit) pour rester testable ; l'appelant
// (cmd/server) décide de la fatalité.
func TrustedProxy(proxyEnv string, isProd bool) (func(http.Handler) http.Handler, error) {
	if proxyEnv == "" {
		if isProd {
			return nil, errors.New("TRUSTED_PROXIES doit être défini en production (sinon X-Forwarded-For est spoofable et bypasse les rate limits)")
		}
		slog.Warn("TRUSTED_PROXIES vide : fallback chi RealIP (X-Forwarded-For accepté de toute source — usage dev uniquement)")
		//nolint:staticcheck // SA1019: fallback dev-only ; la prod refuse de démarrer sans TRUSTED_PROXIES (erreur ci-dessus) et utilise le middleware custom plus bas. chi v5.3.0 déprécie RealIP pour l'IP-spoofing, déjà documenté/maîtrisé ici.
		return chimw.RealIP, nil
	}

	// Supporte IPs exactes ET ranges CIDR (les IPs containers Docker changent au restart).
	trustedIPs := make(map[string]bool)
	var trustedNets []*net.IPNet
	for _, p := range strings.Split(proxyEnv, ",") {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if strings.Contains(p, "/") {
			_, ipnet, err := net.ParseCIDR(p)
			if err != nil {
				return nil, errors.New("TRUSTED_PROXIES : CIDR invalide : " + p)
			}
			trustedNets = append(trustedNets, ipnet)
		} else {
			if net.ParseIP(p) == nil {
				return nil, errors.New("TRUSTED_PROXIES : IP invalide : " + p)
			}
			trustedIPs[p] = true
		}
	}

	isTrusted := func(host string) bool {
		if trustedIPs[host] {
			return true
		}
		ip := net.ParseIP(host)
		if ip == nil {
			return false
		}
		for _, n := range trustedNets {
			if n.Contains(ip) {
				return true
			}
		}
		return false
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			host, _, _ := net.SplitHostPort(r.RemoteAddr)
			if isTrusted(host) {
				if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
					parts := strings.Split(xff, ",")
					clientIP := strings.TrimSpace(parts[0])
					r.RemoteAddr = formatRemoteAddr(clientIP)
				} else if xri := r.Header.Get("X-Real-IP"); xri != "" {
					r.RemoteAddr = formatRemoteAddr(strings.TrimSpace(xri))
				}
			}
			next.ServeHTTP(w, r)
		})
	}, nil
}

// formatRemoteAddr formate une IP en host:port pour r.RemoteAddr.
// Les IPv6 sans brackets ("::1") sont entourées de [ ] pour que
// net.SplitHostPort puisse les parser correctement downstream.
func formatRemoteAddr(ip string) string {
	if ip == "" {
		return ":0"
	}
	// Si déjà bracketé ([...]), garder tel quel
	if strings.HasPrefix(ip, "[") {
		return ip + ":0"
	}
	// IPv6 = contient ":" et n'est pas bracketé → ajouter brackets
	if strings.Contains(ip, ":") {
		return "[" + ip + "]:0"
	}
	return ip + ":0"
}
