// helpers_test.go
package helpers

import (
	"testing"

	"github.com/google/uuid"
)

// Mockable uuid.New
var uuidNew = uuid.New

func TestGenerateUniqueID_Uniqueness(t *testing.T) {
	const n = 500_000
	seen := make(map[uint]struct{}, n)
	uuidNew = uuid.New

	for i := 0; i < n; i++ {
		id := GenerateUniqueID()
		if id == 0 {
			t.Fatal("zero ID")
		}
		if _, dup := seen[id]; dup {
			t.Fatalf("COLLISION at %d: %d (0x%x)", i, id, id)
		}
		seen[id] = struct{}{}
	}
	t.Logf("No collisions in %d generations", n)
}

func TestGenerateUniqueID_DifferentInputs(t *testing.T) {
	var u1, u2 uuid.UUID
	copy(u1[:], []byte("11111111-1111-1111-1111-111111111111"))
	copy(u2[:], []byte("22222222-2222-2222-2222-222222222222"))

	uuidNew = func() uuid.UUID { return u1 }
	id1 := GenerateUniqueID()
	uuidNew = func() uuid.UUID { return u2 }
	id2 := GenerateUniqueID()

	if id1 == id2 {
		t.Error("same ID from different UUIDs")
	}
}

// ——————————————————— BENCHMARKS ———————————————————

func BenchmarkGenerateUniqueID(b *testing.B) {
	uuidNew = uuid.New
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = GenerateUniqueID()
	}
}
