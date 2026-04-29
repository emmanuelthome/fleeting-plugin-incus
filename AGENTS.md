# Repository Instructions

This repository contains a Go implementation of a GitLab Fleeting plugin for Incus.

## Development

- Use Go modules; the module path is `github.com/leomylonas/fleeting-plugin-incus`.
- The binary name is `fleeting-plugin-incus`; the command entrypoint lives in `cmd/fleeting-plugin-incus`.
- Keep provider behavior behind the Fleeting `provider.InstanceGroup` interface.
- Use the official Incus Go client, not shell calls to the `incus` CLI.
- Run `go test ./...`, `go vet ./...`, and `go build ./cmd/fleeting-plugin-incus` before handing off changes.
- Incus-backed integration tests exist in `internal/provider/integration_test.go`, but they must remain skipped unless `INCUS_INTEGRATION=1` is explicitly set. Do not run them casually; they create and delete real Incus resources.
- Release/build metadata defaults live in `cmd/fleeting-plugin-incus/main.go`. `internal/provider/version.go` should only store the value assigned by main.

## Provider Layout

- `internal/provider/group.go` should stay focused on the Fleeting `provider.InstanceGroup` lifecycle methods.
- `internal/provider/instances.go` owns managed-instance filtering, ownership checks, create request construction, snapshots, and naming helpers.
- `internal/provider/state.go` owns Incus-to-Fleeting state mapping and address selection helpers.
- Keep config semantics aligned across `internal/config/config.go`, `internal/provider/instances.go`, and the README when adding or changing plugin fields.
- Keep inline comments explanatory. The project is intentionally comment-heavy because it is also being used to learn Go.

## Incus Safety

- Managed Incus instances must match both the configured `name_prefix` and the configured `user.<pool_config_key>` value.
- Destructive operations must refuse instances that do not match the managed identity.
- The user configures `pool_config_key` without the `user.` prefix; code normalizes it before Incus API calls.
- SSH public key injection is optional. If `ssh_public_key` or `ssh_public_key_path` is set, the plugin owns `cloud-init.user-data`; otherwise images/templates are expected to already contain usable SSH credentials.
- Do not set or overwrite `cloud-init.user-data` unless the plugin is injecting the configured public key.
- The optional `network` config field is a shorthand for injecting a managed NIC device into new instances. If it is unset, networking falls back to explicit `devices` and the selected Incus profiles.
- When `network` is set, `createRequest` must not overwrite an explicit user-defined device with the same interface name.

## Images And Networking

- Image aliases may resolve either through a real Incus alias or through cached local images with matching metadata. Keep cached-image fallback generic; do not hard-code distribution- or release-specific alias parsing.
- The provider should resolve image aliases to fingerprints before `CreateInstance` so Incus receives an exact local image target.
- `network_interface` is used both to choose the preferred NIC for returned addresses and as the default device name when the provider injects a `network` NIC.

## Integration Tests

- The integration harness creates a temporary OVN network backed by `INCUS_INTEGRATION_UPLINK_NETWORK` and attaches test instances to that OVN network. It should not attach test instances directly to the uplink network.
- Integration tests must clean up both the temporary project and the temporary OVN network.
- Keep the integration test environment variable documentation in sync with `scripts/run-integration-tests.local.sh` and `internal/provider/integration_test.go`.
