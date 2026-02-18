//go:build !darwin

package quota

import "errors"

var errNotDarwin = errors.New("keychain operations are only supported on macOS")

// KeychainCredential holds a backup of a keychain credential for rollback.
type KeychainCredential struct {
	ServiceName string
	Token       string
}

func KeychainServiceName(_ string) string                                          { return "" }
func ReadKeychainToken(_ string) (string, error)                                   { return "", errNotDarwin }
func WriteKeychainToken(_, _, _ string) error                                      { return errNotDarwin }
func SwapKeychainCredential(_, _ string) (*KeychainCredential, error)              { return nil, errNotDarwin }
func RestoreKeychainToken(_ *KeychainCredential) error                             { return errNotDarwin }
