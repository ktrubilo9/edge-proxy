package handler

import (
	"net/http"
	"strings"
)

var hopByHopHeaderNames = map[string]struct{}{
	"connection":          {},
	"keep-alive":          {},
	"proxy-authenticate":  {},
	"proxy-authorization": {},
	"proxy-connection":    {},
	"te":                  {},
	"trailer":             {},
	"transfer-encoding":   {},
	"upgrade":             {},
}

func isUpgradeRequest(header http.Header) bool {
	for name, values := range header {
		if strings.EqualFold(name, "Upgrade") {
			for _, value := range values {
				if strings.TrimSpace(value) != "" {
					return true
				}
			}
			continue
		}
	}

	for _, token := range connectionHeaderTokens(header) {
		if strings.EqualFold(token, "upgrade") {
			return true
		}
	}

	return false
}

func removeHopByHopHeaders(header http.Header) {
	blocked := make(map[string]struct{}, len(hopByHopHeaderNames))
	for name := range hopByHopHeaderNames {
		blocked[name] = struct{}{}
	}
	for _, token := range connectionHeaderTokens(header) {
		blocked[strings.ToLower(token)] = struct{}{}
	}

	for name := range header {
		if _, remove := blocked[strings.ToLower(name)]; remove {
			delete(header, name)
		}
	}
}

func connectionHeaderTokens(header http.Header) []string {
	var tokens []string
	for name, values := range header {
		if !strings.EqualFold(name, "Connection") {
			continue
		}

		for _, value := range values {
			for _, token := range strings.Split(value, ",") {
				if token = strings.TrimSpace(token); token != "" {
					tokens = append(tokens, token)
				}
			}
		}
	}
	return tokens
}
