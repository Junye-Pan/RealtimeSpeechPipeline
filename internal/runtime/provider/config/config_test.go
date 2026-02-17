package config

import "testing"

func TestResolveSecretRefWithLookup(t *testing.T) {
	t.Parallel()

	lookup := func(name string) (string, bool) {
		switch name {
		case "PROVIDER_API_KEY":
			return "secret-key", true
		case "PROVIDER_ENDPOINT":
			return "https://secret.example.com", true
		default:
			return "", false
		}
	}

	tests := []struct {
		name    string
		ref     string
		want    string
		wantErr bool
	}{
		{name: "env prefix", ref: "env://PROVIDER_API_KEY", want: "secret-key"},
		{name: "bare env key", ref: "PROVIDER_ENDPOINT", want: "https://secret.example.com"},
		{name: "missing", ref: "env://UNKNOWN", wantErr: true},
		{name: "unsupported scheme", ref: "vault://provider/api-key", wantErr: true},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := ResolveSecretRefWithLookup(tc.ref, lookup)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error for %q", tc.ref)
				}
				return
			}
			if err != nil {
				t.Fatalf("resolve secret ref %q: %v", tc.ref, err)
			}
			if got != tc.want {
				t.Fatalf("expected %q, got %q", tc.want, got)
			}
		})
	}
}

func TestResolveLiteralOrSecret(t *testing.T) {
	t.Setenv("TEST_PROVIDER_KEY", "resolved-key")

	value, err := ResolveLiteralOrSecret("literal-key", "env://TEST_PROVIDER_KEY")
	if err != nil {
		t.Fatalf("resolve secret ref: %v", err)
	}
	if value != "resolved-key" {
		t.Fatalf("expected secret-ref value, got %q", value)
	}

	value, err = ResolveLiteralOrSecret("literal-only", "")
	if err != nil {
		t.Fatalf("resolve literal: %v", err)
	}
	if value != "literal-only" {
		t.Fatalf("expected literal-only, got %q", value)
	}
}

func TestResolveLiteralOrSecretWithFallback(t *testing.T) {
	t.Parallel()

	got := ResolveLiteralOrSecretWithFallback("fallback-key", "env://MISSING_KEY")
	if got != "fallback-key" {
		t.Fatalf("expected literal fallback, got %q", got)
	}
}

func TestResolveEnvValuePrefersSecretRefAndSupportsRotation(t *testing.T) {
	const literalEnv = "RSPP_TEST_LITERAL"
	const secretRefEnv = "RSPP_TEST_SECRET_REF"

	t.Setenv(literalEnv, "literal-value")
	t.Setenv(secretRefEnv, "env://RSPP_TEST_ROTATABLE")
	t.Setenv("RSPP_TEST_ROTATABLE", "rotated-v1")

	first := ResolveEnvValue(literalEnv, secretRefEnv, "")
	if first != "rotated-v1" {
		t.Fatalf("expected first resolved value rotated-v1, got %q", first)
	}

	t.Setenv("RSPP_TEST_ROTATABLE", "rotated-v2")
	second := ResolveEnvValue(literalEnv, secretRefEnv, "")
	if second != "rotated-v2" {
		t.Fatalf("expected rotated-v2 after env update, got %q", second)
	}
}

func TestRedactSecret(t *testing.T) {
	t.Parallel()

	if got := RedactSecret(""); got != "" {
		t.Fatalf("expected empty redaction for empty secret, got %q", got)
	}
	if got := RedactSecret("sensitive"); got != "***redacted***" {
		t.Fatalf("expected redacted marker, got %q", got)
	}
}
