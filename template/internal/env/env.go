package env

import "os"

func GetString(key string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}

	return ""
}
