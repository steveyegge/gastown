package proxy

import (
	"math/big"
	"sync"
)

// DenyList is a thread-safe in-memory set of revoked certificate serial numbers.
// Entries are keyed by the lowercase hexadecimal string of the serial number,
// which is unique per RFC 5280 within a single CA's issued certificates.
//
// The deny list is checked during the TLS handshake via VerifyPeerCertificate.
// Entries are never removed (TTLs are short; the whole CA rotates periodically).
type DenyList struct {
	mu     sync.RWMutex
	denied map[string]bool
}

// NewDenyList returns an empty deny list.
func NewDenyList() *DenyList {
	return &DenyList{denied: make(map[string]bool)}
}

// Deny adds a certificate serial number to the deny list.
// Subsequent IsDenied calls for the same serial return true.
// Calling Deny on an already-denied serial is a no-op.
func (d *DenyList) Deny(serial *big.Int) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.denied[serial.Text(16)] = true
}

// IsDenied reports whether the given serial number is on the deny list.
func (d *DenyList) IsDenied(serial *big.Int) bool {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.denied[serial.Text(16)]
}

// Len returns the number of entries currently in the deny list.
func (d *DenyList) Len() int {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return len(d.denied)
}
