package provider

import fleetingplugin "gitlab.com/gitlab-org/fleeting/fleeting/plugin"

// Version is set by cmd/fleeting-plugin-incus/main.go before plugin.Serve starts.
// The provider package keeps only this storage variable so build defaults live
// in one place: the main package that receives release-time -ldflags.
var Version fleetingplugin.VersionInfo
