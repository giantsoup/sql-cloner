package core

import (
	"os"
	"strings"
	"testing"
)

var osWriteFile = func(path string, data []byte) error {
	return os.WriteFile(path, data, 0o644)
}

func mustMkdir(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", path, err)
	}
}

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func stringsContains(haystack, needle string) bool {
	return strings.Contains(haystack, needle)
}
