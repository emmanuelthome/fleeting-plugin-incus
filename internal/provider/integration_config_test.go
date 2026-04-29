package provider

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/leomylonas/fleeting-plugin-incus/internal/config"
)

func TestLoadIncusClientIntegrationConfigHTTPSRemote(t *testing.T) {
	path := writeIncusClientConfig(t, `
default-remote: prod
remotes:
  prod:
    addr: https://incus.example.test:8443
    protocol: incus
`)
	t.Setenv("INCUS_INTEGRATION_CONFIG_PATH", path)

	got, found, err := loadIncusClientIntegrationConfig()
	if err != nil {
		t.Fatalf("loadIncusClientIntegrationConfig() error = %v", err)
	}
	if !found {
		t.Fatal("loadIncusClientIntegrationConfig() found = false")
	}
	if got.ConnectionType != config.ConnectionHTTPS {
		t.Fatalf("ConnectionType = %q", got.ConnectionType)
	}
	if got.Endpoint != "https://incus.example.test:8443" {
		t.Fatalf("Endpoint = %q", got.Endpoint)
	}
	if got.TLSClientCert == "" || got.TLSClientKey == "" {
		t.Fatal("expected Incus client cert/key defaults")
	}
}

func TestLoadIncusClientIntegrationConfigUnixRemote(t *testing.T) {
	path := writeIncusClientConfig(t, `
default-remote: local
remotes:
  local:
    addr: unix:///run/incus/unix.socket
    protocol: incus
`)
	t.Setenv("INCUS_INTEGRATION_CONFIG_PATH", path)

	got, found, err := loadIncusClientIntegrationConfig()
	if err != nil {
		t.Fatalf("loadIncusClientIntegrationConfig() error = %v", err)
	}
	if !found {
		t.Fatal("loadIncusClientIntegrationConfig() found = false")
	}
	if got.ConnectionType != config.ConnectionUnix {
		t.Fatalf("ConnectionType = %q", got.ConnectionType)
	}
	if got.SocketPath != "/run/incus/unix.socket" {
		t.Fatalf("SocketPath = %q", got.SocketPath)
	}
}

func TestLoadIncusClientIntegrationConfigRemoteOverride(t *testing.T) {
	path := writeIncusClientConfig(t, `
default-remote: local
remotes:
  local:
    addr: unix://
    protocol: incus
  prod:
    addr: https://incus.example.test:8443
    protocol: incus
`)
	t.Setenv("INCUS_INTEGRATION_CONFIG_PATH", path)
	t.Setenv("INCUS_INTEGRATION_REMOTE", "prod")

	got, found, err := loadIncusClientIntegrationConfig()
	if err != nil {
		t.Fatalf("loadIncusClientIntegrationConfig() error = %v", err)
	}
	if !found {
		t.Fatal("loadIncusClientIntegrationConfig() found = false")
	}
	if got.ConnectionType != config.ConnectionHTTPS || got.Endpoint != "https://incus.example.test:8443" {
		t.Fatalf("config = %#v", got)
	}
}

func TestLoadIncusClientIntegrationConfigMissingFile(t *testing.T) {
	t.Setenv("INCUS_INTEGRATION_CONFIG_PATH", filepath.Join(t.TempDir(), "missing.yml"))

	_, found, err := loadIncusClientIntegrationConfig()
	if err != nil {
		t.Fatalf("loadIncusClientIntegrationConfig() error = %v", err)
	}
	if found {
		t.Fatal("loadIncusClientIntegrationConfig() found = true, want false")
	}
}

func writeIncusClientConfig(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yml")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	return path
}
