package sandbox

import (
	"context"
	"errors"

	"github.com/steveyegge/gastown/internal/proxy"
)

// ProxyAdminAdapter adapts proxy.AdminClient to the sandbox.CertIssuer interface.
// proxy.AdminClient returns *proxy.IssueCertResult while CertIssuer requires
// *sandbox.CertResult — this adapter performs the field-by-field conversion.
type ProxyAdminAdapter struct {
	admin *proxy.AdminClient
}

// Compile-time interface assertion.
var _ CertIssuer = (*ProxyAdminAdapter)(nil)

// NewProxyAdminAdapter creates a ProxyAdminAdapter wrapping the given AdminClient.
func NewProxyAdminAdapter(admin *proxy.AdminClient) *ProxyAdminAdapter {
	return &ProxyAdminAdapter{admin: admin}
}

// IssueCert delegates to the underlying AdminClient and converts the result
// from *proxy.IssueCertResult to *sandbox.CertResult.
func (a *ProxyAdminAdapter) IssueCert(ctx context.Context, rig, name, ttl string) (*CertResult, error) {
	if a.admin == nil {
		return nil, errors.New("proxy admin client is nil: proxy not running")
	}
	result, err := a.admin.IssueCert(ctx, rig, name, ttl)
	if err != nil {
		return nil, err
	}
	if result == nil {
		return nil, nil
	}
	return &CertResult{
		CN:        result.CN,
		Cert:      result.Cert,
		Key:       result.Key,
		CA:        result.CA,
		Serial:    result.Serial,
		ExpiresAt: result.ExpiresAt,
	}, nil
}

// DenyCert delegates directly to the underlying AdminClient.
func (a *ProxyAdminAdapter) DenyCert(ctx context.Context, serial string) error {
	if a.admin == nil {
		return errors.New("proxy admin client is nil: proxy not running")
	}
	return a.admin.DenyCert(ctx, serial)
}
