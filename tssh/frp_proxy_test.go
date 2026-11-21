package tssh

import (
	"errors"
	"strings"
	"testing"
)

func TestGenerateTokenUsesCSPRNG(t *testing.T) {
	first, err := generateToken()
	if err != nil {
		t.Fatalf("generateToken() error = %v", err)
	}
	second, err := generateToken()
	if err != nil {
		t.Fatalf("generateToken() second error = %v", err)
	}

	if len(first) != 64 {
		t.Fatalf("token length = %d, want 64 hex chars (256 bits)", len(first))
	}
	if strings.Trim(first, "0123456789abcdef") != "" {
		t.Fatalf("token %q is not lowercase hex", first)
	}
	if first == second {
		t.Fatal("two generated tokens are identical")
	}
	if first == "tssh-frp-default-token" {
		t.Fatal("generateToken() returned the removed static fallback token")
	}
}

func TestGenerateTokenFailsClosedOnRNGError(t *testing.T) {
	original := cryptoRandRead
	t.Cleanup(func() { cryptoRandRead = original })

	cryptoRandRead = func([]byte) (int, error) {
		return 0, errors.New("rng unavailable")
	}

	token, err := generateToken()
	if err == nil {
		t.Fatal("generateToken() must fail when the CSPRNG fails")
	}
	if token != "" {
		t.Fatalf("generateToken() returned token %q on RNG error", token)
	}
	if !strings.Contains(err.Error(), "rng unavailable") {
		t.Fatalf("error %q does not wrap the RNG failure", err)
	}
}

func TestGenerateTokenFailsClosedOnShortRead(t *testing.T) {
	original := cryptoRandRead
	t.Cleanup(func() { cryptoRandRead = original })

	cryptoRandRead = func(buf []byte) (int, error) {
		return len(buf) / 2, nil
	}

	token, err := generateToken()
	if err == nil {
		t.Fatal("generateToken() must fail on a short CSPRNG read")
	}
	if token != "" {
		t.Fatalf("generateToken() returned token %q on short read", token)
	}
}
