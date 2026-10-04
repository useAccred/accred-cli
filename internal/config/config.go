// Package config stores preferences on disk and the API key in the system keychain.
package config

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"

	"github.com/zalando/go-keyring"
)

const (
	keyringService = "accred-cli"
	keyringUser    = "api-key"
	// EnvAPIKey takes precedence over the keychain.
	EnvAPIKey = "ACCRED_API_KEY"
)

type Config struct {
	Model     string `json:"model,omitempty"`
	MaxTokens int    `json:"maxTokens,omitempty"`
	NoMascot  bool   `json:"noMascot,omitempty"`
}

func path() (string, error) {
	dir := os.Getenv("XDG_CONFIG_HOME")
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		dir = filepath.Join(home, ".config")
	}
	return filepath.Join(dir, "accred", "config.json"), nil
}

// Load returns the saved preferences, or defaults when none exist.
func Load() (*Config, error) {
	cfg := &Config{}
	file, err := path()
	if err != nil {
		return cfg, err
	}
	raw, err := os.ReadFile(file)
	if errors.Is(err, os.ErrNotExist) {
		return cfg, nil
	}
	if err != nil {
		return cfg, err
	}
	return cfg, json.Unmarshal(raw, cfg)
}

func Save(cfg *Config) error {
	file, err := path()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(file), 0o700); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(file, append(raw, '\n'), 0o600)
}

// APIKey returns the key and where it came from ("environment" or "keychain").
func APIKey() (key, source string) {
	if key := os.Getenv(EnvAPIKey); key != "" {
		return key, "environment"
	}
	if key, err := keyring.Get(keyringService, keyringUser); err == nil && key != "" {
		return key, "keychain"
	}
	return "", ""
}

func SaveAPIKey(key string) error {
	return keyring.Set(keyringService, keyringUser, key)
}

// DeleteAPIKey removes the stored key. A missing key is not an error.
func DeleteAPIKey() error {
	err := keyring.Delete(keyringService, keyringUser)
	if errors.Is(err, keyring.ErrNotFound) {
		return nil
	}
	return err
}
