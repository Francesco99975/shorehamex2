package enums

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestEnvironment_String(t *testing.T) {
	t.Parallel()

	tests := []struct {
		env  Environment
		want string
	}{
		{Environments.DEVELOPMENT, "development"},
		{Environments.STAGING, "staging"},
		{Environments.PRODUCTION, "production"},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(string(tt.env), func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, tt.env.String())
		})
	}
}

func TestGetEnvironmentFromString_CaseInsensitive(t *testing.T) {
	t.Parallel()

	cases := map[string]Environment{
		"development": Environments.DEVELOPMENT,
		"DEVELOPMENT": Environments.DEVELOPMENT,
		"Development": Environments.DEVELOPMENT,
		"dEvElOpMeNt": Environments.DEVELOPMENT,

		"staging": Environments.STAGING,
		"STAGING": Environments.STAGING,
		"Staging": Environments.STAGING,

		"production": Environments.PRODUCTION,
		"PRODUCTION": Environments.PRODUCTION,
		"Production": Environments.PRODUCTION,
	}

	for input, expected := range cases {
		input := input
		t.Run(input, func(t *testing.T) {
			t.Parallel()
			got := GetEnvironmentFromString(input)
			assert.Equal(t, expected, got)
			assert.Equal(t, expected.String(), got.String())
		})
	}
}

func TestGetEnvironmentFromString_DefaultsToDEVELOPMENT(t *testing.T) {
	t.Parallel()

	invalid := []string{
		"", " ", "dev", "test", "local", "qa", "PREPROD", "STAG", "PRODCTION",
		"development ", " staging", "production!", "null", "undefined", "0",
	}

	for _, s := range invalid {
		s := s
		t.Run(fmt.Sprintf("invalid_%q", s), func(t *testing.T) {
			t.Parallel()
			got := GetEnvironmentFromString(s)
			assert.Equal(t, Environments.DEVELOPMENT, got)
			assert.Equal(t, "development", got.String())
		})
	}
}

func TestIsEnvironmentValid(t *testing.T) {
	t.Parallel()

	valid := []string{
		"development", "DEVELOPMENT", "Development",
		"staging", "STAGING", "Staging",
		"production", "PRODUCTION", "Production",
	}

	invalid := []string{
		"", " ", "dev", "test", "local", "qa", "preprod", "uat",
		"development ", " staging", "production!", "null", "0",
	}

	for _, s := range valid {
		s := s
		t.Run("valid_"+s, func(t *testing.T) {
			t.Parallel()
			assert.True(t, IsEnvironmentValid(s))
		})
	}

	for _, s := range invalid {
		s := s
		t.Run("invalid_"+s, func(t *testing.T) {
			t.Parallel()
			assert.False(t, IsEnvironmentValid(s))
		})
	}
}

func TestEnvironments_AllValuesAreUnique(t *testing.T) {
	t.Parallel()

	seen := make(map[Environment]struct{})
	values := []Environment{
		Environments.DEVELOPMENT,
		Environments.STAGING,
		Environments.PRODUCTION,
	}

	for _, v := range values {
		if _, exists := seen[v]; exists {
			t.Fatalf("duplicate environment value: %v", v)
		}
		seen[v] = struct{}{}
	}

	assert.Equal(t, 3, len(seen), "expected 3 unique environments")
}

func TestEnvironments_CanBeUsedAsMapKeys(t *testing.T) {
	t.Parallel()

	m := make(map[Environment]string)
	m[Environments.PRODUCTION] = "high-availability"
	m[Environments.DEVELOPMENT] = "debug-mode"

	assert.Equal(t, "high-availability", m[Environments.PRODUCTION])
	assert.Equal(t, "debug-mode", m[Environments.DEVELOPMENT])
}

func TestGetEnvironmentFromString_NeverPanics(t *testing.T) {
	t.Parallel()

	for i := 0; i < 1000; i++ {
		input := fmt.Sprintf("stress-%d-%s", i, strings.Repeat("X", i%200))
		assert.NotPanics(t, func() {
			_ = GetEnvironmentFromString(input)
		})
	}
}

// ——————— BENCHMARKS ———————
func BenchmarkGetEnvironmentFromString_Valid(b *testing.B) {
	for b.Loop() {
		_ = GetEnvironmentFromString("PRODUCTION")
	}
}

func BenchmarkGetEnvironmentFromString_CommonProd(b *testing.B) {
	for b.Loop() {
		_ = GetEnvironmentFromString("prod")
	}
}

func BenchmarkGetEnvironmentFromString_Invalid(b *testing.B) {
	for b.Loop() {
		_ = GetEnvironmentFromString("local")
	}
}

func BenchmarkIsEnvironmentValid(b *testing.B) {
	for b.Loop() {
		_ = IsEnvironmentValid("staging")
	}
}
