package cpo

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	netmail "net/mail"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/auth"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/commercial"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/config"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/constants"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/cpopermissions"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/halops"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/liveops"
	cmsmail "github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/mail"
	cmsmiddleware "github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/middleware"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/models"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/operationalrealtime"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/platformops"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/security"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const dummyAppIDPrefix = "cpo_dummy_"

const (
	defaultListLimit            = 50
	maxListLimit                = 200
	defaultLiveSessionListLimit = 100
	maxSearchLength             = 200
	minReasonLength             = 3
	maxReasonLength             = 500
)

var (
	slugPattern                    = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)
	gstinPattern                   = regexp.MustCompile(`^(0[1-9]|[12][0-9]|3[0-8])[A-Z]{5}[0-9]{4}[A-Z][1-9A-Z]Z[0-9A-Z]$`)
	pincodePattern                 = regexp.MustCompile(`^[1-9][0-9]{5}$`)
	appIDPattern                   = regexp.MustCompile(`^[a-z0-9_-]{16,100}$`)
	currentCPOSubscriptionStatuses = []string{"TRIAL", "ACTIVE", "PAUSED", "PAST_DUE"}
)

type Service struct {
	database             *gorm.DB
	outbox               *cmsmail.Outbox
	mailEnabled          bool
	events               *platformops.Service
	now                  func() time.Time
	chargerConnectionURL string
	halOperations        *halops.Service
	liveOperations       *liveops.Service
	operationalEvents    *operationalrealtime.Service
	frontend             config.FrontendLinks
	repository           Repository
}

type sessionRevocationCounts struct {
	sessions      int64
	refreshTokens int64
}

func (service *Service) WithPlatformEvents(events *platformops.Service) *Service {
	service.events = events
	return service
}

func (service *Service) WithFrontendLinks(frontend config.FrontendLinks) *Service {
	service.frontend = frontend
	return service
}

// WithOperationalCapabilities keeps CPO authorization in this service while
// delegating provider mechanics and live reads to CMS-owned capabilities.
func (service *Service) WithOperationalCapabilities(hal *halops.Service, live *liveops.Service) *Service {
	service.halOperations = hal
	service.liveOperations = live
	return service
}

func (service *Service) WithOperationalEvents(events *operationalrealtime.Service) *Service {
	service.operationalEvents = events
	return service
}

func NewService(
	database *gorm.DB,
	outbox *cmsmail.Outbox,
	mailEnabled bool,
	chargerConnectionURL string,
) *Service {
	return &Service{
		database:             database,
		repository:           NewRepository(database),
		outbox:               outbox,
		mailEnabled:          mailEnabled,
		now:                  func() time.Time { return time.Now().UTC() },
		chargerConnectionURL: chargerConnectionURL,
		frontend: config.FrontendLinks{
			CPOOnboardingTemplate: "https://cms.example.invalid/login#cpo_id={cpo_id}",
		},
	}
}

func (service *Service) GetAnalytics(ctx context.Context, principal auth.Principal, query AnalyticsQuery) (AnalyticsResponse, error) {
	if err := requireCPOContext(principal); err != nil {
		return AnalyticsResponse{}, err
	}
	analytics, err := service.repository.GetAnalytics(ctx, *principal.CPOID, query)
	if err != nil {
		return AnalyticsResponse{}, fmt.Errorf("get analytics: %w", err)
	}
	return AnalyticsResponse{
		TotalChargers:   analytics.TotalChargers,
		TotalConnectors: analytics.TotalConnectors,
		TotalRevenue:    analytics.TotalRevenue,
		TotalUsage:      analytics.TotalUsage,
		TotalSessions:   analytics.TotalSessions,
	}, nil
}

func (service *Service) GetChargingSession(
	ctx context.Context,
	principal auth.Principal,
	sessionID uuid.UUID,
) (ChargingSessionView, error) {
	if err := requireCPOContext(principal); err != nil {
		return ChargingSessionView{}, err
	}

	// Repository already preloads Customer, Charger, Charger.Hub, Connector
	session, err := service.repository.GetChargingSession(ctx, *principal.CPOID, sessionID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ChargingSessionView{}, &auth.APIError{
				Status:  http.StatusNotFound,
				Code:    "charging_session_not_found",
				Message: "The charging session was not found.",
			}
		}
		return ChargingSessionView{}, fmt.Errorf("load charging session: %w", err)
	}

	view := toChargingSessionView(*session)

	// Overlay live kWh for active sessions
	if service.liveOperations != nil {
		statusStr := string(session.Status)
		// Active statuses: START_PENDING, CHARGING, STOP_PENDING
		if statusStr == "START_PENDING" || statusStr == "CHARGING" || statusStr == "STOP_PENDING" {
			liveSession, err := service.liveOperations.GetSession(ctx, *principal.CPOID, sessionID)
			if err == nil && liveSession.ConsumedWh != nil {
				// Convert Wh → kWh
				consumedKWh := decimal.NewFromInt(*liveSession.ConsumedWh).Div(decimal.NewFromInt(1000))
				view.TotalKWh = consumedKWh
			}
		}
	}

	return view, nil
}
func (service *Service) ListChargingSessions(
	ctx context.Context,
	principal auth.Principal,
	query ChargingSessionListQuery,
) (ChargingSessionListResponse, error) {
	if err := requireCPOContext(principal); err != nil {
		return ChargingSessionListResponse{}, err
	}

	if query.Limit == 0 {
		query.Limit = defaultListLimit
	}
	if query.Limit < 1 || query.Limit > maxListLimit {
		return ChargingSessionListResponse{}, invalid(
			"limit",
			"Limit must be between 1 and 200.",
		)
	}

	sessions, err := service.repository.ListChargingSessions(ctx, *principal.CPOID, query)
	if err != nil {
		return ChargingSessionListResponse{}, fmt.Errorf("list charging sessions: %w", err)
	}

	hasMore := len(sessions) > query.Limit
	if hasMore {
		sessions = sessions[:query.Limit]
	}

	// Build base views
	result := make([]ChargingSessionView, 0, len(sessions))
	for _, session := range sessions {
		view := toChargingSessionView(session)
		result = append(result, view)
	}

	// Overlay live kWh for active sessions
	if service.liveOperations != nil {
		// Collect IDs of active sessions
		activeSessionIDs := []uuid.UUID{}
		for _, session := range sessions {
			statusStr := string(session.Status)
			if statusStr == "START_PENDING" || statusStr == "CHARGING" || statusStr == "STOP_PENDING" {
				activeSessionIDs = append(activeSessionIDs, session.ID)
			}
		}

		// Fetch live data for each active session (max 200 – acceptable)
		for _, sessionID := range activeSessionIDs {
			liveSession, err := service.liveOperations.GetSession(ctx, *principal.CPOID, sessionID)
			if err == nil && liveSession.ConsumedWh != nil {
				consumedKWh := decimal.NewFromInt(*liveSession.ConsumedWh).Div(decimal.NewFromInt(1000))
				// Update the matching view
				for i := range result {
					if result[i].ID == sessionID {
						result[i].TotalKWh = consumedKWh
						break
					}
				}
			}
		}
	}

	response := ChargingSessionListResponse{
		Sessions: result,
		HasMore:  hasMore,
	}

	if hasMore && len(sessions) > 0 {
		nextBefore := sessions[len(sessions)-1].CreatedAt
		nextBeforeID := sessions[len(sessions)-1].ID
		response.NextBefore = &nextBefore
		response.NextBeforeID = &nextBeforeID
	}

	return response, nil
}

func (service *Service) ListLiveChargingSessions(ctx context.Context, principal auth.Principal, query LiveChargingSessionListQuery) (LiveChargingSessionListResponse, error) {
	if err := requireCPOContext(principal); err != nil {
		return LiveChargingSessionListResponse{}, err
	}
	if service.liveOperations == nil {
		return LiveChargingSessionListResponse{}, errors.New("live operations capability is unavailable")
	}
	if query.Limit == 0 {
		query.Limit = defaultLiveSessionListLimit
	}
	if query.Limit < 1 || query.Limit > maxListLimit {
		return LiveChargingSessionListResponse{}, invalid("limit", "Limit must be between 1 and 200.")
	}

	sessions, err := service.repository.ListLiveChargingSessions(ctx, *principal.CPOID, query)
	if err != nil {
		return LiveChargingSessionListResponse{}, fmt.Errorf("list live charging sessions: %w", err)
	}
	hasMore := len(sessions) > query.Limit
	if hasMore {
		sessions = sessions[:query.Limit]
	}
	sessionIDs := make([]uuid.UUID, 0, len(sessions))
	for _, session := range sessions {
		sessionIDs = append(sessionIDs, session.ID)
	}
	liveBySessionID, err := service.liveOperations.GetSessions(ctx, *principal.CPOID, sessionIDs)
	if err != nil {
		return LiveChargingSessionListResponse{}, fmt.Errorf("load live charging-session telemetry: %w", err)
	}
	asOf := service.now()
	result := make([]LiveChargingSessionView, 0, len(sessions))
	for _, session := range sessions {
		live, ok := liveBySessionID[session.ID]
		if !ok {
			return LiveChargingSessionListResponse{}, fmt.Errorf("live charging-session projection disappeared during read")
		}
		result = append(result, toLiveChargingSessionView(session, live, asOf))
	}
	response := LiveChargingSessionListResponse{Sessions: result, HasMore: hasMore, AsOf: asOf}
	if hasMore && len(sessions) > 0 {
		next := sessions[len(sessions)-1]
		response.NextAfterStartedAt, response.NextAfterID = &next.StartTime, &next.ID
	}
	return response, nil
}

func (service *Service) ListChargerTransactions(
	ctx context.Context,
	principal auth.Principal,
	query ChargerTransactionListQuery,
) (ChargerTransactionListResponse, error) {
	if err := requireCPOContext(principal); err != nil {
		return ChargerTransactionListResponse{}, err
	}

	if query.Limit == 0 {
		query.Limit = defaultListLimit
	}
	if query.Limit < 1 || query.Limit > maxListLimit {
		return ChargerTransactionListResponse{}, invalid(
			"limit",
			"Limit must be between 1 and 200.",
		)
	}

	transactions, err := service.repository.ListChargerTransactions(ctx, *principal.CPOID, query)
	if err != nil {
		return ChargerTransactionListResponse{}, fmt.Errorf("list charger transactions: %w", err)
	}

	hasMore := len(transactions) > query.Limit
	if hasMore {
		transactions = transactions[:query.Limit]
	}

	result := make([]ChargerTransactionView, 0, len(transactions))
	for _, transaction := range transactions {
		result = append(result, toChargerTransactionView(transaction))
	}

	response := ChargerTransactionListResponse{
		Transactions: result,
		HasMore:      hasMore,
	}

	if hasMore && len(transactions) > 0 {
		nextBefore := transactions[len(transactions)-1].CreatedAt
		nextBeforeID := transactions[len(transactions)-1].ID
		response.NextBefore = &nextBefore
		response.NextBeforeID = &nextBeforeID
	}

	return response, nil
}

func toChargerTransactionView(transaction ChargerTransaction) ChargerTransactionView {
	var duration string
	if transaction.EndTime != nil {
		duration = transaction.EndTime.Sub(transaction.StartTime).String()
	}

	return ChargerTransactionView{
		TransactionID:     transaction.ID.String(),
		SessionID:         transaction.ID,
		OCPPTransactionID: transaction.TransactionID,
		HALTransactionID:  transaction.HALTransactionID,
		PaymentStatus:     transaction.PaymentStatus,
		BilledAmount:      transaction.TotalAmount,
		ChargerID:         transaction.ChargerCode,
		OCPPIdentity:      transaction.ChargerOCPPIdentity,
		ChargerName:       transaction.ChargerName,
		Duration:          duration,
		HubID:             transaction.HubID,
		Hub:               transaction.HubName,
		HubAddress:        transaction.HubAddress,
		ConnectorNumber:   transaction.ConnectorNumber,
		ConnectorType:     transaction.ConnectorType,
		Tariff:            transaction.TariffPricePerUnit,
		UsageKWh:          transaction.TotalKWh,
		Owner:             transaction.CPOBusinessName,
		HostDetails: HostDetailsView{
			Name:  transaction.ChargerHostName,
			Phone: transaction.ChargerHostPhoneNo,
		},
		CustomerDetails: CustomerDetailsView{
			Name:  transaction.CustomerFullName,
			Email: transaction.CustomerEmail,
			Phone: transaction.CustomerPhone,
		},
		Timestamp:              transaction.CreatedAt,
		Reason:                 transaction.StopReason,
		SessionStatus:          transaction.Status,
		SettlementStatus:       transaction.SettlementStatus,
		ReconciliationRequired: transaction.Status == constants.SessionStatusReconciliationRequired || transaction.SettlementStatus == "RECONCILIATION_REQUIRED",
	}
}

func toChargingSessionView(session models.ChargingSession) ChargingSessionView {
	view := ChargingSessionView{
		ID:                session.ID,
		TransactionID:     session.TransactionID,
		StartTime:         session.StartTime,
		EndTime:           session.EndTime,
		TotalKWh:          session.TotalKWh,
		TotalAmount:       session.TotalAmount,
		Currency:          session.Currency,
		Status:            session.Status,
		StopReason:        session.StopReason,
		InitialSoCPercent: session.InitialSoCPercent,
		FinalSoCPercent:   session.LatestSoCPercent,
		SoCObservedAt:     session.SoCObservedAt,
		CreatedAt:         session.CreatedAt,
	}

	if session.Customer.ID != uuid.Nil {
		view.Customer = ChargingSessionCustomerView{
			ID:    session.Customer.ID,
			Name:  session.Customer.FullName,
			Email: session.Customer.Email,
			Phone: session.Customer.Phone,
		}
	}

	if session.Charger.ID != uuid.Nil {
		charger := ChargingSessionChargerView{
			ID:           session.Charger.ID,
			ChargerID:    session.Charger.ChargerID,
			OCPPIdentity: session.Charger.OCPPIdentity,
			Name:         session.Charger.ChargerName,
			MaxPowerKW:   session.Charger.MaxPowerKW,
			HubID:        session.Charger.HubID,
		}
		if session.Charger.Vendor != nil {
			charger.Vendor = *session.Charger.Vendor
		}
		if session.Charger.Model != nil {
			charger.Model = *session.Charger.Model
		}
		if session.Charger.Hub != nil {
			charger.HubName = &session.Charger.Hub.Name
			charger.HubAddress = &session.Charger.Hub.Address
		}
		view.Charger = charger
	}

	if session.Connector.ID != uuid.Nil {
		view.Connector = ChargingSessionConnectorView{
			ID:                     session.Connector.ID,
			Number:                 session.Connector.ConnectorNumber,
			ConnectorType:          session.Connector.ConnectorType,
			ConnectorTotalCapacity: session.Connector.ConnectorTotalCapacity,
		}
	}

	return view
}

func toLiveChargingSessionView(session models.ChargingSession, live liveops.SessionState, asOf time.Time) LiveChargingSessionView {
	charger := session.Charger
	if charger.ID == uuid.Nil && session.Connector.Charger.ID != uuid.Nil {
		charger = session.Connector.Charger
	}
	var hubName *string
	if charger.Hub != nil && charger.Hub.Name != "" {
		hubName = &charger.Hub.Name
	}
	durationSeconds := int64(asOf.Sub(session.StartTime) / time.Second)
	if durationSeconds < 0 {
		durationSeconds = 0
	}
	return LiveChargingSessionView{
		SessionID:         session.ID,
		OCPPTransactionID: session.TransactionID,
		Status:            session.Status,
		StartedAt:         session.StartTime,
		DurationSeconds:   durationSeconds,
		CustomerName:      session.Customer.FullName,
		ChargerID:         charger.ChargerID,
		ChargerName:       charger.ChargerName,
		HubName:           hubName,
		ConnectorID:       session.Connector.ID,
		ConnectorNumber:   session.Connector.ConnectorNumber,
		LatestMeterWh:     live.LatestMeterWh,
		ConsumedWh:        live.ConsumedWh,
		MeterObservedAt:   live.MeterObservedAt,
		MeterFreshness:    live.MeterFreshness,
		SoCPercent:        live.LatestSoCPercent,
		SoCObservedAt:     live.SoCObservedAt,
		SoCFreshness:      live.SoCFreshness,
	}
}

// func toChargingSessionView(session models.ChargingSession) ChargingSessionView {
// 	view := ChargingSessionView{
// 		ID:            session.ID,
// 		TransactionID: session.TransactionID,
// 		StartTime:     session.StartTime,
// 		EndTime:       session.EndTime,
// 		TotalKWh:      session.TotalKWh,
// 		TotalAmount:   session.TotalAmount,
// 		Currency:      session.Currency,
// 		Status:        session.Status,
// 		StopReason:    session.StopReason,
// 		CreatedAt:     session.CreatedAt,
// 	}

// 	if session.Customer.ID != uuid.Nil {
// 		view.Customer = ChargingSessionCustomerView{
// 			Name:  session.Customer.FullName,
// 			Email: session.Customer.Email,
// 		}
// 	}

// 	if session.Charger.ID != uuid.Nil {
// 		charger := ChargingSessionChargerView{
// 			Name:       session.Charger.ChargerName,
// 			MaxPowerKW: session.Charger.MaxPowerKW,
// 		}
// 		if session.Charger.Vendor != nil {
// 			charger.Vendor = *session.Charger.Vendor
// 		}
// 		if session.Charger.Model != nil {
// 			charger.Model = *session.Charger.Model
// 		}
// 		if session.Charger.Hub != nil {
// 			charger.HubName = &session.Charger.Hub.Name
// 		}
// 		view.Charger = charger
// 	}

// 	if session.Connector.ID != uuid.Nil {
// 		view.Connector = ChargingSessionConnectorView{
// 			Number: session.Connector.ConnectorNumber,
// 		}
// 	}

// 	return view
// }

func (service *Service) Create(
	ctx context.Context,
	principal auth.Principal,
	request CreateRequest,
) (CreateResponse, error) {
	if err := requirePlatform(principal); err != nil {
		return CreateResponse{}, err
	}
	if !service.mailEnabled {
		return CreateResponse{}, &auth.APIError{
			Status:  http.StatusServiceUnavailable,
			Code:    "mail_unavailable",
			Message: "CPO administrator onboarding email is unavailable.",
		}
	}
	request = normalizeCreateRequest(request)
	if err := validateCreateRequest(request); err != nil {
		return CreateResponse{}, err
	}
	randomID, err := security.RandomHex(16)
	if err != nil {
		return CreateResponse{}, err
	}
	dummyAppID := dummyAppIDPrefix + randomID
	now := service.now()
	statusActorID := principal.UserID
	cpoRecord := models.CPO{
		ID:                    uuid.New(),
		Slug:                  request.Slug,
		BusinessName:          request.BusinessName,
		CompanyType:           request.CompanyType,
		GSTIN:                 request.GSTIN,
		Address:               request.Address,
		City:                  request.City,
		State:                 request.State,
		Pincode:               request.Pincode,
		Status:                constants.CPOStatusPending,
		StatusReason:          "Initial provisioning",
		StatusChangedAt:       now,
		StatusChangedByUserID: &statusActorID,
		AppID:                 dummyAppID,
		AppIDMode:             constants.CPOAppIDModeDummy,
		AppIDUpdatedAt:        now,
		CreatedAt:             now,
		UpdatedAt:             now,
	}

	var admin models.User
	identityCreated := false
	err = service.database.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec(
			"SELECT pg_advisory_xact_lock(hashtextextended(?, 0))",
			"cpo-admin:"+request.Admin.Email,
		).Error; err != nil {
			return fmt.Errorf("serialize initial CPO administrator: %w", err)
		}
		if err := tx.Create(&cpoRecord).Error; err != nil {
			return mapWriteError(err, "create CPO")
		}
		if err := tx.Create(&models.Settings{
			CPOID:                  cpoRecord.ID,
			WalletMinBalance:       0,
			WalletBufferMinBalance: 0,
			CreatedAt:              now,
			UpdatedAt:              now,
		}).Error; err != nil {
			return mapWriteError(err, "create default CPO settings")
		}

		result := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("lower(btrim(email)) = ?", request.Admin.Email).
			First(&admin)
		switch {
		case result.Error == nil:
			if !admin.IsActive {
				return &auth.APIError{
					Status:  http.StatusConflict,
					Code:    "admin_identity_inactive",
					Message: "The administrator identity exists but is inactive.",
				}
			}
		case errors.Is(result.Error, gorm.ErrRecordNotFound):
			temporaryPassword, err := generateTemporaryPassword()
			if err != nil {
				return err
			}
			passwordHash, err := security.HashPassword(temporaryPassword)
			if err != nil {
				return fmt.Errorf("hash temporary administrator password: %w", err)
			}
			admin = models.User{
				ID:                 uuid.New(),
				Email:              request.Admin.Email,
				PasswordHash:       passwordHash,
				FullName:           request.Admin.FullName,
				IsActive:           true,
				IsVerified:         false,
				MFAEnabled:         false,
				MustChangePassword: true,
				PasswordChangedAt:  now,
				CreatedAt:          now,
				UpdatedAt:          now,
			}
			if err := tx.Create(&admin).Error; err != nil {
				return mapWriteError(err, "create initial CPO administrator")
			}
			identityCreated = true
			actionURL, err := service.cpoOnboardingActionURL(cpoRecord.ID)
			if err != nil {
				return err
			}
			if err := service.outbox.EnqueueMessageWithContext(
				tx,
				admin.Email,
				"CPO_STAFF_NEW_IDENTITY",
				cmsmail.MessagePayload{
					RecipientName:     admin.FullName,
					TemporaryPassword: temporaryPassword,
					CPOName:           cpoRecord.BusinessName,
					CPOID:             cpoRecord.ID.String(),
					CPOAppID:          cpoRecord.AppID,
					Role:              string(constants.CPORoleAdmin),
					ActionURL:         actionURL,
				},
				cmsmail.MessageContext{
					CPOID:  &cpoRecord.ID,
					UserID: &admin.ID,
				},
			); err != nil {
				return err
			}
		default:
			return fmt.Errorf("find initial CPO administrator: %w", result.Error)
		}

		membership := models.CPOMembership{
			ID:             uuid.New(),
			CPOID:          cpoRecord.ID,
			UserID:         admin.ID,
			Role:           constants.CPORoleAdmin,
			Status:         constants.MembershipStatusActive,
			IsPrimaryAdmin: true,
			CreatedAt:      now,
			UpdatedAt:      now,
		}
		if err := tx.Create(&membership).Error; err != nil {
			return mapWriteError(err, "create initial CPO administrator membership")
		}
		if !identityCreated {
			actionURL, err := service.cpoOnboardingActionURL(cpoRecord.ID)
			if err != nil {
				return err
			}
			if err := service.outbox.EnqueueMessageWithContext(
				tx,
				admin.Email,
				"CPO_STAFF_EXISTING_IDENTITY",
				cmsmail.MessagePayload{
					RecipientName: admin.FullName,
					CPOName:       cpoRecord.BusinessName,
					CPOID:         cpoRecord.ID.String(),
					CPOAppID:      cpoRecord.AppID,
					Role:          string(constants.CPORoleAdmin),
					ActionURL:     actionURL,
				},
				cmsmail.MessageContext{
					CPOID:  &cpoRecord.ID,
					UserID: &admin.ID,
				},
			); err != nil {
				return err
			}
		}
		if err := writeAudit(
			tx,
			principal.UserID,
			cpoRecord.ID,
			"CPO_CREATED",
			models.JSONB{
				"app_id_mode":      constants.CPOAppIDModeDummy,
				"admin_user_id":    admin.ID,
				"identity_created": identityCreated,
			},
			now,
		); err != nil {
			return err
		}
		return service.emit(
			tx,
			principal.UserID,
			cpoRecord.ID,
			"platform.cpo.created",
			models.JSONB{
				"status":      cpoRecord.Status,
				"app_id_mode": cpoRecord.AppIDMode,
			},
		)
	})
	if err != nil {
		return CreateResponse{}, err
	}
	return CreateResponse{
		CPO: view(cpoRecord),
		Admin: InitialAdminView{
			UserID:          admin.ID,
			Email:           admin.Email,
			FullName:        admin.FullName,
			Role:            constants.CPORoleAdmin,
			IdentityCreated: identityCreated,
		},
	}, nil
}

func (service *Service) List(
	ctx context.Context,
	principal auth.Principal,
	query ListQuery,
) (ListResponse, error) {
	if err := requirePlatform(principal); err != nil {
		return ListResponse{}, err
	}
	query.Search = strings.TrimSpace(query.Search)
	if len(query.Search) > maxSearchLength {
		return ListResponse{}, invalid(
			"q",
			"Search text must not exceed 200 characters.",
		)
	}
	if query.Limit == 0 {
		query.Limit = defaultListLimit
	}
	if query.Limit < 1 || query.Limit > maxListLimit {
		return ListResponse{}, invalid(
			"limit",
			"Limit must be between 1 and 200.",
		)
	}
	if query.Status != nil && !query.Status.Valid() {
		return ListResponse{}, invalid(
			"status",
			"Status must be PENDING, ACTIVE, or SUSPENDED.",
		)
	}
	if query.AppMode != nil && !query.AppMode.Valid() {
		return ListResponse{}, invalid(
			"app_id_mode",
			"App ID mode must be DUMMY or LIVE.",
		)
	}
	if (query.Before == nil) != (query.BeforeID == nil) {
		return ListResponse{}, invalid(
			"cursor",
			"before and before_id must be supplied together.",
		)
	}

	databaseQuery := service.database.WithContext(ctx).Model(&models.CPO{})
	if query.Search != "" {
		search := strings.ToLower(query.Search)
		databaseQuery = databaseQuery.Where(`
			strpos(lower(cpos.business_name), ?) > 0
			OR strpos(lower(cpos.slug), ?) > 0
			OR strpos(lower(coalesce(cpos.gstin, '')), ?) > 0
			OR strpos(lower(cpos.app_id), ?) > 0
			OR EXISTS (
				SELECT 1
				FROM cpo_memberships
				JOIN users ON users.id = cpo_memberships.user_id
				WHERE cpo_memberships.cpo_id = cpos.id
				  AND cpo_memberships.is_primary_admin
				  AND (
				      strpos(lower(users.email), ?) > 0
				      OR strpos(lower(users.full_name), ?) > 0
				  )
			)
		`, search, search, search, search, search, search)
	}
	if query.Status != nil {
		databaseQuery = databaseQuery.Where("cpos.status = ?", *query.Status)
	}
	if query.AppMode != nil {
		databaseQuery = databaseQuery.Where("cpos.app_id_mode = ?", *query.AppMode)
	}
	if query.Before != nil {
		databaseQuery = databaseQuery.Where(
			"(cpos.created_at, cpos.id) < (?, ?)",
			*query.Before,
			*query.BeforeID,
		)
	}

	var records []models.CPO
	if err := databaseQuery.
		Order("cpos.created_at DESC, cpos.id DESC").
		Limit(query.Limit + 1).
		Find(&records).Error; err != nil {
		return ListResponse{}, fmt.Errorf("list CPOs: %w", err)
	}
	hasMore := len(records) > query.Limit
	if hasMore {
		records = records[:query.Limit]
	}
	result := make([]View, 0, len(records))
	for _, record := range records {
		result = append(result, view(record))
	}
	response := ListResponse{CPOs: result, HasMore: hasMore}
	if hasMore && len(records) > 0 {
		nextBefore := records[len(records)-1].CreatedAt
		nextBeforeID := records[len(records)-1].ID
		response.NextBefore = &nextBefore
		response.NextBeforeID = &nextBeforeID
	}
	return response, nil
}

func (service *Service) Get(
	ctx context.Context,
	principal auth.Principal,
	cpoID uuid.UUID,
) (View, error) {
	if err := requirePlatform(principal); err != nil {
		return View{}, err
	}
	record, err := service.find(ctx, cpoID)
	if err != nil {
		return View{}, err
	}
	return view(record), nil
}

func (service *Service) CheckSlugAvailability(
	ctx context.Context,
	principal auth.Principal,
	candidate string,
) (SlugAvailabilityResponse, error) {
	if err := requirePlatform(principal); err != nil {
		return SlugAvailabilityResponse{}, err
	}

	slug := normalizeSlug(candidate)
	if err := validateSlug(slug); err != nil {
		return SlugAvailabilityResponse{}, err
	}

	var count int64
	if err := service.database.WithContext(ctx).
		Model(&models.CPO{}).
		Where("lower(slug) = ?", slug).
		Count(&count).Error; err != nil {
		return SlugAvailabilityResponse{}, fmt.Errorf("check CPO slug availability: %w", err)
	}

	return SlugAvailabilityResponse{
		Slug:      slug,
		Available: count == 0,
	}, nil
}

func (service *Service) UpdateProfile(
	ctx context.Context,
	principal auth.Principal,
	cpoID uuid.UUID,
	request UpdateProfileRequest,
) (View, error) {
	if err := requirePlatform(principal); err != nil {
		return View{}, err
	}
	request = normalizeProfileRequest(request)
	if err := validateProfileRequest(request); err != nil {
		return View{}, err
	}

	var record models.CPO
	err := service.database.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			First(&record, "id = ?", cpoID).Error; err != nil {
			return mapNotFound(err)
		}
		now := service.now()
		updates := map[string]any{
			"business_name": request.BusinessName,
			"company_type":  request.CompanyType,
			"gstin":         request.GSTIN,
			"address":       request.Address,
			"city":          request.City,
			"state":         request.State,
			"pincode":       request.Pincode,
			"updated_at":    now,
		}
		if err := tx.Model(&models.CPO{}).
			Where("id = ?", cpoID).
			Updates(updates).Error; err != nil {
			return mapWriteError(err, "update CPO profile")
		}
		record.BusinessName = request.BusinessName
		record.CompanyType = request.CompanyType
		record.GSTIN = request.GSTIN
		record.Address = request.Address
		record.City = request.City
		record.State = request.State
		record.Pincode = request.Pincode
		record.UpdatedAt = now
		if err := writeAudit(
			tx,
			principal.UserID,
			cpoID,
			"CPO_PROFILE_UPDATED",
			models.JSONB{},
			now,
		); err != nil {
			return err
		}
		return service.emit(
			tx,
			principal.UserID,
			cpoID,
			"platform.cpo.profile_updated",
			models.JSONB{},
		)
	})
	if err != nil {
		return View{}, err
	}
	return view(record), nil
}

func (service *Service) Activate(
	ctx context.Context,
	principal auth.Principal,
	cpoID uuid.UUID,
	request LifecycleRequest,
) (View, error) {
	return service.transitionStatus(
		ctx,
		principal,
		cpoID,
		constants.CPOStatusActive,
		request.Reason,
	)
}

func (service *Service) Suspend(
	ctx context.Context,
	principal auth.Principal,
	cpoID uuid.UUID,
	request LifecycleRequest,
) (View, error) {
	return service.transitionStatus(
		ctx,
		principal,
		cpoID,
		constants.CPOStatusSuspended,
		request.Reason,
	)
}

func (service *Service) SetLiveAppID(
	ctx context.Context,
	principal auth.Principal,
	cpoID uuid.UUID,
	request SetAppIDRequest,
) (View, error) {
	if err := requirePlatform(principal); err != nil {
		return View{}, err
	}
	appID := strings.ToLower(strings.TrimSpace(request.AppID))
	if !appIDPattern.MatchString(appID) || strings.HasPrefix(appID, dummyAppIDPrefix) {
		return View{}, &auth.APIError{
			Status:  http.StatusBadRequest,
			Code:    "invalid_cpo_app_id",
			Message: "The live app ID must be 16 to 100 lowercase URL-safe characters and cannot use the dummy prefix.",
		}
	}
	var record models.CPO
	err := service.database.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			First(&record, "id = ?", cpoID).Error; err != nil {
			return mapNotFound(err)
		}
		now := service.now()
		if err := tx.Model(&models.CPO{}).
			Where("id = ?", cpoID).
			Updates(map[string]any{
				"app_id":            appID,
				"app_id_mode":       constants.CPOAppIDModeLive,
				"app_id_updated_at": now,
				"updated_at":        now,
			}).Error; err != nil {
			return mapWriteError(err, "set live CPO app ID")
		}
		record.AppID = appID
		record.AppIDMode = constants.CPOAppIDModeLive
		record.AppIDUpdatedAt = now
		record.UpdatedAt = now
		if err := writeAudit(
			tx,
			principal.UserID,
			cpoID,
			"CPO_APP_ID_SET_LIVE",
			models.JSONB{"app_id_mode": constants.CPOAppIDModeLive},
			now,
		); err != nil {
			return err
		}
		return service.emit(
			tx,
			principal.UserID,
			cpoID,
			"platform.cpo.app_id_rotated",
			models.JSONB{"app_id_mode": constants.CPOAppIDModeLive},
		)
	})
	if err != nil {
		return View{}, err
	}
	return view(record), nil
}

func (service *Service) GetPrimaryAdmin(
	ctx context.Context,
	principal auth.Principal,
	cpoID uuid.UUID,
) (PrimaryAdminView, error) {
	if err := requirePlatform(principal); err != nil {
		return PrimaryAdminView{}, err
	}
	if _, err := service.find(ctx, cpoID); err != nil {
		return PrimaryAdminView{}, err
	}
	return service.primaryAdminView(service.database.WithContext(ctx), cpoID)
}

func (service *Service) SetPrimaryAdmin(
	ctx context.Context,
	principal auth.Principal,
	cpoID uuid.UUID,
	request PrimaryAdminRequest,
) (PrimaryAdminView, error) {
	if err := requirePlatform(principal); err != nil {
		return PrimaryAdminView{}, err
	}
	if !service.mailEnabled {
		return PrimaryAdminView{}, &auth.APIError{
			Status:  http.StatusServiceUnavailable,
			Code:    "mail_unavailable",
			Message: "CPO administrator onboarding email is unavailable.",
		}
	}
	request.Email = strings.ToLower(strings.TrimSpace(request.Email))
	request.FullName = normalizeCPOText(request.FullName)
	request.Reason = strings.TrimSpace(request.Reason)
	if !validEmail(request.Email) {
		return PrimaryAdminView{}, invalid(
			"email",
			"Primary administrator email is invalid.",
		)
	}
	if !validPersonName(request.FullName) {
		return PrimaryAdminView{}, invalid(
			"full_name",
			"Full name is required and must not exceed 255 characters.",
		)
	}
	if err := validateReason(request.Reason); err != nil {
		return PrimaryAdminView{}, err
	}

	now := service.now()
	err := service.database.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec(
			"SELECT pg_advisory_xact_lock(hashtextextended(?, 0))",
			"cpo-primary-admin:"+cpoID.String(),
		).Error; err != nil {
			return fmt.Errorf("serialize CPO primary administrator: %w", err)
		}
		var cpoRecord models.CPO
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			First(&cpoRecord, "id = ?", cpoID).Error; err != nil {
			return mapNotFound(err)
		}
		if err := tx.Exec(
			"SELECT pg_advisory_xact_lock(hashtextextended(?, 0))",
			"cpo-admin:"+request.Email,
		).Error; err != nil {
			return fmt.Errorf("serialize replacement administrator identity: %w", err)
		}

		var currentMembership models.CPOMembership
		currentResult := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("cpo_id = ? AND is_primary_admin", cpoID).
			First(&currentMembership)
		currentFound := currentResult.Error == nil
		if currentResult.Error != nil &&
			!errors.Is(currentResult.Error, gorm.ErrRecordNotFound) {
			return fmt.Errorf("load current primary administrator: %w", currentResult.Error)
		}

		var targetUser models.User
		identityCreated := false
		temporaryPassword := ""
		userResult := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("lower(btrim(email)) = ?", request.Email).
			First(&targetUser)
		switch {
		case userResult.Error == nil:
			if !targetUser.IsActive {
				return &auth.APIError{
					Status:  http.StatusConflict,
					Code:    "admin_identity_inactive",
					Message: "The administrator identity exists but is inactive.",
				}
			}
		case errors.Is(userResult.Error, gorm.ErrRecordNotFound):
			var err error
			temporaryPassword, err = generateTemporaryPassword()
			if err != nil {
				return err
			}
			passwordHash, err := security.HashPassword(temporaryPassword)
			if err != nil {
				return fmt.Errorf("hash replacement administrator password: %w", err)
			}
			targetUser = models.User{
				ID:                 uuid.New(),
				Email:              request.Email,
				PasswordHash:       passwordHash,
				FullName:           request.FullName,
				IsActive:           true,
				IsVerified:         false,
				MFAEnabled:         false,
				MustChangePassword: true,
				PasswordChangedAt:  now,
				CreatedAt:          now,
				UpdatedAt:          now,
			}
			if err := tx.Create(&targetUser).Error; err != nil {
				return mapWriteError(err, "create replacement administrator")
			}
			identityCreated = true
		default:
			return fmt.Errorf(
				"find replacement administrator identity: %w",
				userResult.Error,
			)
		}

		var targetMembership models.CPOMembership
		targetResult := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("cpo_id = ? AND user_id = ?", cpoID, targetUser.ID).
			First(&targetMembership)
		targetFound := targetResult.Error == nil
		if targetResult.Error != nil &&
			!errors.Is(targetResult.Error, gorm.ErrRecordNotFound) {
			return fmt.Errorf("load replacement administrator membership: %w", targetResult.Error)
		}

		samePrimary := currentFound && currentMembership.UserID == targetUser.ID
		membershipChanged := !samePrimary ||
			currentMembership.Status != constants.MembershipStatusActive ||
			currentMembership.Role != constants.CPORoleAdmin
		if samePrimary && !membershipChanged {
			return nil
		}

		var previousUserID *uuid.UUID
		if currentFound {
			currentUserID := currentMembership.UserID
			previousUserID = &currentUserID
			if !samePrimary {
				if err := tx.Model(&models.CPOMembership{}).
					Where("id = ?", currentMembership.ID).
					Updates(map[string]any{
						"is_primary_admin": false,
						"status":           constants.MembershipStatusRevoked,
						"updated_at":       now,
					}).Error; err != nil {
					return fmt.Errorf("retire previous primary administrator: %w", err)
				}
				scope := constants.AuthScopeCPO
				if _, err := revokeCPOSessions(
					tx,
					cpoID,
					&scope,
					"PRIMARY_ADMIN_REPLACED",
					now,
					&currentMembership.UserID,
				); err != nil {
					return err
				}
			}
		}

		role := constants.CPORoleAdmin
		if targetFound {
			if err := tx.Model(&models.CPOMembership{}).
				Where("id = ?", targetMembership.ID).
				Updates(map[string]any{
					"role":             role,
					"status":           constants.MembershipStatusActive,
					"is_primary_admin": true,
					"updated_at":       now,
				}).Error; err != nil {
				return mapWriteError(err, "restore primary administrator membership")
			}
		} else {
			targetMembership = models.CPOMembership{
				ID:             uuid.New(),
				CPOID:          cpoID,
				UserID:         targetUser.ID,
				Role:           role,
				Status:         constants.MembershipStatusActive,
				IsPrimaryAdmin: true,
				CreatedAt:      now,
				UpdatedAt:      now,
			}
			if err := tx.Create(&targetMembership).Error; err != nil {
				return mapWriteError(err, "create replacement administrator membership")
			}
		}

		template := "CPO_STAFF_EXISTING_IDENTITY"
		actionURL, err := service.cpoOnboardingActionURL(cpoID)
		if err != nil {
			return err
		}
		payload := cmsmail.MessagePayload{
			RecipientName: targetUser.FullName,
			CPOName:       cpoRecord.BusinessName,
			CPOID:         cpoRecord.ID.String(),
			CPOAppID:      cpoRecord.AppID,
			Role:          string(role),
			ActionURL:     actionURL,
		}
		if identityCreated {
			template = "CPO_STAFF_NEW_IDENTITY"
			payload.TemporaryPassword = temporaryPassword
		}
		if samePrimary {
			template = "CPO_ONBOARDING_RESENT"
		}
		if err := service.outbox.EnqueueMessageWithContext(
			tx,
			targetUser.Email,
			template,
			payload,
			cmsmail.MessageContext{
				CPOID:  &cpoID,
				UserID: &targetUser.ID,
			},
		); err != nil {
			return err
		}

		action := "CPO_PRIMARY_ADMIN_REPLACED"
		changeType := "REPLACED"
		if samePrimary {
			action = "CPO_PRIMARY_ADMIN_RESTORED"
			changeType = "RESTORED"
		}
		details := models.JSONB{
			"new_user_id":      targetUser.ID,
			"identity_created": identityCreated,
			"change_type":      changeType,
			"reason":           request.Reason,
		}
		if previousUserID != nil {
			details["previous_user_id"] = *previousUserID
		}
		if err := writeAudit(
			tx,
			principal.UserID,
			cpoID,
			action,
			details,
			now,
		); err != nil {
			return err
		}
		return service.emit(
			tx,
			principal.UserID,
			cpoID,
			"platform.cpo.primary_admin_changed",
			details,
		)
	})
	if err != nil {
		return PrimaryAdminView{}, err
	}
	return service.primaryAdminView(service.database.WithContext(ctx), cpoID)
}

func (service *Service) ResendPrimaryAdminOnboarding(
	ctx context.Context,
	principal auth.Principal,
	cpoID uuid.UUID,
	request ReasonRequest,
) (PrimaryAdminView, error) {
	if err := requirePlatform(principal); err != nil {
		return PrimaryAdminView{}, err
	}
	if !service.mailEnabled {
		return PrimaryAdminView{}, &auth.APIError{
			Status:  http.StatusServiceUnavailable,
			Code:    "mail_unavailable",
			Message: "CPO administrator onboarding email is unavailable.",
		}
	}
	request.Reason = strings.TrimSpace(request.Reason)
	if err := validateReason(request.Reason); err != nil {
		return PrimaryAdminView{}, err
	}
	now := service.now()
	err := service.database.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var cpoRecord models.CPO
		if err := tx.Clauses(clause.Locking{Strength: "SHARE"}).
			First(&cpoRecord, "id = ?", cpoID).Error; err != nil {
			return mapNotFound(err)
		}
		membership, user, err := loadPrimaryAdmin(tx, cpoID, true)
		if err != nil {
			return err
		}
		if !user.IsActive ||
			membership.Status != constants.MembershipStatusActive {
			return &auth.APIError{
				Status:  http.StatusConflict,
				Code:    "primary_admin_unavailable",
				Message: "The primary administrator must be active before onboarding can be resent.",
			}
		}
		actionURL, err := service.cpoOnboardingActionURL(cpoID)
		if err != nil {
			return err
		}
		if err := service.outbox.EnqueueMessageWithContext(
			tx,
			user.Email,
			"CPO_ONBOARDING_RESENT",
			cmsmail.MessagePayload{
				RecipientName: user.FullName,
				CPOName:       cpoRecord.BusinessName,
				CPOID:         cpoRecord.ID.String(),
				CPOAppID:      cpoRecord.AppID,
				ActionURL:     actionURL,
			},
			cmsmail.MessageContext{
				CPOID:  &cpoID,
				UserID: &user.ID,
			},
		); err != nil {
			return err
		}
		details := models.JSONB{
			"primary_admin_user_id": user.ID,
			"reason":                request.Reason,
		}
		if err := writeAudit(
			tx,
			principal.UserID,
			cpoID,
			"CPO_PRIMARY_ADMIN_ONBOARDING_RESENT",
			details,
			now,
		); err != nil {
			return err
		}
		return service.emit(
			tx,
			principal.UserID,
			cpoID,
			"platform.cpo.primary_admin_onboarding_resent",
			details,
		)
	})
	if err != nil {
		return PrimaryAdminView{}, err
	}
	return service.primaryAdminView(service.database.WithContext(ctx), cpoID)
}

func (service *Service) RevokeAdministrativeSessions(
	ctx context.Context,
	principal auth.Principal,
	cpoID uuid.UUID,
	request ReasonRequest,
) (SessionRevocationResponse, error) {
	if err := requirePlatform(principal); err != nil {
		return SessionRevocationResponse{}, err
	}
	request.Reason = strings.TrimSpace(request.Reason)
	if err := validateReason(request.Reason); err != nil {
		return SessionRevocationResponse{}, err
	}
	var counts sessionRevocationCounts
	now := service.now()
	err := service.database.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var cpoRecord models.CPO
		if err := tx.Clauses(clause.Locking{Strength: "SHARE"}).
			First(&cpoRecord, "id = ?", cpoID).Error; err != nil {
			return mapNotFound(err)
		}
		scope := constants.AuthScopeCPO
		var err error
		counts, err = revokeCPOSessions(
			tx,
			cpoID,
			&scope,
			"PLATFORM_CPO_ADMIN_REVOKED",
			now,
			nil,
		)
		if err != nil {
			return err
		}
		details := models.JSONB{
			"reason":                 request.Reason,
			"revoked_sessions":       counts.sessions,
			"revoked_refresh_tokens": counts.refreshTokens,
		}
		if err := writeAudit(
			tx,
			principal.UserID,
			cpoID,
			"CPO_ADMIN_SESSIONS_REVOKED",
			details,
			now,
		); err != nil {
			return err
		}
		return service.emit(
			tx,
			principal.UserID,
			cpoID,
			"platform.cpo.admin_sessions_revoked",
			details,
		)
	})
	if err != nil {
		return SessionRevocationResponse{}, err
	}
	return SessionRevocationResponse{
		RevokedSessions:      counts.sessions,
		RevokedRefreshTokens: counts.refreshTokens,
	}, nil
}

func (service *Service) transitionStatus(
	ctx context.Context,
	principal auth.Principal,
	cpoID uuid.UUID,
	status constants.CPOStatus,
	reason string,
) (View, error) {
	if err := requirePlatform(principal); err != nil {
		return View{}, err
	}
	reason = strings.TrimSpace(reason)
	if err := validateReason(reason); err != nil {
		return View{}, err
	}
	var record models.CPO
	err := service.database.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			First(&record, "id = ?", cpoID).Error; err != nil {
			return mapNotFound(err)
		}
		now := service.now()
		changed := record.Status != status
		previousStatus := record.Status
		if changed {
			if err := tx.Model(&models.CPO{}).
				Where("id = ?", cpoID).
				Updates(map[string]any{
					"status":                    status,
					"status_reason":             reason,
					"status_changed_at":         now,
					"status_changed_by_user_id": principal.UserID,
					"updated_at":                now,
				}).Error; err != nil {
				return fmt.Errorf("update CPO status: %w", err)
			}
			record.Status = status
			record.StatusReason = reason
			record.StatusChangedAt = now
			statusActorID := principal.UserID
			record.StatusChangedByUserID = &statusActorID
			record.UpdatedAt = now
		}
		if status == constants.CPOStatusSuspended {
			if _, err := revokeCPOSessions(
				tx,
				cpoID,
				nil,
				"CPO_SUSPENDED",
				now,
				nil,
			); err != nil {
				return err
			}
		}
		if !changed {
			return nil
		}
		if err := writeAudit(
			tx,
			principal.UserID,
			cpoID,
			"CPO_STATUS_"+string(status),
			models.JSONB{
				"previous_status": previousStatus,
				"status":          status,
				"reason":          reason,
			},
			now,
		); err != nil {
			return err
		}
		eventType := "platform.cpo.activated"
		if status == constants.CPOStatusSuspended {
			eventType = "platform.cpo.suspended"
		}
		return service.emit(
			tx,
			principal.UserID,
			cpoID,
			eventType,
			models.JSONB{
				"previous_status": previousStatus,
				"status":          status,
				"reason":          reason,
			},
		)
	})
	if err != nil {
		return View{}, err
	}
	return view(record), nil
}

func loadPrimaryAdmin(
	database *gorm.DB,
	cpoID uuid.UUID,
	lock bool,
) (models.CPOMembership, models.User, error) {
	membershipQuery := database.Where(
		"cpo_id = ? AND is_primary_admin",
		cpoID,
	)
	if lock {
		membershipQuery = membershipQuery.Clauses(
			clause.Locking{Strength: "UPDATE"},
		)
	}
	var membership models.CPOMembership
	if err := membershipQuery.First(&membership).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return models.CPOMembership{}, models.User{}, &auth.APIError{
				Status:  http.StatusNotFound,
				Code:    "primary_admin_not_found",
				Message: "The CPO does not have a primary administrator.",
			}
		}
		return models.CPOMembership{}, models.User{},
			fmt.Errorf("load primary administrator membership: %w", err)
	}

	userQuery := database.Where("id = ?", membership.UserID)
	if lock {
		userQuery = userQuery.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	var user models.User
	if err := userQuery.First(&user).Error; err != nil {
		return models.CPOMembership{}, models.User{},
			fmt.Errorf("load primary administrator identity: %w", err)
	}
	return membership, user, nil
}

func (service *Service) primaryAdminView(
	database *gorm.DB,
	cpoID uuid.UUID,
) (PrimaryAdminView, error) {
	membership, user, err := loadPrimaryAdmin(database, cpoID, false)
	if err != nil {
		return PrimaryAdminView{}, err
	}
	var latest models.MailOutbox
	mailResult := database.
		Where(
			"cpo_id = ? AND user_id = ? AND template IN ?",
			cpoID,
			user.ID,
			[]string{
				"CPO_STAFF_NEW_IDENTITY",
				"CPO_STAFF_EXISTING_IDENTITY",
				"CPO_ONBOARDING_RESENT",
			},
		).
		Order("created_at DESC, id DESC").
		First(&latest)
	var delivery *OnboardingDeliveryView
	switch {
	case mailResult.Error == nil:
		delivery = &OnboardingDeliveryView{
			JobID:     latest.ID,
			Template:  latest.Template,
			Status:    latest.Status,
			Attempts:  latest.Attempts,
			SentAt:    latest.SentAt,
			CreatedAt: latest.CreatedAt,
			UpdatedAt: latest.UpdatedAt,
		}
	case errors.Is(mailResult.Error, gorm.ErrRecordNotFound):
	default:
		return PrimaryAdminView{}, fmt.Errorf(
			"load primary administrator onboarding delivery: %w",
			mailResult.Error,
		)
	}
	return PrimaryAdminView{
		UserID:                   user.ID,
		Email:                    user.Email,
		FullName:                 user.FullName,
		Role:                     membership.Role,
		MembershipStatus:         membership.Status,
		IdentityActive:           user.IsActive,
		IdentityVerified:         user.IsVerified,
		MustChangePassword:       user.MustChangePassword,
		LastLoginAt:              user.LastLoginAt,
		LatestOnboardingDelivery: delivery,
	}, nil
}

func revokeCPOSessions(
	tx *gorm.DB,
	cpoID uuid.UUID,
	scope *constants.AuthScope,
	revokeReason string,
	now time.Time,
	userID *uuid.UUID,
) (sessionRevocationCounts, error) {
	counts := sessionRevocationCounts{}
	revokeCustomers := userID == nil && (scope == nil || *scope == constants.AuthScopeCustomer)
	if revokeCustomers {
		customerSessionIDs := tx.Model(&models.CustomerAuthSession{}).
			Select("id").Where("cpo_id = ? AND revoked_at IS NULL", cpoID)
		customerRefreshResult := tx.Model(&models.CustomerAuthRefreshToken{}).
			Where("used_at IS NULL AND revoked_at IS NULL").
			Where("session_id IN (?)", customerSessionIDs).
			Update("revoked_at", now)
		if customerRefreshResult.Error != nil {
			return counts, fmt.Errorf("revoke CPO customer refresh tokens: %w", customerRefreshResult.Error)
		}
		customerSessionResult := tx.Model(&models.CustomerAuthSession{}).
			Where("cpo_id = ? AND revoked_at IS NULL", cpoID).
			Updates(map[string]any{"revoked_at": now, "revoke_reason": revokeReason})
		if customerSessionResult.Error != nil {
			return counts, fmt.Errorf("revoke CPO customer sessions: %w", customerSessionResult.Error)
		}
		counts.sessions += customerSessionResult.RowsAffected
		counts.refreshTokens += customerRefreshResult.RowsAffected
	}
	if scope != nil && *scope == constants.AuthScopeCustomer {
		return counts, nil
	}
	sessionIDs := tx.Model(&models.AuthSession{}).
		Select("id").
		Where("cpo_id = ? AND revoked_at IS NULL", cpoID)
	if scope != nil {
		sessionIDs = sessionIDs.Where("scope = ?", *scope)
	}
	if userID != nil {
		sessionIDs = sessionIDs.Where("user_id = ?", *userID)
	}
	refreshResult := tx.Model(&models.AuthRefreshToken{}).
		Where("used_at IS NULL AND revoked_at IS NULL").
		Where("session_id IN (?)", sessionIDs).
		Update("revoked_at", now)
	if refreshResult.Error != nil {
		return counts, fmt.Errorf(
			"revoke CPO refresh tokens: %w",
			refreshResult.Error,
		)
	}

	sessionQuery := tx.Model(&models.AuthSession{}).
		Where("cpo_id = ? AND revoked_at IS NULL", cpoID)
	if scope != nil {
		sessionQuery = sessionQuery.Where("scope = ?", *scope)
	}
	if userID != nil {
		sessionQuery = sessionQuery.Where("user_id = ?", *userID)
	}
	sessionResult := sessionQuery.Updates(map[string]any{
		"revoked_at":    now,
		"revoke_reason": revokeReason,
	})
	if sessionResult.Error != nil {
		return counts, fmt.Errorf(
			"revoke CPO sessions: %w",
			sessionResult.Error,
		)
	}
	counts.sessions += sessionResult.RowsAffected
	counts.refreshTokens += refreshResult.RowsAffected
	return counts, nil
}

func (service *Service) emit(
	tx *gorm.DB,
	actorUserID uuid.UUID,
	cpoID uuid.UUID,
	eventType string,
	data models.JSONB,
) error {
	if service.events == nil {
		return nil
	}
	resourceID := cpoID.String()
	_, err := service.events.Emit(tx, platformops.EventInput{
		Type:         eventType,
		ActorUserID:  &actorUserID,
		ResourceType: "CPO",
		ResourceID:   &resourceID,
		Data:         data,
	})
	return err
}

func (service *Service) find(ctx context.Context, cpoID uuid.UUID) (models.CPO, error) {
	var record models.CPO
	if err := service.database.WithContext(ctx).First(&record, "id = ?", cpoID).Error; err != nil {
		return models.CPO{}, mapNotFound(err)
	}
	return record, nil
}

func normalizeCreateRequest(request CreateRequest) CreateRequest {
	request.Slug = normalizeSlug(request.Slug)
	request.BusinessName = normalizeCPOText(request.BusinessName)
	request.GSTIN = strings.ToUpper(strings.TrimSpace(request.GSTIN))
	request.Address = normalizeCPOText(request.Address)
	request.City = normalizeCPOText(request.City)
	request.State = constants.IndianState(strings.TrimSpace(string(request.State)))
	request.Pincode = strings.TrimSpace(request.Pincode)
	request.Admin.Email = strings.ToLower(strings.TrimSpace(request.Admin.Email))
	request.Admin.FullName = normalizeCPOText(request.Admin.FullName)
	return request
}

func normalizeProfileRequest(request UpdateProfileRequest) UpdateProfileRequest {
	request.BusinessName = normalizeCPOText(request.BusinessName)
	request.GSTIN = strings.ToUpper(strings.TrimSpace(request.GSTIN))
	request.Address = normalizeCPOText(request.Address)
	request.City = normalizeCPOText(request.City)
	request.State = constants.IndianState(strings.TrimSpace(string(request.State)))
	request.Pincode = strings.TrimSpace(request.Pincode)
	return request
}

func validateProfileRequest(request UpdateProfileRequest) error {
	if !validBusinessName(request.BusinessName) {
		return invalid(
			"business_name",
			"Business name is required and must not exceed 255 characters.",
		)
	}
	if !request.CompanyType.Valid() {
		return invalid(
			"company_type",
			"Company type must be INDIVIDUAL or COMPANY.",
		)
	}
	if !validAddress(request.Address) {
		return invalid("address", "Address is required and must not exceed 5000 characters.")
	}
	if !validCity(request.City) {
		return invalid("city", "City is required and must not exceed 100 characters.")
	}
	if !request.State.Valid() {
		return invalid("state", "Invalid state.")
	}
	if err := validateGSTIN(request.GSTIN, request.State); err != nil {
		return err
	}
	if !pincodePattern.MatchString(request.Pincode) {
		return invalid("pincode", "Pincode must be a valid six-digit Indian PIN code.")
	}
	return nil
}

func validateReason(reason string) error {
	length := len(strings.TrimSpace(reason))
	if length < minReasonLength || length > maxReasonLength {
		return invalid(
			"reason",
			"Reason must contain 3 to 500 characters.",
		)
	}
	return nil
}

func validateCreateRequest(request CreateRequest) error {
	if err := validateSlug(request.Slug); err != nil {
		return err
	}
	if !validBusinessName(request.BusinessName) {
		return invalid("business_name", "Business name is required and must not exceed 255 characters.")
	}
	if !request.CompanyType.Valid() {
		return invalid("company_type", "Company type must be INDIVIDUAL or COMPANY.")
	}
	if !validAddress(request.Address) {
		return invalid("address", "Address is required and must not exceed 5000 characters.")
	}
	if !validCity(request.City) {
		return invalid("city", "City is required and must not exceed 100 characters.")
	}
	if !request.State.Valid() {
		return invalid("state", "Invalid state.")
	}
	if err := validateGSTIN(request.GSTIN, request.State); err != nil {
		return err
	}
	if !pincodePattern.MatchString(request.Pincode) {
		return invalid("pincode", "Pincode must be a valid six-digit Indian PIN code.")
	}
	if !validEmail(request.Admin.Email) {
		return invalid("admin.email", "Administrator email is invalid.")
	}
	if !validPersonName(request.Admin.FullName) {
		return invalid("admin.full_name", "Administrator full name is required and must not exceed 255 characters.")
	}
	return nil
}

func normalizeSlug(candidate string) string {
	return strings.ToLower(strings.TrimSpace(candidate))
}

func validateSlug(slug string) error {
	if len(slug) > 80 || !slugPattern.MatchString(slug) {
		return invalid("slug", "Slug must contain lowercase words separated by single hyphens.")
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

func generateTemporaryPassword() (string, error) {
	token, err := security.RandomToken(18)
	if err != nil {
		return "", err
	}
	return "Tmp-" + token, nil
}

func requirePlatform(principal auth.Principal) error {
	if principal.Scope != constants.AuthScopePlatform {
		return &auth.APIError{
			Status:  http.StatusForbidden,
			Code:    "forbidden",
			Message: "Platform superadmin access is required.",
		}
	}
	return nil
}

func invalid(field, message string) error {
	return &auth.APIError{
		Status:  http.StatusBadRequest,
		Code:    "invalid_" + strings.ReplaceAll(field, ".", "_"),
		Message: message,
	}
}

func mapNotFound(err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return &auth.APIError{
			Status:  http.StatusNotFound,
			Code:    "cpo_not_found",
			Message: "The CPO was not found.",
		}
	}
	return fmt.Errorf("load CPO: %w", err)
}

func mapWriteError(err error, operation string) error {
	var postgresError *pgconn.PgError
	if errors.As(err, &postgresError) && postgresError.Code == "23514" {
		switch postgresError.ConstraintName {
		case "chk_cpos_gstin":
			return invalid("gstin", "GSTIN must be a valid 15-character Indian GSTIN with a valid checksum.")
		case "chk_cpos_gstin_state_matches":
			return invalid("gstin.state_mismatch", "GSTIN state code must match the CPO registration state.")
		case "chk_cpos_pincode_format":
			return invalid("pincode", "Pincode must be a valid six-digit Indian PIN code.")
		}
	}
	if errors.As(err, &postgresError) && postgresError.Code == "23505" {
		switch postgresError.ConstraintName {
		case "uq_cpos_slug_normalized":
			return &auth.APIError{
				Status:  http.StatusConflict,
				Code:    "cpo_slug_conflict",
				Message: "The CPO slug is already in use.",
			}
		case "uq_cpos_gstin_normalized":
			return &auth.APIError{
				Status:  http.StatusConflict,
				Code:    "cpo_gstin_conflict",
				Message: "The GSTIN is already assigned to another CPO.",
			}
		case "uq_cpos_app_id":
			return &auth.APIError{
				Status:  http.StatusConflict,
				Code:    "cpo_app_id_conflict",
				Message: "The CPO app ID is already in use.",
			}
		case "uq_users_email_normalized":
			return &auth.APIError{
				Status:  http.StatusConflict,
				Code:    "admin_identity_conflict",
				Message: "An administrator identity with this email was created concurrently. Retry the request.",
			}
		case "uq_cpo_membership":
			return &auth.APIError{
				Status:  http.StatusConflict,
				Code:    "cpo_admin_membership_conflict",
				Message: "The administrator already has a membership for this CPO.",
			}
		case "uq_cpo_memberships_primary_admin":
			return &auth.APIError{
				Status:  http.StatusConflict,
				Code:    "cpo_primary_admin_conflict",
				Message: "The CPO already has a primary administrator.",
			}
		}
		return &auth.APIError{
			Status:  http.StatusConflict,
			Code:    "cpo_conflict",
			Message: "The CPO slug, GSTIN, app ID, or administrator membership already exists.",
		}
	}
	return fmt.Errorf("%s: %w", operation, err)
}

func writeAudit(
	tx *gorm.DB,
	actorID uuid.UUID,
	cpoID uuid.UUID,
	action string,
	details models.JSONB,
	now time.Time,
) error {
	record := models.AuditLog{
		ID:        uuid.New(),
		CPOID:     &cpoID,
		UserID:    &actorID,
		Action:    action,
		Entity:    "CPO",
		EntityID:  &cpoID,
		Details:   details,
		CreatedAt: now,
	}
	if err := tx.Create(&record).Error; err != nil {
		return fmt.Errorf("audit CPO operation: %w", err)
	}
	return nil
}

func view(record models.CPO) View {
	return View{
		ID:                    record.ID,
		Slug:                  record.Slug,
		BusinessName:          record.BusinessName,
		CompanyType:           record.CompanyType,
		GSTIN:                 record.GSTIN,
		Address:               record.Address,
		City:                  record.City,
		State:                 record.State,
		Pincode:               record.Pincode,
		Status:                record.Status,
		StatusReason:          record.StatusReason,
		StatusChangedAt:       record.StatusChangedAt,
		StatusChangedByUserID: record.StatusChangedByUserID,
		AppID:                 record.AppID,
		AppIDMode:             record.AppIDMode,
		AppIDUpdatedAt:        record.AppIDUpdatedAt,
		CreatedAt:             record.CreatedAt,
		UpdatedAt:             record.UpdatedAt,
	}
}

func trimOptionalString(value *string) *string {
	if value == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*value)
	return &trimmed
}

var chargerIDPattern = regexp.MustCompile(`^[a-z0-9]{6}$`)

func (service *Service) GetAdminProfile(
	ctx context.Context,
	principal auth.Principal,
) (AdminProfileView, error) {
	if err := requireCPOContext(principal); err != nil {
		return AdminProfileView{}, err
	}
	var user models.User
	if err := service.database.WithContext(ctx).
		First(&user, "id = ? AND is_active = true", principal.UserID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return AdminProfileView{}, &auth.APIError{
				Status:  http.StatusUnauthorized,
				Code:    "unauthorized",
				Message: "The authenticated identity is no longer active.",
			}
		}
		return AdminProfileView{}, fmt.Errorf("load CPO administrator profile: %w", err)
	}
	access, err := auth.EvaluateCPOAccess(ctx, service.database, principal)
	if err != nil {
		return AdminProfileView{}, forbiddenCPOAccess()
	}
	return adminProfileView(user, *principal.CPOID, access.Membership.Role), nil
}

func (service *Service) UpdateAdminProfile(
	ctx context.Context,
	principal auth.Principal,
	request UpdateAdminProfileRequest,
) (AdminProfileView, error) {
	if err := requireCPOContext(principal); err != nil {
		return AdminProfileView{}, err
	}
	request.FullName = trimOptionalString(request.FullName)
	request.Phone = trimOptionalString(request.Phone)
	if request.FullName == nil && request.Phone == nil {
		return AdminProfileView{}, invalid(
			"admin_profile",
			"At least one administrator profile field must be supplied.",
		)
	}
	if request.FullName != nil && (*request.FullName == "" || len(*request.FullName) > 255) {
		return AdminProfileView{}, invalid(
			"full_name",
			"Full name is required and must not exceed 255 characters.",
		)
	}
	if request.Phone != nil && len(*request.Phone) > 32 {
		return AdminProfileView{}, invalid("phone", "Phone must not exceed 32 characters.")
	}

	var user models.User
	cpoID := *principal.CPOID
	err := service.database.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			First(&user, "id = ? AND is_active = true", principal.UserID).Error; err != nil {
			return err
		}
		updates := map[string]any{}
		changedFields := make([]string, 0, 2)
		if request.FullName != nil {
			updates["full_name"] = *request.FullName
			user.FullName = *request.FullName
			changedFields = append(changedFields, "full_name")
		}
		if request.Phone != nil {
			if *request.Phone == "" {
				updates["phone"] = nil
				user.Phone = nil
			} else {
				updates["phone"] = *request.Phone
				user.Phone = request.Phone
			}
			changedFields = append(changedFields, "phone")
		}
		now := service.now()
		updates["updated_at"] = now
		user.UpdatedAt = now
		if err := tx.Model(&models.User{}).
			Where("id = ?", user.ID).
			Updates(updates).Error; err != nil {
			return fmt.Errorf("update CPO administrator profile: %w", err)
		}
		return writeAudit(
			tx,
			principal.UserID,
			cpoID,
			"CPO_ADMIN_PROFILE_UPDATED",
			models.JSONB{"changed_fields": changedFields},
			now,
		)
	})
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return AdminProfileView{}, &auth.APIError{
				Status:  http.StatusUnauthorized,
				Code:    "unauthorized",
				Message: "The authenticated identity is no longer active.",
			}
		}
		return AdminProfileView{}, err
	}
	access, err := auth.EvaluateCPOAccess(ctx, service.database, principal)
	if err != nil {
		return AdminProfileView{}, forbiddenCPOAccess()
	}
	return adminProfileView(user, cpoID, access.Membership.Role), nil
}

func adminProfileView(user models.User, cpoID uuid.UUID, role constants.CPORole) AdminProfileView {
	return AdminProfileView{
		UserID:     user.ID,
		CPOID:      cpoID,
		Email:      user.Email,
		FullName:   user.FullName,
		Phone:      user.Phone,
		Role:       role,
		IsVerified: user.IsVerified,
		CreatedAt:  user.CreatedAt,
		UpdatedAt:  user.UpdatedAt,
	}
}

func cpoUserView(
	user models.User,
	cpoID uuid.UUID,
	membership models.CPOMembership,
) CPOUserView {
	view := CPOUserView{
		ID:         user.ID,
		CPOID:      cpoID,
		Email:      user.Email,
		FullName:   user.FullName,
		Phone:      user.Phone,
		IsActive:   user.IsActive,
		IsVerified: user.IsVerified,
		CreatedAt:  user.CreatedAt,
		UpdatedAt:  user.UpdatedAt,
	}
	role := membership.Role
	view.Role = &role
	status := membership.Status
	view.MembershipStatus = &status
	return view
}

func (service *Service) GetUser(
	ctx context.Context,
	principal auth.Principal,
	userID uuid.UUID,
) (CPOUserView, error) {
	if err := requireCPOContext(principal); err != nil {
		return CPOUserView{}, err
	}

	cpoID := *principal.CPOID
	var membership models.CPOMembership
	membershipErr := service.database.WithContext(ctx).
		Where("cpo_id = ? AND user_id = ?", cpoID, userID).
		First(&membership).Error
	if membershipErr != nil && !errors.Is(membershipErr, gorm.ErrRecordNotFound) {
		return CPOUserView{}, fmt.Errorf("load CPO membership: %w", membershipErr)
	}

	if membershipErr != nil {
		return CPOUserView{}, &auth.APIError{
			Status:  http.StatusNotFound,
			Code:    "user_not_found",
			Message: "The user was not found for this CPO.",
		}
	}

	var user models.User
	if err := service.database.WithContext(ctx).
		Where("id = ?", userID).
		Where(`EXISTS (
            SELECT 1 FROM cpo_memberships
            WHERE cpo_id = ? AND user_id = users.id
	        )`, cpoID).
		First(&user).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return CPOUserView{}, &auth.APIError{
				Status:  http.StatusNotFound,
				Code:    "user_not_found",
				Message: "The user was not found for this CPO.",
			}
		}
		return CPOUserView{}, fmt.Errorf("load CPO user: %w", err)
	}

	return cpoUserView(user, cpoID, membership), nil
}

func (service *Service) PermissionCatalog(
	principal auth.Principal,
) (PermissionCatalogResponse, error) {
	if err := requireCPOContext(principal); err != nil {
		return PermissionCatalogResponse{}, err
	}
	catalog := cpopermissions.Catalog()
	response := PermissionCatalogResponse{Permissions: make([]PermissionDefinitionView, 0, len(catalog))}
	for _, permission := range catalog {
		response.Permissions = append(response.Permissions, PermissionDefinitionView{
			Key: permission.Key, Module: permission.Module, Name: permission.Name, Description: permission.Description,
		})
	}
	return response, nil
}

// AccessMe reports the active membership and exact access inputs used by the
// shared evaluator. It gives the frontend a durable recovery snapshot instead
// of requiring it to reproduce backend role rules.
func (service *Service) AccessMe(ctx context.Context, principal auth.Principal) (CPOAccessMeResponse, error) {
	access, err := auth.EvaluateCPOAccess(ctx, service.database, principal)
	if err != nil {
		return CPOAccessMeResponse{}, &auth.APIError{Status: http.StatusForbidden, Code: "forbidden", Message: "An active CPO membership is required."}
	}
	return CPOAccessMeResponse{
		MembershipID: access.Membership.ID, Role: access.Membership.Role,
		MembershipStatus: access.Membership.Status, IsPrimaryAdmin: access.Membership.IsPrimaryAdmin,
		RoleDefaults: access.RoleDefaults, Allow: access.Allow, Deny: access.Deny, Effective: access.Effective,
	}, nil
}

func (service *Service) ListStaff(ctx context.Context, principal auth.Principal) (StaffListResponse, error) {
	if err := requireCPOContext(principal); err != nil {
		return StaffListResponse{}, err
	}
	cpoID := *principal.CPOID
	var memberships []models.CPOMembership
	if err := service.database.WithContext(ctx).
		Preload("User").
		Where("cpo_id = ?", cpoID).
		Order("is_primary_admin DESC, created_at ASC, id ASC").
		Find(&memberships).Error; err != nil {
		return StaffListResponse{}, fmt.Errorf("list CPO staff: %w", err)
	}
	return service.staffViews(ctx, cpoID, memberships)
}

func (service *Service) GetStaff(ctx context.Context, principal auth.Principal, membershipID uuid.UUID) (StaffView, error) {
	if err := requireCPOContext(principal); err != nil {
		return StaffView{}, err
	}
	var membership models.CPOMembership
	if err := service.database.WithContext(ctx).Preload("User").
		Where("id = ? AND cpo_id = ?", membershipID, *principal.CPOID).First(&membership).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return StaffView{}, &auth.APIError{Status: http.StatusNotFound, Code: "staff_not_found", Message: "The staff membership was not found."}
		}
		return StaffView{}, fmt.Errorf("load CPO staff membership: %w", err)
	}
	response, err := service.staffViews(ctx, *principal.CPOID, []models.CPOMembership{membership})
	if err != nil {
		return StaffView{}, err
	}
	return response.Staff[0], nil
}

func (service *Service) CreateStaff(ctx context.Context, principal auth.Principal, request CreateStaffRequest) (StaffView, error) {
	if err := requireCPOContext(principal); err != nil {
		return StaffView{}, err
	}
	request.Email = strings.ToLower(strings.TrimSpace(request.Email))
	request.FullName = normalizeCPOText(request.FullName)
	if !validEmail(request.Email) || !validPersonName(request.FullName) {
		return StaffView{}, invalid("staff", "A valid email and full name are required.")
	}
	if request.Role != constants.CPORoleAdmin && request.Role != constants.CPORoleOperator && request.Role != constants.CPORoleViewer {
		return StaffView{}, invalid("role", "Staff role must be ADMIN, OPERATOR, or VIEWER.")
	}
	if err := validatePermissionOverrides(request.Overrides); err != nil {
		return StaffView{}, err
	}
	if err := service.requireDelegation(ctx, principal, request.Role, request.Overrides); err != nil {
		return StaffView{}, err
	}
	cpoID := *principal.CPOID
	var membership models.CPOMembership
	err := service.database.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("SELECT pg_advisory_xact_lock(hashtextextended(?, 0))", "cpo-staff:"+cpoID.String()+":"+request.Email).Error; err != nil {
			return fmt.Errorf("serialize CPO staff invite: %w", err)
		}
		var cpoRecord models.CPO
		if err := tx.First(&cpoRecord, "id = ?", cpoID).Error; err != nil {
			return mapNotFound(err)
		}
		var user models.User
		created := false
		temporaryPassword := ""
		result := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("lower(btrim(email)) = ?", request.Email).First(&user)
		switch {
		case result.Error == nil:
			if !user.IsActive {
				return &auth.APIError{Status: http.StatusConflict, Code: "staff_identity_inactive", Message: "The staff identity exists but is inactive."}
			}
		case errors.Is(result.Error, gorm.ErrRecordNotFound):
			var err error
			temporaryPassword, err = generateTemporaryPassword()
			if err != nil {
				return err
			}
			hash, err := security.HashPassword(temporaryPassword)
			if err != nil {
				return fmt.Errorf("hash staff temporary password: %w", err)
			}
			now := service.now()
			user = models.User{ID: uuid.New(), Email: request.Email, PasswordHash: hash, FullName: request.FullName, IsActive: true, MustChangePassword: true, PasswordChangedAt: now, CreatedAt: now, UpdatedAt: now}
			if err := tx.Create(&user).Error; err != nil {
				return mapWriteError(err, "create staff identity")
			}
			created = true
		default:
			return fmt.Errorf("find staff identity: %w", result.Error)
		}
		var count int64
		if err := tx.Model(&models.CPOMembership{}).Where("cpo_id = ? AND user_id = ?", cpoID, user.ID).Count(&count).Error; err != nil {
			return err
		}
		if count != 0 {
			return &auth.APIError{Status: http.StatusConflict, Code: "staff_membership_exists", Message: "The user already has a membership for this CPO."}
		}
		now := service.now()
		membership = models.CPOMembership{ID: uuid.New(), CPOID: cpoID, UserID: user.ID, Role: request.Role, Status: constants.MembershipStatusActive, CreatedAt: now, UpdatedAt: now, User: user}
		if err := tx.Create(&membership).Error; err != nil {
			return mapWriteError(err, "create staff membership")
		}
		if err := replacePermissionOverrides(tx, membership.ID, principal.UserID, request.Overrides, now); err != nil {
			return err
		}
		template := "CPO_STAFF_EXISTING_IDENTITY"
		actionURL, err := service.cpoOnboardingActionURL(cpoID)
		if err != nil {
			return err
		}
		payload := cmsmail.MessagePayload{RecipientName: user.FullName, CPOName: cpoRecord.BusinessName, CPOID: cpoID.String(), CPOAppID: cpoRecord.AppID, Role: string(request.Role), ActionURL: actionURL}
		if created {
			template = "CPO_STAFF_NEW_IDENTITY"
			payload.TemporaryPassword = temporaryPassword
		}
		if err := service.outbox.EnqueueMessageWithContext(tx, user.Email, template, payload, cmsmail.MessageContext{CPOID: &cpoID, UserID: &user.ID}); err != nil {
			return err
		}
		return writeAudit(tx, principal.UserID, cpoID, "CPO_STAFF_CREATED", models.JSONB{"membership_id": membership.ID, "role": request.Role, "identity_created": created}, now)
	})
	if err != nil {
		return StaffView{}, err
	}
	return service.GetStaff(ctx, principal, membership.ID)
}

func (service *Service) UpdateStaff(ctx context.Context, principal auth.Principal, membershipID uuid.UUID, request UpdateStaffRequest) (StaffView, error) {
	if err := requireCPOContext(principal); err != nil {
		return StaffView{}, err
	}
	if request.Role == nil && request.Overrides == nil {
		return StaffView{}, invalid("staff", "At least one staff field must be supplied.")
	}
	if request.Role != nil && (*request.Role != constants.CPORoleAdmin && *request.Role != constants.CPORoleOperator && *request.Role != constants.CPORoleViewer) {
		return StaffView{}, invalid("role", "Staff role must be ADMIN, OPERATOR, or VIEWER.")
	}
	if request.Overrides != nil {
		if err := validatePermissionOverrides(*request.Overrides); err != nil {
			return StaffView{}, err
		}
	}
	cpoID := *principal.CPOID
	err := service.database.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var membership models.CPOMembership
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND cpo_id = ?", membershipID, cpoID).First(&membership).Error; err != nil {
			return mapStaffNotFound(err)
		}
		if membership.IsPrimaryAdmin {
			return &auth.APIError{Status: http.StatusConflict, Code: "primary_admin_protected", Message: "Use the primary administrator workflow to change the primary administrator."}
		}
		if membership.UserID == principal.UserID {
			return &auth.APIError{Status: http.StatusConflict, Code: "staff_self_lockout_protected", Message: "Use another authorized staff member to change your own role or permissions."}
		}
		candidateRole := membership.Role
		if request.Role != nil {
			candidateRole = *request.Role
		}
		candidateOverrides := make([]MembershipPermissionOverrideRequest, 0)
		if request.Overrides != nil {
			candidateOverrides = *request.Overrides
		} else {
			var current []models.CPOMembershipPermissionOverride
			if err := tx.Where("membership_id = ?", membership.ID).Find(&current).Error; err != nil {
				return err
			}
			for _, override := range current {
				candidateOverrides = append(candidateOverrides, MembershipPermissionOverrideRequest{Permission: override.Permission, Effect: override.Effect})
			}
		}
		if err := service.requireDelegationWithDatabase(ctx, tx, principal, candidateRole, candidateOverrides); err != nil {
			return err
		}
		previousRole := membership.Role
		now := service.now()
		updates := map[string]any{"updated_at": now}
		if request.Role != nil {
			updates["role"] = *request.Role
		}
		if err := tx.Model(&models.CPOMembership{}).Where("id = ?", membership.ID).Updates(updates).Error; err != nil {
			return mapWriteError(err, "update staff membership")
		}
		if request.Overrides != nil {
			if err := replacePermissionOverrides(tx, membership.ID, principal.UserID, *request.Overrides, now); err != nil {
				return err
			}
		}
		if request.Role != nil && previousRole != *request.Role {
			var user models.User
			var cpoRecord models.CPO
			if err := tx.First(&user, "id = ?", membership.UserID).Error; err != nil {
				return err
			}
			if err := tx.First(&cpoRecord, "id = ?", cpoID).Error; err != nil {
				return err
			}
			actionURL, err := service.cpoOnboardingActionURL(cpoID)
			if err != nil {
				return err
			}
			if err := service.outbox.EnqueueMessageWithContext(tx, user.Email, "CPO_STAFF_ROLE_CHANGED", cmsmail.MessagePayload{RecipientName: user.FullName, CPOName: cpoRecord.BusinessName, Role: string(*request.Role), ActionURL: actionURL}, cmsmail.MessageContext{CPOID: &cpoID, UserID: &user.ID}); err != nil {
				return err
			}
		}
		return writeAudit(tx, principal.UserID, cpoID, "CPO_STAFF_UPDATED", models.JSONB{"membership_id": membership.ID}, now)
	})
	if err != nil {
		return StaffView{}, err
	}
	return service.GetStaff(ctx, principal, membershipID)
}

func (service *Service) requireDelegation(ctx context.Context, principal auth.Principal, role constants.CPORole, overrides []MembershipPermissionOverrideRequest) error {
	return service.requireDelegationWithDatabase(ctx, service.database, principal, role, overrides)
}

// requireDelegation prevents a member from creating authority they do not
// currently possess. Removing authority remains possible, while primary-admin
// safety is enforced at the membership mutation boundary.
func (service *Service) requireDelegationWithDatabase(ctx context.Context, database *gorm.DB, principal auth.Principal, role constants.CPORole, overrides []MembershipPermissionOverrideRequest) error {
	access, err := auth.EvaluateCPOAccess(ctx, database, principal)
	if err != nil {
		return &auth.APIError{Status: http.StatusForbidden, Code: "forbidden", Message: "An active CPO membership is required."}
	}
	has := make(map[string]bool, len(access.Effective))
	for _, permission := range access.Effective {
		has[permission] = true
	}
	for _, permission := range cpopermissions.RoleDefaults(role) {
		if !has[permission] {
			return &auth.APIError{Status: http.StatusForbidden, Code: "permission_delegation_denied", Message: "You cannot delegate a role capability you do not currently possess."}
		}
	}
	for _, override := range overrides {
		if strings.EqualFold(strings.TrimSpace(override.Effect), "ALLOW") && !has[strings.TrimSpace(override.Permission)] {
			return &auth.APIError{Status: http.StatusForbidden, Code: "permission_delegation_denied", Message: "You cannot grant a capability you do not currently possess."}
		}
	}
	return nil
}

func (service *Service) TransitionStaff(ctx context.Context, principal auth.Principal, membershipID uuid.UUID, status constants.MembershipStatus, reason string) (StaffView, error) {
	if err := requireCPOContext(principal); err != nil {
		return StaffView{}, err
	}
	if status != constants.MembershipStatusActive && status != constants.MembershipStatusSuspended && status != constants.MembershipStatusRevoked {
		return StaffView{}, invalid("status", "Staff status is invalid.")
	}
	if err := validateReason(strings.TrimSpace(reason)); err != nil {
		return StaffView{}, err
	}
	cpoID := *principal.CPOID
	err := service.database.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var membership models.CPOMembership
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND cpo_id = ?", membershipID, cpoID).First(&membership).Error; err != nil {
			return mapStaffNotFound(err)
		}
		if membership.IsPrimaryAdmin {
			return &auth.APIError{Status: http.StatusConflict, Code: "primary_admin_protected", Message: "The primary administrator cannot be suspended or revoked."}
		}
		now := service.now()
		if err := tx.Model(&models.CPOMembership{}).Where("id = ?", membership.ID).Updates(map[string]any{"status": status, "updated_at": now}).Error; err != nil {
			return mapWriteError(err, "transition staff membership")
		}
		if status != constants.MembershipStatusActive {
			scope := constants.AuthScopeCPO
			if _, err := revokeCPOSessions(tx, cpoID, &scope, "CPO_STAFF_"+string(status), now, &membership.UserID); err != nil {
				return err
			}
		}
		var user models.User
		var cpoRecord models.CPO
		if err := tx.First(&user, "id = ?", membership.UserID).Error; err != nil {
			return err
		}
		if err := tx.First(&cpoRecord, "id = ?", cpoID).Error; err != nil {
			return err
		}
		template := "CPO_STAFF_REACTIVATED"
		if status == constants.MembershipStatusSuspended {
			template = "CPO_STAFF_SUSPENDED"
		}
		if status == constants.MembershipStatusRevoked {
			template = "CPO_STAFF_REVOKED"
		}
		actionURL, err := service.cpoOnboardingActionURL(cpoID)
		if err != nil {
			return err
		}
		if err := service.outbox.EnqueueMessageWithContext(tx, user.Email, template, cmsmail.MessagePayload{RecipientName: user.FullName, CPOName: cpoRecord.BusinessName, Role: string(membership.Role), ActionURL: actionURL}, cmsmail.MessageContext{CPOID: &cpoID, UserID: &user.ID}); err != nil {
			return err
		}
		return writeAudit(tx, principal.UserID, cpoID, "CPO_STAFF_"+string(status), models.JSONB{"membership_id": membership.ID, "reason": strings.TrimSpace(reason)}, now)
	})
	if err != nil {
		return StaffView{}, err
	}
	return service.GetStaff(ctx, principal, membershipID)
}

func (service *Service) staffViews(ctx context.Context, cpoID uuid.UUID, memberships []models.CPOMembership) (StaffListResponse, error) {
	ids := make([]uuid.UUID, 0, len(memberships))
	for _, membership := range memberships {
		ids = append(ids, membership.ID)
	}
	var overrides []models.CPOMembershipPermissionOverride
	if len(ids) > 0 {
		if err := service.database.WithContext(ctx).Where("membership_id IN ?", ids).Order("permission ASC").Find(&overrides).Error; err != nil {
			return StaffListResponse{}, fmt.Errorf("list staff permission overrides: %w", err)
		}
	}
	byMembership := map[uuid.UUID][]MembershipPermissionOverrideView{}
	for _, override := range overrides {
		byMembership[override.MembershipID] = append(byMembership[override.MembershipID], MembershipPermissionOverrideView{Permission: override.Permission, Effect: override.Effect})
	}
	response := StaffListResponse{Staff: make([]StaffView, 0, len(memberships))}
	for _, membership := range memberships {
		view := StaffView{MembershipID: membership.ID, User: cpoUserView(membership.User, cpoID, membership), IsPrimaryAdmin: membership.IsPrimaryAdmin, MembershipStatus: membership.Status, RoleDefaults: cpopermissions.RoleDefaults(membership.Role), Overrides: byMembership[membership.ID]}
		allow, deny := make([]string, 0), make([]string, 0)
		for _, override := range view.Overrides {
			if override.Effect == "ALLOW" {
				allow = append(allow, override.Permission)
			} else if override.Effect == "DENY" {
				deny = append(deny, override.Permission)
			}
		}
		view.Effective = cpopermissions.Effective(membership.Role, allow, deny)
		response.Staff = append(response.Staff, view)
	}
	return response, nil
}

func validatePermissionOverrides(overrides []MembershipPermissionOverrideRequest) error {
	seen := map[string]struct{}{}
	for _, override := range overrides {
		override.Permission = strings.TrimSpace(override.Permission)
		override.Effect = strings.ToUpper(strings.TrimSpace(override.Effect))
		if !cpopermissions.Known(override.Permission) || (override.Effect != "ALLOW" && override.Effect != "DENY") {
			return invalid("overrides", "Each override must use a known permission and ALLOW or DENY effect.")
		}
		if _, exists := seen[override.Permission]; exists {
			return invalid("overrides", "Each permission may be overridden once.")
		}
		seen[override.Permission] = struct{}{}
	}
	return nil
}

func replacePermissionOverrides(tx *gorm.DB, membershipID, actorID uuid.UUID, overrides []MembershipPermissionOverrideRequest, now time.Time) error {
	if err := tx.Where("membership_id = ?", membershipID).Delete(&models.CPOMembershipPermissionOverride{}).Error; err != nil {
		return fmt.Errorf("clear staff permission overrides: %w", err)
	}
	rows := make([]models.CPOMembershipPermissionOverride, 0, len(overrides))
	for _, override := range overrides {
		rows = append(rows, models.CPOMembershipPermissionOverride{ID: uuid.New(), MembershipID: membershipID, Permission: strings.TrimSpace(override.Permission), Effect: strings.ToUpper(strings.TrimSpace(override.Effect)), CreatedBy: actorID, CreatedAt: now, UpdatedAt: now})
	}
	if len(rows) > 0 {
		if err := tx.Create(&rows).Error; err != nil {
			return mapWriteError(err, "create staff permission overrides")
		}
	}
	return nil
}

func mapStaffNotFound(err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return &auth.APIError{Status: http.StatusNotFound, Code: "staff_not_found", Message: "The staff membership was not found."}
	}
	return fmt.Errorf("load staff membership: %w", err)
}

func (service *Service) GetOrganization(
	ctx context.Context,
	principal auth.Principal,
) (OrganizationView, error) {
	if err := requireCPOContext(principal); err != nil {
		return OrganizationView{}, err
	}
	record, err := service.find(ctx, *principal.CPOID)
	if err != nil {
		return OrganizationView{}, err
	}
	return organizationView(record), nil
}

func organizationView(record models.CPO) OrganizationView {
	return OrganizationView{
		ID:              record.ID,
		Slug:            record.Slug,
		BusinessName:    record.BusinessName,
		CompanyType:     record.CompanyType,
		GSTIN:           record.GSTIN,
		Address:         record.Address,
		City:            record.City,
		State:           record.State,
		Pincode:         record.Pincode,
		Status:          record.Status,
		StatusChangedAt: record.StatusChangedAt,
		AppID:           record.AppID,
		AppIDMode:       record.AppIDMode,
		AppIDUpdatedAt:  record.AppIDUpdatedAt,
		CreatedAt:       record.CreatedAt,
		UpdatedAt:       record.UpdatedAt,
	}
}

type subscriptionData struct {
	models.CPOSubscription
	PlanName        string
	PlanDescription string
	Currency        string
	PriceMinor      int64
	BillingInterval string
	IntervalCount   int
	TrialDays       int
}

func (service *Service) GetSubscription(
	ctx context.Context,
	principal auth.Principal,
) (CPOSubscriptionView, error) {
	if err := requireCPOContext(principal); err != nil {
		return CPOSubscriptionView{}, err
	}
	cpoID := *principal.CPOID
	var result subscriptionData
	err := service.database.WithContext(ctx).
		Model(&models.CPOSubscription{}).
		Select("cpo_subscriptions.*, p.name as plan_name, p.description as plan_description, pv.currency, pv.price_minor, pv.billing_interval, pv.interval_count, pv.trial_days").
		Joins("inner join subscription_plan_versions pv on pv.id = cpo_subscriptions.plan_version_id").
		Joins("inner join subscription_plans p on p.id = pv.plan_id").
		Where("cpo_subscriptions.cpo_id = ? AND cpo_subscriptions.status IN ?", cpoID, currentCPOSubscriptionStatuses).
		First(&result).Error

	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return CPOSubscriptionView{}, &auth.APIError{
				Status:  http.StatusNotFound,
				Code:    "subscription_not_found",
				Message: "No subscription found for this CPO.",
			}
		}
		return CPOSubscriptionView{}, fmt.Errorf("error fetching subscription: %w", err)
	}

	view := CPOSubscriptionView{
		ID:                    result.ID,
		Status:                result.Status,
		StartsAt:              result.StartsAt,
		TrialEndsAt:           result.TrialEndsAt,
		CurrentPeriodStartsAt: result.CurrentPeriodStartsAt,
		CurrentPeriodEndsAt:   result.CurrentPeriodEndsAt,
		CancelAtPeriodEnd:     result.CancelAtPeriodEnd,
		CancelledAt:           result.CancelledAt,
		EndedAt:               result.EndedAt,
		Plan: CPOSubscriptionPlanView{
			Name:            result.PlanName,
			Description:     result.PlanDescription,
			Currency:        result.Currency,
			PriceMinor:      result.PriceMinor,
			BillingInterval: result.BillingInterval,
			IntervalCount:   result.IntervalCount,
			TrialDays:       result.TrialDays,
		},
	}

	return view, nil
}

func (service *Service) ListChargersByHub(
	ctx context.Context,
	principal auth.Principal,
	hubID uuid.UUID,
) (ChargerListResponse, error) {
	if err := requireCPOContext(principal); err != nil {
		return ChargerListResponse{}, err
	}

	chargers, err := service.repository.ListChargersByHub(ctx, *principal.CPOID, hubID)
	if err != nil {
		return ChargerListResponse{}, fmt.Errorf("list chargers by hub: %w", err)
	}

	result := make([]ChargerResponse, 0, len(chargers))
	for _, charger := range chargers {
		result = append(result, service.chargerView(charger, principal))
	}

	return ChargerListResponse{
		Chargers: result,
		HasMore:  false,
	}, nil
}

func (service *Service) AssignGSTToHub(
	ctx context.Context,
	principal auth.Principal,
	hubID uuid.UUID,
	request AssignGSTToHubRequest,
) (HubView, error) {
	if err := requireCPOContext(principal); err != nil {
		return HubView{}, err
	}

	cpoID := *principal.CPOID
	var hub models.Hub
	err := service.database.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := lockCPOGSTRelations(tx, cpoID); err != nil {
			return err
		}
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&hub, "id = ? AND cpo_id = ?", hubID, cpoID).Error; err != nil {
			return mapHubNotFound(err)
		}

		var gst models.GST
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&gst, "id = ? AND cpo_id = ?", request.GSTID, cpoID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return &auth.APIError{
					Status:  http.StatusNotFound,
					Code:    "gst_not_found",
					Message: "The GST was not found.",
				}
			}
			return fmt.Errorf("load GST: %w", err)
		}

		if err := validateGSTForHub(hub, gst); err != nil {
			return err
		}

		// Check if GST is already assigned to another hub
		var count int64
		if err := tx.Model(&models.Hub{}).Where("gst_id = ? AND cpo_id = ? AND id != ?", request.GSTID, cpoID, hubID).Count(&count).Error; err != nil {
			return fmt.Errorf("check for existing GST assignment: %w", err)
		}
		if count > 0 {
			return &auth.APIError{
				Status:  http.StatusConflict,
				Code:    "gst_already_assigned",
				Message: "The GST is already assigned to another hub.",
			}
		}

		now := service.now()
		if err := tx.Model(&models.Hub{}).
			Where("id = ?", hubID).
			Updates(map[string]interface{}{
				"gst_id":     &request.GSTID,
				"updated_at": now,
			}).Error; err != nil {
			return fmt.Errorf("assign GST to hub: %w", err)
		}
		hub.GSTID = &request.GSTID
		hub.UpdatedAt = now

		return writeAudit(
			tx,
			principal.UserID,
			cpoID,
			"HUB_GST_ASSIGNED",
			models.JSONB{
				"hub_id": hubID,
				"gst_id": request.GSTID,
			},
			now,
		)
	})

	if err != nil {
		return HubView{}, err
	}

	return toHubView(hub), nil
}

func (service *Service) GetGSTForHub(
	ctx context.Context,
	principal auth.Principal,
	hubID uuid.UUID,
) (GSTView, error) {
	if err := requireCPOContext(principal); err != nil {
		return GSTView{}, err
	}

	cpoID := *principal.CPOID
	var hub models.Hub
	if err := service.database.WithContext(ctx).First(&hub, "id = ? AND cpo_id = ?", hubID, cpoID).Error; err != nil {
		return GSTView{}, mapHubNotFound(err)
	}

	if hub.GSTID == nil {
		return GSTView{}, &auth.APIError{
			Status:  http.StatusNotFound,
			Code:    "gst_not_found",
			Message: "No GST is assigned to this hub.",
		}
	}

	var gst models.GST
	if err := service.database.WithContext(ctx).First(&gst, "id = ? AND cpo_id = ?", *hub.GSTID, cpoID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return GSTView{}, &auth.APIError{
				Status:  http.StatusNotFound,
				Code:    "gst_not_found",
				Message: "The assigned GST was not found.",
			}
		}
		return GSTView{}, fmt.Errorf("load GST: %w", err)
	}

	return toGSTView(gst), nil
}

func (service *Service) UpdateGSTForHub(
	ctx context.Context,
	principal auth.Principal,
	hubID uuid.UUID,
	request AssignGSTToHubRequest,
) (HubView, error) {
	if err := requireCPOContext(principal); err != nil {
		return HubView{}, err
	}

	cpoID := *principal.CPOID
	var hub models.Hub
	err := service.database.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := lockCPOGSTRelations(tx, cpoID); err != nil {
			return err
		}
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&hub, "id = ? AND cpo_id = ?", hubID, cpoID).Error; err != nil {
			return mapHubNotFound(err)
		}

		var gst models.GST
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&gst, "id = ? AND cpo_id = ?", request.GSTID, cpoID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return &auth.APIError{
					Status:  http.StatusNotFound,
					Code:    "gst_not_found",
					Message: "The GST was not found.",
				}
			}
			return fmt.Errorf("load GST: %w", err)
		}

		if err := validateGSTForHub(hub, gst); err != nil {
			return err
		}

		// Check if GST is already assigned to another hub
		var count int64
		if err := tx.Model(&models.Hub{}).Where("gst_id = ? AND cpo_id = ? AND id != ?", request.GSTID, cpoID, hubID).Count(&count).Error; err != nil {
			return fmt.Errorf("check for existing GST assignment: %w", err)
		}
		if count > 0 {
			return &auth.APIError{
				Status:  http.StatusConflict,
				Code:    "gst_already_assigned",
				Message: "The GST is already assigned to another hub.",
			}
		}

		now := service.now()
		if err := tx.Model(&models.Hub{}).
			Where("id = ?", hubID).
			Updates(map[string]interface{}{
				"gst_id":     &request.GSTID,
				"updated_at": now,
			}).Error; err != nil {
			return fmt.Errorf("update GST for hub: %w", err)
		}
		hub.GSTID = &request.GSTID
		hub.UpdatedAt = now

		return writeAudit(
			tx,
			principal.UserID,
			cpoID,
			"HUB_GST_UPDATED",
			models.JSONB{
				"hub_id": hubID,
				"gst_id": request.GSTID,
			},
			now,
		)
	})

	if err != nil {
		return HubView{}, err
	}

	return toHubView(hub), nil
}

func (service *Service) UnassignGSTFromHub(
	ctx context.Context,
	principal auth.Principal,
	hubID uuid.UUID,
) (HubView, error) {
	if err := requireCPOContext(principal); err != nil {
		return HubView{}, err
	}

	cpoID := *principal.CPOID
	var hub models.Hub
	err := service.database.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := lockCPOGSTRelations(tx, cpoID); err != nil {
			return err
		}
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&hub, "id = ? AND cpo_id = ?", hubID, cpoID).Error; err != nil {
			return mapHubNotFound(err)
		}

		if hub.GSTID == nil {
			return nil // Nothing to do
		}

		previousGstID := hub.GSTID

		now := service.now()
		if err := tx.Model(&models.Hub{}).
			Where("id = ?", hubID).
			Updates(map[string]interface{}{
				"gst_id":     nil,
				"updated_at": now,
			}).Error; err != nil {
			return fmt.Errorf("unassign GST from hub: %w", err)
		}
		hub.GSTID = nil
		hub.UpdatedAt = now

		return writeAudit(
			tx,
			principal.UserID,
			cpoID,
			"HUB_GST_UNASSIGNED",
			models.JSONB{
				"hub_id":          hubID,
				"previous_gst_id": previousGstID,
			},
			now,
		)
	})

	if err != nil {
		return HubView{}, err
	}

	return toHubView(hub), nil
}

// lockCPOGSTRelations serializes relationship-changing Hub and GST mutations
// for one tenant before their row locks are acquired. It prevents a concurrent
// Hub-state and GST-rate update from validating different intermediate states.
func lockCPOGSTRelations(tx *gorm.DB, cpoID uuid.UUID) error {
	return tx.Exec(
		"SELECT pg_advisory_xact_lock(hashtextextended(?, 0))",
		"cpo-hub-gst:"+cpoID.String(),
	).Error
}

func validateGSTForHub(hub models.Hub, gst models.GST) error {
	if !gst.IsActive || commercial.ValidateHubGST(hub.State, gst.State, gst.SGSTRate, gst.CGSTRate, gst.IGSTRate) != nil {
		return &auth.APIError{
			Status:  http.StatusBadRequest,
			Code:    "invalid_gst_for_hub",
			Message: "The active GST profile is incompatible with the Hub state and tax components.",
		}
	}
	return nil
}

func toHubView(hub models.Hub) HubView {
	return HubView{
		ID:              hub.ID,
		CPOID:           hub.CPOID,
		Name:            hub.Name,
		Address:         hub.Address,
		State:           hub.State,
		Latitude:        hub.Latitude,
		Longitude:       hub.Longitude,
		Open24Hours:     hub.Open24Hours,
		SanctionLoad:    hub.SanctionLoad,
		CustomerVisible: hub.CustomerVisible,
		GSTID:           hub.GSTID,
		CreatedAt:       hub.CreatedAt,
		UpdatedAt:       hub.UpdatedAt,
	}
}

func toGSTView(gst models.GST) GSTView {
	return GSTView{
		ID:        gst.ID,
		CPOID:     gst.CPOID,
		Name:      gst.Name,
		State:     gst.State,
		SGSTRate:  gst.SGSTRate,
		CGSTRate:  gst.CGSTRate,
		IGSTRate:  gst.IGSTRate,
		IsActive:  gst.IsActive,
		CreatedAt: gst.CreatedAt,
		UpdatedAt: gst.UpdatedAt,
	}
}

func (service *Service) CreateCharger(
	ctx *gin.Context,
	principal auth.Principal,
) (ChargerResponse, error) {
	if err := requireCPOContext(principal); err != nil {
		return ChargerResponse{}, err
	}

	cpoID := *principal.CPOID
	err := ctx.Request.ParseMultipartForm(32 << 20) // 32MB
	if err != nil {
		return ChargerResponse{}, &auth.APIError{
			Status:  http.StatusBadRequest,
			Code:    "invalid_request",
			Message: "The request body is invalid.",
		}
	}

	var request CreateChargerRequest
	if err := json.Unmarshal([]byte(ctx.Request.FormValue("data")), &request); err != nil {
		return ChargerResponse{}, &auth.APIError{
			Status:  http.StatusBadRequest,
			Code:    "invalid_request",
			Message: "The request body is invalid.",
		}
	}

	request = normalizeCreateChargerRequest(request)
	if err := validateCreateChargerRequest(request); err != nil {
		return ChargerResponse{}, err
	}

	var vendor *string
	if request.Vendor != "" {
		v := request.Vendor
		vendor = &v
	}

	var model *string
	if request.Model != "" {
		m := request.Model
		model = &m
	}

	var record models.Charger
	err = service.database.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if request.HubID != nil {
			var hub models.Hub
			if err := tx.First(&hub, "id = ? AND cpo_id = ?", *request.HubID, cpoID).Error; err != nil {
				return mapHubNotFound(err)
			}
		}

		chargerID, err := generateUniqueChargerIDTx(tx)
		if err != nil {
			return err
		}

		ocppIdentity := chargerID

		file, err := ctx.FormFile("charger_image")
		var chargerImagePath string
		if err == nil {
			filename := uuid.New().String() + filepath.Ext(file.Filename)
			uploads := "uploads"
			if _, err := os.Stat(uploads); os.IsNotExist(err) {
				if err := os.Mkdir(uploads, 0755); err != nil {
					return err
				}
			}

			chargerImagePath = filepath.Join(uploads, filename)
			if err := ctx.SaveUploadedFile(file, chargerImagePath); err != nil {
				return err
			}
		}

		now := service.now()
		record = models.Charger{
			ID:                  uuid.New(),
			CPOID:               cpoID,
			HubID:               request.HubID,
			ChargerID:           chargerID,
			OCPPIdentity:        ocppIdentity,
			Vendor:              vendor,
			Model:               model,
			SerialNumber:        request.SerialNumber,
			MaxPowerKW:          request.MaxPowerKW,
			ChargerName:         request.ChargerName,
			ChargerHostName:     request.ChargerHostName,
			ChargerHostPhoneNo:  request.ChargerHostPhoneNo,
			ChargerType:         request.ChargerType,
			Segment:             request.Segment,
			SubSegment:          request.SubSegment,
			ChargerImage:        chargerImagePath,
			ChargerUseType:      request.ChargerUseType,
			NumberOfConnectors:  request.NumberOfConnectors,
			Parking:             request.Parking,
			Protocol:            request.Protocol,
			TwentyFourSevenOpen: request.TwentyFourSevenOpen,
			CustomerVisibility:  false,
			Status:              constants.ChargerStatusInactive,
			OCPPVersion:         "1.6J",
			CreatedAt:           now,
			UpdatedAt:           now,
		}

		if err := tx.Create(&record).Error; err != nil {
			return mapChargerWriteError(err, "create charger")
		}

		for _, connector := range request.Connectors {
			connectorType := strings.TrimSpace(connector.ConnectorType)
			connectorRecord := models.Connector{
				ID:                     uuid.New(),
				CPOID:                  cpoID,
				ChargerID:              record.ID,
				ConnectorNumber:        connector.ConnectorNumber,
				ConnectorType:          connectorType,
				ConnectorTotalCapacity: connector.ConnectorTotalCapacity,
				Status:                 constants.ChargerStatusActive,
				CreatedAt:              now,
				UpdatedAt:              now,
			}

			if err := tx.Create(&connectorRecord).Error; err != nil {
				return mapConnectorWriteError(err, "create connector")
			}
			record.Connectors = append(record.Connectors, connectorRecord)
		}
		if err := tx.Create(&models.HALChargerMapping{CMSChargerID: record.ID, CPOID: cpoID, ChargerOCPPIdentity: record.OCPPIdentity, SyncState: "PENDING", CreatedAt: now, UpdatedAt: now}).Error; err != nil {
			return fmt.Errorf("create pending HAL charger mapping: %w", err)
		}

		return writeAudit(
			tx,
			principal.UserID,
			cpoID,
			"CHARGER_CREATED",
			models.JSONB{
				"charger_id":    record.ChargerID,
				"ocpp_identity": record.OCPPIdentity,
				"hub_id":        record.HubID,
				"connectors":    len(record.Connectors),
			},
			now,
		)
	})
	if err != nil {
		return ChargerResponse{}, err
	}
	if record.HubID != nil {
		var hub models.Hub
		if err := service.database.WithContext(ctx).First(&hub, "id = ?", *record.HubID).Error; err == nil {
			record.Hub = &hub
		}
	}
	if service.halOperations != nil {
		// Inventory is durable even when HAL is unavailable; the capability leaves
		// the mapping pending for its bounded reconciler.
		correlationID := uuid.NewString()
		if requestID, ok := cmsmiddleware.RequestID(ctx); ok {
			correlationID = requestID
		}
		_ = service.halOperations.EnsureChargerMapping(ctx.Request.Context(), record.ID, correlationID)
	}

	return service.chargerView(record, principal), nil
}

func (service *Service) CreateHubTariff(
	ctx context.Context,
	principal auth.Principal,
	hubID uuid.UUID,
	request CreateTariffRequest,
) (TariffView, error) {
	if err := requireCPOContext(principal); err != nil {
		return TariffView{}, err
	}

	cpoID := *principal.CPOID
	request.HubID = &hubID
	request.ChargerID = nil
	request.UserGroupID = nil
	request = normalizeCreateTariffRequest(request)
	if err := validateCreateTariffRequest(request); err != nil {
		return TariffView{}, err
	}

	var record models.Tariff
	err := service.database.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := service.validateTariffScope(tx, cpoID, request.HubID, request.ChargerID, request.UserGroupID); err != nil {
			return err
		}

		now := service.now()
		isActive := true
		if request.IsActive != nil {
			isActive = *request.IsActive
		}

		record = models.Tariff{
			ID:            uuid.New(),
			CPOID:         cpoID,
			HubID:         request.HubID,
			AssignedTo:    constants.TariffAssignedHub,
			ChargerID:     request.ChargerID,
			UserGroupID:   request.UserGroupID,
			PricePerUnit:  request.PricePerUnit,
			IdleFeePerMin: request.IdleFeePerMin,
			Currency:      request.Currency,
			IsActive:      isActive,
			StartDate:     request.StartDate,
			EndDate:       request.EndDate,
			TariffType:    request.TariffType,
			PriceType:     request.PriceType,
			Units:         request.Units,
			CreatedAt:     now,
			UpdatedAt:     now,
		}
		if err := service.validateTariffTopologyMutation(tx, &record, nil); err != nil {
			return err
		}

		if err := tx.Create(&record).Error; err != nil {
			return service.handleTariffError("create hub tariff", err)
		}

		return writeAudit(
			tx,
			principal.UserID,
			cpoID,
			"HUB_TARIFF_CREATED",
			models.JSONB{
				"tariff_id": record.ID,
				"hub_id":    record.HubID,
			},
			now,
		)
	})
	if err != nil {
		return TariffView{}, err
	}

	return service.tariffView(&record), nil
}

func (service *Service) ListHubTariffs(
	ctx context.Context,
	principal auth.Principal,
	hubID uuid.UUID,
	query TenantListQuery,
) (*TariffListResponse, error) {
	if err := requireCPOContext(principal); err != nil {
		return nil, err
	}
	query, err := validateTenantListQuery(query)
	if err != nil {
		return nil, err
	}
	databaseQuery := service.database.WithContext(ctx).
		Where("cpo_id = ? AND assigned_to = ? AND hub_id = ?", *principal.CPOID, constants.TariffAssignedHub, hubID)
	if query.Before != nil {
		databaseQuery = databaseQuery.Where(
			"(created_at, id) < (?, ?)",
			*query.Before,
			*query.BeforeID,
		)
	}
	var records []*models.Tariff
	if err := databaseQuery.
		Order("created_at DESC, id DESC").
		Limit(query.Limit + 1).
		Find(&records).Error; err != nil {
		return nil, fmt.Errorf("list hub tariffs: %w", err)
	}

	hasNext := len(records) > query.Limit
	if hasNext {
		records = records[:query.Limit]
	}

	views := make([]TariffView, len(records))
	for i, record := range records {
		views[i] = service.tariffView(record)
	}

	response := TariffListResponse{Tariffs: views, HasMore: hasNext}
	if hasNext && len(records) > 0 {
		nextBefore := records[len(records)-1].CreatedAt
		nextBeforeID := records[len(records)-1].ID
		response.NextBefore = &nextBefore
		response.NextBeforeID = &nextBeforeID
	}

	return &response, nil
}

func (service *Service) GetHubTariff(
	ctx context.Context,
	principal auth.Principal,
	hubID uuid.UUID,
	tariffID uuid.UUID,
) (TariffView, error) {
	if err := requireCPOContext(principal); err != nil {
		return TariffView{}, err
	}

	var record models.Tariff
	if err := service.database.WithContext(ctx).
		First(&record, "cpo_id = ? AND assigned_to = ? AND hub_id = ? AND id = ?", *principal.CPOID, constants.TariffAssignedHub, hubID, tariffID).Error; err != nil {
		return TariffView{}, service.handleTariffError("load tariff", err)
	}

	return service.tariffView(&record), nil // Fix: Call as method
}

func (service *Service) UpdateHubTariff(
	ctx context.Context,
	principal auth.Principal,
	hubID uuid.UUID,
	tariffID uuid.UUID,
	request UpdateTariffRequest,
) (TariffView, error) {
	if err := requireCPOContext(principal); err != nil {
		return TariffView{}, err
	}

	request = normalizeUpdateTariffRequest(request)
	if err := validateUpdateTariffRequest(request); err != nil {
		return TariffView{}, err
	}

	cpoID := *principal.CPOID
	var record models.Tariff

	err := service.database.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			First(&record, "cpo_id = ? AND assigned_to = ? AND hub_id = ? AND id = ?", cpoID, constants.TariffAssignedHub, hubID, tariffID).Error; err != nil {
			return service.handleTariffError("load tariff", err)
		}

		if err := service.validateTariffScope(tx, cpoID, &hubID, nil, nil); err != nil {
			return err
		}

		updates, changedFields := applyTariffUpdate(&record, request)

		if len(changedFields) == 0 {
			return &auth.APIError{
				Status:  http.StatusBadRequest,
				Code:    "invalid_request",
				Message: "At least one tariff field must be supplied.",
			}
		}
		if err := validateTariffCommercial(record.TariffType, record.PriceType, record.Units, record.IdleFeePerMin, record.IsActive); err != nil {
			return err
		}
		now := service.now()
		updates["updated_at"] = now
		record.UpdatedAt = now
		if err := validateTariffDateRange(record.StartDate, record.EndDate); err != nil {
			return err
		}
		if err := service.validateTariffTopologyMutation(tx, &record, nil); err != nil {
			return err
		}
		if err := tx.Model(&models.Tariff{}).
			Where("id = ?", record.ID).
			Updates(updates).Error; err != nil {
			return service.handleTariffError("update tariff", err)
		}

		return writeAudit(
			tx,
			principal.UserID,
			cpoID,
			"HUB_TARIFF_UPDATED",
			models.JSONB{
				"tariff_id":      record.ID,
				"changed_fields": changedFields,
			},
			now,
		)
	})
	if err != nil {
		return TariffView{}, err
	}

	return service.tariffView(&record), nil
}

func (service *Service) CreateChargerTariff(
	ctx context.Context,
	principal auth.Principal,
	chargerID uuid.UUID,
	request CreateTariffRequest,
) (TariffView, error) {
	if err := requireCPOContext(principal); err != nil {
		return TariffView{}, err
	}

	cpoID := *principal.CPOID
	request.HubID = nil
	request.ChargerID = &chargerID
	request.UserGroupID = nil
	request = normalizeCreateTariffRequest(request)
	if err := validateCreateTariffRequest(request); err != nil {
		return TariffView{}, err
	}

	var record models.Tariff
	err := service.database.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := service.validateTariffScope(tx, cpoID, request.HubID, request.ChargerID, request.UserGroupID); err != nil {
			return err
		}

		now := service.now()
		isActive := true
		if request.IsActive != nil {
			isActive = *request.IsActive
		}

		record = models.Tariff{
			ID:            uuid.New(),
			CPOID:         cpoID,
			HubID:         request.HubID,
			AssignedTo:    constants.TariffAssignedCharger,
			ChargerID:     request.ChargerID,
			UserGroupID:   request.UserGroupID,
			PricePerUnit:  request.PricePerUnit,
			IdleFeePerMin: request.IdleFeePerMin,
			Currency:      request.Currency,
			IsActive:      isActive,
			StartDate:     request.StartDate,
			EndDate:       request.EndDate,
			TariffType:    request.TariffType,
			PriceType:     request.PriceType,
			Units:         request.Units,
			CreatedAt:     now,
			UpdatedAt:     now,
		}
		if err := service.validateTariffTopologyMutation(tx, &record, nil); err != nil {
			return err
		}

		if err := tx.Create(&record).Error; err != nil {
			return service.handleTariffError("create charger tariff", err)
		}

		return writeAudit(
			tx,
			principal.UserID,
			cpoID,
			"CHARGER_TARIFF_CREATED",
			models.JSONB{
				"tariff_id":  record.ID,
				"charger_id": record.ChargerID,
			},
			now,
		)
	})
	if err != nil {
		return TariffView{}, err
	}

	return service.tariffView(&record), nil // Fix: Call as method
}

func (service *Service) ListChargerTariffs(
	ctx context.Context,
	principal auth.Principal,
	chargerID uuid.UUID,
	query TenantListQuery,
) (*TariffListResponse, error) {
	if err := requireCPOContext(principal); err != nil {
		return nil, err
	}
	query, err := validateTenantListQuery(query)
	if err != nil {
		return nil, err
	}
	databaseQuery := service.database.WithContext(ctx).
		Where("cpo_id = ? AND assigned_to = ? AND charger_id = ?", *principal.CPOID, constants.TariffAssignedCharger, chargerID)
	if query.Before != nil {
		databaseQuery = databaseQuery.Where(
			"(created_at, id) < (?, ?)",
			*query.Before,
			*query.BeforeID,
		)
	}
	var records []*models.Tariff
	if err := databaseQuery.
		Order("created_at DESC, id DESC").
		Limit(query.Limit + 1).
		Find(&records).Error; err != nil {
		return nil, fmt.Errorf("list charger tariffs: %w", err)
	}

	hasNext := len(records) > query.Limit
	if hasNext {
		records = records[:query.Limit]
	}

	views := make([]TariffView, len(records))
	for i, record := range records {
		views[i] = service.tariffView(record)
	}

	response := TariffListResponse{Tariffs: views, HasMore: hasNext}
	if hasNext && len(records) > 0 {
		nextBefore := records[len(records)-1].CreatedAt
		nextBeforeID := records[len(records)-1].ID
		response.NextBefore = &nextBefore
		response.NextBeforeID = &nextBeforeID
	}

	return &response, nil
}

func (service *Service) GetChargerTariff(
	ctx context.Context,
	principal auth.Principal,
	chargerID uuid.UUID,
	tariffID uuid.UUID,
) (TariffView, error) {
	if err := requireCPOContext(principal); err != nil {
		return TariffView{}, err
	}

	var record models.Tariff
	if err := service.database.WithContext(ctx).
		First(&record, "cpo_id = ? AND assigned_to = ? AND charger_id = ? AND id = ?", *principal.CPOID, constants.TariffAssignedCharger, chargerID, tariffID).Error; err != nil {
		return TariffView{}, service.handleTariffError("load tariff", err)
	}

	return service.tariffView(&record), nil // Fix: Call as method
}

func (service *Service) UpdateChargerTariff(
	ctx context.Context,
	principal auth.Principal,
	chargerID uuid.UUID,
	tariffID uuid.UUID,
	request UpdateTariffRequest,
) (TariffView, error) {
	if err := requireCPOContext(principal); err != nil {
		return TariffView{}, err
	}

	request = normalizeUpdateTariffRequest(request)
	if err := validateUpdateTariffRequest(request); err != nil {
		return TariffView{}, err
	}

	cpoID := *principal.CPOID
	var record models.Tariff

	err := service.database.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			First(&record, "cpo_id = ? AND assigned_to = ? AND charger_id = ? AND id = ?", cpoID, constants.TariffAssignedCharger, chargerID, tariffID).Error; err != nil {
			return service.handleTariffError("load tariff", err)
		}

		if err := service.validateTariffScope(tx, cpoID, nil, &chargerID, nil); err != nil {
			return err
		}

		updates, changedFields := applyTariffUpdate(&record, request)

		if len(changedFields) == 0 {
			return &auth.APIError{
				Status:  http.StatusBadRequest,
				Code:    "invalid_request",
				Message: "At least one tariff field must be supplied.",
			}
		}
		if err := validateTariffCommercial(record.TariffType, record.PriceType, record.Units, record.IdleFeePerMin, record.IsActive); err != nil {
			return err
		}
		now := service.now()
		updates["updated_at"] = now
		record.UpdatedAt = now
		if err := validateTariffDateRange(record.StartDate, record.EndDate); err != nil {
			return err
		}
		if err := service.validateTariffTopologyMutation(tx, &record, nil); err != nil {
			return err
		}
		if err := tx.Model(&models.Tariff{}).
			Where("id = ?", record.ID).
			Updates(updates).Error; err != nil {
			return service.handleTariffError("update tariff", err)
		}

		return writeAudit(
			tx,
			principal.UserID,
			cpoID,
			"CHARGER_TARIFF_UPDATED",
			models.JSONB{
				"tariff_id":      record.ID,
				"changed_fields": changedFields,
			},
			now,
		)
	})
	if err != nil {
		return TariffView{}, err
	}

	return service.tariffView(&record), nil // Fix: Call as method
}

func (service *Service) CreateUserGroupTariff(
	ctx context.Context,
	principal auth.Principal,
	userGroupID uuid.UUID,
	request CreateTariffRequest,
) (TariffView, error) {
	if err := requireCPOContext(principal); err != nil {
		return TariffView{}, err
	}

	cpoID := *principal.CPOID
	request.HubID = nil
	request.ChargerID = nil
	request.UserGroupID = &userGroupID
	request = normalizeCreateTariffRequest(request)
	if err := validateCreateTariffRequest(request); err != nil {
		return TariffView{}, err
	}
	var record models.Tariff
	err := service.database.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := service.validateTariffScope(tx, cpoID, request.HubID, request.ChargerID, request.UserGroupID); err != nil {
			return err
		}

		now := service.now()
		isActive := true
		if request.IsActive != nil {
			isActive = *request.IsActive
		}

		record = models.Tariff{
			ID:            uuid.New(),
			CPOID:         cpoID,
			HubID:         request.HubID,
			AssignedTo:    constants.TariffAssignedUserGroup,
			ChargerID:     request.ChargerID,
			UserGroupID:   request.UserGroupID,
			PricePerUnit:  request.PricePerUnit,
			IdleFeePerMin: request.IdleFeePerMin,
			Currency:      request.Currency,
			IsActive:      isActive,
			StartDate:     request.StartDate,
			EndDate:       request.EndDate,
			TariffType:    request.TariffType,
			PriceType:     request.PriceType,
			Units:         request.Units,
			CreatedAt:     now,
			UpdatedAt:     now,
		}
		if err := service.validateTariffTopologyMutation(tx, &record, nil); err != nil {
			return err
		}

		if err := tx.Create(&record).Error; err != nil {
			return service.handleTariffError("create user group tariff", err)
		}

		return writeAudit(
			tx,
			principal.UserID,
			cpoID,
			"USER_GROUP_TARIFF_CREATED",
			models.JSONB{
				"tariff_id":     record.ID,
				"user_group_id": record.UserGroupID,
			},
			now,
		)
	})
	if err != nil {
		return TariffView{}, err
	}

	return service.tariffView(&record), nil // Fix: Call as method
}

func (service *Service) ListUserGroupTariffs(
	ctx context.Context,
	principal auth.Principal,
	userGroupID uuid.UUID,
	query TenantListQuery,
) (*TariffListResponse, error) {
	if err := requireCPOContext(principal); err != nil {
		return nil, err
	}
	query, err := validateTenantListQuery(query)
	if err != nil {
		return nil, err
	}
	databaseQuery := service.database.WithContext(ctx).
		Where("cpo_id = ? AND assigned_to = ? AND user_group_id = ?", *principal.CPOID, constants.TariffAssignedUserGroup, userGroupID)
	if query.Before != nil {
		databaseQuery = databaseQuery.Where(
			"(created_at, id) < (?, ?)",
			*query.Before,
			*query.BeforeID,
		)
	}
	var records []*models.Tariff
	if err := databaseQuery.
		Order("created_at DESC, id DESC").
		Limit(query.Limit + 1).
		Find(&records).Error; err != nil {
		return nil, fmt.Errorf("list user group tariffs: %w", err)
	}

	hasNext := len(records) > query.Limit
	if hasNext {
		records = records[:query.Limit]
	}

	views := make([]TariffView, len(records))
	for i, record := range records {
		views[i] = service.tariffView(record)
	}

	response := TariffListResponse{Tariffs: views, HasMore: hasNext}
	if hasNext && len(records) > 0 {
		nextBefore := records[len(records)-1].CreatedAt
		nextBeforeID := records[len(records)-1].ID
		response.NextBefore = &nextBefore
		response.NextBeforeID = &nextBeforeID
	}

	return &response, nil
}

func (service *Service) GetUserGroupTariff(
	ctx context.Context,
	principal auth.Principal,
	userGroupID uuid.UUID,
	tariffID uuid.UUID,
) (TariffView, error) {
	if err := requireCPOContext(principal); err != nil {
		return TariffView{}, err
	}

	var record models.Tariff
	if err := service.database.WithContext(ctx).
		First(&record, "cpo_id = ? AND assigned_to = ? AND user_group_id = ? AND id = ?", *principal.CPOID, constants.TariffAssignedUserGroup, userGroupID, tariffID).Error; err != nil {
		return TariffView{}, service.handleTariffError("load tariff", err)
	}

	return service.tariffView(&record), nil // Fix: Call as method
}

func (service *Service) UpdateUserGroupTariff(
	ctx context.Context,
	principal auth.Principal,
	userGroupID uuid.UUID,
	tariffID uuid.UUID,
	request UpdateTariffRequest,
) (TariffView, error) {
	if err := requireCPOContext(principal); err != nil {
		return TariffView{}, err
	}

	request = normalizeUpdateTariffRequest(request)
	if err := validateUpdateTariffRequest(request); err != nil {
		return TariffView{}, err
	}

	cpoID := *principal.CPOID
	var record models.Tariff

	err := service.database.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			First(&record, "cpo_id = ? AND assigned_to = ? AND user_group_id = ? AND id = ?", cpoID, constants.TariffAssignedUserGroup, userGroupID, tariffID).Error; err != nil {
			return service.handleTariffError("load tariff", err)
		}

		if err := service.validateTariffScope(tx, cpoID, nil, nil, &userGroupID); err != nil {
			return err
		}

		updates, changedFields := applyTariffUpdate(&record, request)

		if len(changedFields) == 0 {
			return &auth.APIError{
				Status:  http.StatusBadRequest,
				Code:    "invalid_request",
				Message: "At least one tariff field must be supplied.",
			}
		}
		if err := validateTariffCommercial(record.TariffType, record.PriceType, record.Units, record.IdleFeePerMin, record.IsActive); err != nil {
			return err
		}
		now := service.now()
		updates["updated_at"] = now
		record.UpdatedAt = now
		if err := validateTariffDateRange(record.StartDate, record.EndDate); err != nil {
			return err
		}
		if err := service.validateTariffTopologyMutation(tx, &record, nil); err != nil {
			return err
		}
		if err := tx.Model(&models.Tariff{}).
			Where("id = ?", record.ID).
			Updates(updates).Error; err != nil {
			return service.handleTariffError("update tariff", err)
		}

		return writeAudit(
			tx,
			principal.UserID,
			cpoID,
			"USER_GROUP_TARIFF_UPDATED",
			models.JSONB{
				"tariff_id":      record.ID,
				"changed_fields": changedFields,
			},
			now,
		)
	})
	if err != nil {
		return TariffView{}, err
	}

	return service.tariffView(&record), nil // Fix: Call as method
}

func (service *Service) deleteScopedTariff(
	ctx context.Context,
	principal auth.Principal,
	targetID, tariffID uuid.UUID,
	assignment constants.TariffAssignmentType,
	targetColumn, auditAction string,
) error {
	if err := requireCPOContext(principal); err != nil {
		return err
	}
	cpoID := *principal.CPOID
	return service.database.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var record models.Tariff
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			First(&record, "cpo_id = ? AND assigned_to = ? AND "+targetColumn+" = ? AND id = ?", cpoID, assignment, targetID, tariffID).Error; err != nil {
			return service.handleTariffError("load tariff", err)
		}
		if err := service.validateTariffTopologyMutation(tx, nil, &record.ID); err != nil {
			return err
		}
		if err := tx.Delete(&record).Error; err != nil {
			return service.handleTariffError("delete tariff", err)
		}
		now := service.now()
		return writeAudit(tx, principal.UserID, cpoID, auditAction, models.JSONB{
			"tariff_id": record.ID,
			"target_id": targetID,
		}, now)
	})
}

func (service *Service) DeleteHubTariff(ctx context.Context, principal auth.Principal, hubID, tariffID uuid.UUID) error {
	return service.deleteScopedTariff(ctx, principal, hubID, tariffID, constants.TariffAssignedHub, "hub_id", "HUB_TARIFF_DELETED")
}

func (service *Service) DeleteChargerTariff(ctx context.Context, principal auth.Principal, chargerID, tariffID uuid.UUID) error {
	return service.deleteScopedTariff(ctx, principal, chargerID, tariffID, constants.TariffAssignedCharger, "charger_id", "CHARGER_TARIFF_DELETED")
}

func (service *Service) DeleteUserGroupTariff(ctx context.Context, principal auth.Principal, userGroupID, tariffID uuid.UUID) error {
	return service.deleteScopedTariff(ctx, principal, userGroupID, tariffID, constants.TariffAssignedUserGroup, "user_group_id", "USER_GROUP_TARIFF_DELETED")
}

func (service *Service) validateTariffScope(
	tx *gorm.DB,
	cpoID uuid.UUID,
	hubID *uuid.UUID,
	chargerID *uuid.UUID,
	userGroupID *uuid.UUID,
) error {
	targetCount := 0
	for _, targetID := range []*uuid.UUID{hubID, chargerID, userGroupID} {
		if targetID != nil {
			targetCount++
		}
	}
	if targetCount != 1 {
		return invalid("tariff_target", "A tariff must target exactly one hub, charger, or user group.")
	}

	if hubID != nil {
		var hub models.Hub
		if err := tx.
			Where("cpo_id = ? AND id = ?", cpoID, *hubID).
			First(&hub).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return &auth.APIError{
					Status:  http.StatusNotFound,
					Code:    "hub_not_found",
					Message: "The hub was not found.",
				}
			}
			return fmt.Errorf("validate tariff scope (hub): %w", err)
		}
	}

	if chargerID != nil {
		var charger models.Charger
		if err := tx.
			Where("cpo_id = ? AND id = ?", cpoID, *chargerID).
			First(&charger).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return &auth.APIError{
					Status:  http.StatusNotFound,
					Code:    "charger_not_found",
					Message: "The charger was not found.",
				}
			}
			return fmt.Errorf("validate tariff scope (charger): %w", err)
		}
		if hubID != nil && (charger.HubID == nil || *charger.HubID != *hubID) {
			return &auth.APIError{
				Status:  http.StatusConflict,
				Code:    "charger_not_in_hub",
				Message: "The charger was not found in the specified hub.",
			}
		}
	}

	if userGroupID != nil {
		var userGroup models.UserGroup
		if err := tx.
			Where("cpo_id = ? AND id = ?", cpoID, *userGroupID).
			First(&userGroup).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return &auth.APIError{
					Status:  http.StatusNotFound,
					Code:    "user_group_not_found",
					Message: "The user group was not found.",
				}
			}
			return fmt.Errorf("validate tariff scope (user_group): %w", err)
		}
	}

	return nil
}

func (service *Service) GetCharger(
	ctx context.Context,
	principal auth.Principal,
	chargerID string,
) (ChargerResponse, error) {
	if err := requireCPOContext(principal); err != nil {
		return ChargerResponse{}, err
	}

	chargerID = normalizeChargerID(chargerID)
	if !chargerIDPattern.MatchString(chargerID) {
		return ChargerResponse{}, &auth.APIError{
			Status:  http.StatusBadRequest,
			Code:    "invalid_charger_id",
			Message: "The charger ID is invalid.",
		}
	}

	var record models.Charger
	if err := service.database.WithContext(ctx).
		Preload("Connectors", func(tx *gorm.DB) *gorm.DB {
			return tx.Order("connector_number ASC")
		}).
		Preload("Hub").
		First(&record, "cpo_id = ? AND charger_id = ?", *principal.CPOID, chargerID).Error; err != nil {
		return ChargerResponse{}, mapChargerNotFound(err)
	}

	response := service.chargerView(record, principal)

	if service.liveOperations != nil {
		live, err := service.liveOperations.GetChargerDetail(ctx, *principal.CPOID, record.ID)
		if err == nil {
			response.Live = &live
		}
		// We ignore the error if live data is not available
	}

	return response, nil
}

func (service *Service) GetOperationalCharger(ctx context.Context, principal auth.Principal, chargerID string) (OperationalChargerResponse, error) {
	if err := requireCPOContext(principal); err != nil {
		return OperationalChargerResponse{}, err
	}
	if service.liveOperations == nil {
		return OperationalChargerResponse{}, errors.New("live operations capability is unavailable")
	}
	chargerID = normalizeChargerID(chargerID)
	if !chargerIDPattern.MatchString(chargerID) {
		return OperationalChargerResponse{}, invalid("charger_id", "The charger ID is invalid.")
	}
	var record models.Charger
	if err := service.database.WithContext(ctx).Preload("Connectors", func(tx *gorm.DB) *gorm.DB { return tx.Order("connector_number ASC") }).Preload("Hub").First(&record, "cpo_id = ? AND charger_id = ?", *principal.CPOID, chargerID).Error; err != nil {
		return OperationalChargerResponse{}, mapChargerNotFound(err)
	}
	live, err := service.liveOperations.GetChargerDetail(ctx, *principal.CPOID, record.ID)
	if err != nil {
		return OperationalChargerResponse{}, fmt.Errorf("load charger operational state: %w", err)
	}
	return OperationalChargerResponse{Charger: service.chargerView(record, principal), Live: live}, nil
}

func (service *Service) GetFleetOperations(ctx context.Context, principal auth.Principal) (FleetOperationsResponse, error) {
	if err := requireCPOContext(principal); err != nil {
		return FleetOperationsResponse{}, err
	}
	if service.liveOperations == nil {
		return FleetOperationsResponse{}, errors.New("live operations capability is unavailable")
	}
	fleet, err := service.liveOperations.GetFleet(ctx, *principal.CPOID)
	if err != nil {
		return FleetOperationsResponse{}, fmt.Errorf("load CPO fleet operational state: %w", err)
	}
	return FleetOperationsResponse{Fleet: fleet}, nil
}

// Platform operational views are observation-only. They deliberately do not
// expose a cross-tenant RemoteStop or any other charger-control command.
func (service *Service) GetPlatformFleetOperations(ctx context.Context, principal auth.Principal, cpoID uuid.UUID) (FleetOperationsResponse, error) {
	if err := requirePlatform(principal); err != nil {
		return FleetOperationsResponse{}, err
	}
	if service.liveOperations == nil {
		return FleetOperationsResponse{}, errors.New("live operations capability is unavailable")
	}
	fleet, err := service.liveOperations.GetFleet(ctx, cpoID)
	if err != nil {
		return FleetOperationsResponse{}, fmt.Errorf("load platform fleet operational state: %w", err)
	}
	return FleetOperationsResponse{Fleet: fleet}, nil
}

func (service *Service) GetPlatformOperationalCharger(ctx context.Context, principal auth.Principal, cpoID uuid.UUID, chargerID string) (OperationalChargerResponse, error) {
	if err := requirePlatform(principal); err != nil {
		return OperationalChargerResponse{}, err
	}
	if service.liveOperations == nil {
		return OperationalChargerResponse{}, errors.New("live operations capability is unavailable")
	}
	chargerID = normalizeChargerID(chargerID)
	if !chargerIDPattern.MatchString(chargerID) {
		return OperationalChargerResponse{}, invalid("charger_id", "The charger ID is invalid.")
	}
	var record models.Charger
	if err := service.database.WithContext(ctx).Preload("Connectors", func(tx *gorm.DB) *gorm.DB { return tx.Order("connector_number ASC") }).Preload("Hub").First(&record, "cpo_id = ? AND charger_id = ?", cpoID, chargerID).Error; err != nil {
		return OperationalChargerResponse{}, mapChargerNotFound(err)
	}
	live, err := service.liveOperations.GetChargerDetail(ctx, cpoID, record.ID)
	if err != nil {
		return OperationalChargerResponse{}, fmt.Errorf("load platform charger operational state: %w", err)
	}
	return OperationalChargerResponse{Charger: service.chargerView(record, principal), Live: live}, nil
}

func (service *Service) ListOperationalEvents(ctx context.Context, principal auth.Principal, after int64, limit int) (operationalrealtime.Page, error) {
	if err := requireCPOContext(principal); err != nil {
		return operationalrealtime.Page{}, err
	}
	if service.operationalEvents == nil {
		return operationalrealtime.Page{}, fmt.Errorf("operational event capability is unavailable")
	}
	return service.operationalEvents.ListCPO(ctx, *principal.CPOID, after, limit)
}

func (service *Service) ListLiveChargingSessionEvents(ctx context.Context, principal auth.Principal, after int64, limit int) (operationalrealtime.Page, error) {
	if err := requireCPOContext(principal); err != nil {
		return operationalrealtime.Page{}, err
	}
	if service.operationalEvents == nil {
		return operationalrealtime.Page{}, fmt.Errorf("operational event capability is unavailable")
	}
	return service.operationalEvents.ListCPOChargingSessionEvents(ctx, *principal.CPOID, after, limit)
}

func (service *Service) LatestLiveChargingSessionEventID(ctx context.Context, principal auth.Principal) (int64, error) {
	if err := requireCPOContext(principal); err != nil {
		return 0, err
	}
	if service.operationalEvents == nil {
		return 0, fmt.Errorf("operational event capability is unavailable")
	}
	return service.operationalEvents.LatestCPOChargingSessionEventID(ctx, *principal.CPOID)
}

func (service *Service) ListPlatformOperationalEvents(ctx context.Context, principal auth.Principal, cpoID uuid.UUID, after int64, limit int) (operationalrealtime.Page, error) {
	if err := requirePlatform(principal); err != nil {
		return operationalrealtime.Page{}, err
	}
	if service.operationalEvents == nil {
		return operationalrealtime.Page{}, fmt.Errorf("operational event capability is unavailable")
	}
	var cpo models.CPO
	if err := service.database.WithContext(ctx).First(&cpo, "id = ?", cpoID).Error; err != nil {
		return operationalrealtime.Page{}, mapNotFound(err)
	}
	return service.operationalEvents.ListCPO(ctx, cpoID, after, limit)
}

func (service *Service) OperationalStreamTiming() (time.Duration, time.Duration, int) {
	if service.operationalEvents == nil {
		return time.Second, 15 * time.Second, 100
	}
	return service.operationalEvents.StreamTiming()
}

func (service *Service) UpdateCharger(
	ctx *gin.Context,
	principal auth.Principal,
	chargerID string,
) (ChargerResponse, error) {
	if err := requireCPOContext(principal); err != nil {
		return ChargerResponse{}, err
	}

	chargerID = normalizeChargerID(chargerID)
	if !chargerIDPattern.MatchString(chargerID) {
		return ChargerResponse{}, &auth.APIError{
			Status:  http.StatusBadRequest,
			Code:    "invalid_charger_id",
			Message: "The charger ID is invalid.",
		}
	}

	err := ctx.Request.ParseMultipartForm(32 << 20) // 32MB
	if err != nil {
		return ChargerResponse{}, &auth.APIError{
			Status:  http.StatusBadRequest,
			Code:    "invalid_request",
			Message: "The request body is invalid.",
		}
	}

	var request UpdateChargerRequest
	if err := json.Unmarshal([]byte(ctx.Request.FormValue("data")), &request); err != nil {
		return ChargerResponse{}, &auth.APIError{
			Status:  http.StatusBadRequest,
			Code:    "invalid_request",
			Message: "The request body is invalid.",
		}
	}

	request = normalizeUpdateChargerRequest(request)
	if err := validateUpdateChargerRequest(request); err != nil {
		return ChargerResponse{}, err
	}

	cpoID := *principal.CPOID
	var record models.Charger

	err = service.database.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Preload("Connectors", func(tx *gorm.DB) *gorm.DB {
				return tx.Order("connector_number ASC")
			}).
			Preload("Hub").
			First(&record, "cpo_id = ? AND charger_id = ?", cpoID, chargerID).Error; err != nil {
			return mapChargerNotFound(err)
		}

		updates := map[string]any{}
		changedFields := models.JSONB{}

		if request.HubID != nil {
			var hub models.Hub
			if err := tx.First(&hub, "id = ? AND cpo_id = ?", *request.HubID, cpoID).Error; err != nil {
				return mapHubNotFound(err)
			}
			updates["hub_id"] = *request.HubID
			record.HubID = request.HubID
			changedFields["hub_id"] = *request.HubID
		}
		if request.Vendor != nil {
			updates["vendor"] = request.Vendor
			record.Vendor = request.Vendor
			changedFields["vendor"] = *request.Vendor
		}
		if request.Model != nil {
			updates["model"] = request.Model
			record.Model = request.Model
			changedFields["model"] = *request.Model
		}
		if request.SerialNumber != nil {
			updates["serial_number"] = *request.SerialNumber
			record.SerialNumber = *request.SerialNumber
			changedFields["serial_number"] = *request.SerialNumber
		}
		if request.MaxPowerKW != nil {
			updates["max_power_kw"] = *request.MaxPowerKW
			record.MaxPowerKW = *request.MaxPowerKW
			changedFields["max_power_kw"] = *request.MaxPowerKW
		}
		if request.ChargerName != nil {
			updates["charger_name"] = *request.ChargerName
			record.ChargerName = *request.ChargerName
			changedFields["charger_name"] = *request.ChargerName
		}
		if request.ChargerHostName != nil {
			updates["charger_host_name"] = *request.ChargerHostName
			record.ChargerHostName = *request.ChargerHostName
			changedFields["charger_host_name"] = *request.ChargerHostName
		}
		if request.ChargerHostPhoneNo != nil {
			updates["charger_host_phone_no"] = *request.ChargerHostPhoneNo
			record.ChargerHostPhoneNo = *request.ChargerHostPhoneNo
			changedFields["charger_host_phone_no"] = *request.ChargerHostPhoneNo
		}
		if request.ChargerType != nil {
			updates["charger_type"] = *request.ChargerType
			record.ChargerType = *request.ChargerType
			changedFields["charger_type"] = *request.ChargerType
		}
		if request.Segment != nil {
			updates["segment"] = *request.Segment
			record.Segment = *request.Segment
			changedFields["segment"] = *request.Segment
		}
		if request.SubSegment != nil {
			updates["sub_segment"] = *request.SubSegment
			record.SubSegment = *request.SubSegment
			changedFields["sub_segment"] = *request.SubSegment
		}
		if request.ChargerUseType != nil {
			updates["charger_use_type"] = *request.ChargerUseType
			record.ChargerUseType = *request.ChargerUseType
			changedFields["charger_use_type"] = *request.ChargerUseType
		}
		if request.NumberOfConnectors != nil {
			updates["number_of_connectors"] = *request.NumberOfConnectors
			record.NumberOfConnectors = *request.NumberOfConnectors
			changedFields["number_of_connectors"] = *request.NumberOfConnectors
		}
		if request.Parking != nil {
			updates["parking"] = *request.Parking
			record.Parking = *request.Parking
			changedFields["parking"] = *request.Parking
		}
		if request.Protocol != nil {
			updates["protocol"] = *request.Protocol
			record.Protocol = *request.Protocol
			changedFields["protocol"] = *request.Protocol
		}
		if request.TwentyFourSevenOpen != nil {
			updates["twenty_four_seven_open_status"] = *request.TwentyFourSevenOpen
			record.TwentyFourSevenOpen = *request.TwentyFourSevenOpen
			changedFields["twenty_four_seven_open_status"] = *request.TwentyFourSevenOpen
		}

		file, err := ctx.FormFile("charger_image")
		if err == nil {
			filename := uuid.New().String() + filepath.Ext(file.Filename)
			uploads := "uploads"
			if _, err := os.Stat(uploads); os.IsNotExist(err) {
				if err := os.Mkdir(uploads, 0755); err != nil {
					return err
				}
			}

			chargerImagePath := filepath.Join(uploads, filename)
			if err := ctx.SaveUploadedFile(file, chargerImagePath); err != nil {
				return err
			}
			updates["charger_image"] = chargerImagePath
			record.ChargerImage = chargerImagePath
			changedFields["charger_image"] = chargerImagePath
		}

		now := service.now()

		if request.Connectors != nil {
			if len(*request.Connectors) == 0 {
				return &auth.APIError{
					Status:  http.StatusBadRequest,
					Code:    "invalid_request",
					Message: "Connectors list cannot be empty.",
				}
			}

			existingByID := make(map[uuid.UUID]*models.Connector, len(record.Connectors))
			for i := range record.Connectors {
				conn := &record.Connectors[i]
				existingByID[conn.ID] = conn
			}

			seenIDs := map[uuid.UUID]struct{}{}

			for _, connectorReq := range *request.Connectors {
				if connectorReq.ID == uuid.Nil {
					// This is a new connector
					if connectorReq.ConnectorNumber == nil || *connectorReq.ConnectorNumber <= 0 {
						return invalid("connector_number", "Connector number must be greater than zero.")
					}
					if connectorReq.ConnectorType == nil || strings.TrimSpace(*connectorReq.ConnectorType) == "" {
						return invalid("connector_type", "Connector type is required.")
					}
					if connectorReq.ConnectorTotalCapacity == nil || *connectorReq.ConnectorTotalCapacity < 0 {
						return invalid("connector_total_capacity", "Connector total capacity cannot be negative.")
					}

					connectorRecord := models.Connector{
						ID:                     uuid.New(),
						CPOID:                  cpoID,
						ChargerID:              record.ID,
						ConnectorNumber:        *connectorReq.ConnectorNumber,
						ConnectorType:          *connectorReq.ConnectorType,
						ConnectorTotalCapacity: *connectorReq.ConnectorTotalCapacity,
						Status:                 constants.ChargerStatusActive,
						CreatedAt:              now,
						UpdatedAt:              now,
					}

					if err := tx.Create(&connectorRecord).Error; err != nil {
						return mapConnectorWriteError(err, "create connector")
					}
					record.Connectors = append(record.Connectors, connectorRecord)
					continue
				}
				if _, dup := seenIDs[connectorReq.ID]; dup {
					return &auth.APIError{
						Status:  http.StatusBadRequest,
						Code:    "duplicate_connector_id",
						Message: "Connector IDs in the request must be unique.",
					}
				}
				seenIDs[connectorReq.ID] = struct{}{}

				existing, ok := existingByID[connectorReq.ID]
				if !ok {
					return mapConnectorNotFound(gorm.ErrRecordNotFound)
				}

				connUpdates := map[string]any{}
				connChanged := false

				if connectorReq.ConnectorNumber != nil {
					if *connectorReq.ConnectorNumber <= 0 {
						return invalid("connector_number", "Connector number must be greater than zero.")
					}
					connUpdates["connector_number"] = *connectorReq.ConnectorNumber
					existing.ConnectorNumber = *connectorReq.ConnectorNumber
					connChanged = true
				}
				if connectorReq.ConnectorType != nil {
					connType := strings.TrimSpace(*connectorReq.ConnectorType)
					if connType == "" {
						return invalid("connector_type", "Connector type is required.")
					}
					connUpdates["connector_type"] = connType
					existing.ConnectorType = connType
					connChanged = true
				}
				if connectorReq.ConnectorTotalCapacity != nil {
					if *connectorReq.ConnectorTotalCapacity < 0 {
						return invalid("connector_total_capacity", "Connector total capacity cannot be negative.")
					}
					connUpdates["connector_total_capacity"] = *connectorReq.ConnectorTotalCapacity
					existing.ConnectorTotalCapacity = *connectorReq.ConnectorTotalCapacity
					connChanged = true
				}

				if !connChanged {
					continue
				}

				connUpdates["updated_at"] = now
				existing.UpdatedAt = now
				changedFields["connectors"] = len(*request.Connectors)

				if err := tx.Model(&models.Connector{}).
					Where("id = ? AND cpo_id = ? AND charger_id = ?", existing.ID, cpoID, record.ID).
					Updates(connUpdates).Error; err != nil {
					return mapConnectorWriteError(err, "update connector")
				}
			}
		}

		if len(changedFields) == 0 {
			return &auth.APIError{
				Status:  http.StatusBadRequest,
				Code:    "invalid_request",
				Message: "At least one charger field must be supplied.",
			}
		}

		updates["updated_at"] = now
		record.UpdatedAt = now

		if len(updates) > 1 {
			if err := tx.Model(&models.Charger{}).
				Where("id = ?", record.ID).
				Updates(updates).Error; err != nil {
				return mapChargerWriteError(err, "update charger")
			}
		}

		return writeAudit(
			tx,
			principal.UserID,
			cpoID,
			"CHARGER_UPDATED",
			models.JSONB{
				"charger_id":     record.ChargerID,
				"changed_fields": changedFields,
			},
			now,
		)
	})
	if err != nil {
		return ChargerResponse{}, err
	}

	if record.HubID != nil {
		var hub models.Hub
		if err := service.database.WithContext(ctx).First(&hub, "id = ?", *record.HubID).Error; err == nil {
			record.Hub = &hub
		}
	}

	return service.chargerView(record, principal), nil
}

func (service *Service) DeleteCharger(
	ctx context.Context,
	principal auth.Principal,
	chargerID string,
) error {
	if err := requireCPOContext(principal); err != nil {
		return err
	}

	chargerID = normalizeChargerID(chargerID)
	if !chargerIDPattern.MatchString(chargerID) {
		return &auth.APIError{
			Status:  http.StatusBadRequest,
			Code:    "invalid_charger_id",
			Message: "The charger ID is invalid.",
		}
	}

	cpoID := *principal.CPOID
	return service.database.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var record models.Charger
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Preload("Connectors").
			First(&record, "cpo_id = ? AND charger_id = ?", cpoID, chargerID).Error; err != nil {
			return mapChargerNotFound(err)
		}

		if err := tx.Where("charger_id = ?", record.ID).Delete(&models.Connector{}).Error; err != nil {
			return mapChargerDeleteError(err)
		}

		if err := tx.Delete(&record).Error; err != nil {
			return mapChargerDeleteError(err)
		}

		return writeAudit(
			tx,
			principal.UserID,
			cpoID,
			"CHARGER_DELETED",
			models.JSONB{
				"charger_id":    record.ChargerID,
				"ocpp_identity": record.OCPPIdentity,
				"hub_id":        record.HubID,
			},
			service.now(),
		)
	})
}

func normalizeCreateChargerRequest(request CreateChargerRequest) CreateChargerRequest {
	request.Vendor = strings.TrimSpace(request.Vendor)
	request.Model = strings.TrimSpace(request.Model)
	request.SerialNumber = strings.TrimSpace(request.SerialNumber)

	for i := range request.Connectors {
		request.Connectors[i].ConnectorType = strings.TrimSpace(request.Connectors[i].ConnectorType)
	}
	return request
}

func normalizeUpdateChargerRequest(request UpdateChargerRequest) UpdateChargerRequest {
	request.Vendor = trimOptionalString(request.Vendor)
	request.Model = trimOptionalString(request.Model)
	request.SerialNumber = trimOptionalString(request.SerialNumber)

	if request.Connectors != nil {
		connectors := *request.Connectors
		for i := range connectors {
			if connectors[i].ConnectorType != nil {
				value := strings.TrimSpace(*connectors[i].ConnectorType)
				connectors[i].ConnectorType = &value
			}
		}
		request.Connectors = &connectors
	}

	return request
}

func normalizeChargerID(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func validateCreateChargerRequest(request CreateChargerRequest) error {
	if len(request.Vendor) > 100 {
		return invalid("vendor", "Vendor must not exceed 100 characters.")
	}
	if len(request.Model) > 100 {
		return invalid("model", "Model must not exceed 100 characters.")
	}
	if request.SerialNumber == "" || len(request.SerialNumber) > 100 {
		return invalid("serial_number", "Serial number is required and must not exceed 100 characters.")
	}
	if request.MaxPowerKW < 0 {
		return invalid("max_power_kw", "Max power kW must not be negative.")
	}
	if len(request.Connectors) == 0 {
		return invalid("connectors", "At least one connector is required.")
	}

	seenNumbers := map[int]struct{}{}
	for _, connector := range request.Connectors {
		if connector.ConnectorNumber <= 0 {
			return invalid("connector_number", "Connector number must be greater than zero.")
		}
		if strings.TrimSpace(connector.ConnectorType) == "" || len(connector.ConnectorType) > 50 {
			return invalid("connector_type", "Connector type is required and must not exceed 50 characters.")
		}
		if _, dup := seenNumbers[connector.ConnectorNumber]; dup {
			return invalid("connector_number", "Connector numbers must be unique within a charger.")
		}
		seenNumbers[connector.ConnectorNumber] = struct{}{}
	}
	return nil
}

func validateUpdateChargerRequest(request UpdateChargerRequest) error {
	if request.HubID == nil &&
		request.Vendor == nil &&
		request.Model == nil &&
		request.SerialNumber == nil &&
		request.MaxPowerKW == nil &&
		request.Connectors == nil {
		return invalid("charger", "At least one charger field must be supplied.")
	}

	if request.Vendor != nil && len(*request.Vendor) > 100 {
		return invalid("vendor", "Vendor must not exceed 100 characters.")
	}
	if request.Model != nil && len(*request.Model) > 100 {
		return invalid("model", "Model must not exceed 100 characters.")
	}
	if request.SerialNumber != nil && (*request.SerialNumber == "" || len(*request.SerialNumber) > 100) {
		return invalid("serial_number", "Serial number must not exceed 100 characters.")
	}
	if request.MaxPowerKW != nil && *request.MaxPowerKW < 0 {
		return invalid("max_power_kw", "Max power kW must not be negative.")
	}

	if request.Connectors != nil {
		if len(*request.Connectors) == 0 {
			return invalid("connectors", "Connectors list cannot be empty when provided.")
		}

		seenIDs := map[uuid.UUID]struct{}{}
		for _, connector := range *request.Connectors {
			if connector.ID == uuid.Nil {
				return invalid("connector_id", "Connector ID is required.")
			}
			if _, dup := seenIDs[connector.ID]; dup {
				return invalid("connector_id", "Connector IDs in the request must be unique.")
			}
			seenIDs[connector.ID] = struct{}{}

			changed := false
			if connector.ConnectorNumber != nil {
				if *connector.ConnectorNumber <= 0 {
					return invalid("connector_number", "Connector number must be greater than zero.")
				}
				changed = true
			}
			if connector.ConnectorType != nil {
				if strings.TrimSpace(*connector.ConnectorType) == "" || len(*connector.ConnectorType) > 50 {
					return invalid("connector_type", "Connector type must not exceed 50 characters.")
				}
				changed = true
			}
			if !changed {
				return invalid("connectors", "At least one connector field must be supplied for each connector.")
			}
		}
	}

	return nil
}

func requireCPOContext(principal auth.Principal) error {
	if principal.Scope != constants.AuthScopeCPO {
		return &auth.APIError{
			Status:  http.StatusForbidden,
			Code:    "forbidden",
			Message: "CPO access is required.",
		}
	}
	if principal.CPOID == nil {
		return &auth.APIError{
			Status:  http.StatusForbidden,
			Code:    "forbidden",
			Message: "CPO tenant context is required.",
		}
	}
	// Capability middleware has already loaded the current active membership
	// from PostgreSQL. This legacy service guard remains a tenant-scope defense
	// for direct callers, but must not reintroduce an ADMIN-only bypass over the
	// route's precise capability decision.
	return nil
}

func forbiddenCPOAccess() error {
	return &auth.APIError{Status: http.StatusForbidden, Code: "forbidden", Message: "An active CPO membership is required."}
}

func (service *Service) cpoOnboardingActionURL(cpoID uuid.UUID) (string, error) {
	actionURL, err := config.BuildActionURL(service.frontend.CPOOnboardingTemplate, map[string]string{"cpo_id": cpoID.String()}, "cpo_id")
	if err != nil {
		return "", fmt.Errorf("build CPO onboarding action URL: %w", err)
	}
	return actionURL, nil
}

func generateUniqueChargerIDTx(tx *gorm.DB) (string, error) {
	for i := 0; i < 32; i++ {
		candidate, err := security.RandomHex(3)
		if err != nil {
			return "", err
		}
		candidate = strings.ToLower(candidate)
		if !chargerIDPattern.MatchString(candidate) {
			continue
		}

		var existing models.Charger
		err = tx.Select("id").Where("charger_id = ?", candidate).Take(&existing).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return candidate, nil
		}
		if err != nil {
			return "", fmt.Errorf("check charger id uniqueness: %w", err)
		}
	}
	return "", &auth.APIError{
		Status:  http.StatusConflict,
		Code:    "charger_conflict",
		Message: "Unable to generate a unique charger ID.",
	}
}

func mapChargerNotFound(err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return &auth.APIError{
			Status:  http.StatusNotFound,
			Code:    "charger_not_found",
			Message: "The charger was not found.",
		}
	}
	return fmt.Errorf("load charger: %w", err)
}

func mapConnectorNotFound(err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return &auth.APIError{
			Status:  http.StatusNotFound,
			Code:    "connector_not_found",
			Message: "The connector was not found.",
		}
	}
	return fmt.Errorf("load connector: %w", err)
}

func mapHubNotFound(err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return &auth.APIError{
			Status:  http.StatusNotFound,
			Code:    "hub_not_found",
			Message: "The hub was not found.",
		}
	}
	return fmt.Errorf("load hub: %w", err)
}

func mapChargerWriteError(err error, operation string) error {
	var postgresError *pgconn.PgError
	if errors.As(err, &postgresError) {
		switch postgresError.Code {
		case "23505":
			return &auth.APIError{
				Status:  http.StatusConflict,
				Code:    "charger_conflict",
				Message: "The charger ID, OCPP identity, or related unique value already exists.",
			}
		case "23503":
			return &auth.APIError{
				Status:  http.StatusConflict,
				Code:    "charger_conflict",
				Message: "The charger references an invalid related record.",
			}
		case "23P01":
			return tariffTemporalConflict()
		}
	}
	return fmt.Errorf("%s: %w", operation, err)
}

func mapConnectorWriteError(err error, operation string) error {
	var postgresError *pgconn.PgError
	if errors.As(err, &postgresError) {
		switch postgresError.Code {
		case "23505":
			return &auth.APIError{
				Status:  http.StatusConflict,
				Code:    "connector_conflict",
				Message: "The connector already exists or conflicts with another record.",
			}
		case "23503":
			return &auth.APIError{
				Status:  http.StatusConflict,
				Code:    "connector_conflict",
				Message: "The connector references an invalid related record.",
			}
		}
	}
	return fmt.Errorf("%s: %w", operation, err)
}

func mapChargerDeleteError(err error) error {
	var postgresError *pgconn.PgError
	if errors.As(err, &postgresError) && (postgresError.Code == "23001" || postgresError.Code == "23503") {
		return &auth.APIError{
			Status:  http.StatusConflict,
			Code:    "charger_in_use",
			Message: "The charger cannot be deleted because it has dependent records.",
		}
	}
	return fmt.Errorf("delete charger: %w", err)
}

func (service *Service) chargerView(record models.Charger, principal auth.Principal) ChargerResponse {
	connectorsView := make([]ConnectorView, 0, len(record.Connectors))

	for _, conn := range record.Connectors {
		connectorsView = append(connectorsView, ConnectorView{
			ID:                     conn.ID,
			CPOID:                  conn.CPOID,
			ChargerID:              conn.ChargerID,
			ConnectorNumber:        conn.ConnectorNumber,
			ConnectorType:          conn.ConnectorType,
			ConnectorTotalCapacity: conn.ConnectorTotalCapacity,
			Status:                 conn.Status,
			CreatedAt:              conn.CreatedAt,
			UpdatedAt:              conn.UpdatedAt,
		})
	}

	var hubName *string
	if record.Hub != nil {
		hubName = &record.Hub.Name
	}

	return ChargerResponse{
		ChargerView: ChargerView{
			ID:                      record.ID,
			CPOID:                   record.CPOID,
			HubID:                   record.HubID,
			HubName:                 hubName,
			ChargerID:               record.ChargerID,
			OCPPIdentity:            record.OCPPIdentity,
			Vendor:                  record.Vendor,
			Model:                   record.Model,
			CustomerVisibility:      record.CustomerVisibility,
			SerialNumber:            record.SerialNumber,
			MaxPowerKW:              record.MaxPowerKW,
			Status:                  record.Status,
			OCPPVersion:             record.OCPPVersion,
			LastSeenAt:              record.LastSeenAt,
			ChargerName:             record.ChargerName,
			ChargerHostName:         record.ChargerHostName,
			ChargerHostPhoneNo:      record.ChargerHostPhoneNo,
			ChargerType:             record.ChargerType,
			Segment:                 record.Segment,
			SubSegment:              record.SubSegment,
			ChargerImage:            record.ChargerImage,
			ChargerUseType:          record.ChargerUseType,
			NumberOfConnectors:      record.NumberOfConnectors,
			Parking:                 record.Parking,
			Protocol:                record.Protocol,
			TwentyFourSevenOpen:     record.TwentyFourSevenOpen,
			Connectors:              connectorsView,
			ChargerConnectionURLWS:  fmt.Sprintf("ws://%s/%s", service.chargerConnectionURL, record.OCPPIdentity),
			ChargerConnectionURLWSS: fmt.Sprintf("wss://%s/%s", service.chargerConnectionURL, record.OCPPIdentity),
			Assigned:                record.HubID != nil,
			CreatedAt:               record.CreatedAt,
			UpdatedAt:               record.UpdatedAt,
		},
		Email: principal.User.Email,
	}
}

func (service *Service) ListChargers(
	ctx context.Context,
	principal auth.Principal,
	query TenantListQuery,
) (ChargerListResponse, error) {
	if err := requireCPOContext(principal); err != nil {
		return ChargerListResponse{}, err
	}
	query, err := validateTenantListQuery(query)
	if err != nil {
		return ChargerListResponse{}, err
	}

	databaseQuery := service.database.WithContext(ctx).
		Where("cpo_id = ?", *principal.CPOID)
	if query.Before != nil {
		databaseQuery = databaseQuery.Where(
			"(created_at, id) < (?, ?)",
			*query.Before,
			*query.BeforeID,
		)
	}
	var chargers []models.Charger
	if err := databaseQuery.
		Preload("Connectors", func(tx *gorm.DB) *gorm.DB {
			return tx.Order("connector_number ASC")
		}).
		Preload("Hub").
		Order("created_at DESC, id DESC").
		Limit(query.Limit + 1).
		Find(&chargers).Error; err != nil {
		return ChargerListResponse{}, fmt.Errorf("list chargers: %w", err)
	}

	hasMore := len(chargers) > query.Limit
	if hasMore {
		chargers = chargers[:query.Limit]
	}
	responses := make([]ChargerResponse, len(chargers))
	for i, charger := range chargers {
		responses[i] = service.chargerView(charger, principal)
	}

	if service.liveOperations != nil {
		chargerIDs := make([]uuid.UUID, len(chargers))
		for i, charger := range chargers {
			chargerIDs[i] = charger.ID
		}
		liveByCharger, err := service.liveOperations.GetChargerDetails(ctx, *principal.CPOID, chargerIDs)
		if err == nil {
			for i, charger := range chargers {
				if live, ok := liveByCharger[charger.ID]; ok {
					responses[i].Live = &live
				}
			}
		}
	}

	result := ChargerListResponse{Chargers: responses, HasMore: hasMore}
	if hasMore && len(chargers) > 0 {
		nextBefore := chargers[len(chargers)-1].CreatedAt
		nextBeforeID := chargers[len(chargers)-1].ID
		result.NextBefore = &nextBefore
		result.NextBeforeID = &nextBeforeID
	}
	return result, nil
}

func validateTenantListQuery(query TenantListQuery) (TenantListQuery, error) {
	if query.Limit == 0 {
		query.Limit = defaultListLimit
	}
	if query.Limit < 1 || query.Limit > maxListLimit {
		return TenantListQuery{}, invalid("limit", "Limit must be between 1 and 200.")
	}
	if (query.Before == nil) != (query.BeforeID == nil) {
		return TenantListQuery{}, invalid(
			"cursor",
			"before and before_id must be supplied together.",
		)
	}
	return query, nil
}

func (service *Service) CreateHub(
	ctx context.Context,
	principal auth.Principal,
	request CreateHubRequest,
) (HubView, error) {
	if err := requireCPOContext(principal); err != nil {
		return HubView{}, err
	}

	request = normalizeCreateHubRequest(request)
	if err := validateCreateHubRequest(request); err != nil {
		return HubView{}, err
	}

	open24Hours := true
	if request.Open24Hours != nil {
		open24Hours = *request.Open24Hours
	}

	var sanctionLoad float64
	if request.SanctionLoad != nil {
		sanctionLoad = *request.SanctionLoad
	}
	customerVisible := false
	if request.CustomerVisible != nil {
		customerVisible = *request.CustomerVisible
	}
	if err := validateInitialHubVisibility(customerVisible); err != nil {
		return HubView{}, err
	}

	cpoID := *principal.CPOID
	var record models.Hub

	err := service.database.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if len(request.ChargerIDs) > 0 {
			var chargers []models.Charger
			if err := tx.Where("id IN ? AND cpo_id = ?", request.ChargerIDs, cpoID).Find(&chargers).Error; err != nil {
				return fmt.Errorf("could not look up chargers: %w", err)
			}
			if len(chargers) != len(request.ChargerIDs) {
				return &auth.APIError{
					Status:  http.StatusNotFound,
					Code:    "charger_not_found",
					Message: "One or more chargers could not be found.",
				}
			}
			for _, charger := range chargers {
				if charger.HubID != nil {
					return &auth.APIError{
						Status:  http.StatusConflict,
						Code:    "charger_already_in_hub",
						Message: fmt.Sprintf("Charger %s is already in a hub.", charger.ChargerID),
					}
				}
			}
		}

		now := service.now()
		record = models.Hub{
			ID:              uuid.New(),
			CPOID:           cpoID,
			Name:            request.Name,
			Address:         request.Address,
			Latitude:        *request.Latitude,
			State:           request.State,
			Longitude:       *request.Longitude,
			Open24Hours:     open24Hours,
			SanctionLoad:    sanctionLoad,
			CustomerVisible: customerVisible,
			CreatedAt:       now,
			UpdatedAt:       now,
		}

		if err := tx.Create(&record).Error; err != nil {
			return mapHubWriteError(err, "create hub")
		}

		if len(request.ChargerIDs) > 0 {
			if err := tx.Model(&models.Charger{}).
				Where("id IN ?", request.ChargerIDs).
				Updates(map[string]any{
					"hub_id":     record.ID,
					"updated_at": now,
				}).Error; err != nil {
				return fmt.Errorf("could not assign chargers to hub: %w", err)
			}
		}

		return writeAudit(
			tx,
			principal.UserID,
			cpoID,
			"HUB_CREATED",
			models.JSONB{
				"hub_id":            record.ID,
				"name":              record.Name,
				"open_24_hours":     record.Open24Hours,
				"sanction_load":     record.SanctionLoad,
				"customer_visible":  record.CustomerVisible,
				"chargers_assigned": len(request.ChargerIDs),
			},
			now,
		)
	})
	if err != nil {
		return HubView{}, err
	}

	return hubView(record), nil
}

func (service *Service) ListHubs(
	ctx context.Context,
	principal auth.Principal,
	query TenantListQuery,
) (HubListResponse, error) {
	if err := requireCPOContext(principal); err != nil {
		return HubListResponse{}, err
	}
	query, err := validateTenantListQuery(query)
	if err != nil {
		return HubListResponse{}, err
	}
	databaseQuery := service.database.WithContext(ctx).
		Where("cpo_id = ?", *principal.CPOID)
	if query.Before != nil {
		databaseQuery = databaseQuery.Where(
			"(created_at, id) < (?, ?)",
			*query.Before,
			*query.BeforeID,
		)
	}
	var records []models.Hub
	if err := databaseQuery.
		Order("created_at DESC, id DESC").
		Limit(query.Limit + 1).
		Find(&records).Error; err != nil {
		return HubListResponse{}, fmt.Errorf("list hubs: %w", err)
	}
	hasMore := len(records) > query.Limit
	if hasMore {
		records = records[:query.Limit]
	}
	hubs := make([]HubView, 0, len(records))
	for _, record := range records {
		hubs = append(hubs, hubView(record))
	}
	response := HubListResponse{Hubs: hubs, HasMore: hasMore}
	if hasMore && len(records) > 0 {
		nextBefore := records[len(records)-1].CreatedAt
		nextBeforeID := records[len(records)-1].ID
		response.NextBefore = &nextBefore
		response.NextBeforeID = &nextBeforeID
	}
	return response, nil
}

func (service *Service) GetHub(
	ctx context.Context,
	principal auth.Principal,
	hubID uuid.UUID,
	query TenantListQuery,
) (HubResponse, error) {
	if err := requireCPOContext(principal); err != nil {
		return HubResponse{}, err
	}

	var hub models.Hub
	if err := service.database.WithContext(ctx).
		First(&hub, "cpo_id = ? AND id = ?", *principal.CPOID, hubID).Error; err != nil {
		return HubResponse{}, mapHubNotFound(err)
	}

	query, err := validateTenantListQuery(query)
	if err != nil {
		return HubResponse{}, err
	}

	databaseQuery := service.database.WithContext(ctx).
		Where("cpo_id = ? AND hub_id = ?", *principal.CPOID, hubID)
	if query.Before != nil {
		databaseQuery = databaseQuery.Where(
			"(created_at, id) < (?, ?)",
			*query.Before,
			*query.BeforeID,
		)
	}

	var chargers []models.Charger
	if err := databaseQuery.
		Preload("Connectors", func(tx *gorm.DB) *gorm.DB {
			return tx.Order("connector_number ASC")
		}).
		Preload("Hub").
		Order("created_at DESC, id DESC").
		Limit(query.Limit + 1).
		Find(&chargers).Error; err != nil {
		return HubResponse{}, fmt.Errorf("list chargers for hub: %w", err)
	}

	hasMore := len(chargers) > query.Limit
	if hasMore {
		chargers = chargers[:query.Limit]
	}

	chargerResponses := make([]ChargerResponse, 0, len(chargers))
	for _, charger := range chargers {
		chargerResponses = append(chargerResponses, service.chargerView(charger, principal))
	}

	chargerListResponse := ChargerListResponse{
		Chargers: chargerResponses,
		HasMore:  hasMore,
	}

	if hasMore && len(chargers) > 0 {
		nextBefore := chargers[len(chargers)-1].CreatedAt
		nextBeforeID := chargers[len(chargers)-1].ID
		chargerListResponse.NextBefore = &nextBefore
		chargerListResponse.NextBeforeID = &nextBeforeID
	}

	return HubResponse{
		ID:              hub.ID,
		CPOID:           hub.CPOID,
		Name:            hub.Name,
		Address:         hub.Address,
		State:           hub.State,
		Latitude:        hub.Latitude,
		Longitude:       hub.Longitude,
		Open24Hours:     hub.Open24Hours,
		SanctionLoad:    hub.SanctionLoad,
		CustomerVisible: hub.CustomerVisible,
		CreatedAt:       hub.CreatedAt,
		UpdatedAt:       hub.UpdatedAt,
		Chargers:        &chargerListResponse,
	}, nil
}

func (service *Service) UpdateHub(
	ctx context.Context,
	principal auth.Principal,
	hubID uuid.UUID,
	request UpdateHubRequest,
) (HubView, error) {
	if err := requireCPOContext(principal); err != nil {
		return HubView{}, err
	}

	request = normalizeUpdateHubRequest(request)
	if err := validateUpdateHubRequest(request); err != nil {
		return HubView{}, err
	}

	cpoID := *principal.CPOID
	var record models.Hub
	changed := false

	err := service.database.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := lockCPOGSTRelations(tx, cpoID); err != nil {
			return err
		}
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			First(&record, "cpo_id = ? AND id = ?", cpoID, hubID).Error; err != nil {
			return mapHubNotFound(err)
		}

		updates := map[string]any{}
		changedFields := models.JSONB{}

		if request.Name != nil && record.Name != *request.Name {
			updates["name"] = *request.Name
			changedFields["name"] = *request.Name
		}
		if request.Address != nil && record.Address != *request.Address {
			updates["address"] = *request.Address
			changedFields["address"] = *request.Address
		}
		if request.Latitude != nil && record.Latitude != *request.Latitude {
			updates["latitude"] = *request.Latitude
			changedFields["latitude"] = *request.Latitude
		}
		if request.Longitude != nil && record.Longitude != *request.Longitude {
			updates["longitude"] = *request.Longitude
			changedFields["longitude"] = *request.Longitude
		}
		if request.Open24Hours != nil && record.Open24Hours != *request.Open24Hours {
			updates["open_24_hours"] = *request.Open24Hours
			changedFields["open_24_hours"] = *request.Open24Hours
		}
		if request.SanctionLoad != nil && record.SanctionLoad != *request.SanctionLoad {
			updates["sanction_load"] = *request.SanctionLoad
			changedFields["sanction_load"] = *request.SanctionLoad
		}
		if request.CustomerVisible != nil && record.CustomerVisible != *request.CustomerVisible {
			updates["customer_visible"] = *request.CustomerVisible
			changedFields["customer_visible"] = *request.CustomerVisible
		}
		if request.State != nil && record.State != *request.State {
			updates["state"] = *request.State
			changedFields["state"] = *request.State
		}

		if len(changedFields) == 0 {
			return nil
		}
		changed = true

		for key, value := range changedFields {
			switch key {
			case "name":
				record.Name = value.(string)
			case "address":
				record.Address = value.(string)
			case "latitude":
				record.Latitude = value.(float64)
			case "longitude":
				record.Longitude = value.(float64)
			case "open_24_hours":
				record.Open24Hours = value.(bool)
			case "sanction_load":
				record.SanctionLoad = value.(float64)
			case "customer_visible":
				record.CustomerVisible = value.(bool)
			case "state":
				record.State = value.(constants.IndianState)
			}
		}
		if record.GSTID != nil {
			var gst models.GST
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
				First(&gst, "cpo_id = ? AND id = ?", cpoID, *record.GSTID).Error; err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					return &auth.APIError{Status: http.StatusConflict, Code: "invalid_gst_for_hub", Message: "The Hub has no valid assigned GST profile."}
				}
				return fmt.Errorf("load Hub GST for validation: %w", err)
			}
			if err := validateGSTForHub(record, gst); err != nil {
				return err
			}
		}
		if record.CustomerVisible {
			if err := service.validateVisibleHubTariffFloor(tx, cpoID, record.ID); err != nil {
				return err
			}
		}

		now := service.now()
		updates["updated_at"] = now
		record.UpdatedAt = now
		if err := tx.Model(&models.Hub{}).
			Where("id = ?", record.ID).
			Updates(updates).Error; err != nil {
			return mapHubWriteError(err, "update hub")
		}

		return writeAudit(
			tx,
			principal.UserID,
			cpoID,
			"HUB_UPDATED",
			models.JSONB{
				"hub_id":         record.ID,
				"changed_fields": changedFields,
			},
			now,
		)
	})
	if err != nil {
		return HubView{}, err
	}
	if !changed {
		if err := service.database.WithContext(ctx).
			First(&record, "cpo_id = ? AND id = ?", cpoID, hubID).Error; err != nil {
			return HubView{}, mapHubNotFound(err)
		}
	}

	return hubView(record), nil
}

func (service *Service) DeleteHub(
	ctx context.Context,
	principal auth.Principal,
	hubID uuid.UUID,
) error {
	if err := requireCPOContext(principal); err != nil {
		return err
	}

	return service.database.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var tariffCount int64
		if err := tx.Model(&models.Tariff{}).
			Where("hub_id = ? AND cpo_id = ?", hubID, *principal.CPOID).
			Count(&tariffCount).Error; err != nil {
			return fmt.Errorf("checking for tariffs associated with hub: %w", err)
		}
		if tariffCount > 0 {
			return &auth.APIError{
				Status:  http.StatusConflict,
				Code:    "hub_has_tariffs",
				Message: "The hub cannot be deleted because it has associated tariffs.",
			}
		}

		if err := tx.Model(&models.Charger{}).
			Where("hub_id = ? AND cpo_id = ?", hubID, *principal.CPOID).
			Updates(map[string]any{"hub_id": nil, "customer_visibility": false, "status": constants.ChargerStatusInactive, "updated_at": service.now()}).Error; err != nil {
			return fmt.Errorf("disassociating chargers from hub: %w", err)
		}

		if err := tx.Exec("DELETE FROM user_group_hubs WHERE hub_id = ?", hubID).Error; err != nil {
			return fmt.Errorf("deleting user group hub associations: %w", err)
		}

		if err := tx.Exec("DELETE FROM customer_favorite_hubs WHERE hub_id = ?", hubID).Error; err != nil {
			return fmt.Errorf("deleting customer favorite hub associations: %w", err)
		}

		if result := tx.Delete(&models.Hub{}, "id = ? AND cpo_id = ?", hubID, *principal.CPOID); result.Error != nil {
			return fmt.Errorf("deleting hub: %w", result.Error)
		} else if result.RowsAffected == 0 {
			return mapHubNotFound(gorm.ErrRecordNotFound)
		}

		if err := writeAudit(
			tx,
			principal.UserID,
			*principal.CPOID,
			"HUB_DELETED",
			models.JSONB{
				"hub_id": hubID,
			},
			service.now(),
		); err != nil {
			return err
		}

		hubResourceID := hubID.String()
		_, err := service.events.Emit(tx, platformops.EventInput{
			Type:         "cpo.hub.deleted",
			ActorUserID:  &principal.User.ID,
			ResourceType: "HUB",
			ResourceID:   &hubResourceID,
			Data:         models.JSONB{},
		})
		return err
	})
}

func (service *Service) ListCustomers(
	ctx context.Context,
	principal auth.Principal,
	query CPOAdminCustomerListQuery,
) (CPOAdminCustomerListResponse, error) {
	if err := requireCPOContext(principal); err != nil {
		return CPOAdminCustomerListResponse{}, err
	}

	query.Search = strings.TrimSpace(query.Search)
	if len(query.Search) > maxSearchLength {
		return CPOAdminCustomerListResponse{}, invalid(
			"q",
			"Search text must not exceed 200 characters.",
		)
	}
	if query.Limit == 0 {
		query.Limit = defaultListLimit
	}
	if query.Limit < 1 || query.Limit > maxListLimit {
		return CPOAdminCustomerListResponse{}, invalid(
			"limit",
			"Limit must be between 1 and 200.",
		)
	}
	if query.Status != nil && !query.Status.Valid() {
		return CPOAdminCustomerListResponse{}, invalid(
			"status",
			"Status must be ACTIVE or BLOCKED.",
		)
	}
	if (query.Before == nil) != (query.BeforeID == nil) {
		return CPOAdminCustomerListResponse{}, invalid(
			"cursor",
			"before and before_id must be supplied together.",
		)
	}

	cpoID := *principal.CPOID
	databaseQuery := service.database.WithContext(ctx).Model(&models.Customer{}).
		Where("cpo_id = ?", cpoID)

	if query.Search != "" {
		search := strings.ToLower(query.Search)
		databaseQuery = databaseQuery.Where(
			`strpos(lower(email), ?) > 0 OR strpos(lower(full_name), ?) > 0 OR strpos(lower(coalesce(phone, '')), ?) > 0`,
			search, search, search,
		)
	}
	if query.Status != nil {
		databaseQuery = databaseQuery.Where("status = ?", *query.Status)
	}
	if query.Before != nil {
		databaseQuery = databaseQuery.Where(
			"(created_at, id) < (?, ?)",
			*query.Before,
			*query.BeforeID,
		)
	}

	var records []models.Customer
	if err := databaseQuery.
		Order("created_at DESC, id DESC").
		Limit(query.Limit + 1).
		Find(&records).Error; err != nil {
		return CPOAdminCustomerListResponse{}, fmt.Errorf("list CPO customers: %w", err)
	}

	hasMore := len(records) > query.Limit
	if hasMore {
		records = records[:query.Limit]
	}

	aggregates, err := service.getCustomerAggregatesByCPO(ctx, cpoID)
	if err != nil {
		return CPOAdminCustomerListResponse{}, fmt.Errorf("getting customer aggregates: %w", err)
	}

	result := make([]CPOAdminCustomerView, 0, len(records))
	for _, record := range records {
		customerAggregates := aggregates[record.ID]
		result = append(result, cpoAdminCustomerView(record, &customerAggregates))
	}

	response := CPOAdminCustomerListResponse{Customers: result, HasMore: hasMore}
	if hasMore && len(records) > 0 {
		nextBefore := records[len(records)-1].CreatedAt
		nextBeforeID := records[len(records)-1].ID
		response.NextBefore = &nextBefore
		response.NextBeforeID = &nextBeforeID
	}
	return response, nil
}

type CustomerAggregates struct {
	TotalUsageKWh decimal.Decimal `gorm:"column:total_usage_kwh"`
	SessionCount  int64           `gorm:"column:session_count"`
	WalletBalance decimal.Decimal `gorm:"column:wallet_balance"`
}

type customerAggregateResult struct {
	CustomerID    uuid.UUID       `gorm:"column:customer_id"`
	TotalUsageKWh decimal.Decimal `gorm:"column:total_usage_kwh"`
	SessionCount  int64           `gorm:"column:session_count"`
}

func (service *Service) GetCustomer(
	ctx context.Context,
	principal auth.Principal,
	customerID uuid.UUID,
) (CPOAdminCustomerView, error) {
	if err := requireCPOContext(principal); err != nil {
		return CPOAdminCustomerView{}, err
	}

	var record models.Customer
	if err := service.database.WithContext(ctx).
		First(&record, "cpo_id = ? AND id = ?", *principal.CPOID, customerID).Error; err != nil {
		return CPOAdminCustomerView{}, mapCustomerNotFound(err)
	}

	aggregates, err := service.getCustomerAggregates(ctx, customerID)
	if err != nil {
		return CPOAdminCustomerView{}, fmt.Errorf("getting customer aggregates: %w", err)
	}

	return cpoAdminCustomerView(record, aggregates), nil
}

func (service *Service) getCustomerAggregates(ctx context.Context, customerID uuid.UUID) (*CustomerAggregates, error) {
	var sessionAgg struct {
		TotalUsageKWh decimal.Decimal
		SessionCount  int64
	}
	err := service.database.WithContext(ctx).Model(&models.ChargingSession{}).
		Select("COALESCE(SUM(total_kwh), 0) as total_usage_kwh, COUNT(*) as session_count").
		Where("customer_id = ? AND status IN (?, ?)", customerID,
			constants.SessionStatusCompleted, constants.SessionStatusReconciliationRequired).
		Scan(&sessionAgg).Error
	if err != nil {
		return nil, err
	}

	var walletBalance decimal.Decimal
	err = service.database.WithContext(ctx).Model(&models.Wallet{}).
		Select("COALESCE(balance, 0) as balance").
		Where("customer_id = ?", customerID).
		Scan(&walletBalance).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}

	return &CustomerAggregates{
		TotalUsageKWh: sessionAgg.TotalUsageKWh,
		SessionCount:  sessionAgg.SessionCount,
		WalletBalance: walletBalance,
	}, nil
}

func (service *Service) getCustomerAggregatesByCPO(ctx context.Context, cpoID uuid.UUID) (map[uuid.UUID]CustomerAggregates, error) {
	var customers []models.Customer
	if err := service.database.WithContext(ctx).Model(&models.Customer{}).Where("cpo_id = ?", cpoID).Find(&customers).Error; err != nil {
		return nil, err
	}

	customerIDs := make([]uuid.UUID, len(customers))
	for i, c := range customers {
		customerIDs[i] = c.ID
	}

	var sessionAggregates []customerAggregateResult
	if len(customerIDs) > 0 {
		err := service.database.WithContext(ctx).Model(&models.ChargingSession{}).
			Select("customer_id, COALESCE(SUM(total_kwh), 0) as total_usage_kwh, COUNT(*) as session_count").
			Where("cpo_id = ? AND customer_id IN (?) AND status IN (?, ?)", cpoID, customerIDs, constants.SessionStatusCompleted, constants.SessionStatusReconciliationRequired).
			Group("customer_id").
			Scan(&sessionAggregates).Error
		if err != nil {
			return nil, err
		}
	}

	type WalletResult struct {
		CustomerID    uuid.UUID
		WalletBalance decimal.Decimal
	}

	var walletAggregates []WalletResult
	if len(customerIDs) > 0 {
		err := service.database.WithContext(ctx).Model(&models.Wallet{}).
			Select("customer_id, balance as wallet_balance").
			Where("customer_id IN (?)", customerIDs).
			Scan(&walletAggregates).Error

		if err != nil {
			return nil, err
		}
	}

	result := make(map[uuid.UUID]CustomerAggregates)
	for _, customer := range customers {
		result[customer.ID] = CustomerAggregates{}
	}

	for _, agg := range sessionAggregates {
		if entry, ok := result[agg.CustomerID]; ok {
			entry.TotalUsageKWh = agg.TotalUsageKWh
			entry.SessionCount = agg.SessionCount
			result[agg.CustomerID] = entry
		}
	}

	for _, agg := range walletAggregates {
		if entry, ok := result[agg.CustomerID]; ok {
			entry.WalletBalance = agg.WalletBalance
			result[agg.CustomerID] = entry
		}
	}

	return result, nil
}

func cpoAdminCustomerView(record models.Customer, aggregates *CustomerAggregates) CPOAdminCustomerView {
	view := CPOAdminCustomerView{
		ID:                record.ID,
		CPOID:             record.CPOID,
		Email:             record.Email,
		FullName:          record.FullName,
		Phone:             record.Phone,
		Status:            record.Status,
		IsVerified:        record.IsVerified,
		LastLoginAt:       record.LastLoginAt,
		UsergroupAssigned: record.UserGroupID != nil,
		CreatedAt:         record.CreatedAt,
		UpdatedAt:         record.UpdatedAt,
	}

	if aggregates != nil {
		view.TotalUsage = aggregates.TotalUsageKWh
		view.NoOfSessions = aggregates.SessionCount
		view.DriverWallet = aggregates.WalletBalance
	}

	return view
}

func mapCustomerNotFound(err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return &auth.APIError{
			Status:  http.StatusNotFound,
			Code:    "customer_not_found",
			Message: "The customer was not found for this CPO.",
		}
	}
	return fmt.Errorf("load customer: %w", err)
}

func (service *Service) AssignChargerToHub(
	ctx context.Context,
	principal auth.Principal,
	hubID uuid.UUID,
	chargerID uuid.UUID,
) (ChargerResponse, error) {
	if err := requireCPOContext(principal); err != nil {
		return ChargerResponse{}, err
	}
	if chargerID == uuid.Nil {
		return ChargerResponse{}, invalid("charger_id", "Charger ID is required.")
	}

	cpoID := *principal.CPOID
	var charger models.Charger

	err := service.database.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var hub models.Hub
		if err := tx.First(&hub, "id = ? AND cpo_id = ?", hubID, cpoID).Error; err != nil {
			return mapHubNotFound(err)
		}

		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			First(&charger, "id = ? AND cpo_id = ?", chargerID, cpoID).Error; err != nil {
			return mapChargerNotFound(err)
		}

		if charger.HubID != nil && *charger.HubID == hubID {
			return nil
		}

		previousHubID := charger.HubID
		now := service.now()
		if err := tx.Model(&charger).
			Where("id = ? AND cpo_id = ?", charger.ID, cpoID).
			Updates(map[string]any{
				"hub_id":     hub.ID,
				"updated_at": now,
			}).Error; err != nil {
			return mapChargerWriteError(err, "assign charger to hub")
		}
		if err := writeAudit(
			tx,
			principal.UserID,
			cpoID,
			"CHARGER_HUB_REASSIGNED",
			models.JSONB{
				"charger_id":      charger.ID,
				"previous_hub_id": previousHubID,
				"new_hub_id":      hub.ID,
			},
			now,
		); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return ChargerResponse{}, err
	}
	if err := service.database.WithContext(ctx).
		Preload("Connectors", func(tx *gorm.DB) *gorm.DB {
			return tx.Order("connector_number ASC")
		}).
		Preload("Hub").
		First(&charger, "id = ? AND cpo_id = ?", chargerID, cpoID).Error; err != nil {
		return ChargerResponse{}, fmt.Errorf("reload charger after assignment: %w", err)
	}

	return service.chargerView(charger, principal), nil
}

func normalizeCreateHubRequest(request CreateHubRequest) CreateHubRequest {
	request.Name = strings.TrimSpace(request.Name)
	request.Address = strings.TrimSpace(request.Address)
	request.State = constants.IndianState(strings.TrimSpace(string(request.State)))
	return request
}

func normalizeUpdateHubRequest(request UpdateHubRequest) UpdateHubRequest {
	request.Name = trimOptionalString(request.Name)
	request.Address = trimOptionalString(request.Address)
	if request.State != nil {
		trimmedState := constants.IndianState(strings.TrimSpace(string(*request.State)))
		request.State = &trimmedState
	}
	return request
}

func validateCreateHubRequest(request CreateHubRequest) error {
	if request.Name == "" || len(request.Name) > 255 {
		return invalid("name", "Hub name is required and must not exceed 255 characters.")
	}
	if request.Address == "" || len(request.Address) > 5000 {
		return invalid("address", "Hub address is required and must not exceed 5000 characters.")
	}
	if !request.State.Valid() {
		return invalid("state", "Invalid state.")
	}
	if request.Latitude == nil {
		return invalid("latitude", "Latitude is required.")
	}
	if *request.Latitude < -90 || *request.Latitude > 90 {
		return invalid("latitude", "Latitude must be between -90 and 90.")
	}
	if request.Longitude == nil {
		return invalid("longitude", "Longitude is required.")
	}
	if *request.Longitude < -180 || *request.Longitude > 180 {
		return invalid("longitude", "Longitude must be between -180 and 180.")
	}
	if request.SanctionLoad != nil && *request.SanctionLoad < 0 {
		return invalid("sanction_load", "Sanction load must not be negative.")
	}
	return nil
}

func validateInitialHubVisibility(customerVisible bool) error {
	if customerVisible {
		return hubTariffRootRequired()
	}
	return nil
}

func validateUpdateHubRequest(request UpdateHubRequest) error {
	if request.Name == nil &&
		request.Address == nil &&
		request.Latitude == nil &&
		request.State == nil &&
		request.Longitude == nil &&
		request.Open24Hours == nil &&
		request.SanctionLoad == nil &&
		request.CustomerVisible == nil {
		return invalid("hub", "At least one hub field must be supplied.")
	}

	if request.Name != nil && (*request.Name == "" || len(*request.Name) > 255) {
		return invalid("name", "Hub name must not exceed 255 characters.")
	}
	if request.Address != nil && (*request.Address == "" || len(*request.Address) > 5000) {
		return invalid("address", "Hub address must not exceed 5000 characters.")
	}
	if request.State != nil && !(*request.State).Valid() {
		return invalid("state", "Invalid state.")
	}
	if request.Latitude != nil && (*request.Latitude < -90 || *request.Latitude > 90) {
		return invalid("latitude", "Latitude must be between -90 and 90.")
	}
	if request.Longitude != nil && (*request.Longitude < -180 || *request.Longitude > 180) {
		return invalid("longitude", "Longitude must be between -180 and 180.")
	}
	if request.SanctionLoad != nil && *request.SanctionLoad < 0 {
		return invalid("sanction_load", "Sanction load must not be negative.")
	}
	return nil
}

func mapHubWriteError(err error, operation string) error {
	var postgresError *pgconn.PgError
	if errors.As(err, &postgresError) {
		switch postgresError.Code {
		case "23505":
			return &auth.APIError{
				Status:  http.StatusConflict,
				Code:    "hub_conflict",
				Message: "The hub already exists or conflicts with another record.",
			}
		case "23503":
			return &auth.APIError{
				Status:  http.StatusConflict,
				Code:    "hub_conflict",
				Message: "The hub references an invalid related record.",
			}
		case "23514":
			if postgresError.ConstraintName == "chk_hubs_sanction_load" {
				return invalid("sanction_load", "Sanction load must not be negative.")
			}
			if strings.Contains(postgresError.Message, "customer-visible hub requires one enabled unbounded hub tariff") {
				return hubTariffRootRequired()
			}
		}
	}
	return fmt.Errorf("%s: %w", operation, err)
}

func hubView(record models.Hub) HubView {
	return HubView{
		ID:              record.ID,
		CPOID:           record.CPOID,
		Name:            record.Name,
		Address:         record.Address,
		State:           record.State,
		GSTID:           record.GSTID,
		Latitude:        record.Latitude,
		Longitude:       record.Longitude,
		Open24Hours:     record.Open24Hours,
		SanctionLoad:    record.SanctionLoad,
		CustomerVisible: record.CustomerVisible,
		CreatedAt:       record.CreatedAt,
		UpdatedAt:       record.UpdatedAt,
	}
}

func normalizeCreateTariffRequest(request CreateTariffRequest) CreateTariffRequest {
	request.Currency = strings.ToUpper(strings.TrimSpace(request.Currency))
	if request.Currency == "" {
		request.Currency = "INR"
	}
	return request
}

func normalizeUpdateTariffRequest(request UpdateTariffRequest) UpdateTariffRequest {
	if request.Currency != nil {
		value := strings.ToUpper(strings.TrimSpace(*request.Currency))
		request.Currency = &value
	}
	return request
}

func validateCreateTariffRequest(request CreateTariffRequest) error {
	if request.PricePerUnit.Sign() < 0 {
		return invalid("price_per_unit", "Price per unit must not be negative.")
	}
	if request.IdleFeePerMin.Sign() < 0 {
		return invalid("idle_fee_per_min", "Idle fee per minute must not be negative.")
	}
	if len(request.Currency) != 3 {
		return invalid("currency", "Currency must be a 3-letter code.")
	}
	if err := validateTariffDateRange(request.StartDate, request.EndDate); err != nil {
		return err
	}
	if request.TariffType != nil && !request.TariffType.Valid() {
		return invalid("tariff_type", "Invalid tariff type.")
	}
	if request.PriceType != nil && !request.PriceType.Valid() {
		return invalid("price_type", "Invalid price type.")
	}
	if request.Units != nil && !request.Units.Valid() {
		return invalid("units", "Invalid units.")
	}
	isActive := request.IsActive == nil || *request.IsActive
	return validateTariffCommercial(request.TariffType, request.PriceType, request.Units, request.IdleFeePerMin, isActive)
}

func validateUpdateTariffRequest(request UpdateTariffRequest) error {
	if request.PricePerUnit == nil &&
		request.IdleFeePerMin == nil &&
		request.Currency == nil &&
		request.IsActive == nil &&
		!request.StartDate.Present() &&
		!request.EndDate.Present() &&
		request.TariffType == nil &&
		request.PriceType == nil &&
		!request.Units.Present() {
		return invalid("tariff", "At least one tariff field must be supplied.")
	}

	if request.PricePerUnit != nil && request.PricePerUnit.Sign() < 0 {
		return invalid("price_per_unit", "Price per unit must not be negative.")
	}
	if request.IdleFeePerMin != nil && request.IdleFeePerMin.Sign() < 0 {
		return invalid("idle_fee_per_min", "Idle fee per minute must not be negative.")
	}
	if request.Currency != nil && len(*request.Currency) != 3 {
		return invalid("currency", "Currency must be a 3-letter code.")
	}
	if request.TariffType != nil && !request.TariffType.Valid() {
		return invalid("tariff_type", "Invalid tariff type.")
	}
	if request.PriceType != nil && !request.PriceType.Valid() {
		return invalid("price_type", "Invalid price type.")
	}
	if request.Units.Present() && request.Units.Value() != nil && !request.Units.Value().Valid() {
		return invalid("units", "Invalid units.")
	}
	return nil
}

// applyTariffUpdate applies every accepted PATCH field to the in-memory tariff
// and returns matching persistence and audit projections. Nullable patch fields
// retain the difference between an omitted key and an explicit JSON null.
func applyTariffUpdate(tariff *models.Tariff, request UpdateTariffRequest) (map[string]any, models.JSONB) {
	updates := map[string]any{}
	changedFields := models.JSONB{}
	if request.PricePerUnit != nil {
		updates["price_per_unit"] = *request.PricePerUnit
		tariff.PricePerUnit = *request.PricePerUnit
		changedFields["price_per_unit"] = *request.PricePerUnit
	}
	if request.IdleFeePerMin != nil {
		updates["idle_fee_per_min"] = *request.IdleFeePerMin
		tariff.IdleFeePerMin = *request.IdleFeePerMin
		changedFields["idle_fee_per_min"] = *request.IdleFeePerMin
	}
	if request.Currency != nil {
		updates["currency"] = *request.Currency
		tariff.Currency = *request.Currency
		changedFields["currency"] = *request.Currency
	}
	if request.IsActive != nil {
		updates["is_active"] = *request.IsActive
		tariff.IsActive = *request.IsActive
		changedFields["is_active"] = *request.IsActive
	}
	if request.StartDate.Present() {
		value := request.StartDate.Value()
		tariff.StartDate = value
		if value == nil {
			updates["start_date"] = nil
			changedFields["start_date"] = nil
		} else {
			updates["start_date"] = *value
			changedFields["start_date"] = *value
		}
	}
	if request.EndDate.Present() {
		value := request.EndDate.Value()
		tariff.EndDate = value
		if value == nil {
			updates["end_date"] = nil
			changedFields["end_date"] = nil
		} else {
			updates["end_date"] = *value
			changedFields["end_date"] = *value
		}
	}
	if request.TariffType != nil {
		updates["tariff_type"] = *request.TariffType
		tariff.TariffType = request.TariffType
		changedFields["tariff_type"] = *request.TariffType
	}
	if request.PriceType != nil {
		updates["price_type"] = *request.PriceType
		tariff.PriceType = request.PriceType
		changedFields["price_type"] = *request.PriceType
	}
	if request.Units.Present() {
		value := request.Units.Value()
		tariff.Units = value
		if value == nil {
			updates["units"] = nil
			changedFields["units"] = nil
		} else {
			updates["units"] = *value
			changedFields["units"] = *value
		}
	}
	return updates, changedFields
}

type tariffTarget struct {
	assignment constants.TariffAssignmentType
	column     string
	id         uuid.UUID
}

func tariffTargetFromRecord(tariff models.Tariff) (tariffTarget, error) {
	switch tariff.AssignedTo {
	case constants.TariffAssignedHub:
		if tariff.HubID != nil {
			return tariffTarget{assignment: tariff.AssignedTo, column: "hub_id", id: *tariff.HubID}, nil
		}
	case constants.TariffAssignedCharger:
		if tariff.ChargerID != nil {
			return tariffTarget{assignment: tariff.AssignedTo, column: "charger_id", id: *tariff.ChargerID}, nil
		}
	case constants.TariffAssignedUserGroup:
		if tariff.UserGroupID != nil {
			return tariffTarget{assignment: tariff.AssignedTo, column: "user_group_id", id: *tariff.UserGroupID}, nil
		}
	}
	return tariffTarget{}, errors.New("tariff has no exact target")
}

func (target tariffTarget) advisoryKey(cpoID uuid.UUID) string {
	return fmt.Sprintf("tariff:%s:%s:%s", cpoID, target.assignment, target.id)
}

func tariffTemporalProjection(tariffs []models.Tariff) []commercial.TemporalTariff {
	projection := make([]commercial.TemporalTariff, 0, len(tariffs))
	for _, tariff := range tariffs {
		projection = append(projection, commercial.TemporalTariff{
			ID: tariff.ID, IsActive: tariff.IsActive, StartDate: tariff.StartDate, EndDate: tariff.EndDate,
		})
	}
	return projection
}

func tariffTemporalConflict() error {
	return &auth.APIError{
		Status:  http.StatusConflict,
		Code:    "tariff_temporal_conflict",
		Message: "Enabled tariffs for this target must have one root, unique open-ended starts, and only nested bounded overrides.",
	}
}

func hubTariffRootRequired() error {
	return &auth.APIError{
		Status:  http.StatusConflict,
		Code:    "hub_tariff_root_required",
		Message: "A hub must first exist with an enabled unbounded hub tariff before it can become customer-visible.",
	}
}

// validateTariffTopologyMutation serializes one exact target, validates the
// complete resulting enabled hierarchy, and preserves the published-Hub floor.
// It is deliberately called for create, activation/deactivation, schedule
// changes, and delete; time itself never mutates policy rows.
func (service *Service) validateTariffTopologyMutation(tx *gorm.DB, candidate *models.Tariff, deletingID *uuid.UUID) error {
	var source models.Tariff
	if candidate != nil {
		source = *candidate
	} else if deletingID != nil {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&source, "id = ?", *deletingID).Error; err != nil {
			return service.handleTariffError("load tariff", err)
		}
	} else {
		return errors.New("tariff topology mutation has no candidate")
	}
	target, err := tariffTargetFromRecord(source)
	if err != nil {
		return invalid("tariff_target", "A tariff must target exactly one hub, charger, or user group.")
	}

	var hub models.Hub
	if target.assignment == constants.TariffAssignedHub {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&hub, "id = ? AND cpo_id = ?", target.id, source.CPOID).Error; err != nil {
			return mapHubNotFound(err)
		}
	}
	if err := tx.Exec("SELECT pg_advisory_xact_lock(hashtextextended(?, 0))", target.advisoryKey(source.CPOID)).Error; err != nil {
		return fmt.Errorf("lock tariff target: %w", err)
	}

	var existing []models.Tariff
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("cpo_id = ? AND assigned_to = ? AND "+target.column+" = ?", source.CPOID, target.assignment, target.id).
		Find(&existing).Error; err != nil {
		return fmt.Errorf("load tariff topology: %w", err)
	}
	result := make([]models.Tariff, 0, len(existing)+1)
	found := false
	for _, tariff := range existing {
		if deletingID != nil && tariff.ID == *deletingID {
			continue
		}
		if candidate != nil && tariff.ID == candidate.ID {
			result = append(result, *candidate)
			found = true
			continue
		}
		result = append(result, tariff)
	}
	if candidate != nil && !found {
		result = append(result, *candidate)
	}
	if err := commercial.ValidateEnabledTariffTopology(tariffTemporalProjection(result)); err != nil {
		if errors.Is(err, commercial.ErrInvalidTariffDateShape) {
			return invalid("schedule", "A tariff schedule must be root, start-only, or a complete increasing interval.")
		}
		return tariffTemporalConflict()
	}
	if target.assignment == constants.TariffAssignedHub && hub.CustomerVisible {
		rootCount := 0
		for _, tariff := range result {
			if tariff.IsActive && tariff.StartDate == nil && tariff.EndDate == nil {
				rootCount++
			}
		}
		if rootCount != 1 {
			return hubTariffRootRequired()
		}
	}
	return nil
}

func (service *Service) validateVisibleHubTariffFloor(tx *gorm.DB, cpoID, hubID uuid.UUID) error {
	target := tariffTarget{assignment: constants.TariffAssignedHub, column: "hub_id", id: hubID}
	if err := tx.Exec("SELECT pg_advisory_xact_lock(hashtextextended(?, 0))", target.advisoryKey(cpoID)).Error; err != nil {
		return fmt.Errorf("lock hub tariff target: %w", err)
	}
	var count int64
	if err := tx.Model(&models.Tariff{}).
		Where("cpo_id = ? AND assigned_to = ? AND hub_id = ? AND is_active = ? AND start_date IS NULL AND end_date IS NULL", cpoID, constants.TariffAssignedHub, hubID, true).
		Count(&count).Error; err != nil {
		return fmt.Errorf("count hub tariff roots: %w", err)
	}
	if count != 1 {
		return hubTariffRootRequired()
	}
	return nil
}

func validateTariffCommercial(tariffType *constants.TariffType, priceType *constants.PriceType, units *constants.Unit, idleFeePerMin decimal.Decimal, isActive bool) error {
	if constants.SupportedChargingTariff(tariffType, priceType, units) {
		if !isActive || idleFeePerMin.IsZero() {
			return nil
		}
		return &auth.APIError{
			Status:  http.StatusBadRequest,
			Code:    "idle_fee_unsupported",
			Message: "A non-zero idle fee is unavailable until an authoritative idle interval exists.",
		}
	}
	return &auth.APIError{
		Status:  http.StatusBadRequest,
		Code:    "unsupported_tariff_pricing",
		Message: "Supported tariffs are fixed energy per kWh, fixed time per minute, or fixed per session.",
	}
}

func validateTariffDateRange(startDate, endDate *time.Time) error {
	if err := commercial.ValidateTariffDateShape(startDate, endDate); errors.Is(err, commercial.ErrInvalidTariffDateShape) {
		if startDate == nil {
			return invalid("schedule", "end_date requires start_date.")
		}
		return invalid("date_range", "Start date must be strictly before end date.")
	}
	return nil
}

func mapGSTNotFound(err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return &auth.APIError{
			Status:  http.StatusNotFound,
			Code:    "gst_not_found",
			Message: "The GST profile was not found.",
		}
	}
	return fmt.Errorf("load gst: %w", err)
}

func mapUserGroupNotFound(err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return &auth.APIError{
			Status:  http.StatusNotFound,
			Code:    "user_group_not_found",
			Message: "The user group was not found.",
		}
	}
	return fmt.Errorf("load user group: %w", err)
}

func (service *Service) tariffView(record *models.Tariff) TariffView {
	return TariffView{
		ID:            record.ID,
		CPOID:         record.CPOID,
		AssignedTo:    record.AssignedTo,
		HubID:         record.HubID,
		ChargerID:     record.ChargerID,
		UserGroupID:   record.UserGroupID,
		PricePerUnit:  record.PricePerUnit,
		IdleFeePerMin: record.IdleFeePerMin,
		Currency:      record.Currency,
		IsActive:      record.IsActive,
		StartDate:     record.StartDate,
		EndDate:       record.EndDate,
		TariffType:    record.TariffType,
		PriceType:     record.PriceType,
		Units:         record.Units,
		CreatedAt:     record.CreatedAt,
		UpdatedAt:     record.UpdatedAt,
	}
}

func (service *Service) handleTariffError(operation string, err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return &auth.APIError{
			Status:  http.StatusNotFound,
			Code:    "tariff_not_found",
			Message: "The tariff was not found.",
		}
	}
	var postgresError *pgconn.PgError
	if errors.As(err, &postgresError) {
		switch postgresError.Code {
		case "23P01", "23505":
			return tariffTemporalConflict()
		case "23514":
			switch postgresError.ConstraintName {
			case "tariffs_exactly_one_target", "tariffs_target_matches_assigned_to":
				return invalid("tariff_target", "A tariff must target exactly one hub, charger, or user group.")
			case "tariffs_temporal_dates_check":
				return invalid("schedule", "A tariff schedule must be root, start-only, or a complete increasing interval.")
			}
			if strings.Contains(postgresError.Message, "customer-visible hub requires one enabled unbounded hub tariff") {
				return hubTariffRootRequired()
			}
		case "23503":
			switch postgresError.ConstraintName {
			case "tariffs_hub_id_fkey":
				return &auth.APIError{
					Status:  http.StatusConflict,
					Code:    "hub_not_found",
					Message: "The hub for this tariff does not exist.",
				}
			case "tariffs_charger_id_fkey":
				return &auth.APIError{
					Status:  http.StatusConflict,
					Code:    "charger_not_found",
					Message: "The charger for this tariff does not exist.",
				}
			case "tariffs_user_group_id_fkey":
				return &auth.APIError{
					Status:  http.StatusConflict,
					Code:    "user_group_not_found",
					Message: "The user group for this tariff does not exist.",
				}
			}
			return &auth.APIError{
				Status:  http.StatusConflict,
				Code:    "tariff_in_use",
				Message: "This tariff cannot be deleted because it is referenced by charging history.",
			}
		}
	}
	return fmt.Errorf("%s: %w", operation, err)
}

func (service *Service) CreateGST(
	ctx context.Context,
	principal auth.Principal,
	request CreateGSTRequest,
) (GSTView, error) {
	if err := requireCPOContext(principal); err != nil {
		return GSTView{}, err
	}

	cpoID := *principal.CPOID
	request = normalizeCreateGSTRequest(request)
	if err := validateCreateGSTRequest(request); err != nil {
		return GSTView{}, err
	}

	var record models.GST
	err := service.database.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		now := service.now()

		isActive := true
		if request.IsActive != nil {
			isActive = *request.IsActive
		}

		record = models.GST{
			ID:        uuid.New(),
			CPOID:     cpoID,
			Name:      request.Name,
			State:     request.State,
			SGSTRate:  request.SGSTRate,
			CGSTRate:  request.CGSTRate,
			IGSTRate:  request.IGSTRate,
			IsActive:  isActive,
			CreatedAt: now,
			UpdatedAt: now,
		}

		if err := tx.Create(&record).Error; err != nil {
			return mapGSTWriteError(err, "create gst")
		}

		return writeAudit(
			tx,
			principal.UserID,
			cpoID,
			"GST_CREATED",
			models.JSONB{
				"gst_id":    record.ID,
				"name":      record.Name,
				"state":     record.State,
				"sgst_rate": record.SGSTRate,
				"cgst_rate": record.CGSTRate,
				"igst_rate": record.IGSTRate,
			},
			now,
		)
	})
	if err != nil {
		return GSTView{}, err
	}

	return gstView(record), nil
}

func (service *Service) ListGSTs(
	ctx context.Context,
	principal auth.Principal,
	query TenantListQuery,
) (GSTListResponse, error) {
	if err := requireCPOContext(principal); err != nil {
		return GSTListResponse{}, err
	}
	query, err := validateTenantListQuery(query)
	if err != nil {
		return GSTListResponse{}, err
	}
	databaseQuery := service.database.WithContext(ctx).
		Where("cpo_id = ?", *principal.CPOID)
	if query.Before != nil {
		databaseQuery = databaseQuery.Where(
			"(created_at, id) < (?, ?)",
			*query.Before,
			*query.BeforeID,
		)
	}
	var records []models.GST
	if err := databaseQuery.
		Order("created_at DESC, id DESC").
		Limit(query.Limit + 1).
		Find(&records).Error; err != nil {
		return GSTListResponse{}, fmt.Errorf("list GST profiles: %w", err)
	}
	hasMore := len(records) > query.Limit
	if hasMore {
		records = records[:query.Limit]
	}
	gsts := make([]GSTView, 0, len(records))
	for _, record := range records {
		gsts = append(gsts, gstView(record))
	}
	response := GSTListResponse{GSTs: gsts, HasMore: hasMore}
	if hasMore && len(records) > 0 {
		nextBefore := records[len(records)-1].CreatedAt
		nextBeforeID := records[len(records)-1].ID
		response.NextBefore = &nextBefore
		response.NextBeforeID = &nextBeforeID
	}
	return response, nil
}

func (service *Service) GetGST(
	ctx context.Context,
	principal auth.Principal,
	gstID uuid.UUID,
) (GSTView, error) {
	if err := requireCPOContext(principal); err != nil {
		return GSTView{}, err
	}

	var record models.GST
	if err := service.database.WithContext(ctx).
		First(&record, "cpo_id = ? AND id = ?", *principal.CPOID, gstID).Error; err != nil {
		return GSTView{}, mapGSTNotFound(err)
	}

	return gstView(record), nil
}

func (service *Service) UpdateGST(
	ctx context.Context,
	principal auth.Principal,
	gstID uuid.UUID,
	request UpdateGSTRequest,
) (GSTView, error) {
	if err := requireCPOContext(principal); err != nil {
		return GSTView{}, err
	}

	request = normalizeUpdateGSTRequest(request)
	if err := validateUpdateGSTRequest(request); err != nil {
		return GSTView{}, err
	}

	cpoID := *principal.CPOID
	var record models.GST

	err := service.database.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := lockCPOGSTRelations(tx, cpoID); err != nil {
			return err
		}
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			First(&record, "cpo_id = ? AND id = ?", cpoID, gstID).Error; err != nil {
			return mapGSTNotFound(err)
		}

		updates := map[string]any{}
		changedFields := models.JSONB{}

		if request.Name != nil {
			updates["name"] = *request.Name
			record.Name = *request.Name
			changedFields["name"] = *request.Name
		}
		if request.State != nil {
			updates["state"] = *request.State
			record.State = *request.State
			changedFields["state"] = *request.State
		}
		if request.SGSTRate != nil {
			updates["sgst_rate"] = *request.SGSTRate
			record.SGSTRate = request.SGSTRate
			changedFields["sgst_rate"] = *request.SGSTRate
		}
		if request.CGSTRate != nil {
			updates["cgst_rate"] = *request.CGSTRate
			record.CGSTRate = request.CGSTRate
			changedFields["cgst_rate"] = *request.CGSTRate
		}
		if request.IGSTRate != nil {
			updates["igst_rate"] = *request.IGSTRate
			record.IGSTRate = request.IGSTRate
			changedFields["igst_rate"] = *request.IGSTRate
		}
		if request.IsActive != nil {
			updates["is_active"] = *request.IsActive
			record.IsActive = *request.IsActive
			changedFields["is_active"] = *request.IsActive
		}

		if len(changedFields) == 0 {
			return &auth.APIError{
				Status:  http.StatusBadRequest,
				Code:    "invalid_request",
				Message: "At least one GST field must be supplied.",
			}
		}
		if commercial.ValidateGSTComponents(record.SGSTRate, record.CGSTRate, record.IGSTRate) != nil {
			return invalid("gst_components", "GST cannot combine split and integrated tax components.")
		}

		var assignedHubs []models.Hub
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("cpo_id = ? AND gst_id = ?", cpoID, record.ID).
			Find(&assignedHubs).Error; err != nil {
			return fmt.Errorf("load GST Hub assignments: %w", err)
		}
		for _, hub := range assignedHubs {
			if err := validateGSTForHub(hub, record); err != nil {
				return err
			}
		}
		now := service.now()
		updates["updated_at"] = now
		record.UpdatedAt = now

		if err := tx.Model(&models.GST{}).
			Where("id = ?", record.ID).
			Updates(updates).Error; err != nil {
			return mapGSTWriteError(err, "update gst")
		}

		return writeAudit(
			tx,
			principal.UserID,
			cpoID,
			"GST_UPDATED",
			models.JSONB{
				"gst_id":         record.ID,
				"changed_fields": changedFields,
			},
			now,
		)
	})
	if err != nil {
		return GSTView{}, err
	}

	return gstView(record), nil
}

func normalizeCreateGSTRequest(request CreateGSTRequest) CreateGSTRequest {
	request.Name = strings.TrimSpace(request.Name)
	request.State = constants.IndianState(strings.TrimSpace(string(request.State)))
	return request
}

func normalizeUpdateGSTRequest(request UpdateGSTRequest) UpdateGSTRequest {
	request.Name = trimOptionalString(request.Name)
	if request.State != nil {
		trimmedState := constants.IndianState(strings.TrimSpace(string(*request.State)))
		request.State = &trimmedState
	}
	return request
}

func validateCreateGSTRequest(request CreateGSTRequest) error {
	if request.Name == "" || len(request.Name) > 100 {
		return invalid("name", "GST name is required and must not exceed 100 characters.")
	}
	if !request.State.Valid() {
		return invalid("state", "Invalid state.")
	}
	if request.SGSTRate == nil {
		return invalid("sgst_rate", "SGST rate is required.")
	}
	if request.CGSTRate == nil {
		return invalid("cgst_rate", "CGST rate is required.")
	}
	if request.IGSTRate == nil {
		return invalid("igst_rate", "IGST rate is required.")
	}
	if request.SGSTRate.Sign() < 0 {
		return invalid("sgst_rate", "SGST rate must not be negative.")
	}
	if request.CGSTRate.Sign() < 0 {
		return invalid("cgst_rate", "CGST rate must not be negative.")
	}
	if request.IGSTRate.Sign() < 0 {
		return invalid("igst_rate", "IGST rate must not be negative.")
	}
	if request.SGSTRate.Cmp(decimal.NewFromInt(100)) > 0 {
		return invalid("sgst_rate", "SGST rate must not exceed 100.")
	}
	if request.CGSTRate.Cmp(decimal.NewFromInt(100)) > 0 {
		return invalid("cgst_rate", "CGST rate must not exceed 100.")
	}
	if request.IGSTRate.Cmp(decimal.NewFromInt(100)) > 0 {
		return invalid("igst_rate", "IGST rate must not exceed 100.")
	}
	if commercial.ValidateGSTComponents(request.SGSTRate, request.CGSTRate, request.IGSTRate) != nil {
		return invalid("gst_components", "GST cannot combine split and integrated tax components.")
	}
	return nil
}

func validateUpdateGSTRequest(request UpdateGSTRequest) error {
	if request.Name == nil &&
		request.State == nil &&
		request.SGSTRate == nil &&
		request.CGSTRate == nil &&
		request.IGSTRate == nil &&
		request.IsActive == nil {
		return invalid("gst", "At least one GST field must be supplied.")
	}

	if request.Name != nil && (*request.Name == "" || len(*request.Name) > 100) {
		return invalid("name", "GST name must not exceed 100 characters.")
	}
	if request.State != nil && !(*request.State).Valid() {
		return invalid("state", "Invalid state.")
	}
	if request.SGSTRate != nil {
		if request.SGSTRate.Sign() < 0 || request.SGSTRate.Cmp(decimal.NewFromInt(100)) > 0 {
			return invalid("sgst_rate", "SGST rate must be between 0 and 100.")
		}
	}
	if request.CGSTRate != nil {
		if request.CGSTRate.Sign() < 0 || request.CGSTRate.Cmp(decimal.NewFromInt(100)) > 0 {
			return invalid("cgst_rate", "CGST rate must be between 0 and 100.")
		}
	}
	if request.IGSTRate != nil {
		if request.IGSTRate.Sign() < 0 || request.IGSTRate.Cmp(decimal.NewFromInt(100)) > 0 {
			return invalid("igst_rate", "IGST rate must be between 0 and 100.")
		}
	}
	return nil
}

func mapGSTWriteError(err error, operation string) error {
	var postgresError *pgconn.PgError
	if errors.As(err, &postgresError) {
		switch postgresError.Code {
		case "23505":
			return &auth.APIError{
				Status:  http.StatusConflict,
				Code:    "gst_conflict",
				Message: "The GST profile already exists or conflicts with another record.",
			}
		case "23503":
			return &auth.APIError{
				Status:  http.StatusConflict,
				Code:    "gst_conflict",
				Message: "The GST profile references an invalid related record.",
			}
		}
	}
	return fmt.Errorf("%s: %w", operation, err)
}

func gstView(record models.GST) GSTView {
	return GSTView{
		ID:        record.ID,
		CPOID:     record.CPOID,
		Name:      record.Name,
		State:     record.State,
		SGSTRate:  record.SGSTRate,
		CGSTRate:  record.CGSTRate,
		IGSTRate:  record.IGSTRate,
		IsActive:  record.IsActive,
		CreatedAt: record.CreatedAt,
		UpdatedAt: record.UpdatedAt,
	}
}

func (service *Service) UpdateChargerStatus(
	ctx context.Context,
	principal auth.Principal,
	chargerID uuid.UUID,
	request UpdateChargerStatusRequest,
) (ChargerStatusResponse, error) {
	return service.updateChargerStatus(ctx, principal, chargerID, request, uuid.NewString())
}

func (service *Service) updateChargerStatus(
	ctx context.Context,
	principal auth.Principal,
	chargerID uuid.UUID,
	request UpdateChargerStatusRequest,
	correlationID string,
) (ChargerStatusResponse, error) {
	if err := requireCPOContext(principal); err != nil {
		return ChargerStatusResponse{}, err
	}

	if !request.Status.Valid() {
		return ChargerStatusResponse{}, invalid("status", "Invalid charger status.")
	}

	var charger models.Charger
	err := service.database.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			First(&charger, "id = ? AND cpo_id = ?", chargerID, *principal.CPOID).Error; err != nil {
			return mapChargerNotFound(err)
		}

		if request.OCPPIdentity != "" && charger.OCPPIdentity != request.OCPPIdentity {
			return &auth.APIError{
				Status:  http.StatusConflict,
				Code:    "ocpp_identity_mismatch",
				Message: "OCPP identity does not match the charger.",
			}
		}
		if request.Status == constants.ChargerStatusActive && charger.HubID == nil {
			return &auth.APIError{Status: http.StatusConflict, Code: "charger_hub_required", Message: "A charger must belong to a hub before it can be active."}
		}

		now := service.now()
		if err := tx.Model(&charger).
			Updates(map[string]any{
				"status":     request.Status,
				"updated_at": now,
			}).Error; err != nil {
			return mapChargerWriteError(err, "update charger status")
		}
		charger.Status = request.Status

		return writeAudit(
			tx,
			principal.UserID,
			*principal.CPOID,
			"CHARGER_STATUS_UPDATED",
			models.JSONB{
				"charger_id": charger.ID,
				"status":     request.Status,
			},
			now,
		)
	})

	if err != nil {
		return ChargerStatusResponse{}, err
	}
	if service.halOperations != nil {
		// Keep HAL's enabled projection aligned after the CMS source of truth
		// commits; failed delivery is recorded for the reconciler.
		_ = service.halOperations.EnsureChargerMapping(ctx, charger.ID, correlationID)
	}

	return ChargerStatusResponse{
		ChargerID:    charger.ID,
		OCPPIdentity: charger.OCPPIdentity,
		Status:       charger.Status,
	}, nil
}

func (service *Service) GetChargerStatus(
	ctx context.Context,
	principal auth.Principal,
	chargerID uuid.UUID,
) (ChargerStatusResponse, error) {
	if err := requireCPOContext(principal); err != nil {
		return ChargerStatusResponse{}, err
	}

	var charger models.Charger
	if err := service.database.WithContext(ctx).
		First(&charger, "id = ? AND cpo_id = ?", chargerID, *principal.CPOID).Error; err != nil {
		return ChargerStatusResponse{}, mapChargerNotFound(err)
	}

	return ChargerStatusResponse{
		ChargerID:    charger.ID,
		OCPPIdentity: charger.OCPPIdentity,
		Status:       charger.Status,
	}, nil
}

type ImageDownload struct {
	Content      io.ReadSeeker
	OriginalName string
	DetectedMIME string
	ModTime      time.Time
}

func (service *Service) DownloadChargerImage(ctx context.Context, principal auth.Principal, chargerID string) (*ImageDownload, error) {
	if err := requireCPOContext(principal); err != nil {
		return nil, err
	}

	chargerID = normalizeChargerID(chargerID)
	if !chargerIDPattern.MatchString(chargerID) {
		return nil, &auth.APIError{
			Status:  http.StatusBadRequest,
			Code:    "invalid_charger_id",
			Message: "The charger ID is invalid.",
		}
	}

	var record models.Charger
	if err := service.database.WithContext(ctx).
		First(&record, "cpo_id = ? AND charger_id = ?", *principal.CPOID, chargerID).Error; err != nil {
		return nil, mapChargerNotFound(err)
	}

	if record.ChargerImage == "" {
		return nil, &auth.APIError{
			Status:  http.StatusNotFound,
			Code:    "image_not_found",
			Message: "The charger does not have an image.",
		}
	}

	if strings.Contains(record.ChargerImage, "..") {
		return nil, &auth.APIError{
			Status:  http.StatusBadRequest,
			Code:    "invalid_image_path",
			Message: "The image path is invalid.",
		}
	}

	imagePath := record.ChargerImage

	file, err := os.Open(imagePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, &auth.APIError{
				Status:  http.StatusNotFound,
				Code:    "image_not_found",
				Message: "The charger image file was not found.",
			}
		}
		return nil, fmt.Errorf("open charger image: %w", err)
	}

	info, err := file.Stat()
	if err != nil {
		file.Close()
		return nil, fmt.Errorf("stat charger image: %w", err)
	}

	buffer := make([]byte, 512)
	_, err = file.Read(buffer)
	if err != nil && err != io.EOF {
		file.Close()
		return nil, fmt.Errorf("read charger image for mime type detection: %w", err)
	}

	if _, err := file.Seek(0, 0); err != nil {
		file.Close()
		return nil, fmt.Errorf("seek charger image after mime type detection: %w", err)
	}

	mimeType := http.DetectContentType(buffer)

	return &ImageDownload{
		Content:      file,
		OriginalName: filepath.Base(imagePath),
		DetectedMIME: mimeType,
		ModTime:      info.ModTime(),
	}, nil
}

func (service *Service) UpdateHubCustomerVisibility(
	ctx context.Context,
	principal auth.Principal,
	hubID uuid.UUID,
	request UpdateHubCustomerVisibilityRequest,
) (HubView, error) {
	if err := requireCPOContext(principal); err != nil {
		return HubView{}, err
	}

	var hub models.Hub
	err := service.database.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			First(&hub, "id = ? AND cpo_id = ?", hubID, *principal.CPOID).Error; err != nil {
			return mapHubNotFound(err)
		}
		if request.CustomerVisible {
			if err := service.validateVisibleHubTariffFloor(tx, *principal.CPOID, hubID); err != nil {
				return err
			}
		}

		now := service.now()
		if err := tx.Model(&hub).
			Updates(map[string]any{
				"customer_visible": request.CustomerVisible,
				"updated_at":       now,
			}).Error; err != nil {
			return mapHubWriteError(err, "update hub customer visibility")
		}
		hub.CustomerVisible = request.CustomerVisible

		return writeAudit(
			tx,
			principal.UserID,
			*principal.CPOID,
			"HUB_CUSTOMER_VISIBILITY_UPDATED",
			models.JSONB{
				"hub_id":           hub.ID,
				"customer_visible": request.CustomerVisible,
			},
			now,
		)
	})

	if err != nil {
		return HubView{}, err
	}

	return hubView(hub), nil
}

func (service *Service) CreateUserGroup(
	ctx context.Context,
	principal auth.Principal,
	request CreateUserGroupRequest,
) (UserGroupView, error) {
	if err := requireCPOContext(principal); err != nil {
		return UserGroupView{}, err
	}

	cpoID := *principal.CPOID
	request.Name = strings.TrimSpace(request.Name)
	request.Description = strings.TrimSpace(request.Description)

	if request.Name == "" {
		return UserGroupView{}, invalid("name", "User group name is required.")
	}

	isActive := true
	if request.IsActive != nil {
		isActive = *request.IsActive
	}

	now := service.now()
	record := models.UserGroup{
		ID:          uuid.New(),
		CPOID:       cpoID,
		Name:        request.Name,
		Description: request.Description,
		IsActive:    isActive,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	err := service.database.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&record).Error; err != nil {
			return mapUserGroupWriteError(err, "create user group")
		}

		return writeAudit(
			tx,
			principal.UserID,
			cpoID,
			"USER_GROUP_CREATED",
			models.JSONB{
				"user_group_id": record.ID,
				"name":          record.Name,
			},
			now,
		)
	})

	if err != nil {
		return UserGroupView{}, err
	}

	return userGroupView(record), nil
}

func (service *Service) ListUserGroups(
	ctx context.Context,
	principal auth.Principal,
	query TenantListQuery,
) (UserGroupListResponse, error) {
	if err := requireCPOContext(principal); err != nil {
		return UserGroupListResponse{}, err
	}
	query, err := validateTenantListQuery(query)
	if err != nil {
		return UserGroupListResponse{}, err
	}

	databaseQuery := service.database.WithContext(ctx).
		Where("cpo_id = ?", *principal.CPOID)
	if query.Before != nil {
		databaseQuery = databaseQuery.Where(
			"(created_at, id) < (?, ?)",
			*query.Before,
			*query.BeforeID,
		)
	}

	var records []models.UserGroup
	if err := databaseQuery.
		Order("created_at DESC, id DESC").
		Limit(query.Limit + 1).
		Find(&records).Error; err != nil {
		return UserGroupListResponse{}, fmt.Errorf("list user groups: %w", err)
	}

	hasMore := len(records) > query.Limit
	if hasMore {
		records = records[:query.Limit]
	}

	// Get user group IDs
	userGroupIDs := make([]uuid.UUID, len(records))
	for i, record := range records {
		userGroupIDs[i] = record.ID
	}

	// Fetch all members for the user groups
	var members []models.Customer
	if len(userGroupIDs) > 0 {
		if err := service.database.WithContext(ctx).
			Where("user_group_id IN ?", userGroupIDs).
			Find(&members).Error; err != nil {
			return UserGroupListResponse{}, fmt.Errorf("list user group members: %w", err)
		}
	}

	// Map members to user group IDs
	membersByGroup := make(map[uuid.UUID][]CPOAdminCustomerView)
	for _, member := range members {
		if member.UserGroupID != nil {
			membersByGroup[*member.UserGroupID] = append(membersByGroup[*member.UserGroupID], cpoAdminCustomerView(member, nil))
		}
	}

	userGroups := make([]UserGroupView, 0, len(records))
	for _, record := range records {
		userGroups = append(userGroups, userGroupView(record, membersByGroup[record.ID]))
	}

	response := UserGroupListResponse{UserGroups: userGroups, HasMore: hasMore}
	if hasMore && len(records) > 0 {
		nextBefore := records[len(records)-1].CreatedAt
		nextBeforeID := records[len(records)-1].ID
		response.NextBefore = &nextBefore
		response.NextBeforeID = &nextBeforeID
	}
	return response, nil
}

func (service *Service) GetUserGroup(
	ctx context.Context,
	principal auth.Principal,
	userGroupID uuid.UUID,
) (UserGroupView, error) {
	if err := requireCPOContext(principal); err != nil {
		return UserGroupView{}, err
	}

	var record models.UserGroup
	if err := service.database.WithContext(ctx).
		First(&record, "cpo_id = ? AND id = ?", *principal.CPOID, userGroupID).Error; err != nil {
		return UserGroupView{}, mapUserGroupNotFound(err)
	}

	var members []models.Customer
	if err := service.database.WithContext(ctx).
		Where("user_group_id = ?", userGroupID).
		Find(&members).Error; err != nil {
		return UserGroupView{}, fmt.Errorf("list user group members: %w", err)
	}

	memberViews := make([]CPOAdminCustomerView, 0, len(members))
	for _, member := range members {
		memberViews = append(memberViews, cpoAdminCustomerView(member, nil))
	}

	return userGroupView(record, memberViews), nil
}

func (service *Service) UpdateUserGroup(
	ctx context.Context,
	principal auth.Principal,
	userGroupID uuid.UUID,
	request UpdateUserGroupRequest,
) (UserGroupView, error) {
	if err := requireCPOContext(principal); err != nil {
		return UserGroupView{}, err
	}

	request.Name = trimOptionalString(request.Name)
	request.Description = trimOptionalString(request.Description)

	if request.Name == nil && request.Description == nil && request.IsActive == nil {
		return UserGroupView{}, invalid("user_group", "At least one user group field must be supplied.")
	}

	if request.Name != nil && *request.Name == "" {
		return UserGroupView{}, invalid("name", "User group name is required.")
	}

	cpoID := *principal.CPOID
	var record models.UserGroup

	err := service.database.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			First(&record, "cpo_id = ? AND id = ?", cpoID, userGroupID).Error; err != nil {
			return mapUserGroupNotFound(err)
		}

		updates := map[string]any{}
		changedFields := models.JSONB{}

		if request.Name != nil {
			updates["name"] = *request.Name
			record.Name = *request.Name
			changedFields["name"] = *request.Name
		}
		if request.Description != nil {
			updates["description"] = *request.Description
			record.Description = *request.Description
			changedFields["description"] = *request.Description
		}
		if request.IsActive != nil {
			updates["is_active"] = *request.IsActive
			record.IsActive = *request.IsActive
			changedFields["is_active"] = *request.IsActive
		}

		now := service.now()
		updates["updated_at"] = now
		record.UpdatedAt = now

		if err := tx.Model(&models.UserGroup{}).
			Where("id = ?", record.ID).
			Updates(updates).Error; err != nil {
			return mapUserGroupWriteError(err, "update user group")
		}

		return writeAudit(
			tx,
			principal.UserID,
			cpoID,
			"USER_GROUP_UPDATED",
			models.JSONB{
				"user_group_id":  record.ID,
				"changed_fields": changedFields,
			},
			now,
		)
	})

	if err != nil {
		return UserGroupView{}, err
	}

	return userGroupView(record), nil
}

func (service *Service) DeleteUserGroup(
	ctx context.Context,
	principal auth.Principal,
	userGroupID uuid.UUID,
) error {
	if err := requireCPOContext(principal); err != nil {
		return err
	}

	cpoID := *principal.CPOID
	return service.database.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var record models.UserGroup
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			First(&record, "cpo_id = ? AND id = ?", cpoID, userGroupID).Error; err != nil {
			return mapUserGroupNotFound(err)
		}

		if err := tx.Delete(&record).Error; err != nil {
			return mapUserGroupDeleteError(err)
		}

		return writeAudit(
			tx,
			principal.UserID,
			cpoID,
			"USER_GROUP_DELETED",
			models.JSONB{"user_group_id": userGroupID},
			service.now(),
		)
	})
}

func (service *Service) AddMemberToUserGroup(
	ctx context.Context,
	principal auth.Principal,
	userGroupID uuid.UUID,
	request AddMemberToUserGroupRequest,
) error {
	if err := requireCPOContext(principal); err != nil {
		return err
	}

	cpoID := *principal.CPOID
	return service.database.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var userGroup models.UserGroup
		if err := tx.First(&userGroup, "id = ? AND cpo_id = ?", userGroupID, cpoID).Error; err != nil {
			return mapUserGroupNotFound(err)
		}

		var customer models.Customer
		if err := tx.First(&customer, "id = ? AND cpo_id = ?", request.CustomerID, cpoID).Error; err != nil {
			return mapCustomerNotFound(err)
		}

		if customer.UserGroupID != nil {
			if *customer.UserGroupID == userGroupID {
				return nil // Already in the group, do nothing.
			}
			return &auth.APIError{
				Status:  http.StatusConflict,
				Code:    "customer_already_in_group",
				Message: "The customer is already a member of another user group.",
			}
		}

		now := service.now()
		if err := tx.Model(&customer).
			Updates(map[string]any{
				"user_group_id": userGroupID,
				"updated_at":    now,
			}).Error; err != nil {
			return fmt.Errorf("add member to user group: %w", err)
		}

		return writeAudit(
			tx,
			principal.UserID,
			cpoID,
			"USER_GROUP_MEMBER_ADDED",
			models.JSONB{
				"user_group_id": userGroupID,
				"customer_id":   request.CustomerID,
			},
			now,
		)
	})
}

func (service *Service) RemoveMemberFromUserGroup(
	ctx context.Context,
	principal auth.Principal,
	userGroupID uuid.UUID,
	customerID uuid.UUID,
) error {
	if err := requireCPOContext(principal); err != nil {
		return err
	}

	cpoID := *principal.CPOID
	return service.database.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var userGroup models.UserGroup
		if err := tx.First(&userGroup, "id = ? AND cpo_id = ?", userGroupID, cpoID).Error; err != nil {
			return mapUserGroupNotFound(err)
		}

		var customer models.Customer
		if err := tx.First(&customer, "id = ? AND cpo_id = ?", customerID, cpoID).Error; err != nil {
			return mapCustomerNotFound(err)
		}

		if customer.UserGroupID == nil || *customer.UserGroupID != userGroupID {
			return nil // Not in the group, do nothing.
		}

		now := service.now()
		if err := tx.Model(&customer).
			Updates(map[string]any{
				"user_group_id": nil,
				"updated_at":    now,
			}).Error; err != nil {
			return fmt.Errorf("remove member from user group: %w", err)
		}

		return writeAudit(
			tx,
			principal.UserID,
			cpoID,
			"USER_GROUP_MEMBER_REMOVED",
			models.JSONB{
				"user_group_id": userGroupID,
				"customer_id":   customerID,
			},
			now,
		)
	})
}
func mapUserGroupDeleteError(err error) error {
	var postgresError *pgconn.PgError
	if errors.As(err, &postgresError) && (postgresError.Code == "23001" || postgresError.Code == "23503") {
		return &auth.APIError{
			Status:  http.StatusConflict,
			Code:    "user_group_in_use",
			Message: "The user group cannot be deleted because it has dependent records, such as tariffs.",
		}
	}
	return fmt.Errorf("delete user group: %w", err)
}

func userGroupView(record models.UserGroup, members ...[]CPOAdminCustomerView) UserGroupView {
	view := UserGroupView{
		ID:          record.ID,
		CPOID:       record.CPOID,
		Name:        record.Name,
		Description: record.Description,
		IsActive:    record.IsActive,
		CreatedAt:   record.CreatedAt,
		UpdatedAt:   record.UpdatedAt,
	}
	if len(members) > 0 {
		view.Members = members[0]
	}
	return view
}

func mapUserGroupWriteError(err error, operation string) error {
	var postgresError *pgconn.PgError
	if errors.As(err, &postgresError) {
		switch postgresError.Code {
		case "23505":
			return &auth.APIError{
				Status:  http.StatusConflict,
				Code:    "user_group_conflict",
				Message: "The user group already exists or conflicts with another record.",
			}
		case "23503":
			return &auth.APIError{
				Status:  http.StatusConflict,
				Code:    "user_group_conflict",
				Message: "The user group references an invalid related record.",
			}
		}
	}
	return fmt.Errorf("%s: %w", operation, err)
}

func (service *Service) GetSettings(
	ctx context.Context,
	principal auth.Principal,
) (SettingsView, error) {
	if err := requireCPOContext(principal); err != nil {
		return SettingsView{}, err
	}

	cpoID := *principal.CPOID
	var settings models.Settings
	if err := service.database.WithContext(ctx).Where("cpo_id = ?", cpoID).First(&settings).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return SettingsView{}, &auth.APIError{
				Status:  http.StatusNotFound,
				Code:    "settings_not_found",
				Message: "Settings for this CPO not found.",
			}
		}
		return SettingsView{}, fmt.Errorf("failed to get settings: %w", err)
	}

	return SettingsView{
		InvoiceLogo:            settings.InvoiceLogo,
		InvoiceNote:            settings.InvoiceNote,
		WalletMinBalance:       settings.WalletMinBalance,
		WalletBufferMinBalance: settings.WalletBufferMinBalance,
	}, nil
}

func (service *Service) CreateOrUpdateSettings(
	ctx *gin.Context,
	principal auth.Principal,
) (SettingsView, error) {
	if err := requireCPOContext(principal); err != nil {
		return SettingsView{}, err
	}

	cpoID := *principal.CPOID
	err := ctx.Request.ParseMultipartForm(10 << 20) // 10 MB
	if err != nil {
		return SettingsView{}, &auth.APIError{
			Status:  http.StatusBadRequest,
			Code:    "invalid_request",
			Message: "Invalid multipart form.",
		}
	}

	invoiceNote := ctx.Request.FormValue("invoice_note")
	walletMinBalance, err := optionalNonNegativeWholeCurrencyFormValue(ctx, "wallet_min_balance")
	if err != nil {
		return SettingsView{}, err
	}
	walletBufferMinBalance, err := optionalNonNegativeWholeCurrencyFormValue(ctx, "wallet_buffer_min_balance")
	if err != nil {
		return SettingsView{}, err
	}

	var settings models.Settings
	err = service.database.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("cpo_id = ?", cpoID).First(&settings).Error; err != nil {
			if !errors.Is(err, gorm.ErrRecordNotFound) {
				return fmt.Errorf("failed to get settings: %w", err)
			}
			// Settings not found, create new
			settings = models.Settings{
				CPOID: cpoID,
			}
		}

		file, header, err := ctx.Request.FormFile("invoice_logo")
		if err == nil {
			defer file.Close()
			//- TODO: delete old file if it exists
			filename := uuid.New().String() + filepath.Ext(header.Filename)
			uploadsDir := "uploads"
			if _, err := os.Stat(uploadsDir); os.IsNotExist(err) {
				if err := os.Mkdir(uploadsDir, 0755); err != nil {
					return fmt.Errorf("failed to create uploads directory: %w", err)
				}
			}
			filePath := filepath.Join(uploadsDir, filename)
			out, err := os.Create(filePath)
			if err != nil {
				return fmt.Errorf("failed to create file: %w", err)
			}
			defer out.Close()
			_, err = io.Copy(out, file)
			if err != nil {
				return fmt.Errorf("failed to save file: %w", err)
			}
			settings.InvoiceLogo = &filePath
		} else if !errors.Is(err, http.ErrMissingFile) {
			return fmt.Errorf("failed to get invoice logo: %w", err)
		}

		if invoiceNote != "" {
			settings.InvoiceNote = &invoiceNote
		}
		if walletMinBalance != nil {
			settings.WalletMinBalance = *walletMinBalance
		}
		if walletBufferMinBalance != nil {
			settings.WalletBufferMinBalance = *walletBufferMinBalance
		}

		if err := tx.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "cpo_id"}},
			DoUpdates: clause.AssignmentColumns([]string{"invoice_logo", "invoice_note", "wallet_min_balance", "wallet_buffer_min_balance", "updated_at"}),
		}).Create(&settings).Error; err != nil {
			return fmt.Errorf("failed to save settings: %w", err)
		}

		return nil
	})

	if err != nil {
		return SettingsView{}, err
	}

	return SettingsView{
		InvoiceLogo:            settings.InvoiceLogo,
		InvoiceNote:            settings.InvoiceNote,
		WalletMinBalance:       settings.WalletMinBalance,
		WalletBufferMinBalance: settings.WalletBufferMinBalance,
	}, nil
}

func optionalNonNegativeWholeCurrencyFormValue(ctx *gin.Context, field string) (*int, error) {
	raw, present := ctx.GetPostForm(field)
	if !present {
		return nil, nil
	}
	value, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || value < 0 {
		return nil, invalid(field, "Wallet policy values must be non-negative whole currency amounts.")
	}
	return &value, nil
}

// DownloadInvoiceLogo retrieves the invoice logo file for the authenticated CPO.
func (service *Service) DownloadInvoiceLogo(ctx context.Context, principal auth.Principal) (*ImageDownload, error) {
	if err := requireCPOContext(principal); err != nil {
		return nil, err
	}

	cpoID := *principal.CPOID
	var settings models.Settings
	if err := service.database.WithContext(ctx).
		Where("cpo_id = ?", cpoID).
		First(&settings).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, &auth.APIError{
				Status:  http.StatusNotFound,
				Code:    "settings_not_found",
				Message: "Settings for this CPO not found.",
			}
		}
		return nil, fmt.Errorf("failed to get settings: %w", err)
	}

	if settings.InvoiceLogo == nil || *settings.InvoiceLogo == "" {
		return nil, &auth.APIError{
			Status:  http.StatusNotFound,
			Code:    "invoice_logo_not_found",
			Message: "No invoice logo has been uploaded.",
		}
	}

	imagePath := *settings.InvoiceLogo
	if strings.Contains(imagePath, "..") {
		return nil, &auth.APIError{
			Status:  http.StatusBadRequest,
			Code:    "invalid_image_path",
			Message: "The image path is invalid.",
		}
	}

	file, err := os.Open(imagePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, &auth.APIError{
				Status:  http.StatusNotFound,
				Code:    "invoice_logo_not_found",
				Message: "The invoice logo file was not found.",
			}
		}
		return nil, fmt.Errorf("open invoice logo: %w", err)
	}

	info, err := file.Stat()
	if err != nil {
		file.Close()
		return nil, fmt.Errorf("stat invoice logo: %w", err)
	}

	buffer := make([]byte, 512)
	_, err = file.Read(buffer)
	if err != nil && err != io.EOF {
		file.Close()
		return nil, fmt.Errorf("read invoice logo for mime detection: %w", err)
	}
	if _, err := file.Seek(0, 0); err != nil {
		file.Close()
		return nil, fmt.Errorf("seek invoice logo: %w", err)
	}

	mimeType := http.DetectContentType(buffer)

	return &ImageDownload{
		Content:      file,
		OriginalName: filepath.Base(imagePath),
		DetectedMIME: mimeType,
		ModTime:      info.ModTime(),
	}, nil
}

// UpdateChargerCustomerVisibility updates the customer visibility of a specific charger.
func (service *Service) UpdateChargerCustomerVisibility(
	ctx context.Context,
	principal auth.Principal,
	chargerID uuid.UUID,
	request UpdateChargerCustomerVisibilityRequest,
) (ChargerResponse, error) {
	if err := requireCPOContext(principal); err != nil {
		return ChargerResponse{}, err
	}

	cpoID := *principal.CPOID
	var charger models.Charger

	err := service.database.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Preload("Connectors", func(tx *gorm.DB) *gorm.DB {
				return tx.Order("connector_number ASC")
			}).
			Preload("Hub").
			First(&charger, "id = ? AND cpo_id = ?", chargerID, cpoID).Error; err != nil {
			return mapChargerNotFound(err)
		}
		if request.CustomerVisible && charger.HubID == nil {
			return &auth.APIError{Status: http.StatusConflict, Code: "charger_hub_required", Message: "A charger must belong to a hub before it can be customer-visible."}
		}

		now := service.now()
		if err := tx.Model(&charger).
			Updates(map[string]any{
				"customer_visibility": request.CustomerVisible,
				"updated_at":          now,
			}).Error; err != nil {
			return mapChargerWriteError(err, "update charger customer visibility")
		}
		charger.CustomerVisibility = request.CustomerVisible
		charger.UpdatedAt = now

		return writeAudit(
			tx,
			principal.UserID,
			cpoID,
			"CHARGER_CUSTOMER_VISIBILITY_UPDATED",
			models.JSONB{
				"charger_id":          charger.ID,
				"customer_visibility": request.CustomerVisible,
			},
			now,
		)
	})

	if err != nil {
		return ChargerResponse{}, err
	}

	// Reload associations if needed (they are already preloaded in the transaction)
	return service.chargerView(charger, principal), nil
}

// Add this method to the Service (around line 350, after existing list methods)
func (service *Service) ListWalletTransactions(
	ctx context.Context,
	principal auth.Principal,
	query WalletTransactionListQuery,
) (WalletTransactionListResponse, error) {
	if err := requireCPOContext(principal); err != nil {
		return WalletTransactionListResponse{}, err
	}

	if query.Limit == 0 {
		query.Limit = defaultListLimit
	}
	if query.Limit < 1 || query.Limit > maxListLimit {
		return WalletTransactionListResponse{}, invalid(
			"limit",
			"Limit must be between 1 and 200.",
		)
	}

	transactions, err := service.repository.ListWalletTransactions(ctx, *principal.CPOID, query)
	if err != nil {
		return WalletTransactionListResponse{}, fmt.Errorf("list wallet transactions: %w", err)
	}

	hasMore := len(transactions) > query.Limit
	if hasMore {
		transactions = transactions[:query.Limit]
	}

	result := make([]WalletTransactionView, 0, len(transactions))
	for _, tx := range transactions {
		result = append(result, WalletTransactionView{
			ID:              tx.ID,
			CustomerID:      tx.CustomerID,
			CustomerName:    tx.CustomerName,
			CustomerEmail:   tx.CustomerEmail,
			Amount:          tx.Amount,
			Currency:        tx.Currency,
			TransactionType: string(tx.TransactionType), // Assuming this field exists in the model
			Status:          string(tx.Status),          // Assuming this field exists in the model
			Description:     &tx.Description,            // Assuming this field exists in the model
			SessionID:       tx.SessionID,
			RechargeOrderID: tx.RechargeOrderID,
			CreatedAt:       tx.CreatedAt,
		})
	}

	response := WalletTransactionListResponse{
		Transactions: result,
		HasMore:      hasMore,
	}

	if hasMore && len(transactions) > 0 {
		nextBefore := transactions[len(transactions)-1].CreatedAt
		nextBeforeID := transactions[len(transactions)-1].ID
		response.NextBefore = &nextBefore
		response.NextBeforeID = &nextBeforeID
	}

	return response, nil
}
