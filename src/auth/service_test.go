package auth

import (
	"bytes"
	"testing"

	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/config"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/constants"
	"github.com/google/uuid"
)

func TestOTPHashBindsChallengePurposeAndCode(t *testing.T) {
	t.Parallel()

	service := &Service{config: config.Auth{OTPHMACKey: bytes.Repeat([]byte{5}, 32)}}
	challengeID := uuid.New()
	hash := service.otpHash(challengeID, constants.ChallengeLogin2FA, "123456")
	if bytes.Equal(
		hash,
		service.otpHash(uuid.New(), constants.ChallengeLogin2FA, "123456"),
	) {
		t.Fatal("OTP hash was not bound to challenge ID")
	}
	if bytes.Equal(
		hash,
		service.otpHash(challengeID, constants.ChallengePasswordReset, "123456"),
	) {
		t.Fatal("OTP hash was not bound to purpose")
	}
	if bytes.Equal(
		hash,
		service.otpHash(challengeID, constants.ChallengeLogin2FA, "654321"),
	) {
		t.Fatal("different OTP produced the same hash")
	}
}

func TestAuthInputShapes(t *testing.T) {
	t.Parallel()

	if !validOTP("012345") || validOTP("12345") || validOTP("12345x") {
		t.Fatal("OTP shape validation is incorrect")
	}
	if !validEmail("admin@example.com") || validEmail("not-an-email") {
		t.Fatal("email validation is incorrect")
	}
	if boundedUserAgent(string(bytes.Repeat([]byte{'a'}, 600))) == string(bytes.Repeat([]byte{'a'}, 600)) {
		t.Fatal("user agent was not bounded")
	}
}
