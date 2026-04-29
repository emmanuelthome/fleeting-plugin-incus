package provider

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/leomylonas/fleeting-plugin-incus/internal/cloudinit"
	"github.com/leomylonas/fleeting-plugin-incus/internal/config"
	"github.com/leomylonas/fleeting-plugin-incus/internal/incusclient"
	"github.com/lxc/incus/v6/shared/api"
)

func (g *InstanceGroup) managedInstances(ctx context.Context) ([]api.Instance, error) {
	// Incus does not have a first-class tag API for instances. Ownership is
	// therefore encoded as a normal instance config key under user.*, and we also
	// require the configured name prefix. Both must match.
	cfg, client := g.snapshot()

	// Request only the configured Incus instance type. That keeps container and
	// VM pools from seeing each other even if they share a project.
	instances, err := client.GetInstances(api.InstanceType(cfg.InstanceType))
	if err != nil {
		return nil, err
	}

	managed := make([]api.Instance, 0, len(instances))
	for _, inst := range instances {
		// Listing can return a lot of instances in a busy project. Check the
		// context during filtering so Runner shutdown does not wait for a full
		// scan if cancellation has already been requested.
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if g.isManaged(inst) {
			managed = append(managed, inst)
		}
	}

	return managed, nil
}

func (g *InstanceGroup) isManaged(inst api.Instance) bool {
	// The double check is deliberate. The prefix makes human inspection easy and
	// avoids scanning unrelated names; the user.* config key is the authoritative
	// pool identity that prevents prefix collisions from becoming destructive.
	cfg, _ := g.snapshot()

	// Both checks are required:
	// 1. the prefix marks the naming scheme this provider owns;
	// 2. the user.* config value marks the exact pool identity.
	return strings.HasPrefix(inst.Name, cfg.NamePrefix) &&
		inst.Config[cfg.NormalizedPoolConfigKey] == cfg.PoolID
}

func (g *InstanceGroup) createRequest(name string) api.InstancesPost {
	// Build the Incus create payload. Start with user-provided instance config,
	// add the managed-pool identity, and optionally add cloud-init data when the
	// plugin is responsible for injecting an SSH public key.
	cfg, _ := g.snapshot()
	instanceConfig := map[string]string{}
	for k, v := range cfg.InstanceConfig {
		// Copy the map instead of mutating cfg.InstanceConfig. cfg is shared
		// between calls, so mutation here would leak per-instance fields into
		// future creates.
		instanceConfig[k] = v
	}

	// Every created instance gets the pool marker. Later Update/Decrease calls
	// depend on this value to distinguish managed instances from unrelated ones.
	instanceConfig[cfg.NormalizedPoolConfigKey] = cfg.PoolID
	if cfg.Privileged {
		// Incus uses security.privileged to request an un-namespaced container.
		// Keep this as a top-level plugin option so Runner configs do not need to
		// know the Incus-specific instance config key.
		instanceConfig["security.privileged"] = "true"
	}
	if cfg.PublicKey != "" {
		// Only set cloud-init.user-data when the plugin is injecting a key. When
		// PublicKey is empty, the image/template is expected to be preconfigured
		// and any user-provided cloud-init config should be left untouched.
		instanceConfig["cloud-init.user-data"] = cloudinit.SSHUserData(cfg.ConnectorConfig.Username, cfg.PublicKey)
	}

	devices := map[string]map[string]string{}
	for deviceName, device := range cfg.Devices {
		copiedDevice := map[string]string{}
		for key, value := range device {
			copiedDevice[key] = value
		}
		devices[deviceName] = copiedDevice
	}
	if cfg.Network != "" {
		// Allow a simple network name in plugin config without forcing users to
		// spell out a full NIC device stanza when the defaults are sufficient.
		deviceName := cfg.NetworkInterface
		if deviceName == "" {
			deviceName = "eth0"
		}
		if _, exists := devices[deviceName]; !exists {
			devices[deviceName] = map[string]string{
				"type":    "nic",
				"network": cfg.Network,
				"name":    deviceName,
			}
		}
	}

	// InstancesPost is the Incus API request body for creating either a container
	// or VM. Start=true asks Incus to boot/start the instance immediately after
	// creating it.
	req := api.InstancesPost{
		Name: name,
		Type: api.InstanceType(cfg.InstanceType),
		InstancePut: api.InstancePut{
			Config:   api.ConfigMap(instanceConfig),
			Devices:  api.DevicesMap(devices),
			Profiles: append([]string(nil), cfg.Profiles...),
		},
		Start: true,
	}

	if cfg.Image != nil {
		// Image mode launches from an Incus image alias, fingerprint, or property
		// selector in the target project.
		req.Source = api.InstanceSource{
			Type:        "image",
			Alias:       cfg.Image.Alias,
			Fingerprint: cfg.Image.Fingerprint,
			Properties:  cfg.Image.Properties,
			Project:     cfg.Image.Project,
		}
	} else {
		// Template mode copies an existing stopped instance or snapshot. This is
		// useful when the runner image is heavily customized ahead of time.
		req.Source = api.InstanceSource{
			Type:         "copy",
			Source:       cfg.Template.Name,
			Project:      cfg.Template.Project,
			InstanceOnly: cfg.Template.InstanceOnly,
		}
	}

	return req
}

func (g *InstanceGroup) resolveImageSource(client incusclient.Client, req *api.InstancesPost) error {
	// Incus' low-level CreateInstance API is most reliable when the local image
	// source is a fingerprint. Accept aliases in plugin config for usability, but
	// resolve them just before creation so Incus receives an exact image target.
	if req.Source.Type != "image" || req.Source.Fingerprint != "" || req.Source.Alias == "" {
		return nil
	}

	imageClient := imageLookupClient(client)
	if req.Source.Project != "" {
		imageClient = imageLookupClient(client.UseProject(req.Source.Project))
	}

	alias, _, err := imageClient.GetImageAlias(req.Source.Alias)
	if err != nil {
		fingerprint, fallbackErr := cachedImageFingerprintForAlias(imageClient, req.Source.Alias, req.Type)
		if fallbackErr != nil {
			return fmt.Errorf("get image alias %q: %w; cached-image fallback: %w", req.Source.Alias, err, fallbackErr)
		}

		req.Source.Fingerprint = fingerprint
		req.Source.Alias = ""
		return nil
	}
	if alias.Target == "" {
		return fmt.Errorf("image alias %q has no target fingerprint", req.Source.Alias)
	}

	req.Source.Fingerprint = alias.Target
	req.Source.Alias = ""
	return nil
}

func (g *InstanceGroup) resolveTemplateSource(client incusclient.Client, req *api.InstancesPost) error {
	// Copy requests inherit devices from the source instance, but Incus treats an
	// explicit Devices map on the request as an override. Merge the source
	// devices in first so shorthand network injection or user device overrides do
	// not accidentally drop the template's root disk.
	if req.Source.Type != "copy" || req.Source.Source == "" {
		return nil
	}

	templateClient := client
	if req.Source.Project != "" {
		templateClient = client.UseProject(req.Source.Project)
	}

	inst, _, err := templateClient.GetInstance(req.Source.Source)
	if err != nil {
		return fmt.Errorf("get template %q: %w", req.Source.Source, err)
	}

	mergedDevices := map[string]map[string]string{}
	for deviceName, device := range inst.Devices {
		copiedDevice := map[string]string{}
		for key, value := range device {
			copiedDevice[key] = value
		}
		mergedDevices[deviceName] = copiedDevice
	}
	for deviceName, device := range req.Devices {
		copiedDevice := map[string]string{}
		for key, value := range device {
			copiedDevice[key] = value
		}
		mergedDevices[deviceName] = copiedDevice
	}
	req.Devices = api.DevicesMap(mergedDevices)

	return nil
}

type imageLookupClient interface {
	GetImageAlias(name string) (*api.ImageAliasesEntry, string, error)
	GetImages() ([]api.Image, error)
}

func cachedImageFingerprintForAlias(client imageLookupClient, alias string, instanceType api.InstanceType) (string, error) {
	// Incus' web UI can select a remote image and transparently cache it locally.
	// The cached image often has no local alias, so resolving the remote alias
	// from the local project fails even though a matching cached image exists.
	// Match cached images generically by the alias information Incus keeps on the
	// image itself instead of trying to reverse-engineer distribution-specific
	// naming schemes.

	images, err := client.GetImages()
	if err != nil {
		return "", err
	}

	var best *api.Image
	bestScore := 0
	for _, image := range images {
		if image.Fingerprint == "" {
			continue
		}
		if image.Type != "" && image.Type != string(instanceType) && image.Type != "all" {
			continue
		}
		score := imageAliasMatchScore(image, alias)
		if score == 0 {
			continue
		}
		if best == nil || score > bestScore || (score == bestScore && imageIsBetterAliasMatch(image, *best)) {
			candidate := image
			best = &candidate
			bestScore = score
		}
	}
	if best != nil {
		return best.Fingerprint, nil
	}

	return "", fmt.Errorf("no cached local image matched alias %q", alias)
}


func imageAliasMatchScore(image api.Image, alias string) int {
	alias = strings.Trim(strings.ToLower(alias), "/")
	if alias == "" {
		return 0
	}

	for _, knownAlias := range image.Aliases {
		if strings.EqualFold(strings.Trim(knownAlias.Name, "/"), alias) {
			return 100
		}
	}
	if image.UpdateSource != nil && strings.EqualFold(strings.Trim(image.UpdateSource.Alias, "/"), alias) {
		return 100
	}

	haystacks := imageAliasMetadata(image)
	segments := strings.Split(alias, "/")
	if len(segments) == 0 {
		return 0
	}

	score := 0
	primaryMatched := false
	for index, segment := range segments {
		segment = strings.TrimSpace(strings.ToLower(segment))
		if segment == "" {
			continue
		}
		matched := metadataContainsSegment(haystacks, segment)
		if index == 0 {
			if !matched {
				return 0
			}
			primaryMatched = true
			score += 3
			continue
		}
		if matched {
			score += 2
			continue
		}
		if looksLikeVersionSegment(segment) {
			continue
		}
	}
	if !primaryMatched {
		return 0
	}
	if len(segments) > 1 && score < 5 {
		return 0
	}
	return score
}

func imageAliasMetadata(image api.Image) []string {
	values := make([]string, 0, len(image.Properties)+len(image.Aliases)+4)
	for _, knownAlias := range image.Aliases {
		values = append(values, strings.ToLower(knownAlias.Name))
	}
	for _, value := range image.Properties {
		if value != "" {
			values = append(values, strings.ToLower(value))
		}
	}
	if image.UpdateSource != nil {
		values = append(values, strings.ToLower(image.UpdateSource.Alias))
		values = append(values, strings.ToLower(image.UpdateSource.Server))
	}
	if image.Architecture != "" {
		values = append(values, strings.ToLower(image.Architecture))
	}
	return values
}

func metadataContainsSegment(haystacks []string, needle string) bool {
	for _, haystack := range haystacks {
		if strings.Contains(haystack, needle) {
			return true
		}
	}
	return false
}

func looksLikeVersionSegment(value string) bool {
	for _, r := range value {
		if (r >= '0' && r <= '9') || r == '.' || r == '-' || r == '_' {
			continue
		}
		return false
	}
	return true
}

func imageIsBetterAliasMatch(candidate api.Image, current api.Image) bool {
	if candidate.Cached != current.Cached {
		return candidate.Cached
	}
	if !candidate.UploadedAt.Equal(current.UploadedAt) {
		return candidate.UploadedAt.After(current.UploadedAt)
	}
	if !candidate.CreatedAt.Equal(current.CreatedAt) {
		return candidate.CreatedAt.After(current.CreatedAt)
	}
	return candidate.Fingerprint > current.Fingerprint
}

func (g *InstanceGroup) snapshot() (config.Normalized, incusclient.Client) {
	// Copy the current config/client references under a read lock. The returned
	// values are immutable enough for method-local use and avoid holding a mutex
	// while network calls are in flight.
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.cfg, g.client
}

func (g *InstanceGroup) newName() (string, error) {
	// Incus instance names must be unique. A cryptographically random suffix is
	// simple, process-safe, and avoids maintaining shared counters.
	cfg, _ := g.snapshot()
	var b [6]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}

	// Twelve hex characters gives enough space for practical uniqueness while
	// keeping instance names readable in `incus list`.
	return cfg.NamePrefix + hex.EncodeToString(b[:]), nil
}
