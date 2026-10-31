package main

import (
	"os"
	"strings"
	"testing"
)

func TestLoadDotEnv(t *testing.T) {
	t.Run("no .env", func(t *testing.T) {
		t.Chdir(t.TempDir())
		if err := loadDotEnv(); err != nil {
			t.Errorf("err = %v, want nil", err)
		}
	})

	t.Run("loads without overriding", func(t *testing.T) {
		t.Chdir(t.TempDir())
		t.Setenv("SETLISTED_TEST_SET", "from-env")
		os.Unsetenv("SETLISTED_TEST_NEW")
		t.Cleanup(func() { os.Unsetenv("SETLISTED_TEST_NEW") })
		writeDotEnv(t, "SETLISTED_TEST_SET=from-file\nSETLISTED_TEST_NEW=new\n")
		if err := loadDotEnv(); err != nil {
			t.Fatal(err)
		}
		if got := os.Getenv("SETLISTED_TEST_SET"); got != "from-env" {
			t.Errorf("SETLISTED_TEST_SET = %q, want the environment's value", got)
		}
		if got := os.Getenv("SETLISTED_TEST_NEW"); got != "new" {
			t.Errorf("SETLISTED_TEST_NEW = %q, want %q", got, "new")
		}
	})

	t.Run("parse error hides the contents", func(t *testing.T) {
		t.Chdir(t.TempDir())
		// A value pasted onto its own line, as happened with a real key.
		writeDotEnv(t, "SETLISTFM_API_KEY=\nsecret-key-value-1234\nSESSION_KEY=other-secret\n")
		err := loadDotEnv()
		if err == nil {
			t.Fatal("err = nil, want a parse error")
		}
		if msg := err.Error(); strings.Contains(msg, "secret") || !strings.Contains(msg, ".env") {
			t.Errorf("err = %q, want a message naming .env without its contents", msg)
		}
	})
}

func writeDotEnv(t *testing.T, content string) {
	t.Helper()
	if err := os.WriteFile(".env", []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}
