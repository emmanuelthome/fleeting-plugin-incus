package provider

import (
	"net"

	"github.com/lxc/incus/v6/shared/api"
	fleeting "gitlab.com/gitlab-org/fleeting/fleeting/provider"
)

func StateFromIncus(status api.StatusCode) fleeting.State {
	// Fleeting has a smaller state model than Incus, so this function is the
	// single translation point between the two APIs.
	switch status {
	case api.Pending, api.Starting, api.Ready:
		// Incus has accepted or is preparing the instance, but Runner should not
		// try to connect yet.
		return fleeting.StateCreating
	case api.Started, api.Running:
		// Running is the only normal state where Runner can use the worker.
		return fleeting.StateRunning
	case api.Stopping:
		// A stopping instance is on its way out from Fleeting's perspective.
		return fleeting.StateDeleting
	case api.Freezing:
		// Incus freeze is the implementation behind Fleeting suspend.
		return fleeting.StateSuspending
	case api.Frozen:
		return fleeting.StateSuspended
	case api.Thawed:
		// Thawed is transient after unfreeze; report it as resuming until Update
		// sees Running again.
		return fleeting.StateResuming
	case api.Error, api.Failure, api.Cancelled, api.Aborting:
		// Fleeting does not have a generic "error" state; timeout is the closest
		// unhealthy terminal-ish state available to the provider interface.
		return fleeting.StateTimeout
	case api.Stopped:
		// Stopped instances cannot run jobs. Treat them as deleted so Fleeting can
		// request replacement capacity when needed.
		return fleeting.StateDeleted
	default:
		return fleeting.StateTimeout
	}
}

func SelectAddress(state *api.InstanceState, preferredInterface string, family string) string {
	// Incus reports addresses per network interface. Prefer a configured
	// interface when supplied; otherwise use the first non-loopback global
	// address in the requested family.
	if state == nil {
		// Be defensive for tests and unusual Incus responses.
		return ""
	}
	if family == "" {
		// Incus uses "inet" for IPv4 and "inet6" for IPv6.
		family = "inet"
	}

	if preferredInterface != "" {
		// A configured interface removes ambiguity on images with multiple NICs,
		// such as a management network plus a job network.
		if network, ok := state.Network[preferredInterface]; ok {
			if addr := selectAddressFromNetwork(network, family); addr != "" {
				return addr
			}
		}
	}

	for name, network := range state.Network {
		// Skip loopback even if it has a global-looking address; Runner needs an
		// address reachable from the manager process.
		if name == "lo" || network.Type == "loopback" {
			continue
		}
		if addr := selectAddressFromNetwork(network, family); addr != "" {
			return addr
		}
	}

	return ""
}

func selectAddressFromNetwork(network api.InstanceStateNetwork, family string) string {
	// Only return globally scoped addresses from interfaces that are up. Link
	// local IPv6 or loopback addresses are not useful for Runner SSH access.
	if network.State != "" && network.State != "up" {
		return ""
	}
	for _, addr := range network.Addresses {
		// Scope "global" excludes link-local IPv6 addresses and local-only
		// addresses that are not useful for SSH from GitLab Runner.
		if addr.Family == family && addr.Scope == "global" && addr.Address != "" {
			return addr.Address
		}
	}
	return ""
}

func isPrivateAddress(addr string) bool {
	// This classification only affects which ConnectInfo field is populated; it
	// does not block private addresses, because Incus worker networks are often
	// private and reachable from the runner manager.
	ip := net.ParseIP(addr)
	if ip == nil {
		// Hostnames should normally not appear here because Incus reports IP
		// addresses, but treat anything unparsable as external rather than hiding
		// it from Runner.
		return false
	}
	return ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast()
}
