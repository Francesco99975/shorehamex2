package helpers

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"math/big"
	"strings"

	"github.com/Francesco99975/shorehamex2/internal/enums"
	"github.com/google/uuid"
	"golang.org/x/crypto/blake2b"
)

func GenerateProofUUIDV7(isCollision func(error) bool, attempt func(uuid.UUID) error) (uuid.UUID, error) {
	const maxAttempts = 3

	for range maxAttempts {
		id, err := uuid.NewV7()
		if err != nil {
			return uuid.Nil, fmt.Errorf("generating uuid: %w", err)
		}

		err = attempt(id)
		if err == nil {
			return id, nil
		}

		if isCollision(err) {
			continue
		}

		return uuid.Nil, err
	}

	return uuid.Nil, fmt.Errorf("failed to generate unique uuid after %d attempts", maxAttempts)
}

func GenerateProofUUIDv7WithMRN(
	classify func(error) enums.CollisionKind,
	attempt func(id uuid.UUID, mrn string) error,
) (uuid.UUID, string, error) {
	const maxAttempts = 5
	randChars := 3 // starting MRN entropy width

	for range maxAttempts {
		id, err := uuid.NewV7()
		if err != nil {
			return uuid.Nil, "", fmt.Errorf("generating uuid: %w", err)
		}

		mrn, err := DeriveMRN(id, randChars, "PT-")
		if err != nil {
			return uuid.Nil, "", fmt.Errorf("deriving mrn: %w", err)
		}

		if !VerifyMRN(mrn, "PT-") {
			return uuid.Nil, "", fmt.Errorf("invalid mrn: %w", err)
		}

		// attempt MUST insert id + mrn atomically, with UNIQUE on both.
		err = attempt(id, mrn)
		if err == nil {
			return id, mrn, nil
		}

		switch classify(err) {
		case enums.UUIDCollision:
			continue // fresh uuid next loop (mrn regenerates too)
		case enums.MRNCollision:
			randChars++ // widen -> more entropy, exponentially fewer clashes
			continue
		default:
			return uuid.Nil, "", err // a real error; don't mask it
		}
	}
	return uuid.Nil, "", fmt.Errorf("failed to generate unique record after %d attempts", maxAttempts)
}

func GenerateProofUUIDV4(isCollision func(error) bool, attempt func(uuid.UUID) error) (uuid.UUID, error) {
	const maxAttempts = 3

	for range maxAttempts {
		id := uuid.New()

		err := attempt(id)
		if err == nil {
			return id, nil
		}

		if isCollision(err) {
			continue
		}

		return uuid.Nil, err
	}

	return uuid.Nil, fmt.Errorf("failed to generate unique uuid after %d attempts", maxAttempts)
}

func GenerateProofBulkUUIDV4(bulk int, isCollision func(error) bool, attempt func([]uuid.UUID) error) ([]uuid.UUID, error) {
	const maxAttempts = 3

	if bulk < 1 || bulk > 100 {
		return nil, fmt.Errorf("bulk must be 1-100, got %d", bulk)
	}

	uuids := make([]uuid.UUID, bulk)
	for i := range bulk {
		uuids[i] = uuid.New()
	}

	for range maxAttempts {

		err := attempt(uuids)
		if err == nil {
			return uuids, nil
		}

		if isCollision(err) {
			continue
		}

		return nil, err
	}

	return nil, fmt.Errorf("failed to generate unique uuid after %d attempts", maxAttempts)
}

func GenerateUniqueID() uint {
	u := uuid.New()
	hash := sha256.Sum256(u[:])
	return uint(binary.BigEndian.Uint64(hash[:8]))
}

const charset = "abcdefghkmnpqrstuvwxyzABCDEFGHJKMNPQRSTUVWXYZ123456789%$#@"

func GenerateBase62Token(length int) (string, error) {
	token := make([]byte, length)
	max := big.NewInt(int64(len(charset)))

	for i := range length {
		n, err := rand.Int(rand.Reader, max)
		if err != nil {
			return "", err
		}
		token[i] = charset[n.Int64()]
	}

	return string(token), nil
}

type BackupCodes struct {
	IDs    []uuid.UUID
	Plain  []string
	Hashed []string
}

func hashBackupCode(code string) (string, error) {
	h, err := blake2b.New512(nil)
	if err != nil {
		return "", err
	}
	h.Write([]byte(code))
	return hex.EncodeToString(h.Sum(nil)), nil
}

// GenerateBackupCodes creates 8–10 secure backup codes
func GenerateBackupCodes(codesIds []uuid.UUID) (*BackupCodes, error) {
	if len(codesIds) < 5 || len(codesIds) > 12 {
		return nil, fmt.Errorf("recommended count is 8-10, got %d", len(codesIds))
	}

	ids := make([]uuid.UUID, len(codesIds))
	codes_plain := make([]string, len(codesIds))
	codes_hashes := make([]string, len(codesIds))

	for i := range len(codesIds) {
		// 10 chars = ~59.8 bits entropy (very strong for one-time use)
		plain, err := generateSecureCode(10)
		if err != nil {
			return nil, err
		}

		// Hash it for storage (bcrypt default cost=10 is fine; 12–14 for more security)
		hashed, err := hashBackupCode(plain)
		if err != nil {
			return nil, err
		}

		ids[i] = codesIds[i]
		codes_plain[i] = plain
		codes_hashes[i] = string(hashed)

	}

	return &BackupCodes{ids, codes_plain, codes_hashes}, nil
}

// generateSecureCode creates a random uppercase + digits string
func generateSecureCode(length int) (string, error) {
	const charset = "ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	bytes := make([]byte, length)

	_, err := rand.Read(bytes)
	if err != nil {
		return "", err
	}

	// Map random bytes to charset
	for i := range bytes {
		bytes[i] = charset[bytes[i]%byte(len(charset))]
	}

	return string(bytes), nil
}

// pretty-print for display (e.g., "ABCD-EFGH-IJKL")
func FormatCode(code string) string {
	var parts []string
	for i := 0; i < len(code); i += 4 {
		end := min(i+4, len(code))
		parts = append(parts, code[i:end])
	}
	return strings.Join(parts, "-")
}

func GenerateSecurePassword(length int) (string, error) {
	const charset = "abcdefghkmnpqrstuvwxyz" +
		"ABCDEFGHKMNPQRSTUVWXYZ" +
		"12356789" +
		"!@#$&"

	if length <= 0 {
		return "", fmt.Errorf("invalid length")
	}

	password := make([]byte, length)
	max := big.NewInt(int64(len(charset)))

	for i := range length {
		n, err := rand.Int(rand.Reader, max)
		if err != nil {
			return "", err
		}
		password[i] = charset[n.Int64()]
	}

	return string(password), nil
}
