package main

import (
	"testing"
)

func TestGetenv(t *testing.T) {
	const key = "TEST_GETENV_KEY"

	t.Run("not set -> fallback", func(t *testing.T) {
		if got := getenv(key, "def"); got != "def" {
			t.Errorf("got %q, want %q", got, "def")
		}
	})

	t.Run("set -> value", func(t *testing.T) {
		t.Setenv(key, "custom")
		if got := getenv(key, "def"); got != "custom" {
			t.Errorf("got %q, want %q", got, "custom")
		}
	})

	t.Run("set but empty -> empty, not fallback", func(t *testing.T) {
		t.Setenv(key, "")
		if got := getenv(key, "def"); got != "" {
			t.Errorf("got %q, want empty", got)
		}
	})
}
