package cloudinit

import (
	"strings"
	"testing"
)

func TestSSHUserData(t *testing.T) {
	got := SSHUserData("runner", "ssh-ed25519 AAAATEST test@example")

	for _, want := range []string{"#cloud-config", "name: runner", "ssh-ed25519 AAAATEST test@example"} {
		if !strings.Contains(got, want) {
			t.Fatalf("SSHUserData() missing %q in:\n%s", want, got)
		}
	}
}

func TestSSHUserDataWithoutKey(t *testing.T) {
	if got := SSHUserData("runner", ""); got != "" {
		t.Fatalf("SSHUserData() = %q, want empty", got)
	}
}
