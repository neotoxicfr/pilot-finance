package middleware

import (
	"errors"
	"log/slog"
	"net"
	"net/http"
	"strings"

	chimw "github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/httprate"
)

// ClientIP retourne l'IP client résolue par TrustedProxy (stockée dans le
// contexte par les middlewares chi ClientIPFrom*). Repli sur l'hôte de
// r.RemoteAddr quand aucun middleware n'a résolu d'IP (tests, appels directs).
func ClientIP(r *http.Request) string {
	if ip := chimw.GetClientIP(r.Context()); ip != "" {
		return ip
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		// RemoteAddr sans port (tests, connexions exotiques) : utiliser tel quel.
		return r.RemoteAddr
	}
	return host
}

// ClientIPKey est une httprate.KeyFunc basée sur ClientIP — donc non
// falsifiable via X-Forwarded-For par un pair non approuvé. CanonicalizeIP
// regroupe les clients IPv6 par /64 (un client SLAAC contrôle tout son /64 et
// pourrait sinon tourner dedans pour vider le bucket).
func ClientIPKey(r *http.Request) (string, error) {
	return httprate.CanonicalizeIP(ClientIP(r)), nil
}

// TrustedProxy construit le middleware de résolution d'IP client.
//
// proxyEnv est la valeur de TRUSTED_PROXIES (liste d'IPs exactes et/ou de ranges
// CIDR séparés par des virgules). isProd indique si l'on tourne en production.
//
// L'IP résolue est stockée dans le contexte de la requête (middlewares chi
// ClientIPFrom*, chi ≥ 5.3) et se lit via ClientIP ; r.RemoteAddr n'est plus
// réécrit.
//
// Comportement :
//   - proxyEnv vide + production → erreur (refus de démarrer : accepter
//     X-Forwarded-For de n'importe quelle source est spoofable).
//   - proxyEnv vide + dev → X-Forwarded-For accepté de toute source (entrée la
//     plus à droite), repli RemoteAddr. Usage dev uniquement.
//   - proxyEnv renseigné → X-Forwarded-For / X-Real-IP ne sont honorés que si
//     le pair direct est dans l'allowlist. XFF est parcouru de droite à gauche
//     en sautant les proxys de confiance (sémantique chi) : la première IP non
//     approuvée est celle posée par notre proxy, non forgeable par le client.
//     Pair non approuvé → RemoteAddr seul.
//
// Retourne une erreur (au lieu de os.Exit) pour rester testable ; l'appelant
// (cmd/server) décide de la fatalité.
func TrustedProxy(proxyEnv string, isProd bool) (func(http.Handler) http.Handler, error) {
	if proxyEnv == "" {
		if isProd {
			return nil, errors.New("TRUSTED_PROXIES doit être défini en production (sinon X-Forwarded-For est spoofable et bypasse les rate limits)")
		}
		slog.Warn("TRUSTED_PROXIES vide : X-Forwarded-For accepté de toute source — usage dev uniquement")
		xff := chimw.ClientIPFromXFF()
		return func(next http.Handler) http.Handler {
			// Le middleware le plus interne qui résout une IP écrase les
			// précédents : XFF prioritaire, RemoteAddr en repli.
			return chimw.ClientIPFromRemoteAddr(xff(next))
		}, nil
	}

	// Supporte IPs exactes ET ranges CIDR (les IPs containers Docker changent au restart).
	trustedIPs := make(map[string]bool)
	var trustedNets []*net.IPNet
	// prefixes reprend l'allowlist au format CIDR pour le parcours XFF de chi
	// (IP exacte → /32 ou /128).
	var prefixes []string
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
			prefixes = append(prefixes, ipnet.String())
		} else {
			ip := net.ParseIP(p)
			if ip == nil {
				return nil, errors.New("TRUSTED_PROXIES : IP invalide : " + p)
			}
			trustedIPs[p] = true
			if ip.To4() != nil {
				prefixes = append(prefixes, ip.String()+"/32")
			} else {
				prefixes = append(prefixes, ip.String()+"/128")
			}
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

	fromXFF := chimw.ClientIPFromXFF(prefixes...)
	fromXRealIP := chimw.ClientIPFromHeader("X-Real-IP")
	return func(next http.Handler) http.Handler {
		// Priorité XFF > X-Real-IP > RemoteAddr : le middleware le plus interne
		// qui résout une IP écrase les précédents.
		viaProxy := chimw.ClientIPFromRemoteAddr(fromXRealIP(fromXFF(next)))
		direct := chimw.ClientIPFromRemoteAddr(next)
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			host, _, err := net.SplitHostPort(r.RemoteAddr)
			if err != nil {
				host = r.RemoteAddr
			}
			if isTrusted(host) {
				viaProxy.ServeHTTP(w, r)
			} else {
				direct.ServeHTTP(w, r)
			}
		})
	}, nil
}
