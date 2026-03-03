package proxy

import (
	"math/big"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDenyList(t *testing.T) {
	t.Run("empty list: IsDenied returns false", func(t *testing.T) {
		d := NewDenyList()
		assert.False(t, d.IsDenied(big.NewInt(42)))
		assert.Equal(t, 0, d.Len())
	})

	t.Run("after Deny: IsDenied returns true", func(t *testing.T) {
		d := NewDenyList()
		serial := big.NewInt(0xdeadbeef)
		d.Deny(serial)
		assert.True(t, d.IsDenied(serial))
		assert.Equal(t, 1, d.Len())
	})

	t.Run("different serial is not denied", func(t *testing.T) {
		d := NewDenyList()
		d.Deny(big.NewInt(1))
		assert.False(t, d.IsDenied(big.NewInt(2)))
	})

	t.Run("Deny is idempotent", func(t *testing.T) {
		d := NewDenyList()
		serial := big.NewInt(99)
		d.Deny(serial)
		d.Deny(serial)
		assert.True(t, d.IsDenied(serial))
		assert.Equal(t, 1, d.Len())
	})

	t.Run("large serial (128-bit)", func(t *testing.T) {
		d := NewDenyList()
		// Simulate a real cert serial: random 128-bit value constructed via SetString.
		serial := new(big.Int)
		serial.SetString("ffffffffffffffffffffffffffffffff", 16)
		d.Deny(serial)
		assert.True(t, d.IsDenied(serial))
		// A different 128-bit value is not denied
		other := new(big.Int).Sub(serial, big.NewInt(1))
		assert.False(t, d.IsDenied(other))
	})

	t.Run("concurrent Deny and IsDenied", func(t *testing.T) {
		d := NewDenyList()
		const n = 100
		var wg sync.WaitGroup

		// Concurrently deny n serials.
		for i := range n {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				d.Deny(big.NewInt(int64(i)))
			}(i)
		}

		// Concurrently call IsDenied while denying.
		for range n {
			wg.Add(1)
			go func() {
				defer wg.Done()
				_ = d.IsDenied(big.NewInt(42))
			}()
		}

		wg.Wait()
		// After all goroutines finish, all n serials must be denied.
		for i := range n {
			assert.True(t, d.IsDenied(big.NewInt(int64(i))), "serial %d should be denied", i)
		}
	})
}
