package customerauth

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	netmail "net/mail"
	"regexp"
	"strings"
	"time"

	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/config"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/constants"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/halops"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/liveops"
	cmsmail "github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/mail"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/models"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/operationalrealtime"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/security"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const signupMailTemplate = "CUSTOMER_SIGNUP_OTP"

var phonePattern = regexp.MustCompile(`^\+?[0-9]{7,15}$`)

var (
	errInvalidChallenge  = &APIError{http.StatusUnauthorized, "invalid_challenge", "The verification challenge or code is invalid."}
	errMailUnavailable   = &APIError{http.StatusServiceUnavailable, "mail_unavailable", "Email verification is temporarily unavailable."}
	errRateLimited       = &APIError{http.StatusTooManyRequests, "rate_limited", "Too many attempts. Try again later."}
	errSignupUnavailable = &APIError{
		http.StatusForbidden, "signup_unavailable",
		"Customer signup is not available for this application.",
	}
	errAlreadyRegistered = &APIError{
		http.StatusConflict, "customer_already_registered",
		"This email is already registered with this charging provider.",
	}
)

type Service struct {
	database           *gorm.DB
	config             config.Auth
	mailEnabled        bool
	outbox             *cmsmail.Outbox
	tokens             *security.TokenManager
	dummyHash          string
	now                func() time.Time
	razorpayResolver   RazorpayCredentialResolver
	razorpayFactory    RazorpayClientFactory
	hal                *halops.Service
	live               *liveops.Service
	factIngestor       *halops.FactIngestor
	operationalEvents  *operationalrealtime.Service
	halFactBearer      string
	halMeterStaleAfter time.Duration
}

// WithHALOperations connects User App charging to the shared CMS operational
// capabilities. User App business code never constructs HAL wire requests.
func (service *Service) WithHALOperations(operations *halops.Service, live *liveops.Service, cfg config.HAL) *Service {
	service.hal = operations
	if operations != nil {
		operations.WithStartCommandAbsentHandler(service.ReconcileConfirmedAbsentStartCommand)
		operations.WithStartMaterializer(service.MaterializeAuthoritativeStart)
		operations.WithStopCommandAbsentHandler(service.ReconcileConfirmedAbsentStopCommand)
		operations.WithStopCommandReconciler(service.ReconcileStopCommand)
		operations.WithSettlementReconciler(service.ReconcileCompletedSettlements)
	}
	service.live = live
	service.halFactBearer = cfg.FactBearerToken
	service.halMeterStaleAfter = cfg.MeterStaleAfter
	service.factIngestor = halops.NewFactIngestor(service.database, cfg.FactBearerToken, service)
	return service
}

func (service *Service) HALFactIngestor() *halops.FactIngestor { return service.factIngestor }

func (service *Service) WithOperationalEvents(events *operationalrealtime.Service) *Service {
	service.operationalEvents = events
	return service
}

func (service *Service) ListOperationalEvents(ctx context.Context, principal Principal, after int64, limit int) (operationalrealtime.Page, error) {
	if service.operationalEvents == nil {
		return operationalrealtime.Page{}, fmt.Errorf("operational event capability is unavailable")
	}
	return service.operationalEvents.ListCustomer(ctx, principal.CPOID, principal.CustomerID, after, limit)
}

func (service *Service) OperationalStreamTiming() (time.Duration, time.Duration, int) {
	if service.operationalEvents == nil {
		return time.Second, 15 * time.Second, 100
	}
	return service.operationalEvents.StreamTiming()
}

// WithRazorpayCredentialResolver connects the User App payment flow to the
// existing encrypted CPO integration store without exposing that store or its
// credentials through a CPO/User App HTTP contract.
func (service *Service) WithRazorpayCredentialResolver(
	resolver RazorpayCredentialResolver,
) *Service {
	service.razorpayResolver = resolver
	return service
}

func NewService(
	database *gorm.DB,
	cfg config.Auth,
	mailEnabled bool,
	outbox *cmsmail.Outbox,
	tokens *security.TokenManager,
) (*Service, error) {
	dummyHash, err := security.HashPassword("invalid-customer-login-password")
	if err != nil {
		return nil, fmt.Errorf("initialize customer password verifier: %w", err)
	}
	service := &Service{
		database: database, config: cfg, mailEnabled: mailEnabled, outbox: outbox,
		tokens: tokens, dummyHash: dummyHash, razorpayFactory: newRazorpayClient,
		now: func() time.Time { return time.Now().UTC() },
	}
	// Keep the persisted CMS projection readable in isolated service tests and
	// non-HAL startup paths. main replaces this with the configured shared
	// capability before routes are registered.
	service.live = liveops.New(database, config.HAL{MeterStaleAfter: 30 * time.Second, ConnectionStaleAfter: 15 * time.Minute})
	service.factIngestor = halops.NewFactIngestor(database, "", service)
	return service, nil
}

func (service *Service) Start(
	ctx context.Context,
	appID string,
	request SignupRequest,
	metadata RequestMetadata,
) (ChallengeResponse, error) {
	appID = strings.TrimSpace(appID)
	email := strings.ToLower(strings.TrimSpace(request.Email))
	fullName := strings.TrimSpace(request.FullName)
	phone, err := normalizePhone(request.Phone)
	if appID == "" {
		return ChallengeResponse{}, &APIError{http.StatusBadRequest, "missing_cpo_app_id", "X-CPO-App-ID is required."}
	}
	if !validEmail(email) {
		return ChallengeResponse{}, &APIError{http.StatusBadRequest, "invalid_email", "A valid email address is required."}
	}
	if fullName == "" || len(fullName) > 255 {
		return ChallengeResponse{}, &APIError{http.StatusBadRequest, "invalid_full_name", "Full name must contain 1 to 255 characters."}
	}
	if err != nil {
		return ChallengeResponse{}, err
	}
	if err := security.ValidatePassword(request.Password); err != nil {
		return ChallengeResponse{}, &APIError{http.StatusBadRequest, "invalid_password", err.Error()}
	}
	if !service.mailEnabled {
		return ChallengeResponse{}, errMailUnavailable
	}
	if err := service.checkRateLimit(ctx, "CUSTOMER_SIGNUP_IP", rateLimitAddress(metadata)); err != nil {
		return ChallengeResponse{}, err
	}
	if err := service.checkRateLimit(ctx, "CUSTOMER_SIGNUP_EMAIL", appID+"\x00"+email); err != nil {
		return ChallengeResponse{}, err
	}
	passwordHash, err := security.HashPassword(request.Password)
	if err != nil {
		return ChallengeResponse{}, fmt.Errorf("hash signup password: %w", err)
	}

	var response ChallengeResponse
	err = service.database.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		cpo, err := activeCPO(tx, appID)
		if err != nil {
			return err
		}
		response, err = service.createChallenge(
			tx, cpo.ID, email, passwordHash, fullName, phone, metadata, service.now(),
		)
		return err
	})
	return response, err
}

func (service *Service) UpdateProfile(
	ctx context.Context,
	principal Principal,
	request UpdateProfileRequest,
) (UserView, error) {
	fullName := strings.TrimSpace(request.FullName)
	if fullName == "" || len(fullName) > 255 {
		return UserView{}, &APIError{http.StatusBadRequest, "invalid_full_name", "Full name must contain 1 to 255 characters."}
	}
	var phone *string
	if request.phoneSet || request.Phone != nil {
		var err error
		phone, err = normalizePhone(request.Phone)
		if err != nil {
			return UserView{}, err
		}
	}

	var profile UserView
	err := service.database.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var customer models.Customer
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where(
			"id = ? AND cpo_id = ? AND status = ?", principal.CustomerID, principal.CPOID, constants.CustomerStatusActive,
		).First(&customer).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return errUnauthorized
			}
			return fmt.Errorf("load customer profile: %w", err)
		}

		updates := map[string]any{}
		changedFields := make([]string, 0, 2)
		if customer.FullName != fullName {
			updates["full_name"] = fullName
			customer.FullName = fullName
			changedFields = append(changedFields, "full_name")
		}
		if request.phoneSet || request.Phone != nil {
			phoneChanged := (customer.Phone == nil) != (phone == nil)
			if !phoneChanged && customer.Phone != nil && phone != nil {
				phoneChanged = *customer.Phone != *phone
			}
			if phoneChanged {
				updates["phone"] = phone
				customer.Phone = phone
				changedFields = append(changedFields, "phone")
			}
		}

		now := service.now()
		if len(changedFields) > 0 {
			updates["updated_at"] = now
			customer.UpdatedAt = now
			if err := tx.Model(&models.Customer{}).Where(
				"id = ? AND cpo_id = ?", customer.ID, principal.CPOID,
			).Updates(updates).Error; err != nil {
				return fmt.Errorf("update customer profile: %w", err)
			}
			if err := createCustomerAudit(
				tx, customer.ID, principal.CPOID, "CUSTOMER_PROFILE_UPDATED", "CUSTOMER", customer.ID,
				models.JSONB{"changed_fields": changedFields}, now,
			); err != nil {
				return err
			}
		}
		profile = userView(customer)
		return nil
	})
	if err != nil {
		return UserView{}, err
	}
	return profile, nil
}

func userView(customer models.Customer) UserView {
	return UserView{
		ID: customer.ID, Email: customer.Email, FullName: customer.FullName,
		Phone: customer.Phone, IsVerified: customer.IsVerified, LastLoginAt: customer.LastLoginAt,
	}
}

func (service *Service) Verify(
	ctx context.Context,
	appID string,
	request ChallengeRequest,
	metadata RequestMetadata,
) (SignupResponse, error) {
	if strings.TrimSpace(appID) == "" {
		return SignupResponse{}, &APIError{http.StatusBadRequest, "missing_cpo_app_id", "X-CPO-App-ID is required."}
	}
	if request.ChallengeID == uuid.Nil || !validOTP(request.Code) {
		return SignupResponse{}, errInvalidChallenge
	}
	if err := service.checkRateLimit(ctx, "VERIFY_CUSTOMER_SIGNUP", rateLimitAddress(metadata)); err != nil {
		return SignupResponse{}, err
	}

	var response SignupResponse
	var outcome error
	err := service.database.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		identity, err := service.findSignupChallengeIdentity(tx, request.ChallengeID)
		if err != nil {
			outcome = err
			return nil
		}
		if err := lockSignupIdentity(tx, identity.CPOID, identity.Email); err != nil {
			return err
		}
		challenge, err := service.lockChallenge(tx, request.ChallengeID)
		if err != nil {
			outcome = err
			return nil
		}
		cpo, err := activeCPO(tx, strings.TrimSpace(appID))
		if err != nil || cpo.ID != challenge.CPOID {
			outcome = errInvalidChallenge
			return nil
		}
		now := service.now()
		if !challengeUsable(challenge, now) {
			outcome = errInvalidChallenge
			return nil
		}
		if !service.verifyOTP(challenge, request.Code) {
			if err := recordChallengeFailure(tx, challenge, now); err != nil {
				return err
			}
			outcome = errInvalidChallenge
			return nil
		}
		if err := tx.Model(&models.CustomerSignupChallenge{}).
			Where("id = ? AND consumed_at IS NULL", challenge.ID).
			Updates(map[string]any{
				"consumed_at":   now,
				"password_hash": "CONSUMED",
			}).Error; err != nil {
			return fmt.Errorf("consume signup challenge: %w", err)
		}
		var existing int64
		if err := tx.Model(&models.Customer{}).
			Where("cpo_id = ? AND lower(btrim(email)) = ?", challenge.CPOID, challenge.Email).
			Count(&existing).Error; err != nil {
			return fmt.Errorf("check existing customer: %w", err)
		}
		if existing != 0 {
			outcome = errAlreadyRegistered
			return nil
		}
		customer := models.Customer{
			ID: uuid.New(), CPOID: challenge.CPOID, Email: challenge.Email,
			PasswordHash: challenge.PasswordHash, FullName: challenge.FullName, Phone: challenge.Phone,
			IsVerified: true, PasswordChangedAt: now,
			Status: constants.CustomerStatusActive, CreatedAt: now, UpdatedAt: now,
		}
		if err := tx.Create(&customer).Error; err != nil {
			return fmt.Errorf("create customer: %w", err)
		}
		wallet := models.Wallet{
			ID: uuid.New(), CPOID: challenge.CPOID, CustomerID: customer.ID,
			Balance: decimal.Zero, Currency: "INR", CreatedAt: now, UpdatedAt: now,
		}
		if err := tx.Create(&wallet).Error; err != nil {
			return fmt.Errorf("create customer wallet: %w", err)
		}
		audit := models.AuditLog{
			ID: uuid.New(), CPOID: &challenge.CPOID,
			Action: "CUSTOMER_SIGNED_UP", Entity: "CUSTOMER", EntityID: &customer.ID,
			Details: models.JSONB{}, CreatedAt: now,
		}
		if err := tx.Create(&audit).Error; err != nil {
			return fmt.Errorf("record customer signup audit: %w", err)
		}
		response = SignupResponse{
			CustomerID: customer.ID, CPOID: challenge.CPOID, WalletID: wallet.ID,
		}
		return nil
	})
	if err != nil {
		return SignupResponse{}, err
	}
	if outcome != nil {
		return SignupResponse{}, outcome
	}
	return response, nil
}

func (service *Service) Resend(
	ctx context.Context,
	appID string,
	request ResendRequest,
	metadata RequestMetadata,
) (ChallengeResponse, error) {
	if strings.TrimSpace(appID) == "" {
		return ChallengeResponse{}, &APIError{http.StatusBadRequest, "missing_cpo_app_id", "X-CPO-App-ID is required."}
	}
	if request.ChallengeID == uuid.Nil {
		return ChallengeResponse{}, errInvalidChallenge
	}
	if !service.mailEnabled {
		return ChallengeResponse{}, errMailUnavailable
	}
	if err := service.checkRateLimit(ctx, "RESEND_CUSTOMER_SIGNUP", rateLimitAddress(metadata)); err != nil {
		return ChallengeResponse{}, err
	}
	var response ChallengeResponse
	var outcome error
	err := service.database.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		identity, err := service.findSignupChallengeIdentity(tx, request.ChallengeID)
		if err != nil {
			outcome = err
			return nil
		}
		if err := lockSignupIdentity(tx, identity.CPOID, identity.Email); err != nil {
			return err
		}
		challenge, err := service.lockChallenge(tx, request.ChallengeID)
		if err != nil {
			outcome = err
			return nil
		}
		cpo, err := activeCPO(tx, strings.TrimSpace(appID))
		now := service.now()
		if err != nil || cpo.ID != challenge.CPOID || challenge.ConsumedAt != nil ||
			challenge.InvalidatedAt != nil || !now.Before(challenge.ExpiresAt) ||
			now.Before(challenge.ResendAvailableAt) {
			outcome = errInvalidChallenge
			return nil
		}
		if err := tx.Model(&models.CustomerSignupChallenge{}).
			Where("id = ?", challenge.ID).
			Updates(map[string]any{
				"invalidated_at": now,
				"password_hash":  "INVALIDATED",
			}).Error; err != nil {
			return fmt.Errorf("invalidate signup challenge: %w", err)
		}
		response, err = service.createChallenge(
			tx, challenge.CPOID, challenge.Email, challenge.PasswordHash,
			challenge.FullName, challenge.Phone, metadata, now,
		)
		return err
	})
	if err != nil {
		return ChallengeResponse{}, err
	}
	if outcome != nil {
		return ChallengeResponse{}, outcome
	}
	return response, nil
}

func (service *Service) createChallenge(
	tx *gorm.DB,
	cpoID uuid.UUID,
	email, passwordHash, fullName string,
	phone *string,
	metadata RequestMetadata,
	now time.Time,
) (ChallengeResponse, error) {
	if err := lockSignupIdentity(tx, cpoID, email); err != nil {
		return ChallengeResponse{}, err
	}
	code, err := security.RandomDigits(6)
	if err != nil {
		return ChallengeResponse{}, err
	}
	challenge := models.CustomerSignupChallenge{
		ID: uuid.New(), CPOID: cpoID, Email: email, PasswordHash: passwordHash,
		FullName: fullName, Phone: phone, ExpiresAt: now.Add(service.config.OTPExpiry),
		MaxAttempts: 5, ResendAvailableAt: now.Add(service.config.OTPResendCooldown),
		RequestIP: metadata.IPAddress, UserAgent: boundedUserAgent(metadata.UserAgent),
		CreatedAt: now,
	}
	challenge.CodeHash = service.otpHash(challenge.ID, code)
	if err := tx.Create(&challenge).Error; err != nil {
		return ChallengeResponse{}, fmt.Errorf("create customer signup challenge: %w", err)
	}
	if err := service.outbox.EnqueueMessageWithContext(
		tx,
		email,
		signupMailTemplate,
		cmsmail.MessagePayload{
			RecipientName: fullName, Code: code, ExpiresAt: challenge.ExpiresAt,
		},
		cmsmail.MessageContext{CPOID: &cpoID},
	); err != nil {
		return ChallengeResponse{}, err
	}
	return ChallengeResponse{
		ChallengeID: challenge.ID, ExpiresAt: challenge.ExpiresAt,
		ResendAvailableAt: challenge.ResendAvailableAt,
	}, nil
}

func (service *Service) lockChallenge(tx *gorm.DB, id uuid.UUID) (models.CustomerSignupChallenge, error) {
	var challenge models.CustomerSignupChallenge
	err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&challenge, "id = ?", id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return challenge, errInvalidChallenge
	}
	if err != nil {
		return challenge, fmt.Errorf("lock customer signup challenge: %w", err)
	}
	return challenge, nil
}

type signupChallengeIdentity struct {
	CPOID uuid.UUID
	Email string
}

func (service *Service) findSignupChallengeIdentity(tx *gorm.DB, id uuid.UUID) (signupChallengeIdentity, error) {
	var identity signupChallengeIdentity
	if err := tx.Model(&models.CustomerSignupChallenge{}).Select("cpo_id, email").Where("id = ?", id).Take(&identity).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return signupChallengeIdentity{}, errInvalidChallenge
		}
		return signupChallengeIdentity{}, fmt.Errorf("find signup challenge identity: %w", err)
	}
	return identity, nil
}

func lockSignupIdentity(tx *gorm.DB, cpoID uuid.UUID, email string) error {
	if err := tx.Exec(
		"SELECT pg_advisory_xact_lock(hashtextextended(?, 0))",
		"customer-signup:"+cpoID.String()+":"+strings.ToLower(strings.TrimSpace(email)),
	).Error; err != nil {
		return fmt.Errorf("lock signup identity: %w", err)
	}
	return nil
}

func activeCPO(tx *gorm.DB, appID string) (models.CPO, error) {
	var cpo models.CPO
	err := tx.Where("app_id = ? AND status = ?", appID, constants.CPOStatusActive).First(&cpo).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return cpo, errSignupUnavailable
	}
	if err != nil {
		return cpo, fmt.Errorf("resolve CPO application: %w", err)
	}
	return cpo, nil
}

func (service *Service) otpHash(id uuid.UUID, code string) []byte {
	mac := hmac.New(sha256.New, service.config.OTPHMACKey)
	_, _ = mac.Write([]byte(id.String()))
	_, _ = mac.Write([]byte{0})
	_, _ = mac.Write([]byte("CUSTOMER_SIGNUP"))
	_, _ = mac.Write([]byte{0})
	_, _ = mac.Write([]byte(code))
	return mac.Sum(nil)
}

func (service *Service) verifyOTP(challenge models.CustomerSignupChallenge, code string) bool {
	return hmac.Equal(service.otpHash(challenge.ID, code), challenge.CodeHash)
}

func challengeUsable(challenge models.CustomerSignupChallenge, now time.Time) bool {
	return challenge.ConsumedAt == nil && challenge.InvalidatedAt == nil &&
		challenge.Attempts < challenge.MaxAttempts && now.Before(challenge.ExpiresAt)
}

func recordChallengeFailure(tx *gorm.DB, challenge models.CustomerSignupChallenge, now time.Time) error {
	updates := map[string]any{"attempts": challenge.Attempts + 1}
	if challenge.Attempts+1 >= challenge.MaxAttempts {
		updates["invalidated_at"] = now
		updates["password_hash"] = "INVALIDATED"
	}
	return tx.Model(&models.CustomerSignupChallenge{}).
		Where("id = ?", challenge.ID).Updates(updates).Error
}

func (service *Service) checkRateLimit(ctx context.Context, action, material string) error {
	sum := sha256.Sum256([]byte(material))
	scopeKey := hex.EncodeToString(sum[:])
	now := service.now()
	var blockedUntil sql.NullTime
	result := service.database.WithContext(ctx).Raw(`
		INSERT INTO auth_rate_limits (
		    scope_key, action, window_started_at, attempt_count, blocked_until, updated_at
		) VALUES (?, ?, ?, 1, NULL, ?)
		ON CONFLICT (scope_key, action) DO UPDATE
		SET window_started_at = CASE WHEN auth_rate_limits.window_started_at <= ? THEN excluded.window_started_at ELSE auth_rate_limits.window_started_at END,
		    attempt_count = CASE WHEN auth_rate_limits.window_started_at <= ? THEN 1 ELSE auth_rate_limits.attempt_count + 1 END,
		    blocked_until = CASE
		        WHEN auth_rate_limits.blocked_until > ? THEN auth_rate_limits.blocked_until
		        WHEN auth_rate_limits.window_started_at > ? AND auth_rate_limits.attempt_count + 1 > ? THEN ?
		        ELSE NULL
		    END,
		    updated_at = excluded.updated_at
		RETURNING blocked_until
	`, scopeKey, action, now, now, now.Add(-service.config.RateLimitWindow),
		now.Add(-service.config.RateLimitWindow), now, now.Add(-service.config.RateLimitWindow),
		service.config.RateLimitMax, now.Add(service.config.RateLimitWindow)).Scan(&blockedUntil)
	if result.Error != nil {
		return fmt.Errorf("apply customer signup rate limit: %w", result.Error)
	}
	if err := service.database.WithContext(ctx).
		Where(
			"updated_at < ? AND (blocked_until IS NULL OR blocked_until < ?)",
			now.Add(-7*24*time.Hour),
			now,
		).
		Delete(&models.AuthRateLimit{}).Error; err != nil {
		return fmt.Errorf("prune customer signup rate limits: %w", err)
	}
	if blockedUntil.Valid && blockedUntil.Time.After(now) {
		return errRateLimited
	}
	return nil
}

func validEmail(value string) bool {
	if value == "" || len(value) > 320 {
		return false
	}
	address, err := netmail.ParseAddress(value)
	return err == nil && strings.EqualFold(address.Address, value)
}

func normalizePhone(value *string) (*string, error) {
	if value == nil {
		return nil, nil
	}
	phone := strings.TrimSpace(*value)
	if !phonePattern.MatchString(phone) {
		return nil, &APIError{http.StatusBadRequest, "invalid_phone", "Phone must contain 7 to 15 digits with an optional leading +."}
	}
	return &phone, nil
}

func validOTP(code string) bool {
	if len(code) != 6 {
		return false
	}
	for _, character := range code {
		if character < '0' || character > '9' {
			return false
		}
	}
	return true
}

func rateLimitAddress(metadata RequestMetadata) string {
	if metadata.IPAddress == nil {
		return "unknown"
	}
	return *metadata.IPAddress
}

func boundedUserAgent(value string) string {
	value = strings.TrimSpace(value)
	if len(value) > 512 {
		return value[:512]
	}
	return value
}
