package config

import (
	"fmt"
	"os"
	"strings"
)

func ResolveEnvVars(v interface{}) interface{} {
	switch val := v.(type) {

	case string:
		if strings.HasPrefix(val, "env:") {
			key := strings.TrimPrefix(val, "env:")
			env := os.Getenv(key)
			if env == "" {
				fmt.Printf("WARNING: env var %s not set\n", key)
			}
			return env
		}
		return val

	case map[string]interface{}:
		// Resolve nested objects so env-backed values work anywhere in the JSON tree.
		for k, v2 := range val {
			val[k] = ResolveEnvVars(v2)
		}
		return val

	case []interface{}:
		// Arrays often hold backend lists or route targets that may also use env: placeholders.
		for i, v2 := range val {
			val[i] = ResolveEnvVars(v2)
		}
		return val

	default:
		return v
	}
}
