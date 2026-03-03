// Package proxy provides mTLS CA management and server for sandboxed polecat execution.
package proxy

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"time"
)

// CA holds the CA certificate and private key.
type CA struct {
	Cert    *x509.Certificate
	CertPEM []byte
	Key     *ecdsa.PrivateKey
}

// GenerateCA creates a new self-signed CA cert+key and writes them to dir.
func GenerateCA(dir string) (*CA, error) {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, fmt.Errorf("create ca dir: %w", err)
	}

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("generate ca key: %w", err)
	}

	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return nil, fmt.Errorf("generate ca serial: %w", err)
	}

	tmpl := &x509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{CommonName: "GasTown CA"},
		NotBefore:             time.Now().Add(-time.Minute),
		NotAfter:              time.Now().Add(10 * 365 * 24 * time.Hour),
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
	}

	certDER, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		return nil, fmt.Errorf("create ca cert: %w", err)
	}

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})

	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return nil, fmt.Errorf("marshal ca key: %w", err)
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})

	// Write each file to a *.tmp sibling then rename atomically so that a
	// crash between writes never leaves ca.crt and ca.key in an inconsistent state.
	certTmp := filepath.Join(dir, "ca.crt.tmp")
	if err := os.WriteFile(certTmp, certPEM, 0644); err != nil {
		return nil, fmt.Errorf("write ca.crt.tmp: %w", err)
	}
	if err := os.Rename(certTmp, filepath.Join(dir, "ca.crt")); err != nil {
		return nil, fmt.Errorf("rename ca.crt: %w", err)
	}
	keyTmp := filepath.Join(dir, "ca.key.tmp")
	if err := os.WriteFile(keyTmp, keyPEM, 0600); err != nil {
		return nil, fmt.Errorf("write ca.key.tmp: %w", err)
	}
	if err := os.Rename(keyTmp, filepath.Join(dir, "ca.key")); err != nil {
		return nil, fmt.Errorf("rename ca.key: %w", err)
	}

	cert, err := x509.ParseCertificate(certDER)
	if err != nil {
		return nil, fmt.Errorf("parse ca cert: %w", err)
	}

	return &CA{Cert: cert, CertPEM: certPEM, Key: key}, nil
}

// LoadOrGenerateCA loads the CA from dir if present, otherwise generates and saves it.
func LoadOrGenerateCA(dir string) (*CA, error) {
	certPath := filepath.Join(dir, "ca.crt")
	keyPath := filepath.Join(dir, "ca.key")

	certPEM, err := os.ReadFile(certPath)
	if errors.Is(err, os.ErrNotExist) {
		return GenerateCA(dir)
	}
	if err != nil {
		return nil, fmt.Errorf("read ca.crt: %w", err)
	}

	keyPEM, err := os.ReadFile(keyPath)
	if err != nil {
		return nil, fmt.Errorf("read ca.key: %w", err)
	}

	tlsCert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		return nil, fmt.Errorf("parse ca keypair: %w", err)
	}

	cert, err := x509.ParseCertificate(tlsCert.Certificate[0])
	if err != nil {
		return nil, fmt.Errorf("parse ca cert: %w", err)
	}

	if time.Now().After(cert.NotAfter) {
		return nil, fmt.Errorf("ca cert expired at %v; delete %s and restart to regenerate",
			cert.NotAfter, certPath)
	}

	key, ok := tlsCert.PrivateKey.(*ecdsa.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("ca key is not ECDSA")
	}

	return &CA{Cert: cert, CertPEM: certPEM, Key: key}, nil
}

// IssueServer issues a leaf certificate signed by the CA for use as a TLS server cert.
// The cn is always included as a DNS Subject Alternative Name so that modern TLS clients
// (Go 1.15+) can verify it without relying on the deprecated Common Name field.
// extraDNSNames are additional DNS SANs (e.g. operator-specified hostnames).
// extraIPs are added as IP Subject Alternative Names so clients connecting by IP address
// (e.g. a container reaching the proxy at 172.17.0.1) can verify the cert without
// setting an explicit ServerName override.
func (ca *CA) IssueServer(cn string, extraIPs []net.IP, extraDNSNames []string, ttl time.Duration) (certPEM, keyPEM []byte, err error) {
	dnsNames := append([]string{cn}, extraDNSNames...)
	return ca.issue(cn, dnsNames, extraIPs, ttl, x509.ExtKeyUsageServerAuth)
}

// IssuePolecat issues a leaf certificate signed by the CA for a polecat (client auth).
// cn must be in the format "gt-<rig>-<name>" with non-empty rig and name segments
// (e.g. "gt-gastown-furiosa"). Returns an error for malformed CNs to prevent issuing
// certs whose rig/name parsing would be inconsistent across exec and git auth.
func (ca *CA) IssuePolecat(cn string, ttl time.Duration) (certPEM, keyPEM []byte, err error) {
	if cnToIdentity(cn) == "" {
		return nil, nil, fmt.Errorf("invalid polecat CN %q: must be gt-<rig>-<name> with non-empty rig and name", cn)
	}
	return ca.issue(cn, nil, nil, ttl, x509.ExtKeyUsageClientAuth)
}

// issue creates and signs a leaf certificate. dnsNames and ipAddrs are added as SANs
// for server certs so that modern TLS stacks (Go 1.15+) accept them without relying on CN.
func (ca *CA) issue(cn string, dnsNames []string, ipAddrs []net.IP, ttl time.Duration, eku x509.ExtKeyUsage) (certPEM, keyPEM []byte, err error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, fmt.Errorf("generate leaf key: %w", err)
	}

	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return nil, nil, fmt.Errorf("generate serial: %w", err)
	}

	tmpl := &x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{CommonName: cn},
		DNSNames:     dnsNames, // required by modern TLS clients for server certs
		IPAddresses:  ipAddrs,  // required for clients connecting by IP address
		NotBefore:    time.Now().Add(-time.Minute),
		NotAfter:     time.Now().Add(ttl),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{eku},
	}

	certDER, err := x509.CreateCertificate(rand.Reader, tmpl, ca.Cert, &key.PublicKey, ca.Key)
	if err != nil {
		return nil, nil, fmt.Errorf("create leaf cert: %w", err)
	}

	certPEM = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})

	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return nil, nil, fmt.Errorf("marshal leaf key: %w", err)
	}
	keyPEM = pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})

	return certPEM, keyPEM, nil
}
