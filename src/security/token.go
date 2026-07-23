package security

import (
	"errors"
	"fmt"
	"time"

	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/constants"
	"github.com/go-jose/go-jose/v4"
	"github.com/go-jose/go-jose/v4/jwt"
	"github.com/google/uuid"
)

const accessTokenType = "ev-cms-access+jwt"

type AccessClaims struct {
	jwt.Claims
	SessionID    uuid.UUID           `json:"sid"`
	Scope        constants.AuthScope `json:"scope"`
	CPOID        *uuid.UUID          `json:"cpo_id,omitempty"`
	Role         *constants.CPORole  `json:"role,omitempty"`
	TokenVersion int                 `json:"token_version"`
	TokenType    string              `json:"token_type"`
}

type TokenManager struct {
	issuer        string
	audience      string
	accessTTL     time.Duration
	signingKey    []byte
	encryptionKey []byte
	signer        jose.Signer
	encrypter     jose.Encrypter
}

func NewTokenManager(
	issuer string,
	audience string,
	accessTTL time.Duration,
	signingKey []byte,
	encryptionKey []byte,
) (*TokenManager, error) {
	signer, err := jose.NewSigner(
		jose.SigningKey{Algorithm: jose.HS256, Key: signingKey},
		new(jose.SignerOptions).WithType(accessTokenType),
	)
	if err != nil {
		return nil, fmt.Errorf("create JWT signer: %w", err)
	}
	encrypter, err := jose.NewEncrypter(
		jose.A256GCM,
		jose.Recipient{Algorithm: jose.DIRECT, Key: encryptionKey},
		new(jose.EncrypterOptions).
			WithType("JWE").
			WithContentType("JWT"),
	)
	if err != nil {
		return nil, fmt.Errorf("create JWT encrypter: %w", err)
	}
	return &TokenManager{
		issuer:        issuer,
		audience:      audience,
		accessTTL:     accessTTL,
		signingKey:    append([]byte(nil), signingKey...),
		encryptionKey: append([]byte(nil), encryptionKey...),
		signer:        signer,
		encrypter:     encrypter,
	}, nil
}

func (manager *TokenManager) Issue(
	now time.Time,
	userID uuid.UUID,
	sessionID uuid.UUID,
	scope constants.AuthScope,
	cpoID *uuid.UUID,
	role *constants.CPORole,
	tokenVersion int,
) (string, time.Time, error) {
	expiresAt := now.Add(manager.accessTTL)
	claims := AccessClaims{
		Claims: jwt.Claims{
			Issuer:    manager.issuer,
			Subject:   userID.String(),
			Audience:  jwt.Audience{manager.audience},
			Expiry:    jwt.NewNumericDate(expiresAt),
			NotBefore: jwt.NewNumericDate(now.Add(-5 * time.Second)),
			IssuedAt:  jwt.NewNumericDate(now),
			ID:        uuid.NewString(),
		},
		SessionID:    sessionID,
		Scope:        scope,
		CPOID:        cpoID,
		Role:         role,
		TokenVersion: tokenVersion,
		TokenType:    accessTokenType,
	}
	token, err := jwt.SignedAndEncrypted(manager.signer, manager.encrypter).
		Claims(claims).
		Serialize()
	if err != nil {
		return "", time.Time{}, fmt.Errorf("issue encrypted access token: %w", err)
	}
	return token, expiresAt, nil
}

func (manager *TokenManager) Parse(raw string, now time.Time) (AccessClaims, error) {
	nested, err := jwt.ParseSignedAndEncrypted(
		raw,
		[]jose.KeyAlgorithm{jose.DIRECT},
		[]jose.ContentEncryption{jose.A256GCM},
		[]jose.SignatureAlgorithm{jose.HS256},
	)
	if err != nil {
		return AccessClaims{}, errors.New("invalid access token")
	}
	if len(nested.Headers) != 1 ||
		nested.Headers[0].ExtraHeaders[jose.HeaderType] != "JWE" ||
		nested.Headers[0].ExtraHeaders[jose.HeaderContentType] != "JWT" {
		return AccessClaims{}, errors.New("invalid access token headers")
	}

	signed, err := nested.Decrypt(manager.encryptionKey)
	if err != nil {
		return AccessClaims{}, errors.New("invalid access token")
	}
	if len(signed.Headers) != 1 ||
		signed.Headers[0].ExtraHeaders[jose.HeaderType] != accessTokenType {
		return AccessClaims{}, errors.New("invalid access token type")
	}

	var claims AccessClaims
	if err := signed.Claims(manager.signingKey, &claims); err != nil {
		return AccessClaims{}, errors.New("invalid access token signature")
	}
	if err := claims.ValidateWithLeeway(jwt.Expected{
		Issuer:      manager.issuer,
		AnyAudience: jwt.Audience{manager.audience},
		Time:        now,
	}, 30*time.Second); err != nil {
		return AccessClaims{}, errors.New("invalid access token claims")
	}
	if claims.TokenType != accessTokenType ||
		claims.Subject == "" ||
		claims.SessionID == uuid.Nil ||
		!claims.Scope.Valid() ||
		claims.TokenVersion < 1 {
		return AccessClaims{}, errors.New("invalid access token claims")
	}
	userID, err := uuid.Parse(claims.Subject)
	if err != nil || userID == uuid.Nil {
		return AccessClaims{}, errors.New("invalid access token subject")
	}
	if claims.Scope == constants.AuthScopePlatform && (claims.CPOID != nil || claims.Role != nil) {
		return AccessClaims{}, errors.New("invalid platform token context")
	}
	if claims.Scope == constants.AuthScopeCPO &&
		(claims.CPOID == nil || *claims.CPOID == uuid.Nil || claims.Role == nil || !claims.Role.Valid()) {
		return AccessClaims{}, errors.New("invalid CPO token context")
	}
	return claims, nil
}
