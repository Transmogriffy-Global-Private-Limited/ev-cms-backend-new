package integrations

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/auth"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/constants"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/models"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/security"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type RazorpayCredentials struct {
	KeyID         string `json:"key_id"`
	KeySecret     string `json:"key_secret"`
	WebhookSecret string `json:"webhook_secret,omitempty"`
}

type IntegrationView struct {
	Provider     string    `json:"provider"`
	DisplayHint  string    `json:"display_hint"`
	IsActive     bool      `json:"is_active"`
	ConfiguredAt time.Time `json:"configured_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type Service struct {
	database *gorm.DB
	box      *security.SecretBox
	now      func() time.Time
}

func NewService(database *gorm.DB, box *security.SecretBox) *Service {
	return &Service{
		database: database,
		box:      box,
		now:      func() time.Time { return time.Now().UTC() },
	}
}

func (service *Service) List(
	ctx context.Context,
	principal auth.Principal,
) ([]IntegrationView, error) {
	cpoID, err := requireCPOPrincipal(principal)
	if err != nil {
		return nil, err
	}
	var records []models.CPOIntegration
	if err := service.database.WithContext(ctx).
		Where("cpo_id = ?", cpoID).
		Order("provider").
		Find(&records).Error; err != nil {
		return nil, fmt.Errorf("list CPO integrations: %w", err)
	}
	result := make([]IntegrationView, 0, len(records))
	for _, record := range records {
		result = append(result, view(record))
	}
	return result, nil
}

func (service *Service) Get(
	ctx context.Context,
	principal auth.Principal,
	provider string,
) (IntegrationView, error) {
	cpoID, err := requireCPOPrincipal(principal)
	if err != nil {
		return IntegrationView{}, err
	}
	provider, err = validateProvider(provider)
	if err != nil {
		return IntegrationView{}, err
	}
	var record models.CPOIntegration
	if err := service.database.WithContext(ctx).
		Where("cpo_id = ? AND provider = ?", cpoID, provider).
		First(&record).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return IntegrationView{}, &auth.APIError{
				Status: http.StatusNotFound, Code: "integration_not_found",
				Message: "The integration is not configured.",
			}
		}
		return IntegrationView{}, fmt.Errorf("get CPO integration: %w", err)
	}
	return view(record), nil
}

func (service *Service) PutRazorpay(
	ctx context.Context,
	principal auth.Principal,
	provider string,
	credentials RazorpayCredentials,
) (IntegrationView, error) {
	cpoID, err := requireCPOPrincipal(principal)
	if err != nil {
		return IntegrationView{}, err
	}
	provider, err = validateProvider(provider)
	if err != nil {
		return IntegrationView{}, err
	}
	credentials.KeyID = strings.TrimSpace(credentials.KeyID)
	credentials.KeySecret = strings.TrimSpace(credentials.KeySecret)
	credentials.WebhookSecret = strings.TrimSpace(credentials.WebhookSecret)
	if len(credentials.KeyID) < 8 || len(credentials.KeyID) > 100 ||
		len(credentials.KeySecret) < 16 || len(credentials.KeySecret) > 255 ||
		len(credentials.WebhookSecret) > 255 {
		return IntegrationView{}, &auth.APIError{
			Status: http.StatusBadRequest, Code: "invalid_integration_credentials",
			Message: "The Razorpay credential fields are invalid.",
		}
	}
	plaintext, err := json.Marshal(credentials)
	if err != nil {
		return IntegrationView{}, fmt.Errorf("encode integration credentials: %w", err)
	}
	ciphertext, err := service.box.Seal(
		plaintext,
		integrationAAD(cpoID, provider),
	)
	if err != nil {
		return IntegrationView{}, err
	}
	now := service.now()
	record := models.CPOIntegration{
		ID:                   uuid.New(),
		CPOID:                cpoID,
		Provider:             provider,
		CredentialCiphertext: ciphertext,
		EncryptionKeyID:      service.box.KeyID(),
		DisplayHint:          maskedKeyID(credentials.KeyID),
		IsActive:             true,
		UpdatedByUserID:      principal.UserID,
		CreatedAt:            now,
		UpdatedAt:            now,
	}
	err = service.database.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "cpo_id"}, {Name: "provider"}},
			DoUpdates: clause.Assignments(map[string]any{
				"credential_ciphertext": record.CredentialCiphertext,
				"encryption_key_id":     record.EncryptionKeyID,
				"display_hint":          record.DisplayHint,
				"is_active":             true,
				"updated_by_user_id":    record.UpdatedByUserID,
				"updated_at":            now,
			}),
		}).Create(&record).Error; err != nil {
			return fmt.Errorf("store CPO integration credentials: %w", err)
		}
		if err := tx.Where("cpo_id = ? AND provider = ?", cpoID, provider).
			First(&record).Error; err != nil {
			return fmt.Errorf("reload CPO integration metadata: %w", err)
		}
		return audit(
			tx,
			principal,
			"CPO_INTEGRATION_CREDENTIALS_SET",
			&record.ID,
			models.JSONB{"provider": provider},
			now,
		)
	})
	if err != nil {
		return IntegrationView{}, err
	}
	return view(record), nil
}

func (service *Service) Delete(
	ctx context.Context,
	principal auth.Principal,
	provider string,
) error {
	cpoID, err := requireCPOPrincipal(principal)
	if err != nil {
		return err
	}
	provider, err = validateProvider(provider)
	if err != nil {
		return err
	}
	return service.database.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var record models.CPOIntegration
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("cpo_id = ? AND provider = ?", cpoID, provider).
			First(&record).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return &auth.APIError{
					Status: http.StatusNotFound, Code: "integration_not_found",
					Message: "The integration is not configured.",
				}
			}
			return fmt.Errorf("lock CPO integration credentials: %w", err)
		}
		if err := tx.Delete(&record).Error; err != nil {
			return fmt.Errorf("delete CPO integration credentials: %w", err)
		}
		return audit(
			tx,
			principal,
			"CPO_INTEGRATION_CREDENTIALS_REMOVED",
			&record.ID,
			models.JSONB{"provider": provider},
			service.now(),
		)
	})
}

// ResolveRazorpay is intentionally internal service behavior, not an HTTP read
// endpoint. Payment orchestration may use it later after independently proving
// its trusted CPO context.
func (service *Service) ResolveRazorpay(
	ctx context.Context,
	cpoID uuid.UUID,
) (RazorpayCredentials, error) {
	var record models.CPOIntegration
	if err := service.database.WithContext(ctx).
		Where(
			"cpo_id = ? AND provider = ? AND is_active = true",
			cpoID,
			constants.IntegrationProviderRazorpay,
		).
		First(&record).Error; err != nil {
		return RazorpayCredentials{}, fmt.Errorf("resolve Razorpay credentials: %w", err)
	}
	if record.EncryptionKeyID != service.box.KeyID() {
		return RazorpayCredentials{}, fmt.Errorf(
			"integration encryption key %q unavailable",
			record.EncryptionKeyID,
		)
	}
	plaintext, err := service.box.Open(
		record.CredentialCiphertext,
		integrationAAD(cpoID, record.Provider),
	)
	if err != nil {
		return RazorpayCredentials{}, err
	}
	var credentials RazorpayCredentials
	if err := json.Unmarshal(plaintext, &credentials); err != nil {
		return RazorpayCredentials{}, fmt.Errorf("decode Razorpay credentials: %w", err)
	}
	return credentials, nil
}

func requireCPOPrincipal(principal auth.Principal) (uuid.UUID, error) {
	if principal.Scope != constants.AuthScopeCPO ||
		principal.CPOID == nil ||
		principal.Role == nil ||
		(*principal.Role != constants.CPORoleOwner &&
			*principal.Role != constants.CPORoleAdmin) {
		return uuid.Nil, &auth.APIError{
			Status: http.StatusForbidden, Code: "forbidden",
			Message: "CPO administrator access is required.",
		}
	}
	return *principal.CPOID, nil
}

func validateProvider(provider string) (string, error) {
	provider = strings.ToUpper(strings.TrimSpace(provider))
	if provider != constants.IntegrationProviderRazorpay {
		return "", &auth.APIError{
			Status: http.StatusBadRequest, Code: "unsupported_integration_provider",
			Message: "The integration provider is not supported.",
		}
	}
	return provider, nil
}

func integrationAAD(cpoID uuid.UUID, provider string) []byte {
	return []byte("ev-cms-cpo-integration:" + cpoID.String() + ":" + provider)
}

func maskedKeyID(keyID string) string {
	if len(keyID) <= 4 {
		return "****"
	}
	return "****" + keyID[len(keyID)-4:]
}

func view(record models.CPOIntegration) IntegrationView {
	return IntegrationView{
		Provider:     record.Provider,
		DisplayHint:  record.DisplayHint,
		IsActive:     record.IsActive,
		ConfiguredAt: record.CreatedAt,
		UpdatedAt:    record.UpdatedAt,
	}
}

func audit(
	tx *gorm.DB,
	principal auth.Principal,
	action string,
	entityID *uuid.UUID,
	details models.JSONB,
	now time.Time,
) error {
	record := models.AuditLog{
		ID:        uuid.New(),
		CPOID:     principal.CPOID,
		UserID:    &principal.UserID,
		Action:    action,
		Entity:    "CPO_INTEGRATION",
		EntityID:  entityID,
		Details:   details,
		CreatedAt: now,
	}
	if err := tx.Create(&record).Error; err != nil {
		return fmt.Errorf("audit CPO integration mutation: %w", err)
	}
	return nil
}
