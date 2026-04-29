package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolvePEMValueEmpty(t *testing.T) {
	got, err := ResolvePEMValue("")
	if err != nil {
		t.Fatalf("ResolvePEMValue() error = %v", err)
	}
	if got != "" {
		t.Fatalf("ResolvePEMValue() = %q, want empty", got)
	}
}

func TestResolvePEMValueInline(t *testing.T) {
	pem := "-----BEGIN CERTIFICATE-----\nabc\n-----END CERTIFICATE-----"
	got, err := ResolvePEMValue(pem)
	if err != nil {
		t.Fatalf("ResolvePEMValue() error = %v", err)
	}
	if got != pem {
		t.Fatalf("ResolvePEMValue() = %q, want inline PEM unchanged", got)
	}
}

func TestResolvePEMValuePath(t *testing.T) {
	path := filepath.Join(t.TempDir(), "client.crt")
	want := "-----BEGIN CERTIFICATE-----\nabc\n-----END CERTIFICATE-----"
	if err := os.WriteFile(path, []byte(want+"\n"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	got, err := ResolvePEMValue(path)
	if err != nil {
		t.Fatalf("ResolvePEMValue() error = %v", err)
	}
	if got != want {
		t.Fatalf("ResolvePEMValue() = %q, want file contents", got)
	}
}

func TestDefaultIncusClientCertAndKey(t *testing.T) {
	cert, key := DefaultIncusClientCertAndKey("", "")
	if !strings.HasSuffix(cert, filepath.Join(".config", "incus", "client.crt")) {
		t.Fatalf("cert path = %q, want Incus client.crt convention", cert)
	}
	if !strings.HasSuffix(key, filepath.Join(".config", "incus", "client.key")) {
		t.Fatalf("key path = %q, want Incus client.key convention", key)
	}

	cert, key = DefaultIncusClientCertAndKey("/custom/client.crt", "/custom/client.key")
	if cert != "/custom/client.crt" || key != "/custom/client.key" {
		t.Fatalf("custom cert/key = %q %q", cert, key)
	}
}
