package handler

import (
	"net/http"
	"strings"
)

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
		if !strings.EqualFold(name, "Connection") {
			continue
		}

		for _, value := range values {
			for _, token := range strings.Split(value, ",") {
				if strings.EqualFold(strings.TrimSpace(token), "upgrade") {
					return true
				}
			}
		}
	}

	return false
}
