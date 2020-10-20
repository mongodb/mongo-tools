package sh

import "os"

// Env looks up an environment variable and if not found, returns the fallback.
func Env(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}

	return fallback
}