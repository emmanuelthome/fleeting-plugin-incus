package incusclient

import (
	"context"
	"fmt"

	"github.com/leomylonas/fleeting-plugin-incus/internal/config"
	incus "github.com/lxc/incus/v6/client"
	"github.com/lxc/incus/v6/shared/api"
)

const userAgent = "fleeting-plugin-incus"

// Client is the small part of incus.InstanceServer that the provider needs.
// Narrowing this interface keeps unit tests readable because tests can fake
// only these lifecycle calls instead of the entire Incus API.
type Client interface {
	Disconnect()
	UseProject(name string) incus.InstanceServer
	UseTarget(name string) incus.InstanceServer
	GetInstances(api.InstanceType) ([]api.Instance, error)
	GetInstance(name string) (*api.Instance, string, error)
	GetInstanceState(name string) (*api.InstanceState, string, error)
	GetImage(aliasOrFingerprint string) (*api.Image, string, error)
	GetImageAlias(name string) (*api.ImageAliasesEntry, string, error)
	GetImages() ([]api.Image, error)
	CreateInstance(api.InstancesPost) (incus.Operation, error)
	CreateInstanceFromImage(incus.ImageServer, api.Image, api.InstancesPost) (incus.RemoteOperation, error)
	UpdateInstanceState(name string, state api.InstanceStatePut, ETag string) (incus.Operation, error)
	DeleteInstance(name string) (incus.Operation, error)
}

func New(ctx context.Context, cfg config.Normalized) (incus.InstanceServer, error) {
	// The Incus Go client stores the constructor context on the returned client
	// and reuses it for later requests. Fleeting's Init context is RPC-scoped,
	// so retaining it causes all later API calls to fail with "context canceled"
	// once Init returns. Build the client with a long-lived background context
	// instead of the caller's request context.
	_ = ctx
	clientCtx := context.Background()

	tlsClientCert, err := config.ResolvePEMValue(cfg.TLSClientCert)
	if err != nil {
		return nil, fmt.Errorf("read tls_client_cert: %w", err)
	}
	tlsClientKey, err := config.ResolvePEMValue(cfg.TLSClientKey)
	if err != nil {
		return nil, fmt.Errorf("read tls_client_key: %w", err)
	}
	tlsServerCert, err := config.ResolvePEMValue(cfg.TLSServerCert)
	if err != nil {
		return nil, fmt.Errorf("read tls_server_cert: %w", err)
	}
	tlsCA, err := config.ResolvePEMValue(cfg.TLSCA)
	if err != nil {
		return nil, fmt.Errorf("read tls_ca: %w", err)
	}

	// The Incus client library only applies InsecureSkipVerify when no server
	// certificate is provided. If tls_server_cert is also set, the library
	// silently ignores InsecureSkipVerify and falls back to cert-pinning mode,
	// which still enforces hostname verification. To honour the user's intent of
	// skipping all TLS verification, clear the server cert so the library sees
	// tlsRemoteCert == nil and applies InsecureSkipVerify unconditionally.
	if cfg.InsecureSkipVerify {
		tlsServerCert = ""
	}

	// ConnectionArgs is shared by Unix and HTTPS clients. TLS fields are ignored
	// for Unix sockets, but setting them once keeps the connection branch simple.
	args := &incus.ConnectionArgs{
		UserAgent:          userAgent,
		TLSClientCert:      tlsClientCert,
		TLSClientKey:       tlsClientKey,
		TLSServerCert:      tlsServerCert,
		TLSCA:              tlsCA,
		InsecureSkipVerify: cfg.InsecureSkipVerify,
	}

	var srv incus.InstanceServer
	switch cfg.ConnectionType {
	case config.ConnectionUnix:
		// Empty SocketPath intentionally delegates to Incus' default socket
		// discovery logic, including INCUS_SOCKET and INCUS_DIR.
		srv, err = incus.ConnectIncusUnixWithContext(clientCtx, cfg.SocketPath, args)
	case config.ConnectionHTTPS:
		// Remote Incus access uses the official HTTPS client with mutual TLS.
		srv, err = incus.ConnectIncusWithContext(clientCtx, cfg.Endpoint, args)
	default:
		err = fmt.Errorf("unsupported connection_type %q", cfg.ConnectionType)
	}
	if err != nil {
		return nil, err
	}

	if cfg.Project != "" {
		// UseProject returns a scoped client rather than mutating srv.
		srv = srv.UseProject(cfg.Project)
	}
	if cfg.Target != "" {
		// Target is optional; when empty, Incus' own cluster scheduler chooses
		// where a new instance lands.
		srv = srv.UseTarget(cfg.Target)
	}

	return srv, nil
}
