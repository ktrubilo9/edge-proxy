package middleware

import (
	"fmt"
	"net"
	"net/http"
	"strings"
)

type ClientIPResolver struct {
	trustedProxies []*net.IPNet
}

func NewClientIPResolver(rawCIDRs string) (*ClientIPResolver, error) {
	resolver := &ClientIPResolver{}
	if strings.TrimSpace(rawCIDRs) == "" {
		return resolver, nil
	}

	for _, rawCIDR := range strings.Split(rawCIDRs, ",") {
		cidr := strings.TrimSpace(rawCIDR)
		if cidr == "" {
			continue
		}

		_, network, err := net.ParseCIDR(cidr)
		if err != nil {
			return nil, fmt.Errorf("invalid trusted proxy CIDR %q: %w", cidr, err)
		}
		resolver.trustedProxies = append(resolver.trustedProxies, network)
	}

	return resolver, nil
}

func (r *ClientIPResolver) Resolve(req *http.Request) string {
	remoteIP := parseRemoteIP(req.RemoteAddr)
	if remoteIP == nil {
		return req.RemoteAddr
	}

	if !r.isTrusted(remoteIP) {
		return remoteIP.String()
	}

	forwarded := req.Header.Get("X-Forwarded-For")
	if forwarded == "" {
		return remoteIP.String()
	}

	parts := strings.Split(forwarded, ",")
	chain := make([]net.IP, 0, len(parts))
	for _, part := range parts {
		ip := net.ParseIP(strings.TrimSpace(part))
		if ip == nil {
			return remoteIP.String()
		}
		chain = append(chain, ip)
	}

	for i := len(chain) - 1; i >= 0; i-- {
		if !r.isTrusted(chain[i]) {
			return chain[i].String()
		}
	}

	if len(chain) > 0 {
		return chain[0].String()
	}
	return remoteIP.String()
}

func (r *ClientIPResolver) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		req.Header.Set("X-Forwarded-For", r.Resolve(req))
		next.ServeHTTP(w, req)
	})
}

func (r *ClientIPResolver) isTrusted(ip net.IP) bool {
	for _, network := range r.trustedProxies {
		if network.Contains(ip) {
			return true
		}
	}
	return false
}

func parseRemoteIP(remoteAddr string) net.IP {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err == nil {
		return net.ParseIP(host)
	}
	return net.ParseIP(remoteAddr)
}
