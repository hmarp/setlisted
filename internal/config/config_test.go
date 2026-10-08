package config

import (
	"strings"
	"testing"
)

// validKey is base64 for 32 zero bytes.
const validKey = "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA="

func validEnv() map[string]string {
	return map[string]string{
		EnvBaseURL:             "http://127.0.0.1:8080",
		EnvSpotifyClientID:     "client-id-value",
		EnvSpotifyClientSecret: "client-secret-value",
		EnvSetlistFMAPIKey:     "setlistfm-key-value",
		EnvSessionKey:          validKey,
	}
}

func getenvFrom(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

func TestLoadValid(t *testing.T) {
	env := validEnv()
	env[EnvPort] = "9090"
	cfg, err := Load(getenvFrom(env))
	if err != nil {
		t.Fatalf("Load: unexpected error: %v", err)
	}
	if cfg.Port != "9090" {
		t.Errorf("Port = %q, want 9090", cfg.Port)
	}
	if cfg.BaseURL != "http://127.0.0.1:8080" {
		t.Errorf("BaseURL = %q", cfg.BaseURL)
	}
	if cfg.SpotifyClientID != "client-id-value" || cfg.SpotifyClientSecret != "client-secret-value" ||
		cfg.SetlistFMAPIKey != "setlistfm-key-value" {
		t.Errorf("credentials not loaded: %+v", cfg)
	}
	if len(cfg.SessionKey) != SessionKeySize {
		t.Errorf("len(SessionKey) = %d, want %d", len(cfg.SessionKey), SessionKeySize)
	}
}

func TestLoadDefaultPort(t *testing.T) {
	cfg, err := Load(getenvFrom(validEnv()))
	if err != nil {
		t.Fatalf("Load: unexpected error: %v", err)
	}
	if cfg.Port != DefaultPort {
		t.Errorf("Port = %q, want %q", cfg.Port, DefaultPort)
	}
}

func TestLoadAcceptsHTTPSBaseURL(t *testing.T) {
	env := validEnv()
	env[EnvBaseURL] = "https://setlisted-abc123.a.run.app"
	if _, err := Load(getenvFrom(env)); err != nil {
		t.Fatalf("Load: unexpected error: %v", err)
	}
}

func TestLoadInvalid(t *testing.T) {
	tests := []struct {
		name    string
		varName string
		value   string // "" means unset
	}{
		{"missing BASE_URL", EnvBaseURL, ""},
		{"missing SPOTIFY_CLIENT_ID", EnvSpotifyClientID, ""},
		{"missing SPOTIFY_CLIENT_SECRET", EnvSpotifyClientSecret, ""},
		{"missing SETLISTFM_API_KEY", EnvSetlistFMAPIKey, ""},
		{"missing SESSION_KEY", EnvSessionKey, ""},
		{"BASE_URL relative", EnvBaseURL, "setlisted.example.com"},
		{"BASE_URL wrong scheme", EnvBaseURL, "ftp://setlisted.example.com"},
		{"BASE_URL trailing slash", EnvBaseURL, "https://setlisted.example.com/"},
		{"BASE_URL with path", EnvBaseURL, "https://setlisted.example.com/app"},
		{"BASE_URL with query", EnvBaseURL, "https://setlisted.example.com?x=secretish"},
		{"BASE_URL with credentials", EnvBaseURL, "https://user:pass@setlisted.example.com"},
		{"BASE_URL unparseable", EnvBaseURL, "http://[::1"},
		{"SESSION_KEY not base64", EnvSessionKey, "not*base64*at*all"},
		{"SESSION_KEY too short", EnvSessionKey, "c2hvcnRrZXk="},                                    // "shortkey"
		{"SESSION_KEY too long", EnvSessionKey, "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA="}, // 35 bytes
		{"PORT not numeric", EnvPort, "eighty"},
		{"PORT out of range", EnvPort, "70000"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := validEnv()
			if tt.value == "" {
				delete(env, tt.varName)
			} else {
				env[tt.varName] = tt.value
			}
			_, err := Load(getenvFrom(env))
			if err == nil {
				t.Fatal("Load: expected error, got nil")
			}
			msg := err.Error()
			if !strings.Contains(msg, tt.varName) {
				t.Errorf("error %q does not name %s", msg, tt.varName)
			}
			if tt.value != "" && strings.Contains(msg, tt.value) {
				t.Errorf("error %q contains the variable's value", msg)
			}
		})
	}
}

func TestLoadReportsEveryProblemWithoutValues(t *testing.T) {
	env := map[string]string{
		EnvPort:       "nope",
		EnvSessionKey: "c2hvcnRrZXk=",
	}
	_, err := Load(getenvFrom(env))
	if err == nil {
		t.Fatal("Load: expected error, got nil")
	}
	msg := err.Error()
	for _, name := range []string{EnvPort, EnvBaseURL, EnvSpotifyClientID, EnvSpotifyClientSecret, EnvSetlistFMAPIKey, EnvSessionKey} {
		if !strings.Contains(msg, name) {
			t.Errorf("error %q does not name %s", msg, name)
		}
	}
	for _, v := range env {
		if strings.Contains(msg, v) {
			t.Errorf("error %q contains a value", msg)
		}
	}
}
