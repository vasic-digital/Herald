package commons_auth

import (
	"errors"
	"fmt"

	"github.com/google/uuid"
)

// TenantFromClaims extracts and parses the "tenant" claim as a UUID.
// Returns uuid.Nil + error if the claim is missing, not a string, or
// not a valid UUID.
func TenantFromClaims(claims map[string]any) (uuid.UUID, error) {
	v, ok := claims["tenant"]
	if !ok {
		return uuid.Nil, errors.New("missing 'tenant' claim")
	}
	s, ok := v.(string)
	if !ok {
		return uuid.Nil, fmt.Errorf("'tenant' claim must be a string, got %T", v)
	}
	return uuid.Parse(s)
}

// SubjectFromClaims extracts the "sub" claim as a string.
// Returns error if the claim is missing or not a string.
func SubjectFromClaims(claims map[string]any) (string, error) {
	v, ok := claims["sub"]
	if !ok {
		return "", errors.New("missing 'sub' claim")
	}
	s, ok := v.(string)
	if !ok {
		return "", fmt.Errorf("'sub' claim must be a string, got %T", v)
	}
	return s, nil
}
