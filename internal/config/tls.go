package config

import (
	"os"
	"path/filepath"
	"strings"
)

func DefaultIncusClientCertPath() string {
	return filepath.Join(defaultIncusConfigDir(), "client.crt")
}

func DefaultIncusClientKeyPath() string {
	return filepath.Join(defaultIncusConfigDir(), "client.key")
}

func DefaultIncusClientConfigPath() string {
	return filepath.Join(defaultIncusConfigDir(), "config.yml")
}

func DefaultIncusServerCertPath(remote string) string {
	return filepath.Join(defaultIncusConfigDir(), "servercerts", remote+".crt")
}

func DefaultIncusClientCertAndKey(cert string, key string) (string, string) {
	// The Incus CLI convention is ~/.config/incus/client.crt and client.key. For
	// HTTPS connections, use those paths when the caller leaves both fields
	// empty, so plugin config can match normal `incus remote` setups.
	if strings.TrimSpace(cert) == "" && strings.TrimSpace(key) == "" {
		return DefaultIncusClientCertPath(), DefaultIncusClientKeyPath()
	}
	return cert, key
}

func ResolvePEMValue(value string) (string, error) {
	// Incus' Go client expects certificate/key PEM contents, but plugin config
	// is much nicer to operate with when it accepts file paths. Treat strings
	// that already look like PEM as inline PEM, and otherwise read the string as
	// a path. Empty remains empty for optional TLS fields.
	value = strings.TrimSpace(value)
	if value == "" || strings.Contains(value, "-----BEGIN ") {
		return value, nil
	}

	b, err := os.ReadFile(value)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(b)), nil
}

func defaultIncusConfigDir() string {
	// Match the common Incus client convention first. INCUS_CONF is supported by
	// Incus tooling for alternate client config locations; if it is present, it
	// should win over the home-directory default.
	if dir := os.Getenv("INCUS_CONF"); strings.TrimSpace(dir) != "" {
		return dir
	}
	if dir, err := os.UserConfigDir(); err == nil && dir != "" {
		return filepath.Join(dir, "incus")
	}
	return filepath.Join(os.Getenv("HOME"), ".config", "incus")
}
