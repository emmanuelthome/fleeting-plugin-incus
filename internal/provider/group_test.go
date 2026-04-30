package provider

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/leomylonas/fleeting-plugin-incus/internal/config"
	incus "github.com/lxc/incus/v6/client"
	"github.com/lxc/incus/v6/shared/api"
	"gitlab.com/gitlab-org/fleeting/fleeting/provider"
)

func TestStateFromIncus(t *testing.T) {
	tests := map[api.StatusCode]provider.State{
		api.Pending:  provider.StateCreating,
		api.Running:  provider.StateRunning,
		api.Stopping: provider.StateDeleting,
		api.Freezing: provider.StateSuspending,
		api.Frozen:   provider.StateSuspended,
		api.Error:    provider.StateTimeout,
		api.Stopped:  provider.StateDeleted,
	}

	for status, want := range tests {
		if got := StateFromIncus(status); got != want {
			t.Fatalf("StateFromIncus(%v) = %q, want %q", status, got, want)
		}
	}
}

func TestSelectAddress(t *testing.T) {
	state := &api.InstanceState{Network: map[string]api.InstanceStateNetwork{
		"lo": {
			Type: "loopback",
			Addresses: []api.InstanceStateNetworkAddress{
				{Family: "inet", Scope: "local", Address: "127.0.0.1"},
			},
		},
		"eth0": {
			State: "up",
			Type:  "broadcast",
			Addresses: []api.InstanceStateNetworkAddress{
				{Family: "inet6", Scope: "link", Address: "fe80::1"},
				{Family: "inet", Scope: "global", Address: "10.10.10.20"},
			},
		},
	}}

	if got := SelectAddress(state, "", "inet"); got != "10.10.10.20" {
		t.Fatalf("SelectAddress() = %q", got)
	}
	if got := SelectAddress(state, "eth0", "inet6"); got != "" {
		t.Fatalf("SelectAddress() link-local inet6 = %q, want empty", got)
	}
}

func TestDecreaseRefusesUnmanagedInstance(t *testing.T) {
	cfg := config.Normalized{
		Config: config.Config{
			NamePrefix: "ci-",
			PoolID:     "pool-a",
		},
		NormalizedPoolConfigKey: "user.fleeting.pool",
	}
	client := &fakeClient{instances: map[string]api.Instance{
		"ci-owned": {
			Name:       "ci-owned",
			StatusCode: api.Stopped,
			InstancePut: api.InstancePut{Config: map[string]string{
				"user.fleeting.pool": "pool-a",
			}},
		},
		"ci-other": {
			Name:       "ci-other",
			StatusCode: api.Stopped,
			InstancePut: api.InstancePut{Config: map[string]string{
				"user.fleeting.pool": "pool-b",
			}},
		},
	}}

	group := &InstanceGroup{cfg: cfg, client: client}
	succeeded, err := group.Decrease(context.Background(), []string{"ci-owned", "ci-other"})
	if err == nil {
		t.Fatal("Decrease() error = nil, want unmanaged error")
	}
	if len(succeeded) != 1 || succeeded[0] != "ci-owned" {
		t.Fatalf("Decrease() succeeded = %#v", succeeded)
	}
	if !client.deleted["ci-owned"] || client.deleted["ci-other"] {
		t.Fatalf("deleted = %#v", client.deleted)
	}
}

func TestCreateRequestOmitsCloudInitWithoutPublicKey(t *testing.T) {
	group := &InstanceGroup{cfg: config.Normalized{
		Config: config.Config{
			NamePrefix:   "ci-",
			PoolID:       "pool-a",
			InstanceType: config.InstanceContainer,
			Image:        &config.ImageSource{Alias: "ubuntu/24.04"},
		},
		NormalizedPoolConfigKey: "user.fleeting.pool",
		ConnectorConfig: provider.ConnectorConfig{
			Username: "runner",
		},
	}}

	req := group.createRequest("ci-test")
	if _, ok := req.Config["cloud-init.user-data"]; ok {
		t.Fatal("cloud-init.user-data was set without a public key")
	}
	if got := req.Config["user.fleeting.pool"]; got != "pool-a" {
		t.Fatalf("pool marker = %q, want pool-a", got)
	}
}

func TestCreateRequestSetsPrivilegedContainerConfig(t *testing.T) {
	group := &InstanceGroup{cfg: config.Normalized{
		Config: config.Config{
			NamePrefix:   "ci-",
			PoolID:       "pool-a",
			InstanceType: config.InstanceContainer,
			Privileged:   true,
			Image:        &config.ImageSource{Alias: "ubuntu/24.04"},
		},
		NormalizedPoolConfigKey: "user.fleeting.pool",
		ConnectorConfig: provider.ConnectorConfig{
			Username: "runner",
		},
	}}

	req := group.createRequest("ci-test")
	if got := req.Config["security.privileged"]; got != "true" {
		t.Fatalf("security.privileged = %q, want true", got)
	}
}

func TestUpdateKeepsRunningInstanceCreatingUntilAddressAssigned(t *testing.T) {
	group := &InstanceGroup{cfg: config.Normalized{
		Config: config.Config{
			NamePrefix:    "ci-",
			PoolID:        "pool-a",
			InstanceType:  config.InstanceContainer,
			AddressFamily: "inet",
		},
		NormalizedPoolConfigKey: "user.fleeting.pool",
	}, client: &fakeClient{
		instances: map[string]api.Instance{
			"ci-test": {
				Name:       "ci-test",
				StatusCode: api.Running,
				InstancePut: api.InstancePut{Config: map[string]string{
					"user.fleeting.pool": "pool-a",
				}},
			},
		},
		states: map[string]api.InstanceState{
			"ci-test": {
				Network: map[string]api.InstanceStateNetwork{
					"eth0": {
						State: "up",
						Type:  "broadcast",
						Addresses: []api.InstanceStateNetworkAddress{
							{Family: "inet6", Scope: "link", Address: "fe80::1"},
						},
					},
				},
			},
		},
	}}

	seen := map[string]provider.State{}
	err := group.Update(context.Background(), func(instance string, state provider.State) {
		seen[instance] = state
	})
	if err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	if seen["ci-test"] != provider.StateCreating {
		t.Fatalf("Update() state = %q, want creating", seen["ci-test"])
	}
}

func TestImageAliasMatchScore(t *testing.T) {
	image := api.Image{
		Fingerprint: "newer",
		Cached:      true,
		UploadedAt:  time.Unix(20, 0),
		ImagePut: api.ImagePut{
			Properties: map[string]string{
				"os":      "ubuntu",
				"release": "noble",
				"variant": "cloud",
				"name":    "ubuntu-noble-amd64-cloud-20260429_07:42",
			},
		},
	}
	if score := imageAliasMatchScore(image, "ubuntu/24.04/cloud"); score == 0 {
		t.Fatal("imageAliasMatchScore() = 0, want positive match")
	}
	older := api.Image{Fingerprint: "older", Cached: true, UploadedAt: time.Unix(10, 0)}
	if !imageIsBetterAliasMatch(image, older) {
		t.Fatal("imageIsBetterAliasMatch() = false, want true")
	}
}

func TestCreateRequestAddsConfiguredNetworkDevice(t *testing.T) {
	group := &InstanceGroup{cfg: config.Normalized{
		Config: config.Config{
			NamePrefix:       "ci-",
			PoolID:           "pool-a",
			InstanceType:     config.InstanceContainer,
			Image:            &config.ImageSource{Alias: "ubuntu/24.04"},
			Network:          "uplink",
			NetworkInterface: "enp5s0",
		},
		NormalizedPoolConfigKey: "user.fleeting.pool",
		ConnectorConfig: provider.ConnectorConfig{
			Username: "runner",
		},
	}}

	req := group.createRequest("ci-test")
	device, ok := req.Devices["enp5s0"]
	if !ok {
		t.Fatal("network device not added")
	}
	if device["type"] != "nic" || device["network"] != "uplink" || device["name"] != "enp5s0" {
		t.Fatalf("network device = %#v", device)
	}
}

func TestCreateRequestKeepsExplicitNetworkDevice(t *testing.T) {
	group := &InstanceGroup{cfg: config.Normalized{
		Config: config.Config{
			NamePrefix:       "ci-",
			PoolID:           "pool-a",
			InstanceType:     config.InstanceContainer,
			Image:            &config.ImageSource{Alias: "ubuntu/24.04"},
			Network:          "uplink",
			NetworkInterface: "eth0",
			Devices: map[string]map[string]string{
				"eth0": {
					"type":    "nic",
					"network": "custom-override",
					"name":    "eth0",
				},
			},
		},
		NormalizedPoolConfigKey: "user.fleeting.pool",
		ConnectorConfig: provider.ConnectorConfig{
			Username: "runner",
		},
	}}

	req := group.createRequest("ci-test")
	device, ok := req.Devices["eth0"]
	if !ok {
		t.Fatal("explicit network device missing")
	}
	if device["network"] != "custom-override" {
		t.Fatalf("network device override lost: %#v", device)
	}
}

func TestCreateRequestAddsRootDiskOverrides(t *testing.T) {
	group := &InstanceGroup{cfg: config.Normalized{
		Config: config.Config{
			NamePrefix:   "ci-",
			PoolID:       "pool-a",
			InstanceType: config.InstanceContainer,
			Image:        &config.ImageSource{Alias: "ubuntu/24.04"},
			StoragePool:  "fast",
			RootDiskSize: "30GiB",
		},
		NormalizedPoolConfigKey: "user.fleeting.pool",
		ConnectorConfig: provider.ConnectorConfig{
			Username: "runner",
		},
	}}

	req := group.createRequest("ci-test")
	device, ok := req.Devices["root"]
	if !ok {
		t.Fatal("root disk device not added")
	}
	if device["type"] != "disk" || device["path"] != "/" || device["pool"] != "fast" || device["size"] != "30GiB" {
		t.Fatalf("root disk device = %#v", device)
	}
}

func TestCreateRequestMergesRootDiskOverrides(t *testing.T) {
	group := &InstanceGroup{cfg: config.Normalized{
		Config: config.Config{
			NamePrefix:   "ci-",
			PoolID:       "pool-a",
			InstanceType: config.InstanceContainer,
			Image:        &config.ImageSource{Alias: "ubuntu/24.04"},
			RootDiskSize: "50GiB",
			Devices: map[string]map[string]string{
				"custom-root": {
					"type": "disk",
					"path": "/",
					"pool": "slow",
				},
			},
		},
		NormalizedPoolConfigKey: "user.fleeting.pool",
		ConnectorConfig: provider.ConnectorConfig{
			Username: "runner",
		},
	}}

	req := group.createRequest("ci-test")
	device, ok := req.Devices["custom-root"]
	if !ok {
		t.Fatal("custom root disk device missing")
	}
	if device["pool"] != "slow" || device["size"] != "50GiB" {
		t.Fatalf("root disk override did not merge: %#v", device)
	}
}

func TestResolveTemplateSourceMergesTemplateDevices(t *testing.T) {
	group := &InstanceGroup{}
	client := &fakeClient{instances: map[string]api.Instance{
		"template": {
			Name: "template",
			InstancePut: api.InstancePut{Devices: map[string]map[string]string{
				"root": {
					"type": "disk",
					"path": "/",
					"pool": "default",
				},
			}},
		},
	}}
	req := api.InstancesPost{
		InstancePut: api.InstancePut{Devices: api.DevicesMap{
			"eth0": {
				"type":    "nic",
				"name":    "eth0",
				"network": "managed-net",
			},
		}},
		Source: api.InstanceSource{
			Type:   "copy",
			Source: "template",
		},
	}

	if err := group.resolveTemplateSource(client, &req); err != nil {
		t.Fatalf("resolveTemplateSource() error = %v", err)
	}
	if _, ok := req.Devices["root"]; !ok {
		t.Fatalf("template root device missing after merge: %#v", req.Devices)
	}
	if req.Devices["eth0"]["network"] != "managed-net" {
		t.Fatalf("explicit device override lost: %#v", req.Devices["eth0"])
	}
}

func TestResolveTemplateSourceAppliesRootDiskOverrides(t *testing.T) {
	group := &InstanceGroup{cfg: config.Normalized{
		Config: config.Config{
			StoragePool:  "fast",
			RootDiskSize: "40GiB",
		},
	}}
	client := &fakeClient{instances: map[string]api.Instance{
		"template": {
			Name: "template",
			InstancePut: api.InstancePut{Devices: map[string]map[string]string{
				"root": {
					"type": "disk",
					"path": "/",
					"pool": "default",
				},
			}},
		},
	}}
	req := api.InstancesPost{
		Source: api.InstanceSource{
			Type:   "copy",
			Source: "template",
		},
	}

	if err := group.resolveTemplateSource(client, &req); err != nil {
		t.Fatalf("resolveTemplateSource() error = %v", err)
	}
	device := req.Devices["root"]
	if device["pool"] != "fast" || device["size"] != "40GiB" {
		t.Fatalf("root disk override not applied: %#v", device)
	}
}

type fakeClient struct {
	incus.InstanceServer
	instances map[string]api.Instance
	states    map[string]api.InstanceState
	deleted   map[string]bool
}

func (f *fakeClient) Disconnect() {}

func (f *fakeClient) GetInstances(instanceType api.InstanceType) ([]api.Instance, error) {
	out := make([]api.Instance, 0, len(f.instances))
	for _, inst := range f.instances {
		out = append(out, inst)
	}
	return out, nil
}

func (f *fakeClient) GetInstance(name string) (*api.Instance, string, error) {
	inst, ok := f.instances[name]
	if !ok {
		return nil, "", errors.New("not found")
	}
	return &inst, "", nil
}

func (f *fakeClient) GetInstanceState(name string) (*api.InstanceState, string, error) {
	state := f.states[name]
	return &state, "", nil
}

func (f *fakeClient) CreateInstance(api.InstancesPost) (incus.Operation, error) {
	return fakeOperation{}, nil
}

func (f *fakeClient) UpdateInstanceState(name string, state api.InstanceStatePut, ETag string) (incus.Operation, error) {
	return fakeOperation{}, nil
}

func (f *fakeClient) DeleteInstance(name string) (incus.Operation, error) {
	if f.deleted == nil {
		f.deleted = map[string]bool{}
	}
	f.deleted[name] = true
	return fakeOperation{}, nil
}

type fakeOperation struct{}

func (fakeOperation) AddHandler(func(api.Operation)) (*incus.EventTarget, error) { return nil, nil }
func (fakeOperation) Cancel() error                                              { return nil }
func (fakeOperation) Get() api.Operation                                         { return api.Operation{} }
func (fakeOperation) GetWebsocket(string) (*websocket.Conn, error)               { return nil, nil }
func (fakeOperation) RemoveHandler(*incus.EventTarget) error                     { return nil }
func (fakeOperation) Refresh() error                                             { return nil }
func (fakeOperation) Wait() error                                                { return nil }
func (fakeOperation) WaitContext(context.Context) error                          { return nil }
