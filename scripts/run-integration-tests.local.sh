#!/usr/bin/env bash
set -euo pipefail

# Local Incus integration test runner.

export INCUS_INTEGRATION=1

# Required by the integration tests. The image must already exist in the
# configured image project and must be usable for the selected instance type.
export INCUS_INTEGRATION_IMAGE_ALIAS="${INCUS_INTEGRATION_IMAGE_ALIAS:-ubuntu/24.04/cloud}"

# If INCUS_INTEGRATION_CONNECTION_TYPE is empty, the tests try to read the Incus
# client config from ~/.config/incus/config.yml and use its current/default
# remote. Set INCUS_INTEGRATION_REMOTE to select a non-default remote.
export INCUS_INTEGRATION_CONNECTION_TYPE="${INCUS_INTEGRATION_CONNECTION_TYPE:-}"
export INCUS_INTEGRATION_REMOTE="${INCUS_INTEGRATION_REMOTE:-}"
export INCUS_INTEGRATION_CONFIG_PATH="${INCUS_INTEGRATION_CONFIG_PATH:-}"
export INCUS_INTEGRATION_SOCKET_PATH="${INCUS_INTEGRATION_SOCKET_PATH:-}"
export INCUS_INTEGRATION_ENDPOINT="${INCUS_INTEGRATION_ENDPOINT:-}"

# HTTPS-only settings. Client cert/key default to the Incus CLI convention.
# These may be file paths or inline PEM contents. Leave server CA/cert empty if
# system trust or INSECURE_SKIP_VERIFY is enough for your test endpoint.
export INCUS_INTEGRATION_TLS_CLIENT_CERT="${INCUS_INTEGRATION_TLS_CLIENT_CERT:-}"
export INCUS_INTEGRATION_TLS_CLIENT_KEY="${INCUS_INTEGRATION_TLS_CLIENT_KEY:-}"
export INCUS_INTEGRATION_TLS_CA="${INCUS_INTEGRATION_TLS_CA:-}"
export INCUS_INTEGRATION_TLS_SERVER_CERT="${INCUS_INTEGRATION_TLS_SERVER_CERT:-}"
export INCUS_INTEGRATION_INSECURE_SKIP_VERIFY="${INCUS_INTEGRATION_INSECURE_SKIP_VERIFY:-1}"

# Test resource settings.
export INCUS_INTEGRATION_IMAGE_PROJECT="${INCUS_INTEGRATION_IMAGE_PROJECT:-default}"
export INCUS_INTEGRATION_INSTANCE_TYPE="${INCUS_INTEGRATION_INSTANCE_TYPE:-container}"
export INCUS_INTEGRATION_PROFILES="${INCUS_INTEGRATION_PROFILES:-default}"
export INCUS_INTEGRATION_UPLINK_NETWORK="${INCUS_INTEGRATION_UPLINK_NETWORK:-uplink}"
export INCUS_INTEGRATION_SSH_USERNAME="${INCUS_INTEGRATION_SSH_USERNAME:-root}"
export INCUS_INTEGRATION_WAIT_TIMEOUT="${INCUS_INTEGRATION_WAIT_TIMEOUT:-2m}"
export INCUS_INTEGRATION_PROJECT_PREFIX="${INCUS_INTEGRATION_PROJECT_PREFIX:-fleeting-plugin-incus-it}"

GO_BIN="${GO_BIN:-go}"
if ! command -v "${GO_BIN}" >/dev/null 2>&1 && [[ -x /tmp/go/bin/go ]]; then
  GO_BIN=/tmp/go/bin/go
fi

"${GO_BIN}" test ./internal/provider -run Integration -count=1 -v
