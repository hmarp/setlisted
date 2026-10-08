// Package config loads and validates the app's settings from environment
// variables (ADR-007).
package config

import (
	"encoding/base64"
	"errors"
	"fmt"
	"net/url"
	"strconv"
)

// Environment variable names.
const (
	EnvPort                = "PORT"
	EnvBaseURL             = "BASE_URL"
	EnvSpotifyClientID     = "SPOTIFY_CLIENT_ID"
	EnvSpotifyClientSecret = "SPOTIFY_CLIENT_SECRET"
	EnvSetlistFMAPIKey     = "SETLISTFM_API_KEY"
	EnvSessionKey          = "SESSION_KEY"
)

// DefaultPort is used when PORT is unset.
const DefaultPort = "8080"

// SessionKeySize is the required length of the decoded SESSION_KEY (AES-256).
const SessionKeySize = 32

// Config holds the validated settings. It contains secrets, so it must never
// be logged or otherwise printed.
type Config struct {
	Port                string // numeric, 1–65535
	BaseURL             string // absolute http(s) URL, no trailing slash
	SpotifyClientID     string
	SpotifyClientSecret string
	SetlistFMAPIKey     string
	SessionKey          []byte // exactly SessionKeySize bytes
}

// Load reads and validates every variable using getenv (normally os.Getenv).
// If anything is missing or invalid it returns an error naming each bad
// variable. Error messages never include a variable's value.
func Load(getenv func(string) string) (Config, error) {
	var cfg Config
	var errs []error
	fail := func(name, problem string) {
		errs = append(errs, fmt.Errorf("%s: %s", name, problem))
	}

	cfg.Port = getenv(EnvPort)
	if cfg.Port == "" {
		cfg.Port = DefaultPort
	} else if n, err := strconv.Atoi(cfg.Port); err != nil || n < 1 || n > 65535 {
		fail(EnvPort, "must be a port number between 1 and 65535")
	}

	cfg.BaseURL = getenv(EnvBaseURL)
	if cfg.BaseURL == "" {
		fail(EnvBaseURL, "is required")
	} else if problem := checkBaseURL(cfg.BaseURL); problem != "" {
		fail(EnvBaseURL, problem)
	}

	required := []struct {
		name string
		dst  *string
	}{
		{EnvSpotifyClientID, &cfg.SpotifyClientID},
		{EnvSpotifyClientSecret, &cfg.SpotifyClientSecret},
		{EnvSetlistFMAPIKey, &cfg.SetlistFMAPIKey},
	}
	for _, r := range required {
		*r.dst = getenv(r.name)
		if *r.dst == "" {
			fail(r.name, "is required")
		}
	}

	if raw := getenv(EnvSessionKey); raw == "" {
		fail(EnvSessionKey, "is required")
	} else if key, err := base64.StdEncoding.DecodeString(raw); err != nil {
		fail(EnvSessionKey, "is not valid base64")
	} else if len(key) != SessionKeySize {
		fail(EnvSessionKey, fmt.Sprintf("must decode to exactly %d bytes", SessionKeySize))
	} else {
		cfg.SessionKey = key
	}

	if len(errs) > 0 {
		return Config{}, errors.Join(errs...)
	}
	return cfg, nil
}

// checkBaseURL returns a description of what's wrong with s, or "" if it's
// valid. It must not include s in the description.
func checkBaseURL(s string) string {
	const want = "must be an absolute http(s) URL with no path, query or trailing slash, e.g. http://127.0.0.1:8080"
	u, err := url.Parse(s)
	if err != nil {
		return want
	}
	if (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil ||
		u.Path != "" || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || u.Opaque != "" {
		return want
	}
	return ""
}
