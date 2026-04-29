package cloudinit

import (
	"fmt"
	"strings"
)

func SSHUserData(username string, publicKey string) string {
	// Only render cloud-init when the plugin is injecting a key. If the key is
	// empty, the caller can skip setting cloud-init.user-data so pre-configured
	// images/templates do not need cloud-init at all.
	username = strings.TrimSpace(username)
	publicKey = strings.TrimSpace(publicKey)
	if publicKey == "" {
		return ""
	}

	return fmt.Sprintf(`#cloud-config
users:
  - default
  - name: %s
    ssh_authorized_keys:
      - %s
    sudo: ALL=(ALL) NOPASSWD:ALL
    shell: /bin/bash
`, username, publicKey)
}
