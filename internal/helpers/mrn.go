// Package mrn derives human-friendly Medical Record Numbers (MRNs) from a
// UUIDv7. The derivation is deliberately LOSSY (128 bits -> a short code), so
// two different UUIDv7s CAN produce the same MRN. Uniqueness must therefore be
// guaranteed by a UNIQUE(mrn) database constraint plus a retry-on-collision
// loop (see GenerateProofRecord) -- never by the math here alone.
//
// An MRN looks like:
//
//	PT-01J8QK2Z-X9FK
//	│  │        │  └─ 1 check character (mod-37 checksum, catches typos)
//	│  │        └──── random suffix, derived from the UUID's entropy bits
//	│  └───────────── time part, derived from the UUID's 48-bit timestamp
//	└──────────────── fixed human prefix
//
// Because the time part comes from the UUIDv7 timestamp, MRNs sort
// chronologically, exactly like the UUIDs they are derived from.
package helpers

import (
	"encoding/binary"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
)

// Crockford Base32: 32 symbols, excludes I, L, O, U to avoid ambiguity
// (I/L vs 1, O vs 0) and to avoid spelling accidental words.
const alphabet = "0123456789ABCDEFGHJKMNPQRSTVWXYZ"

// The 5 extra symbols make a 37-symbol set used only for the check character.
// 37 is prime, which gives good detection of single-digit and transposition
// errors for a mod-37 checksum.
const checkAlphabet = alphabet + "*~$=U"

const (

	// timeChars encodes the high 40 bits of the 48-bit millisecond timestamp.
	// 40 bits keeps ordering with ~256 ms resolution and stays fixed-width so
	// MRNs align visually. (Unix-ms only needs ~41 bits total, so the high 40
	// bits stay stable and monotonic for the lifetime of the system.)
	timeChars = 8
	timeBits  = timeChars * 5 // 40

	// randChars bounds for the entropy suffix. rand_b holds 62 bits, so at most
	// 12 Base32 chars (60 bits) can be pulled from it.
	minRandChars     = 1
	maxRandChars     = 12
	defaultRandChars = 3
)

// ErrInvalidRandChars is returned when randChars is outside [1, 12].
var ErrInvalidRandChars = errors.New("mrn: randChars out of range [1,12]")

// DeriveMRN produces a candidate MRN from a UUIDv7.
//
// randChars controls the length (and entropy) of the random suffix. Start at
// DefaultRandChars and increase it on an MRN collision to exponentially shrink
// the odds of clashing again. The result is deterministic: the same (id,
// randChars) always yields the same MRN.
//
// This is only a CANDIDATE. The caller must still enforce uniqueness with a
// UNIQUE(mrn) constraint; see GenerateProofRecord.
func DeriveMRN(id uuid.UUID, randChars int, prefix string) (string, error) {
	if randChars < minRandChars || randChars > maxRandChars {
		return "", fmt.Errorf("%w: got %d", ErrInvalidRandChars, randChars)
	}

	timestamp, entropy := extractParts(id)

	// Keep the HIGH timeBits of the 48-bit timestamp so ordering is preserved.
	timePart := encodeBits(timestamp>>(48-timeBits), timeChars)
	// Take the LOW randChars*5 bits of the entropy. Widening randChars adds a
	// higher-order character, producing a fresh candidate on retry.
	randPart := encodeBits(entropy, randChars)

	body := timePart + randPart
	check := computeCheck(body)

	return prefix + timePart + "-" + randPart + string(check), nil
}

// DefaultRandChars is the recommended starting suffix width.
const DefaultRandChars = defaultRandChars

// extractParts pulls the 48-bit millisecond timestamp and the 62-bit rand_b
// entropy field out of a UUIDv7.
//
//	 0                   1                   2                   3
//	 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1
//	┌───────────────────────────────────────────────────────────────┐
//	│                        unix_ts_ms (48)                        …│ bytes 0..5
//	├───────────────────────────────────────────────────────────────┤
//	│…│ ver(4) │        rand_a (12)        │var(2)│    rand_b (62)  …│ bytes 6..15
//	└───────────────────────────────────────────────────────────────┘
func extractParts(id uuid.UUID) (timestamp uint64, entropy uint64) {
	// Bytes 0..5 = 48-bit big-endian millisecond timestamp.
	timestamp = uint64(id[0])<<40 |
		uint64(id[1])<<32 |
		uint64(id[2])<<24 |
		uint64(id[3])<<16 |
		uint64(id[4])<<8 |
		uint64(id[5])

	// Bytes 8..15 hold the variant (top 2 bits) + rand_b (low 62 bits).
	low := binary.BigEndian.Uint64(id[8:16])
	entropy = low & ((uint64(1) << 62) - 1) // clear the 2 variant bits
	return timestamp, entropy
}

// encodeBits encodes the low nchars*5 bits of v as fixed-width Crockford Base32.
func encodeBits(v uint64, nchars int) string {
	b := make([]byte, nchars)
	for i := nchars - 1; i >= 0; i-- {
		b[i] = alphabet[v&31]
		v >>= 5
	}
	return string(b)
}

// computeCheck returns a single mod-37 check character over the MRN body
// (the time + random chars, without prefix, dashes, or the check char itself).
// It treats each body char as a base-32 digit and folds them into a running
// value mod 37.
func computeCheck(body string) byte {
	acc := 0
	for i := 0; i < len(body); i++ {
		d := decodeChar(body[i])
		acc = (acc*32 + d) % 37
	}
	return checkAlphabet[acc]
}

// decodeChar maps a Crockford Base32 character to its 0..31 value, applying the
// standard input normalizations (case-insensitive; I/L -> 1, O -> 0).
func decodeChar(c byte) int {
	switch {
	case c >= 'a' && c <= 'z':
		c -= 'a' - 'A' // uppercase
	}
	switch c {
	case 'I', 'L':
		return 1
	case 'O':
		return 0
	}
	return strings.IndexByte(alphabet, c)
}

// VerifyMRN checks the structural shape and the check character of an MRN.
// It returns true only if the MRN is well-formed and the checksum matches,
// which catches the common single-character typos and transpositions before a
// lookup ever hits the database.
func VerifyMRN(m string, prefix string) bool {
	m = strings.ToUpper(strings.TrimSpace(m))
	if !strings.HasPrefix(m, prefix) {
		return false
	}
	rest := m[len(prefix):]

	dash := strings.IndexByte(rest, '-')
	if dash != timeChars { // time part must be exactly timeChars long
		return false
	}
	timePart := rest[:dash]
	tail := rest[dash+1:]
	if len(tail) < 2 { // at least 1 random char + 1 check char
		return false
	}
	randPart := tail[:len(tail)-1]
	got := tail[len(tail)-1]

	if len(randPart) < minRandChars || len(randPart) > maxRandChars {
		return false
	}
	for i := 0; i < len(timePart); i++ {
		if decodeChar(timePart[i]) < 0 {
			return false
		}
	}
	for i := 0; i < len(randPart); i++ {
		if decodeChar(randPart[i]) < 0 {
			return false
		}
	}
	return computeCheck(timePart+randPart) == got
}
