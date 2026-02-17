package config

import (
	"fmt"
	"os"
	"strings"
)

const (
	envSecretRefPrefix = "env://"
)

// ResolveSecretRef resolves a secret reference using process environment lookup.
// Supported reference forms are "env://VARIABLE_NAME" and "VARIABLE_NAME".
func ResolveSecretRef(ref string) (string, error) {
	return ResolveSecretRefWithLookup(ref, os.LookupEnv)
}

// ResolveSecretRefWithLookup resolves a secret reference using the supplied lookup function.
func ResolveSecretRefWithLookup(ref string, lookup func(string) (string, bool)) (string, error) {
	name, err := parseSecretRefName(ref)
	if err != nil {
		return "", err
	}
	if lookup == nil {
		return "", fmt.Errorf("secret lookup function is required")
	}
	value, ok := lookup(name)
	if !ok || strings.TrimSpace(value) == "" {
		return "", fmt.Errorf("secret_ref %q resolved empty value", name)
	}
	return value, nil
}

// ResolveLiteralOrSecret resolves a value from a secret ref when provided; otherwise it returns the literal.
func ResolveLiteralOrSecret(literal string, secretRef string) (string, error) {
	trimmedLiteral := strings.TrimSpace(literal)
	trimmedRef := strings.TrimSpace(secretRef)
	if trimmedRef == "" {
		return trimmedLiteral, nil
	}
	value, err := ResolveSecretRef(trimmedRef)
	if err != nil {
		return "", err
	}
	return value, nil
}

// ResolveLiteralOrSecretWithFallback resolves via secret ref when possible and falls back to literal on failure.
func ResolveLiteralOrSecretWithFallback(literal string, secretRef string) string {
	value, err := ResolveLiteralOrSecret(literal, secretRef)
	if err != nil {
		return strings.TrimSpace(literal)
	}
	return value
}

// ResolveEnvValue resolves a config value using literal and secret-ref env variables.
// `fallback` is applied before secret-ref resolution when the literal env var is empty.
func ResolveEnvValue(literalEnvVar string, secretRefEnvVar string, fallback string) string {
	literal := strings.TrimSpace(os.Getenv(literalEnvVar))
	if literal == "" {
		literal = fallback
	}
	secretRef := strings.TrimSpace(os.Getenv(secretRefEnvVar))
	return ResolveLiteralOrSecretWithFallback(literal, secretRef)
}

// RedactSecret returns a deterministic redacted marker for non-empty secret material.
func RedactSecret(raw string) string {
	if strings.TrimSpace(raw) == "" {
		return ""
	}
	return "***redacted***"
}

func parseSecretRefName(ref string) (string, error) {
	trimmed := strings.TrimSpace(ref)
	if trimmed == "" {
		return "", fmt.Errorf("secret_ref is required")
	}
	if strings.HasPrefix(trimmed, envSecretRefPrefix) {
		name := strings.TrimSpace(strings.TrimPrefix(trimmed, envSecretRefPrefix))
		if name == "" {
			return "", fmt.Errorf("secret_ref %q is missing env var name", ref)
		}
		if strings.Contains(name, "/") {
			return "", fmt.Errorf("secret_ref %q contains unsupported path separator", ref)
		}
		return name, nil
	}
	if strings.Contains(trimmed, "://") {
		return "", fmt.Errorf("secret_ref %q uses unsupported scheme", ref)
	}
	if strings.Contains(trimmed, "/") {
		return "", fmt.Errorf("secret_ref %q contains unsupported path separator", ref)
	}
	return trimmed, nil
}
