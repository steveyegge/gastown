package proxy

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGenerateCA(t *testing.T) {
	t.Run("creates ca.crt and ca.key", func(t *testing.T) {
		dir := t.TempDir()
		ca, err := GenerateCA(dir)
		require.NoError(t, err)
		require.NotNil(t, ca)

		_, err = os.Stat(filepath.Join(dir, "ca.crt"))
		assert.NoError(t, err, "ca.crt should exist")
		_, err = os.Stat(filepath.Join(dir, "ca.key"))
		assert.NoError(t, err, "ca.key should exist")
	})

	t.Run("cert is valid self-signed CA", func(t *testing.T) {
		dir := t.TempDir()
		ca, err := GenerateCA(dir)
		require.NoError(t, err)

		assert.True(t, ca.Cert.IsCA, "IsCA should be true")
		assert.True(t, ca.Cert.BasicConstraintsValid)
		assert.NotZero(t, ca.Cert.KeyUsage&x509.KeyUsageCertSign, "KeyUsageCertSign should be set")

		// CA should verify against itself.
		pool := x509.NewCertPool()
		pool.AddCert(ca.Cert)
		_, err = ca.Cert.Verify(x509.VerifyOptions{Roots: pool})
		assert.NoError(t, err, "CA should verify against itself")
	})

	t.Run("ca.key file permissions are 0600", func(t *testing.T) {
		dir := t.TempDir()
		_, err := GenerateCA(dir)
		require.NoError(t, err)

		info, err := os.Stat(filepath.Join(dir, "ca.key"))
		require.NoError(t, err)
		assert.Equal(t, os.FileMode(0600), info.Mode().Perm())
	})

	t.Run("second call to same dir overwrites cleanly (no panic)", func(t *testing.T) {
		dir := t.TempDir()
		_, err := GenerateCA(dir)
		require.NoError(t, err)
		// Second call must not panic; may succeed (overwrite) or return error.
		assert.NotPanics(t, func() {
			_, _ = GenerateCA(dir)
		})
	})

	t.Run("unwritable dir returns error", func(t *testing.T) {
		if os.Getuid() == 0 {
			t.Skip("running as root; chmod restrictions do not apply")
		}
		parent := t.TempDir()
		require.NoError(t, os.Chmod(parent, 0500))
		t.Cleanup(func() { os.Chmod(parent, 0700) }) //nolint:errcheck
		_, err := GenerateCA(filepath.Join(parent, "ca"))
		assert.Error(t, err)
	})
}

func TestLoadOrGenerateCA(t *testing.T) {
	t.Run("nonexistent dir generates fresh CA", func(t *testing.T) {
		dir := filepath.Join(t.TempDir(), "newca")
		ca, err := LoadOrGenerateCA(dir)
		require.NoError(t, err)
		require.NotNil(t, ca)
		assert.True(t, ca.Cert.IsCA)
	})

	t.Run("existing files load matching CA", func(t *testing.T) {
		dir := t.TempDir()
		orig, err := GenerateCA(dir)
		require.NoError(t, err)

		loaded, err := LoadOrGenerateCA(dir)
		require.NoError(t, err)
		require.NotNil(t, loaded)

		assert.Equal(t, orig.Cert.SerialNumber, loaded.Cert.SerialNumber)
		assert.Equal(t, orig.CertPEM, loaded.CertPEM)
	})

	t.Run("ca.crt exists but ca.key missing returns error", func(t *testing.T) {
		dir := t.TempDir()
		_, err := GenerateCA(dir)
		require.NoError(t, err)
		require.NoError(t, os.Remove(filepath.Join(dir, "ca.key")))

		_, err = LoadOrGenerateCA(dir)
		assert.Error(t, err)
	})

	t.Run("corrupt ca.crt returns error", func(t *testing.T) {
		dir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(dir, "ca.crt"), []byte("not-valid-pem"), 0644))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "ca.key"), []byte("not-valid-pem"), 0600))

		_, err := LoadOrGenerateCA(dir)
		assert.Error(t, err)
	})

	t.Run("corrupt ca.key returns error", func(t *testing.T) {
		dir := t.TempDir()
		_, err := GenerateCA(dir)
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(filepath.Join(dir, "ca.key"), []byte("not-valid-pem"), 0600))

		_, err = LoadOrGenerateCA(dir)
		assert.Error(t, err)
	})

	t.Run("loaded CA can sign cert that verifies against same pool", func(t *testing.T) {
		dir := t.TempDir()
		_, err := GenerateCA(dir)
		require.NoError(t, err)

		ca, err := LoadOrGenerateCA(dir)
		require.NoError(t, err)

		certPEM, _, err := ca.IssueServer("test-server", nil, nil, time.Hour)
		require.NoError(t, err)

		block, _ := pem.Decode(certPEM)
		require.NotNil(t, block)
		cert, err := x509.ParseCertificate(block.Bytes)
		require.NoError(t, err)

		pool := x509.NewCertPool()
		pool.AddCert(ca.Cert)
		_, err = cert.Verify(x509.VerifyOptions{
			Roots:     pool,
			KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		})
		assert.NoError(t, err)
	})
}

func TestIssueServer(t *testing.T) {
	dir := t.TempDir()
	ca, err := GenerateCA(dir)
	require.NoError(t, err)

	t.Run("cert parses and verifies against CA", func(t *testing.T) {
		certPEM, keyPEM, err := ca.IssueServer("test-server", nil, nil, time.Hour)
		require.NoError(t, err)
		assert.NotNil(t, certPEM)
		assert.NotNil(t, keyPEM)

		block, _ := pem.Decode(certPEM)
		require.NotNil(t, block)
		cert, err := x509.ParseCertificate(block.Bytes)
		require.NoError(t, err)

		pool := x509.NewCertPool()
		pool.AddCert(ca.Cert)
		_, err = cert.Verify(x509.VerifyOptions{
			Roots:     pool,
			KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		})
		assert.NoError(t, err)
	})

	t.Run("ExtKeyUsage contains ServerAuth and NOT ClientAuth", func(t *testing.T) {
		certPEM, _, err := ca.IssueServer("test-server", nil, nil, time.Hour)
		require.NoError(t, err)

		block, _ := pem.Decode(certPEM)
		cert, err := x509.ParseCertificate(block.Bytes)
		require.NoError(t, err)

		hasServer, hasClient := false, false
		for _, eku := range cert.ExtKeyUsage {
			if eku == x509.ExtKeyUsageServerAuth {
				hasServer = true
			}
			if eku == x509.ExtKeyUsageClientAuth {
				hasClient = true
			}
		}
		assert.True(t, hasServer, "should have ServerAuth")
		assert.False(t, hasClient, "should NOT have ClientAuth")
	})

	t.Run("CN matches argument", func(t *testing.T) {
		certPEM, _, err := ca.IssueServer("my-proxy-server", nil, nil, time.Hour)
		require.NoError(t, err)

		block, _ := pem.Decode(certPEM)
		cert, err := x509.ParseCertificate(block.Bytes)
		require.NoError(t, err)
		assert.Equal(t, "my-proxy-server", cert.Subject.CommonName)
	})

	t.Run("NotAfter is approximately now+ttl within 5s tolerance", func(t *testing.T) {
		ttl := 2 * time.Hour
		before := time.Now().Add(ttl)
		certPEM, _, err := ca.IssueServer("test", nil, nil, ttl)
		require.NoError(t, err)
		after := time.Now().Add(ttl)

		block, _ := pem.Decode(certPEM)
		cert, err := x509.ParseCertificate(block.Bytes)
		require.NoError(t, err)

		assert.False(t, cert.NotAfter.Before(before.Add(-5*time.Second)),
			"NotAfter should be >= now+ttl-5s, got %v", cert.NotAfter)
		assert.False(t, cert.NotAfter.After(after.Add(5*time.Second)),
			"NotAfter should be <= now+ttl+5s, got %v", cert.NotAfter)
	})

	t.Run("random serial not big.NewInt(1)", func(t *testing.T) {
		certPEM1, _, err := ca.IssueServer("a", nil, nil, time.Hour)
		require.NoError(t, err)
		certPEM2, _, err := ca.IssueServer("b", nil, nil, time.Hour)
		require.NoError(t, err)

		block1, _ := pem.Decode(certPEM1)
		cert1, err := x509.ParseCertificate(block1.Bytes)
		require.NoError(t, err)

		block2, _ := pem.Decode(certPEM2)
		cert2, err := x509.ParseCertificate(block2.Bytes)
		require.NoError(t, err)

		assert.NotEqual(t, big.NewInt(1), cert1.SerialNumber,
			"serial should be random, not fixed 1")
		assert.NotEqual(t, cert1.SerialNumber, cert2.SerialNumber,
			"serials should differ between issuances")
	})
}

func TestIssuePolecat(t *testing.T) {
	dir := t.TempDir()
	ca, err := GenerateCA(dir)
	require.NoError(t, err)

	t.Run("cert parses and verifies against CA", func(t *testing.T) {
		certPEM, _, err := ca.IssuePolecat("gt-gastown-furiosa", time.Hour)
		require.NoError(t, err)

		block, _ := pem.Decode(certPEM)
		require.NotNil(t, block)
		cert, err := x509.ParseCertificate(block.Bytes)
		require.NoError(t, err)

		pool := x509.NewCertPool()
		pool.AddCert(ca.Cert)
		_, err = cert.Verify(x509.VerifyOptions{
			Roots:     pool,
			KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
		})
		assert.NoError(t, err)
	})

	t.Run("ExtKeyUsage contains ClientAuth and NOT ServerAuth", func(t *testing.T) {
		certPEM, _, err := ca.IssuePolecat("gt-gastown-furiosa", time.Hour)
		require.NoError(t, err)

		block, _ := pem.Decode(certPEM)
		cert, err := x509.ParseCertificate(block.Bytes)
		require.NoError(t, err)

		hasClient, hasServer := false, false
		for _, eku := range cert.ExtKeyUsage {
			if eku == x509.ExtKeyUsageClientAuth {
				hasClient = true
			}
			if eku == x509.ExtKeyUsageServerAuth {
				hasServer = true
			}
		}
		assert.True(t, hasClient, "should have ClientAuth")
		assert.False(t, hasServer, "should NOT have ServerAuth")
	})

	t.Run("CN matches and hyphenated rig round-trips", func(t *testing.T) {
		cases := []string{
			"gt-gastown-furiosa",
			"gt-gas-town-furiosa", // hyphenated rig name
		}
		for _, cn := range cases {
			cn := cn
			t.Run(cn, func(t *testing.T) {
				certPEM, _, err := ca.IssuePolecat(cn, time.Hour)
				require.NoError(t, err)

				block, _ := pem.Decode(certPEM)
				cert, err := x509.ParseCertificate(block.Bytes)
				require.NoError(t, err)
				assert.Equal(t, cn, cert.Subject.CommonName)
			})
		}
	})

	t.Run("malformed CNs are rejected", func(t *testing.T) {
		cases := []string{
			"gt--furiosa",     // empty rig segment
			"gt-gastown-",     // empty name segment
			"gt-",             // no rig or name
			"notgt-rig-name",  // missing gt- prefix
			"gt-nodashinrest", // no rig/name separator
			"",                // empty string
		}
		for _, cn := range cases {
			cn := cn
			t.Run(cn, func(t *testing.T) {
				_, _, err := ca.IssuePolecat(cn, time.Hour)
				assert.Error(t, err, "expected error for malformed CN %q", cn)
			})
		}
	})

	t.Run("TTL is respected", func(t *testing.T) {
		ttl := 30 * time.Minute
		certPEM, _, err := ca.IssuePolecat("gt-gastown-test", ttl)
		require.NoError(t, err)

		block, _ := pem.Decode(certPEM)
		cert, err := x509.ParseCertificate(block.Bytes)
		require.NoError(t, err)

		remaining := time.Until(cert.NotAfter)
		assert.True(t, remaining > 0, "cert should not be expired")
		assert.True(t, remaining <= ttl+5*time.Second, "cert TTL should match requested TTL")
	})
}

func TestCertEdgeCases(t *testing.T) {
	dir := t.TempDir()
	ca, err := GenerateCA(dir)
	require.NoError(t, err)

	t.Run("ttl=0 produces cert that is already expired or expiring immediately", func(t *testing.T) {
		certPEM, _, err := ca.IssueServer("test", nil, nil, 0)
		require.NoError(t, err)

		block, _ := pem.Decode(certPEM)
		cert, err := x509.ParseCertificate(block.Bytes)
		require.NoError(t, err)

		// NotAfter = Now + 0 ≈ now; with the -1min NotBefore skew it is essentially now.
		assert.False(t, cert.NotAfter.After(time.Now().Add(time.Minute)),
			"cert with ttl=0 should have NotAfter <= now+1min")
	})

	t.Run("very long CN does not panic", func(t *testing.T) {
		longCN := "gt-gastown-" + strings.Repeat("a", 1000)
		assert.NotPanics(t, func() {
			_, _, _ = ca.IssuePolecat(longCN, time.Hour)
		})
	})
}

func TestIssueServerIPSANs(t *testing.T) {
	dir := t.TempDir()
	ca, err := GenerateCA(dir)
	require.NoError(t, err)

	t.Run("IP SAN is embedded in cert", func(t *testing.T) {
		targetIP := net.ParseIP("192.168.1.100")
		loopback := net.ParseIP("127.0.0.1")
		certPEM, _, err := ca.IssueServer("gt-proxy-server", []net.IP{targetIP, loopback}, nil, time.Hour)
		require.NoError(t, err)

		block, _ := pem.Decode(certPEM)
		require.NotNil(t, block)
		cert, err := x509.ParseCertificate(block.Bytes)
		require.NoError(t, err)

		require.Len(t, cert.IPAddresses, 2, "cert should have 2 IP SANs")
		assert.True(t, cert.IPAddresses[0].Equal(targetIP), "first IP SAN should be the target IP")
		assert.True(t, cert.IPAddresses[1].Equal(loopback), "second IP SAN should be 127.0.0.1")
	})

	t.Run("cert verifies for IP SAN host", func(t *testing.T) {
		ip := net.ParseIP("10.0.0.1")
		certPEM, _, err := ca.IssueServer("gt-proxy-server", []net.IP{ip}, nil, time.Hour)
		require.NoError(t, err)

		block, _ := pem.Decode(certPEM)
		cert, err := x509.ParseCertificate(block.Bytes)
		require.NoError(t, err)

		pool := x509.NewCertPool()
		pool.AddCert(ca.Cert)
		_, err = cert.Verify(x509.VerifyOptions{
			Roots:     pool,
			DNSName:   "10.0.0.1",
			KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		})
		assert.NoError(t, err, "cert with IP SAN should verify for that IP")
	})

	t.Run("nil extraIPs produces no IP SANs", func(t *testing.T) {
		certPEM, _, err := ca.IssueServer("gt-proxy-server", nil, nil, time.Hour)
		require.NoError(t, err)

		block, _ := pem.Decode(certPEM)
		cert, err := x509.ParseCertificate(block.Bytes)
		require.NoError(t, err)

		assert.Empty(t, cert.IPAddresses, "nil extraIPs should produce no IP SANs")
	})
}

func TestLoadOrGenerateCAExpired(t *testing.T) {
	dir := t.TempDir()

	// Build an expired CA cert manually (GenerateCA always produces a fresh one).
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	require.NoError(t, err)

	tmpl := &x509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{CommonName: "GasTown CA (expired)"},
		NotBefore:             time.Now().Add(-2 * time.Hour),
		NotAfter:              time.Now().Add(-time.Hour), // already expired
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
	}

	certDER, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	require.NoError(t, err)

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	keyDER, err := x509.MarshalECPrivateKey(key)
	require.NoError(t, err)
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})

	require.NoError(t, os.WriteFile(filepath.Join(dir, "ca.crt"), certPEM, 0644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "ca.key"), keyPEM, 0600))

	_, err = LoadOrGenerateCA(dir)
	assert.Error(t, err, "LoadOrGenerateCA should return error for expired CA cert")
	assert.Contains(t, err.Error(), "expired", "error message should mention 'expired'")
}
