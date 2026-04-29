package main

import (
	incusprovider "github.com/leomylonas/fleeting-plugin-incus/internal/provider"
	"gitlab.com/gitlab-org/fleeting/fleeting/plugin"
)

var (
	// Release builds replace these values with -ldflags. The defaults keep local
	// development builds and `--version` useful before the first tagged release.
	name      = "fleeting-plugin-incus"
	version   = "dev"
	revision  = "HEAD"
	reference = "HEAD"
	builtAt   = "unknown"
)

func main() {
	// plugin.Main provides the Fleeting binary behavior: serve the plugin for
	// GitLab Runner, print version data, or bootstrap OCI metadata depending on
	// the command-line arguments.
	v := plugin.VersionInfo{
		Name:      name,
		Version:   version,
		Revision:  revision,
		Reference: reference,
		BuiltAt:   builtAt,
	}
	// The provider also reports build metadata during Init, so keep that value
	// synchronized with the CLI version information.
	incusprovider.Version = v

	plugin.Main(&incusprovider.InstanceGroup{}, v)
}
