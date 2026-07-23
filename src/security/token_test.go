package security

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/constants"
	"github.com/google/uuid"
)

func TestEncryptedAccessTokenRoundTrip(t *testing.T) {
	t.Parallel()

	manager := testTokenManager(t, "ev-cms-api")
	now := time.Date(2026, 7, 23, 10, 0, 0, 0, time.UTC)
	userID := uuid.New()
	sessionID := uuid.New()
	cpoID := uuid.New()
	role := constants.CPORoleAdmin

	token, expiresAt, err := manager.Issue(
		now,
		userID,
		sessionID,
		constants.AuthScopeCPO,
		&cpoID,
		&role,
		3,
	)
	if err != nil {
		t.Fatalf("issue token: %v", err)
	}
	if strings.Count(token, ".") != 4 {
		t.Fatalf("got non-compact-JWE token %q", token)
	}
	if strings.Contains(token, userID.String()) || strings.Contains(token, cpoID.String()) {
		t.Fatal("encrypted token exposes identifiers")
	}
	claims, err := manager.Parse(token, now.Add(time.Minute))
	if err != nil {
		t.Fatalf("parse token: %v", err)
	}
	if claims.Subject != userID.String() ||
		claims.SessionID != sessionID ||
		claims.CPOID == nil ||
		*claims.CPOID != cpoID ||
		claims.Role == nil ||
		*claims.Role != role ||
		claims.TokenVersion != 3 {
		t.Fatalf("unexpected claims: %#v", claims)
	}
	if !expiresAt.Equal(now.Add(15 * time.Minute)) {
		t.Fatalf("got expiry %s", expiresAt)
	}
}

func TestEncryptedAccessTokenRejectsWrongAudienceAndTampering(t *testing.T) {
	t.Parallel()

	manager := testTokenManager(t, "ev-cms-api")
	now := time.Date(2026, 7, 23, 10, 0, 0, 0, time.UTC)
	token, _, err := manager.Issue(
		now,
		uuid.New(),
		uuid.New(),
		constants.AuthScopePlatform,
		nil,
		nil,
		1,
	)
	if err != nil {
		t.Fatalf("issue token: %v", err)
	}
	otherAudience := testTokenManager(t, "other-api")
	if _, err := otherAudience.Parse(token, now); err == nil {
		t.Fatal("expected wrong audience to fail")
	}
	parts := strings.Split(token, ".")
	position := len(parts[3]) / 2
	current := parts[3][position]
	replacement := byte('A')
	if current == replacement {
		replacement = 'B'
	}
	parts[3] = parts[3][:position] + string(replacement) + parts[3][position+1:]
	tampered := strings.Join(parts, ".")
	if _, err := manager.Parse(tampered, now); err == nil {
		t.Fatal("expected tampered token to fail")
	}
}

func testTokenManager(t *testing.T, audience string) *TokenManager {
	t.Helper()
	manager, err := NewTokenManager(
		"ev-cms",
		audience,
		15*time.Minute,
		bytes.Repeat([]byte{3}, 32),
		bytes.Repeat([]byte{4}, 32),
	)
	if err != nil {
		t.Fatalf("create token manager: %v", err)
	}
	return manager
}
