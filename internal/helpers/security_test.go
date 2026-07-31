// nonce_test.go
package helpers

import (
	"encoding/base64"
	"errors"

	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGenerateNonce_ReturnsValidBase64(t *testing.T) {
	t.Parallel()

	nonce, err := GenerateNonce()
	require.NoError(t, err)
	require.NotEmpty(t, nonce)

	// Must be exactly 24 chars (16 bytes → base64 → 22 chars with padding (==))
	assert.Len(t, nonce, 24)

	// Must decode without error
	decoded, err := base64.StdEncoding.DecodeString(nonce)
	require.NoError(t, err)
	assert.Len(t, decoded, 16)

}

func TestGenerateNonce_UsesCryptographicRandom(t *testing.T) {
	t.Parallel()

	// Generate two nonces — collision probability is 1 in 2^64 → impossible
	n1, err1 := GenerateNonce()
	n2, err2 := GenerateNonce()
	require.NoError(t, err1)
	require.NoError(t, err2)

	assert.NotEqual(t, n1, n2, "two nonces must be different (cryptographic randomness)")
}

func TestGenerateNonce_ConcurrentSafety(t *testing.T) {
	t.Parallel()

	const goroutines = 100
	const callsPerGoroutine = 1000

	var wg sync.WaitGroup
	seen := make(map[string]struct{})
	var mu sync.Mutex
	var collision bool

	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < callsPerGoroutine; j++ {
				nonce, err := GenerateNonce()
				if err != nil {
					t.Errorf("GenerateNonce failed: %v", err)
					return
				}

				mu.Lock()
				if _, exists := seen[nonce]; exists {
					collision = true
				}
				seen[nonce] = struct{}{}
				mu.Unlock()
			}
		}()
	}

	wg.Wait()

	assert.False(t, collision, "no nonce collisions in 100,000 generations")
	assert.Equal(t, goroutines*callsPerGoroutine, len(seen), "all nonces were unique")
}

// Replace your entire TestGenerateNonce_ErrorPath with this:
func TestGenerateNonce_ErrorPath(t *testing.T) {
	t.Parallel()

	// Use a reader that returns error
	fakeReader := errorReader{}
	nonce, err := generateNonceWithReader(fakeReader)

	assert.Empty(t, nonce)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "mock read error")
}

type errorReader struct{}

func (errorReader) Read(p []byte) (int, error) {
	return 0, errors.New("mock read error")
}

// ——————————————————— BENCHMARKS ———————————————————

func BenchmarkGenerateNonce(b *testing.B) {
	b.Run("Single", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_, _ = GenerateNonce()
		}
	})

	b.Run("Parallel", func(b *testing.B) {
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				_, _ = GenerateNonce()
			}
		})
	})
}

func BenchmarkGenerateNonce_Allocations(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		nonce, err := GenerateNonce()
		if err != nil {
			b.Fatal(err)
		}
		_ = nonce // prevent optimization
	}
}
