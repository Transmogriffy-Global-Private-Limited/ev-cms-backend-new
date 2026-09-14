// Package invoice owns the downstream charging-session invoice artifact. It
// never mutates charging, wallet, payment, or HAL state.
package invoice

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/color"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"log"
	"net/http"
	"net/mail"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
	"unicode"

	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/config"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/constants"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/models"
	"github.com/go-fonts/dejavu/dejavusans"
	"github.com/go-fonts/dejavu/dejavusansbold"
	"github.com/google/uuid"
	"github.com/phpdave11/gofpdf"
	"github.com/shopspring/decimal"
	"github.com/tdewolff/canvas"
	canvaspdf "github.com/tdewolff/canvas/renderers/pdf"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	rendererVersion       = "canvas-noto-v2"
	legacyCanvasRenderer  = "canvas-noto-v1"
	snapshotVersion       = 1
	invoiceGenerationLock = 5 * time.Minute
	invoiceDeliveryLock   = 5 * time.Minute
	maxLogoBytes          = 2 << 20
	maxInvoiceSerial      = 999999
	maxGenerationAttempts = 8
)

// Noto Sans Bengali and Noto Sans Devanagari are embedded under the SIL Open
// Font License 1.1 (assets/OFL.txt). Canvas uses its pure-Go text-processing
// backend for OpenType shaping; no system font, CGO, browser, or office suite
// is required at runtime.
//
//go:embed assets/NotoSansBengali-Variable.ttf
var notoSansBengali []byte

//go:embed assets/NotoSansDevanagari-Variable.ttf
var notoSansDevanagari []byte

var ErrInvoiceNumberExhausted = errors.New("invoice number sequence exhausted")

type PublicState string

const (
	PublicNotAvailable PublicState = "NOT_AVAILABLE"
	PublicPending      PublicState = "PENDING"
	PublicReady        PublicState = "READY"
	PublicFailed       PublicState = "FAILED"
)

// Summary is safe projection metadata. It deliberately excludes the issuance
// snapshot, storage path, claim state, recipient, and safe internal errors.
type Summary struct {
	State             PublicState `json:"state"`
	InvoiceID         *uuid.UUID  `json:"invoice_id,omitempty"`
	InvoiceNumber     *string     `json:"invoice_number,omitempty"`
	IssuedAt          *time.Time  `json:"issued_at,omitempty"`
	ReadyAt           *time.Time  `json:"ready_at,omitempty"`
	DownloadAvailable bool        `json:"download_available"`
}

// CPOSummary is operationally safe metadata for the CPO only. Customer
// summaries deliberately omit all delivery state.
type CPOSummary struct {
	Summary
	DeliveryStatus *string `json:"delivery_status,omitempty"`
}

// DeliveryRecovery is the safe, non-PII result of a deliberate CPO resend
// request. Queueing does not claim that SMTP delivery has happened.
type DeliveryRecovery struct {
	InvoiceID      uuid.UUID `json:"invoice_id"`
	PreviousStatus string    `json:"previous_status,omitempty"`
	DeliveryStatus string    `json:"delivery_status"`
	Queued         bool      `json:"queued"`
}

type Download struct {
	Content io.ReadSeekCloser
	Name    string
	ModTime time.Time
}

// DeliverySender performs deterministic message construction before the SMTP
// boundary. Transport errors after PreparedDelivery.Send are ambiguous.
type DeliverySender interface {
	PrepareInvoice(string, string, string, []byte, string, string) (func(context.Context) error, error)
}

type WorkerObserver interface {
	Heartbeat(context.Context, string, string) error
	JobCompleted(context.Context, string, string) error
	MarkUnhealthy(context.Context, string, string) error
}

type Service struct {
	database      *gorm.DB
	storageRoot   string
	pollEvery     time.Duration
	batchSize     int
	displayZone   *time.Location
	now           func() time.Time
	sender        DeliverySender
	observer      WorkerObserver
	workerName    string
	instanceKey   string
	lastHeartbeat time.Time
}

func NewService(database *gorm.DB, cfg config.Invoice, displayZone *time.Location) (*Service, error) {
	if database == nil {
		return nil, errors.New("invoice database is required")
	}
	if displayZone == nil {
		return nil, errors.New("invoice display timezone is required")
	}
	if strings.TrimSpace(cfg.StorageRoot) == "" || cfg.WorkerPoll <= 0 || cfg.BatchSize < 1 || cfg.BatchSize > 100 {
		return nil, errors.New("invoice storage root must be nonblank, worker interval must be positive, and batch size must be between 1 and 100")
	}
	root, err := filepath.Abs(cfg.StorageRoot)
	if err != nil {
		return nil, fmt.Errorf("resolve invoice storage root: %w", err)
	}
	return &Service{database: database, storageRoot: root, pollEvery: cfg.WorkerPoll, batchSize: cfg.BatchSize, displayZone: displayZone, now: func() time.Time { return time.Now().UTC() }}, nil
}

func (service *Service) WithDeliverySender(sender DeliverySender) *Service {
	service.sender = sender
	return service
}

func (service *Service) WithWorkerObserver(observer WorkerObserver, workerName, instanceKey string) *Service {
	service.observer, service.workerName, service.instanceKey = observer, workerName, instanceKey
	return service
}

// Run performs bounded discovery. Invoice artifact recovery is independent of
// mail delivery, and both are independent of charging settlement.
func (service *Service) Run(ctx context.Context) {
	ticker := time.NewTicker(service.pollEvery)
	defer ticker.Stop()
	for {
		service.heartbeat(ctx)
		if err := service.Reconcile(ctx, service.batchSize); err != nil && ctx.Err() == nil {
			log.Printf("invoice reconciliation failed: %v", err)
			service.unhealthy(ctx)
		}
		if service.sender != nil {
			if err := service.DeliverPending(ctx, service.batchSize); err != nil && ctx.Err() == nil {
				log.Printf("invoice delivery reconciliation failed: %v", err)
				service.unhealthy(ctx)
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (service *Service) heartbeat(ctx context.Context) {
	if service.observer == nil || time.Since(service.lastHeartbeat) < 10*time.Second {
		return
	}
	if err := service.observer.Heartbeat(ctx, service.workerName, service.instanceKey); err == nil {
		service.lastHeartbeat = service.now()
	}
}
func (service *Service) unhealthy(ctx context.Context) {
	if service.observer != nil {
		_ = service.observer.MarkUnhealthy(ctx, service.workerName, service.instanceKey)
	}
}
func (service *Service) completed(ctx context.Context) {
	if service.observer != nil {
		_ = service.observer.JobCompleted(ctx, service.workerName, service.instanceKey)
	}
}

// Reconcile first creates a canonical invoice row for every settled session,
// including historical rows, then advances a bounded set of safe generation
// candidates. The migration-time rollout guard controls email separately.
func (service *Service) Reconcile(ctx context.Context, limit int) error {
	if limit < 1 {
		limit = 20
	}
	var sessionIDs []uuid.UUID
	if err := service.database.WithContext(ctx).Raw(`
		SELECT s.id
		  FROM charging_sessions s
		 WHERE s.status = ? AND s.settlement_status = 'SETTLED'
		   AND NOT EXISTS (SELECT 1 FROM charging_session_invoices i WHERE i.session_id = s.id)
		 ORDER BY s.end_time NULLS LAST, s.id
		 LIMIT ?`, constants.SessionStatusCompleted, limit).Scan(&sessionIDs).Error; err != nil {
		return fmt.Errorf("discover settled sessions: %w", err)
	}
	ensureErr := service.ensureInvoices(ctx, sessionIDs)
	// Continue bounded artifact generation even when one historical session
	// cannot yet be issued. The returned error makes the worker health/report
	// truthfully reflect that an issuance remains unresolved.
	for count := 0; count < limit; count++ {
		invoice, claimed, err := service.claimGeneration(ctx)
		if err != nil {
			return errors.Join(ensureErr, err)
		}
		if !claimed {
			break
		}
		generated, err := service.generate(ctx, invoice)
		if err != nil {
			return errors.Join(ensureErr, err)
		}
		if generated {
			service.completed(ctx)
		}
	}
	return ensureErr
}

func (service *Service) ensureInvoices(ctx context.Context, sessionIDs []uuid.UUID) error {
	return ensureInvoiceBatch(ctx, sessionIDs, service.EnsureInvoice)
}

func ensureInvoiceBatch(ctx context.Context, sessionIDs []uuid.UUID, ensure func(context.Context, uuid.UUID) (models.ChargingSessionInvoice, error)) error {
	var failures []error
	for _, sessionID := range sessionIDs {
		if _, err := ensure(ctx, sessionID); err != nil {
			failures = append(failures, fmt.Errorf("issue session %s: %w", sessionID, err))
		}
	}
	if len(failures) == 0 {
		return nil
	}
	return fmt.Errorf("%d charging-session invoice issuances failed: %w", len(failures), errors.Join(failures...))
}

// EnsureInvoice allocates the number and captures immutable issuance inputs in
// one transaction only after rechecking the financial-finality predicate.
func (service *Service) EnsureInvoice(ctx context.Context, sessionID uuid.UUID) (models.ChargingSessionInvoice, error) {
	var result models.ChargingSessionInvoice
	err := service.database.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var session models.ChargingSession
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			First(&session, "id = ?", sessionID).Error; err != nil {
			return err
		}
		if session.Status != constants.SessionStatusCompleted || session.SettlementStatus != "SETTLED" {
			return nil
		}
		// The session lock serializes callers. Recheck after taking it so a
		// concurrent creator returns the already-issued canonical row instead
		// of surfacing the unique constraint as a false idempotency failure.
		var existing models.ChargingSessionInvoice
		if err := tx.First(&existing, "session_id = ?", sessionID).Error; err == nil {
			result = existing
			return nil
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		inputs, err := hydrateInvoiceIssuance(tx, session)
		if err != nil {
			return err
		}
		asset, hasLogo, migratedLogoPath, err := snapshotLogo(inputs.Settings.InvoiceLogo)
		if err != nil {
			return fmt.Errorf("snapshot configured CPO invoice logo: %w", err)
		}
		if migratedLogoPath != nil {
			if err := tx.Model(&models.Settings{}).Where("cpo_id = ?", session.CPOID).Update("invoice_logo", *migratedLogoPath).Error; err != nil {
				return fmt.Errorf("record imported legacy CPO invoice logo: %w", err)
			}
		}
		issuedAt := service.now()
		financialYear := indianFinancialYear(issuedAt, service.displayZone)
		serial, err := allocateSerial(tx, session.CPOID, financialYear, issuedAt)
		if err != nil {
			return err
		}
		invoiceID := uuid.New()
		hydratedSession := inputs.snapshotSession()
		snapshot := buildSnapshot(invoiceID, hydratedSession, inputs.CPO, inputs.Settings, issuedAt, financialYear, serial)
		if hasLogo {
			snapshot.LogoSHA256 = asset.hash
		}
		snapshotJSON, err := toJSONB(snapshot)
		if err != nil {
			return err
		}
		result = models.ChargingSessionInvoice{ID: invoiceID, CPOID: session.CPOID, CustomerID: session.CustomerID, SessionID: session.ID, InvoiceNumber: snapshot.InvoiceNumber, FinancialYear: financialYear, Serial: serial, IssuedAt: issuedAt, SnapshotVersion: snapshotVersion, RendererVersion: rendererVersion, Snapshot: snapshotJSON, GenerationStatus: "PENDING", AvailableAt: issuedAt, CreatedAt: issuedAt, UpdatedAt: issuedAt}
		if err := tx.Create(&result).Error; err != nil {
			return err
		}
		if hasLogo {
			if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&models.InvoiceAsset{MIMEType: asset.mimeType, SHA256: asset.hash, Content: asset.content, CreatedAt: issuedAt}).Error; err != nil {
				return err
			}
			if err := tx.Create(&models.InvoiceAssetReference{InvoiceID: invoiceID, SHA256: asset.hash, CreatedAt: issuedAt}).Error; err != nil {
				return err
			}
		}
		var rollout models.InvoiceRollout
		if err := tx.First(&rollout, 1).Error; err != nil {
			return err
		}
		// Artifacts are eligible for all settled sessions. Delivery is a separate
		// rollout policy based on the durable financial-finality transition, and
		// no intent is created when SMTP is unavailable at issuance time.
		if service.sender != nil && session.SettledAt != nil && !session.SettledAt.Before(rollout.AutomaticEmailFrom) && strings.TrimSpace(hydratedSession.Customer.Email) != "" {
			return tx.Create(&models.InvoiceDelivery{ID: uuid.New(), InvoiceID: invoiceID, Recipient: hydratedSession.Customer.Email, Status: "PENDING", MaxAttempts: 8, AvailableAt: issuedAt, CreatedAt: issuedAt, UpdatedAt: issuedAt}).Error
		}
		return nil
	})
	if err != nil {
		return models.ChargingSessionInvoice{}, fmt.Errorf("ensure charging-session invoice: %w", err)
	}
	return result, nil
}

// invoiceIssuanceInputs is the explicit boundary between mutable CMS records
// and an immutable customer document. Do not add associations to the session
// query and rely on GORM to populate them incidentally: every rendered input
// below is loaded by its authoritative ID and tenant-checked before a number
// is allocated.
type invoiceIssuanceInputs struct {
	Session     models.ChargingSession
	Customer    models.Customer
	Connector   models.Connector
	Charger     models.Charger
	Hub         *models.Hub
	StartIntent *models.ChargingStartIntent
	Payment     *models.Payment
	CPO         models.CPO
	Settings    models.Settings
}

func hydrateInvoiceIssuance(tx *gorm.DB, session models.ChargingSession) (invoiceIssuanceInputs, error) {
	inputs := invoiceIssuanceInputs{Session: session}
	if err := tx.First(&inputs.Customer, "id = ?", session.CustomerID).Error; err != nil {
		return invoiceIssuanceInputs{}, fmt.Errorf("hydrate invoice customer: %w", err)
	}
	if err := tx.First(&inputs.Connector, "id = ?", session.ConnectorID).Error; err != nil {
		return invoiceIssuanceInputs{}, fmt.Errorf("hydrate invoice connector: %w", err)
	}
	if err := tx.First(&inputs.Charger, "id = ?", session.ChargerID).Error; err != nil {
		return invoiceIssuanceInputs{}, fmt.Errorf("hydrate invoice charger: %w", err)
	}
	if inputs.Charger.HubID != nil {
		var hub models.Hub
		if err := tx.First(&hub, "id = ?", *inputs.Charger.HubID).Error; err != nil {
			return invoiceIssuanceInputs{}, fmt.Errorf("hydrate invoice charging hub: %w", err)
		}
		inputs.Hub = &hub
	}
	if session.StartIntentID != nil {
		var intent models.ChargingStartIntent
		if err := tx.First(&intent, "id = ?", *session.StartIntentID).Error; err != nil {
			return invoiceIssuanceInputs{}, fmt.Errorf("hydrate invoice start intent: %w", err)
		}
		inputs.StartIntent = &intent
	}
	var payment models.Payment
	if err := tx.Where("session_id = ?", session.ID).First(&payment).Error; err == nil {
		inputs.Payment = &payment
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return invoiceIssuanceInputs{}, fmt.Errorf("hydrate invoice payment: %w", err)
	}
	if err := tx.First(&inputs.CPO, "id = ?", session.CPOID).Error; err != nil {
		return invoiceIssuanceInputs{}, fmt.Errorf("hydrate invoice CPO: %w", err)
	}
	if err := tx.Where("cpo_id = ?", session.CPOID).First(&inputs.Settings).Error; err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return invoiceIssuanceInputs{}, fmt.Errorf("hydrate invoice CPO settings: %w", err)
	}
	if err := inputs.validate(); err != nil {
		return invoiceIssuanceInputs{}, err
	}
	return inputs, nil
}

func (inputs invoiceIssuanceInputs) validate() error {
	session := inputs.Session
	switch {
	case inputs.Customer.ID != session.CustomerID:
		return errors.New("invoice customer does not match the charging session")
	case inputs.Customer.CPOID != session.CPOID:
		return errors.New("invoice customer does not belong to the charging-session CPO")
	case inputs.Connector.ID != session.ConnectorID:
		return errors.New("invoice connector does not match the charging session")
	case inputs.Connector.CPOID != session.CPOID:
		return errors.New("invoice connector does not belong to the charging-session CPO")
	case inputs.Connector.ChargerID != session.ChargerID:
		return errors.New("invoice connector does not belong to the charging-session charger")
	case inputs.Charger.ID != session.ChargerID:
		return errors.New("invoice charger does not match the charging session")
	case inputs.Charger.CPOID != session.CPOID:
		return errors.New("invoice charger does not belong to the charging-session CPO")
	case inputs.Charger.HubID == nil && inputs.Hub != nil:
		return errors.New("invoice hydration found a hub for a hubless charger")
	case inputs.Charger.HubID != nil && inputs.Hub == nil:
		return errors.New("invoice charger hub is required but was not hydrated")
	case inputs.Hub != nil && (inputs.Hub.ID != *inputs.Charger.HubID || inputs.Hub.CPOID != session.CPOID):
		return errors.New("invoice charging hub does not belong to the charging-session CPO")
	case inputs.CPO.ID != session.CPOID:
		return errors.New("invoice CPO does not match the charging session")
	case session.StartIntentID != nil && (inputs.StartIntent == nil || inputs.StartIntent.ID != *session.StartIntentID):
		return errors.New("invoice start intent does not match the charging session")
	case inputs.StartIntent != nil && (inputs.StartIntent.CPOID != session.CPOID || inputs.StartIntent.CustomerID != session.CustomerID || inputs.StartIntent.ChargerID != session.ChargerID || inputs.StartIntent.ConnectorID != session.ConnectorID):
		return errors.New("invoice start intent does not match the charging session")
	case inputs.Payment != nil && (inputs.Payment.CPOID != session.CPOID || inputs.Payment.SessionID != session.ID):
		return errors.New("invoice payment does not match the charging session")
	}
	return nil
}

func (inputs invoiceIssuanceInputs) snapshotSession() models.ChargingSession {
	session := inputs.Session
	session.Customer = inputs.Customer
	session.Connector = inputs.Connector
	session.Charger = inputs.Charger
	session.Charger.Hub = inputs.Hub
	session.StartIntent = inputs.StartIntent
	session.Payment = inputs.Payment
	return session
}

func allocateSerial(tx *gorm.DB, cpoID uuid.UUID, year string, now time.Time) (int64, error) {
	var serial int64
	row := tx.Raw(`INSERT INTO invoice_number_sequences (cpo_id, financial_year, next_serial, updated_at) VALUES (?, ?, 2, ?)
		ON CONFLICT (cpo_id, financial_year) DO UPDATE
		SET next_serial = invoice_number_sequences.next_serial + 1, updated_at = EXCLUDED.updated_at
		WHERE invoice_number_sequences.next_serial <= ?
		RETURNING next_serial - 1`, cpoID, year, now, maxInvoiceSerial).Row()
	if err := row.Scan(&serial); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) || errors.Is(err, sql.ErrNoRows) {
			return 0, ErrInvoiceNumberExhausted
		}
		return 0, fmt.Errorf("allocate invoice serial: %w", err)
	}
	if serial < 1 || serial > maxInvoiceSerial {
		return 0, ErrInvoiceNumberExhausted
	}
	return serial, nil
}

func indianFinancialYear(at time.Time, zone *time.Location) string {
	local := at.In(zone)
	start := local.Year()
	if local.Month() < time.April {
		start--
	}
	return fmt.Sprintf("%02d-%02d", start%100, (start+1)%100)
}
func invoiceNumber(year string, serial int64) string { return fmt.Sprintf("INV/%s/%06d", year, serial) }

type issuanceSnapshot struct {
	SchemaVersion int                `json:"schema_version"`
	InvoiceID     string             `json:"invoice_id"`
	InvoiceNumber string             `json:"invoice_number"`
	FinancialYear string             `json:"financial_year"`
	IssuedAt      time.Time          `json:"issued_at"`
	Supplier      supplierSnapshot   `json:"supplier"`
	Customer      customerSnapshot   `json:"customer"`
	Location      locationSnapshot   `json:"location"`
	Charging      chargingSnapshot   `json:"charging"`
	Commercial    commercialSnapshot `json:"commercial"`
	InvoiceNote   string             `json:"invoice_note,omitempty"`
	LogoSHA256    string             `json:"logo_sha256,omitempty"`
}
type supplierSnapshot struct{ Name, CompanyType, GSTIN, Address, City, State, Pincode string }
type customerSnapshot struct {
	ID, FullName, Email string
	Phone               *string `json:"phone,omitempty"`
}
type locationSnapshot struct {
	HubName, HubAddress, HubState, ChargerCode, ChargerName, ChargerType, ConnectorType string
	ConnectorNumber                                                                     int
	RatedPowerKW                                                                        float64
}
type chargingSnapshot struct {
	SessionID                                                   string
	OCPPTransactionID                                           int64
	StartedAt                                                   time.Time
	EndedAt                                                     *time.Time
	DurationSeconds                                             int64
	MeterStartWh                                                int64
	MeterStopWh                                                 *int64
	LatestMeterWh                                               *int64
	InitialSoCPercent                                           *string
	LatestSoCPercent                                            *string
	SoCObservedAt                                               *time.Time
	RequestedStopInitiator, RequestedStopReason, OCPPStopReason *string
	LimitType                                                   *string
	RequestedLimitValue                                         *string
	EnergyLimitWh                                               *int64  `json:"energy_limit_wh,omitempty"`
	MaxDurationSeconds                                          *int64  `json:"max_duration_seconds,omitempty"`
	EnergyLimitSource, DurationLimitSource                      *string `json:"-"`
}
type commercialSnapshot struct {
	TotalKWh, TotalAmount, Currency string
	SettlementStatus                string
	PaymentMethod                   *string `json:"payment_method,omitempty"`
	Tariff                          invoiceTariffSnapshot
	Tax                             invoiceTaxSnapshot
}

// invoiceTariffSnapshot and invoiceTaxSnapshot are a small read-only
// customer-document projection of the authoritative frozen JSON. They do not
// recalculate price selection or invent tax components absent from the source.
type invoiceTariffSnapshot struct {
	BillingUnit  *string `json:"billing_unit,omitempty"`
	PricePerUnit *string `json:"price_per_unit,omitempty"`
	PriceType    *string `json:"price_type,omitempty"`
	TariffType   *string `json:"tariff_type,omitempty"`
}
type invoiceTaxSnapshot struct {
	CGSTRate *string `json:"cgst_rate,omitempty"`
	SGSTRate *string `json:"sgst_rate,omitempty"`
	IGSTRate *string `json:"igst_rate,omitempty"`
}

func buildSnapshot(invoiceID uuid.UUID, s models.ChargingSession, cpo models.CPO, settings models.Settings, issuedAt time.Time, year string, serial int64) issuanceSnapshot {
	var duration int64
	if s.EndTime != nil && !s.EndTime.Before(s.StartTime) {
		duration = int64(s.EndTime.Sub(s.StartTime).Seconds())
	}
	var limitType *string
	var energySource, durationSource *string
	var requested *string
	var energy, maximum *int64
	if s.StartIntent != nil {
		limit, energyProvenance, durationProvenance := string(s.StartIntent.LimitType), string(s.StartIntent.EnergyLimitSource), string(s.StartIntent.DurationLimitSource)
		limitType, energySource, durationSource = &limit, &energyProvenance, &durationProvenance
		if s.StartIntent.EnergyLimitSource != constants.ChargingLimitSourceNone {
			value := s.StartIntent.EnergyLimitWh
			energy = &value
		}
		if s.StartIntent.DurationLimitSource != constants.ChargingLimitSourceNone {
			value := s.StartIntent.MaxDurationSeconds
			maximum = &value
		}
		if s.StartIntent.RequestedLimitValue != nil {
			text := s.StartIntent.RequestedLimitValue.String()
			requested = &text
		}
	}
	var method *string
	if s.Payment != nil {
		m := s.Payment.PaymentMethod
		method = &m
	}
	var initial, latest *string
	if s.InitialSoCPercent != nil {
		value := s.InitialSoCPercent.String()
		initial = &value
	}
	if s.LatestSoCPercent != nil {
		value := s.LatestSoCPercent.String()
		latest = &value
	}
	note := ""
	if settings.InvoiceNote != nil {
		note = strings.TrimSpace(*settings.InvoiceNote)
	}
	location := locationSnapshot{ChargerCode: s.Charger.ChargerID, ChargerName: s.Charger.ChargerName, ChargerType: s.Charger.ChargerType, ConnectorNumber: s.Connector.ConnectorNumber, ConnectorType: s.Connector.ConnectorType, RatedPowerKW: s.Connector.ConnectorTotalCapacity}
	if s.Charger.Hub != nil {
		location.HubName, location.HubAddress, location.HubState = s.Charger.Hub.Name, s.Charger.Hub.Address, string(s.Charger.Hub.State)
	}
	return issuanceSnapshot{SchemaVersion: snapshotVersion, InvoiceID: invoiceID.String(), InvoiceNumber: invoiceNumber(year, serial), FinancialYear: year, IssuedAt: issuedAt, Supplier: supplierSnapshot{Name: cpo.BusinessName, CompanyType: string(cpo.CompanyType), GSTIN: cpo.GSTIN, Address: cpo.Address, City: cpo.City, State: string(cpo.State), Pincode: cpo.Pincode}, Customer: customerSnapshot{ID: s.Customer.ID.String(), FullName: s.Customer.FullName, Email: s.Customer.Email, Phone: s.Customer.Phone}, Location: location, Charging: chargingSnapshot{SessionID: s.ID.String(), OCPPTransactionID: s.TransactionID, StartedAt: s.StartTime, EndedAt: s.EndTime, DurationSeconds: duration, MeterStartWh: s.MeterStartWh, MeterStopWh: s.MeterStopWh, LatestMeterWh: s.LatestMeterWh, InitialSoCPercent: initial, LatestSoCPercent: latest, SoCObservedAt: s.SoCObservedAt, RequestedStopInitiator: s.RequestedStopInitiator, RequestedStopReason: s.RequestedStopReason, OCPPStopReason: s.OCPPStopReason, LimitType: limitType, RequestedLimitValue: requested, EnergyLimitWh: energy, MaxDurationSeconds: maximum, EnergyLimitSource: energySource, DurationLimitSource: durationSource}, Commercial: commercialSnapshot{TotalKWh: s.TotalKWh.StringFixed(3), TotalAmount: s.TotalAmount.StringFixed(2), Currency: s.Currency, SettlementStatus: s.SettlementStatus, PaymentMethod: method, Tariff: invoiceTariffProjection(s.TariffSnapshot), Tax: invoiceTaxProjection(s.TaxSnapshot)}, InvoiceNote: note}
}

func invoiceTariffProjection(snapshot models.JSONB) invoiceTariffSnapshot {
	return invoiceTariffSnapshot{BillingUnit: snapshotStringValue(snapshot, "units"), PricePerUnit: snapshotDecimalValue(snapshot, "price_per_unit"), PriceType: snapshotStringValue(snapshot, "price_type"), TariffType: snapshotStringValue(snapshot, "tariff_type")}
}
func invoiceTaxProjection(snapshot models.JSONB) invoiceTaxSnapshot {
	return invoiceTaxSnapshot{CGSTRate: snapshotDecimalValue(snapshot, "cgst_rate"), SGSTRate: snapshotDecimalValue(snapshot, "sgst_rate"), IGSTRate: snapshotDecimalValue(snapshot, "igst_rate")}
}
func snapshotStringValue(snapshot models.JSONB, key string) *string {
	value, ok := snapshot[key].(string)
	if !ok || strings.TrimSpace(value) == "" {
		return nil
	}
	return &value
}
func snapshotDecimalValue(snapshot models.JSONB, key string) *string {
	value := snapshotStringValue(snapshot, key)
	if value == nil {
		return nil
	}
	if _, err := decimal.NewFromString(*value); err != nil {
		return nil
	}
	return value
}

func cloneJSONB(value models.JSONB) models.JSONB {
	raw, _ := json.Marshal(value)
	var out models.JSONB
	_ = json.Unmarshal(raw, &out)
	if out == nil {
		out = models.JSONB{}
	}
	return out
}
func toJSONB(value any) (models.JSONB, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	var out models.JSONB
	err = json.Unmarshal(raw, &out)
	return out, err
}

type logoAsset struct {
	content        []byte
	mimeType, hash string
}

// snapshotLogo freezes a configured CPO logo before invoice creation. A blank
// setting is intentionally optional. Any nonblank but unreadable, invalid, or
// unsafe setting is an issuance failure, never an invisible loss of branding.
// Valid pre-invoice paths directly under the historical uploads root are
// imported to the current private root before the database setting is updated.
func snapshotLogo(path *string) (logoAsset, bool, *string, error) {
	if path == nil || strings.TrimSpace(*path) == "" {
		return logoAsset{}, false, nil, nil
	}
	resolved, info, err := controlledLogoFile(*path)
	legacy := false
	if err != nil {
		resolved, info, err = controlledLegacyLogoFile(*path)
		if err != nil {
			return logoAsset{}, false, nil, fmt.Errorf("configured invoice logo is unsafe or unavailable: %w", err)
		}
		legacy = true
	}
	asset, err := logoAssetFromFile(resolved, info)
	if err != nil {
		return logoAsset{}, false, nil, fmt.Errorf("configured invoice logo is invalid: %w", err)
	}
	if !legacy {
		return asset, true, nil, nil
	}
	migrated, err := importLegacyLogo(asset)
	if err != nil {
		return logoAsset{}, false, nil, fmt.Errorf("import legacy invoice logo: %w", err)
	}
	return asset, true, &migrated, nil
}

func logoAssetFromFile(path string, info os.FileInfo) (logoAsset, error) {
	if info == nil || !info.Mode().IsRegular() || info.Size() <= 0 || info.Size() > maxLogoBytes {
		return logoAsset{}, errors.New("invoice logo file is not a bounded regular file")
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return logoAsset{}, err
	}
	asset, ok := logoAssetFromContent(content)
	if !ok {
		return logoAsset{}, errors.New("invoice logo is not a complete PNG or JPEG")
	}
	return asset, nil
}

func logoAssetFromContent(content []byte) (logoAsset, bool) {
	if len(content) == 0 || len(content) > maxLogoBytes {
		return logoAsset{}, false
	}
	mimeType := http.DetectContentType(content)
	if mimeType != "image/png" && mimeType != "image/jpeg" {
		return logoAsset{}, false
	}
	config, _, err := image.DecodeConfig(bytes.NewReader(content))
	if err != nil || config.Width < 1 || config.Height < 1 || config.Width > 4096 || config.Height > 4096 {
		return logoAsset{}, false
	}
	decoded, format, err := image.Decode(bytes.NewReader(content))
	if err != nil || decoded.Bounds().Dx() != config.Width || decoded.Bounds().Dy() != config.Height || (mimeType == "image/png" && format != "png") || (mimeType == "image/jpeg" && format != "jpeg") {
		return logoAsset{}, false
	}
	digest := sha256.Sum256(content)
	return logoAsset{content: content, mimeType: mimeType, hash: hex.EncodeToString(digest[:])}, true
}

// controlledLogoFile accepts only a regular, non-symlink file directly below
// the private logo root. Stored legacy paths are treated as untrusted input.
func controlledLogoFile(storedPath string) (string, os.FileInfo, error) {
	root, err := filepath.Abs(filepath.Join("uploads", "invoice-logos"))
	if err != nil {
		return "", nil, err
	}
	candidate, err := filepath.Abs(storedPath)
	if err != nil {
		return "", nil, err
	}
	relative, err := filepath.Rel(root, candidate)
	if err != nil || relative == "." || relative != filepath.Base(relative) || filepath.IsAbs(relative) || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", nil, errors.New("invoice logo is outside controlled storage")
	}
	rootInfo, err := os.Lstat(root)
	if err != nil || !rootInfo.IsDir() || rootInfo.Mode()&os.ModeSymlink != 0 {
		return "", nil, errors.New("invoice logo root is not a controlled directory")
	}
	info, err := os.Lstat(candidate)
	if err != nil || info.Mode()&os.ModeSymlink != 0 {
		return "", nil, errors.New("invoice logo path is unsafe")
	}
	return candidate, info, nil
}

// controlledLegacyLogoFile recognizes only the old application behavior:
// settings referenced a UUID-named image immediately below uploads/. It does
// not treat arbitrary paths in the database as legacy files.
func controlledLegacyLogoFile(storedPath string) (string, os.FileInfo, error) {
	root, err := filepath.Abs("uploads")
	if err != nil {
		return "", nil, err
	}
	candidate, err := filepath.Abs(storedPath)
	if err != nil {
		return "", nil, err
	}
	relative, err := filepath.Rel(root, candidate)
	if err != nil || relative == "." || relative != filepath.Base(relative) || filepath.IsAbs(relative) || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", nil, errors.New("invoice logo is outside the historical controlled uploads root")
	}
	base := filepath.Base(candidate)
	if _, err := uuid.Parse(strings.TrimSuffix(base, filepath.Ext(base))); err != nil {
		return "", nil, errors.New("historical invoice logo does not have an application-generated name")
	}
	rootInfo, err := os.Lstat(root)
	if err != nil || !rootInfo.IsDir() || rootInfo.Mode()&os.ModeSymlink != 0 {
		return "", nil, errors.New("historical invoice logo root is not a controlled directory")
	}
	info, err := os.Lstat(candidate)
	if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return "", nil, errors.New("historical invoice logo path is unsafe")
	}
	return candidate, info, nil
}

func importLegacyLogo(asset logoAsset) (string, error) {
	root, err := filepath.Abs(filepath.Join("uploads", "invoice-logos"))
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(root, 0750); err != nil {
		return "", err
	}
	rootInfo, err := os.Lstat(root)
	if err != nil || !rootInfo.IsDir() || rootInfo.Mode()&os.ModeSymlink != 0 {
		return "", errors.New("invoice logo root is not a controlled directory")
	}
	temporary, err := os.CreateTemp(root, ".invoice-logo-*")
	if err != nil {
		return "", err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(0600); err != nil {
		temporary.Close()
		return "", err
	}
	if _, err := temporary.Write(asset.content); err != nil {
		temporary.Close()
		return "", err
	}
	if err := temporary.Sync(); err != nil {
		temporary.Close()
		return "", err
	}
	if err := temporary.Close(); err != nil {
		return "", err
	}
	extension := ".jpg"
	if asset.mimeType == "image/png" {
		extension = ".png"
	}
	name := asset.hash + extension
	destination := filepath.Join(root, name)
	if existing, err := os.Lstat(destination); err == nil {
		stored, readErr := logoAssetFromFile(destination, existing)
		if readErr != nil || stored.hash != asset.hash {
			return "", errors.New("existing imported invoice logo does not match its content hash")
		}
		return filepath.Join("uploads", "invoice-logos", name), nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	if err := os.Rename(temporaryPath, destination); err != nil {
		if existing, statErr := os.Lstat(destination); statErr == nil {
			stored, readErr := logoAssetFromFile(destination, existing)
			if readErr == nil && stored.hash == asset.hash {
				return filepath.Join("uploads", "invoice-logos", name), nil
			}
		}
		return "", err
	}
	return filepath.Join("uploads", "invoice-logos", name), nil
}

func (service *Service) claimGeneration(ctx context.Context) (models.ChargingSessionInvoice, bool, error) {
	stale := service.database.WithContext(ctx).Exec(`UPDATE charging_session_invoices
		SET generation_status = 'FAILED', generation_locked_at = NULL,
			last_generation_error = COALESCE(last_generation_error, 'Generation attempt limit reached after stale claim.'),
			available_at = now(), updated_at = now()
		WHERE generation_status = 'GENERATING' AND generation_attempts >= ?
		  AND generation_locked_at < now() - (? * interval '1 second')`, maxGenerationAttempts, invoiceGenerationLock.Seconds())
	if stale.Error != nil {
		return models.ChargingSessionInvoice{}, false, stale.Error
	}
	var invoice models.ChargingSessionInvoice
	// CORRUPT is a terminal post-READY integrity state. Only records that have
	// never reached issuance may be retried automatically.
	result := service.database.WithContext(ctx).Raw(`UPDATE charging_session_invoices SET generation_status = 'GENERATING', generation_locked_at = now(), generation_attempts = generation_attempts + 1, updated_at = now() WHERE id = (SELECT id FROM charging_session_invoices WHERE available_at <= now() AND generation_attempts < ? AND (generation_status = 'PENDING' OR (generation_status = 'GENERATING' AND generation_locked_at < now() - (? * interval '1 second'))) ORDER BY available_at, created_at FOR UPDATE SKIP LOCKED LIMIT 1) RETURNING *`, maxGenerationAttempts, invoiceGenerationLock.Seconds()).Scan(&invoice)
	if result.Error != nil {
		return invoice, false, result.Error
	}
	return invoice, result.RowsAffected == 1, nil
}

// generate reports whether an artifact was actually made READY. A retryable
// render failure is durably recorded but is never counted as completed work.
func (service *Service) generate(ctx context.Context, invoice models.ChargingSessionInvoice) (bool, error) {
	snapshot, err := decodeSnapshot(invoice.Snapshot)
	if err != nil {
		return false, service.failGeneration(ctx, invoice, err)
	}
	var asset models.InvoiceAsset
	assetErr := service.database.WithContext(ctx).
		Table("invoice_assets").
		Joins("JOIN invoice_asset_references ON invoice_asset_references.sha256 = invoice_assets.sha256").
		Where("invoice_asset_references.invoice_id = ?", invoice.ID).
		First(&asset).Error
	if assetErr != nil && !errors.Is(assetErr, gorm.ErrRecordNotFound) {
		return false, service.failGeneration(ctx, invoice, assetErr)
	}
	hasAsset := assetErr == nil
	pdf, err := renderInvoice(snapshot, invoice.RendererVersion, asset, hasAsset, service.displayZone)
	if err != nil {
		return false, service.failGeneration(ctx, invoice, err)
	}
	path, size, hash, err := service.publish(invoice, pdf)
	if err != nil {
		return false, service.failGeneration(ctx, invoice, err)
	}
	now := service.now()
	mimeType := "application/pdf"
	if err := service.database.WithContext(ctx).Model(&models.ChargingSessionInvoice{}).Where("id = ? AND generation_status = 'GENERATING'", invoice.ID).Updates(map[string]any{"generation_status": "READY", "generation_locked_at": nil, "last_generation_error": nil, "storage_path": path, "mime_type": mimeType, "file_size": size, "sha256": hash, "ready_at": now, "updated_at": now}).Error; err != nil {
		return false, fmt.Errorf("mark invoice ready: %w", err)
	}
	return true, nil
}

func decodeSnapshot(value models.JSONB) (issuanceSnapshot, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return issuanceSnapshot{}, err
	}
	var snapshot issuanceSnapshot
	if err := json.Unmarshal(raw, &snapshot); err != nil {
		return issuanceSnapshot{}, err
	}
	if snapshot.SchemaVersion != snapshotVersion || snapshot.InvoiceNumber == "" {
		return issuanceSnapshot{}, errors.New("unsupported invoice snapshot")
	}
	return snapshot, nil
}

func (service *Service) failGeneration(ctx context.Context, invoice models.ChargingSessionInvoice, cause error) error {
	now := service.now()
	status := generationFailureStatus(invoice.GenerationAttempts)
	available := now.Add(retryDelay(invoice.GenerationAttempts))
	if status == "FAILED" {
		available = now
	}
	message := boundedError(cause)
	if err := service.database.WithContext(ctx).Model(&models.ChargingSessionInvoice{}).Where("id = ? AND generation_status = 'GENERATING'", invoice.ID).Updates(map[string]any{"generation_status": status, "generation_locked_at": nil, "available_at": available, "last_generation_error": message, "updated_at": now}).Error; err != nil {
		return err
	}
	return nil
}

func generationFailureStatus(attempts int) string {
	if attempts >= maxGenerationAttempts {
		return "FAILED"
	}
	return "PENDING"
}

func retryDelay(attempt int) time.Duration {
	delay := time.Minute
	for n := 1; n < attempt && delay < time.Hour; n++ {
		delay *= 2
	}
	if delay > time.Hour {
		return time.Hour
	}
	return delay
}
func boundedError(err error) string {
	text := err.Error()
	if len(text) > 500 {
		return text[:500]
	}
	return text
}

// publish uses one deterministic pre-ready object identity per invoice. A
// restart after rename but before the DB READY transition adopts matching bytes
// instead of creating an orphaned second artifact. It never overwrites an
// existing different object.
func (service *Service) publish(invoice models.ChargingSessionInvoice, pdf []byte) (string, int64, string, error) {
	if len(pdf) == 0 {
		return "", 0, "", errors.New("renderer produced an empty PDF")
	}
	relative := filepath.Join(invoice.CPOID.String(), invoice.FinancialYear, invoice.ID.String()+".pdf")
	if err := service.ensureControlledDirectory(filepath.Join(service.storageRoot, filepath.Dir(relative))); err != nil {
		return "", 0, "", err
	}
	final, err := service.safePath(relative)
	if err != nil {
		return "", 0, "", err
	}
	temp, err := os.CreateTemp(filepath.Dir(final), ".invoice-*.tmp")
	if err != nil {
		return "", 0, "", err
	}
	tempName := temp.Name()
	defer os.Remove(tempName)
	if err := temp.Chmod(0600); err != nil {
		temp.Close()
		return "", 0, "", err
	}
	if _, err := temp.Write(pdf); err != nil {
		temp.Close()
		return "", 0, "", err
	}
	if err := temp.Sync(); err != nil {
		temp.Close()
		return "", 0, "", err
	}
	if err := temp.Close(); err != nil {
		return "", 0, "", err
	}
	if existing, err := os.ReadFile(final); err == nil {
		if !bytes.Equal(existing, pdf) {
			return "", 0, "", errors.New("existing pre-ready invoice object differs from deterministic rendering")
		}
		digest := sha256.Sum256(pdf)
		return filepath.ToSlash(relative), int64(len(pdf)), hex.EncodeToString(digest[:]), nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", 0, "", err
	}
	if err := os.Rename(tempName, final); err != nil {
		return "", 0, "", fmt.Errorf("publish invoice PDF: %w", err)
	}
	if err := syncDirectory(filepath.Dir(final)); err != nil {
		return "", 0, "", err
	}
	digest := sha256.Sum256(pdf)
	return filepath.ToSlash(relative), int64(len(pdf)), hex.EncodeToString(digest[:]), nil
}

func (service *Service) safePath(relative string) (string, error) {
	if filepath.IsAbs(relative) || strings.Contains(relative, "..") {
		return "", errors.New("invalid invoice storage path")
	}
	full := filepath.Join(service.storageRoot, filepath.FromSlash(relative))
	if !strings.HasPrefix(filepath.Clean(full), filepath.Clean(service.storageRoot)+string(os.PathSeparator)) {
		return "", errors.New("invoice path escapes storage root")
	}
	if err := service.verifyControlledDirectory(filepath.Dir(full), false); err != nil {
		return "", err
	}
	return full, nil
}

// ensureControlledDirectory rejects a pre-existing symlink at every component
// below the configured root. This makes a hostile or accidental symlink unable
// to redirect invoice publication outside the configured store.
func (service *Service) ensureControlledDirectory(directory string) error {
	if err := os.MkdirAll(service.storageRoot, 0750); err != nil {
		return fmt.Errorf("create invoice storage root: %w", err)
	}
	return service.verifyControlledDirectory(directory, true)
}

// verifyControlledDirectory checks every component both before publication and
// every later private read. A later parent-directory symlink replacement is
// therefore rejected rather than followed outside the configured store.
func (service *Service) verifyControlledDirectory(directory string, create bool) error {
	rootInfo, err := os.Lstat(service.storageRoot)
	if err != nil || rootInfo.Mode()&os.ModeSymlink != 0 || !rootInfo.IsDir() {
		return errors.New("invoice storage root is not a regular directory")
	}
	relative, err := filepath.Rel(service.storageRoot, directory)
	if err != nil || relative == "." || strings.HasPrefix(relative, "..") {
		return errors.New("invoice directory escapes storage root")
	}
	current := service.storageRoot
	for _, part := range strings.Split(relative, string(os.PathSeparator)) {
		current = filepath.Join(current, part)
		info, statErr := os.Lstat(current)
		if errors.Is(statErr, os.ErrNotExist) && create {
			if err := os.Mkdir(current, 0750); err != nil && !errors.Is(err, os.ErrExist) {
				return fmt.Errorf("create invoice storage directory: %w", err)
			}
			info, statErr = os.Lstat(current)
		}
		if statErr != nil || info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return errors.New("invoice storage directory is unsafe")
		}
	}
	return nil
}

func syncDirectory(directory string) error {
	// Directory sync is not supported by every Windows filesystem. Publication
	// remains atomic there; Unix-like deployments additionally receive the
	// durability barrier required after rename.
	directoryHandle, err := os.Open(directory)
	if err != nil {
		return err
	}
	defer directoryHandle.Close()
	if err := directoryHandle.Sync(); err != nil && !errors.Is(err, os.ErrInvalid) && !(runtime.GOOS == "windows" && errors.Is(err, os.ErrPermission)) {
		return err
	}
	return nil
}

func renderInvoice(snapshot issuanceSnapshot, version string, asset models.InvoiceAsset, hasAsset bool, zone *time.Location) ([]byte, error) {
	if snapshot.SchemaVersion != snapshotVersion {
		return nil, fmt.Errorf("unsupported invoice snapshot version %d", snapshot.SchemaVersion)
	}
	switch version {
	case rendererVersion:
		return renderPDFV2(snapshot, asset, hasAsset, zone)
	case legacyCanvasRenderer:
		return renderPDFV1(snapshot, asset, hasAsset, zone)
	case "gofpdf-dejavu-v1":
		return renderPDFLegacyGofpdf(snapshot, asset, hasAsset, zone)
	default:
		return nil, fmt.Errorf("unsupported invoice renderer version %q", version)
	}
}

type invoiceFonts struct {
	latin, bengali, devanagari *canvas.FontFamily
}

func newInvoiceFonts() (invoiceFonts, error) {
	latin := canvas.NewFontFamily("invoice-latin")
	if err := latin.LoadFont(dejavusans.TTF, 0, canvas.FontRegular); err != nil {
		return invoiceFonts{}, fmt.Errorf("load embedded Latin font: %w", err)
	}
	latin.LoadFont(dejavusansbold.TTF, 0, canvas.FontBold)
	bengali := canvas.NewFontFamily("invoice-bengali")
	if err := bengali.LoadFont(notoSansBengali, 0, canvas.FontRegular); err != nil {
		return invoiceFonts{}, fmt.Errorf("load embedded Bengali font: %w", err)
	}
	devanagari := canvas.NewFontFamily("invoice-devanagari")
	if err := devanagari.LoadFont(notoSansDevanagari, 0, canvas.FontRegular); err != nil {
		return invoiceFonts{}, fmt.Errorf("load embedded Devanagari font: %w", err)
	}
	return invoiceFonts{latin: latin, bengali: bengali, devanagari: devanagari}, nil
}

type invoiceScript uint8

const (
	invoiceLatin invoiceScript = iota
	invoiceBengali
	invoiceDevanagari
	invoiceCommon
)

func invoiceScriptFor(r rune) invoiceScript {
	if unicode.Is(unicode.Common, r) || unicode.Is(unicode.Inherited, r) {
		return invoiceCommon
	}
	if r >= 0x0980 && r <= 0x09FF {
		return invoiceBengali
	}
	if r >= 0x0900 && r <= 0x097F {
		return invoiceDevanagari
	}
	return invoiceLatin
}

func (fonts invoiceFonts) faceFor(script invoiceScript, size float64, bold bool) *canvas.FontFace {
	return fonts.faceForColor(script, size, bold, canvas.Black)
}

func (fonts invoiceFonts) faceForColor(script invoiceScript, size float64, bold bool, fill color.Color) *canvas.FontFace {
	style := canvas.FontRegular
	if bold && script == invoiceLatin {
		style = canvas.FontBold
	}
	if script == invoiceBengali {
		return fonts.bengali.Face(size, fill, canvas.FontRegular, canvas.FontNormal)
	}
	if script == invoiceDevanagari {
		return fonts.devanagari.Face(size, fill, canvas.FontRegular, canvas.FontNormal)
	}
	return fonts.latin.Face(size, fill, style, canvas.FontNormal)
}

func (fonts invoiceFonts) textBox(value string, size, width float64, bold bool) *canvas.Text {
	return fonts.textBoxColor(value, size, width, bold, canvas.Black)
}

func (fonts invoiceFonts) textBoxColor(value string, size, width float64, bold bool, fill color.Color) *canvas.Text {
	defaultFace := fonts.faceForColor(invoiceLatin, size, bold, fill)
	rich := canvas.NewRichText(defaultFace)
	var run strings.Builder
	current := defaultFace
	currentScript := invoiceLatin
	flush := func() {
		if run.Len() > 0 {
			rich.WriteFace(current, run.String())
			run.Reset()
		}
	}
	for _, r := range value {
		script := invoiceScriptFor(r)
		// Common punctuation and inherited marks continue the preceding run;
		// splitting Indic text per rune would defeat OpenType shaping.
		if script != invoiceCommon && script != currentScript {
			flush()
			currentScript = script
			current = fonts.faceForColor(script, size, bold, fill)
		}
		run.WriteRune(r)
	}
	flush()
	return rich.ToText(width, 0, canvas.Left, canvas.Top, nil)
}

// renderPDFV1 remains available for invoice rows durably stamped before the
// redesigned layout. READY artifacts are immutable; pending work continues to
// use the renderer version recorded at issuance.
func renderPDFV1(snapshot issuanceSnapshot, asset models.InvoiceAsset, hasAsset bool, zone *time.Location) ([]byte, error) {
	fonts, err := newInvoiceFonts()
	if err != nil {
		return nil, err
	}
	defer fonts.latin.Destroy()
	defer fonts.bengali.Destroy()
	defer fonts.devanagari.Destroy()
	var output bytes.Buffer
	renderer := canvaspdf.New(&output, 210, 297, &canvaspdf.Options{Compress: false, SubsetFonts: true, ImageEncoding: canvas.Lossless})
	renderer.SetInfo("Charging Session Invoice "+snapshot.InvoiceNumber, "Immutable charging-session invoice", "charging,invoice", snapshot.Supplier.Name, legacyCanvasRenderer)
	var document *canvas.Canvas
	var context *canvas.Context
	y := 0.0
	page := 0
	finishPage := func() {
		if document == nil {
			return
		}
		footer := fonts.textBox(fmt.Sprintf("Invoice %s · Page %d", snapshot.InvoiceNumber, page), 7, 178, false)
		context.DrawText(16, 10, footer)
		document.RenderTo(renderer)
	}
	var newPage func()
	newPage = func() {
		if document != nil {
			finishPage()
			renderer.NewPage(210, 297)
		}
		page++
		document = canvas.New(210, 297)
		context = canvas.NewContext(document)
		context.SetFillColor(canvas.Black)
		y = 282
		if page == 1 {
			heading := fonts.textBox("Charging Session Invoice", 15, 178, true)
			context.DrawText(16, y, heading)
			issued := fonts.textBox("Invoice number: "+snapshot.InvoiceNumber+"\nIssued: "+formatInvoiceTime(snapshot.IssuedAt, zone), 8.5, 104, false)
			context.DrawText(16, y-heading.Bounds().H()-1.8, issued)
			amount := fonts.textBox("Final amount\n"+formatMoney(snapshot.Commercial.Currency, snapshot.Commercial.TotalAmount), 13, 58, true)
			context.DrawText(136, y, amount)
			status := invoiceHeaderStatus(snapshot.Commercial)
			if status != "" {
				statusText := fonts.textBox(status, 7.5, 58, false)
				context.DrawText(136, y-amount.Bounds().H()-1.2, statusText)
			}
			if hasAsset && asset.SHA256 == snapshot.LogoSHA256 {
				if imageValue, _, imageErr := image.Decode(bytes.NewReader(asset.Content)); imageErr == nil {
					context.DrawImage(170, 249, imageValue, canvas.DPMM(4))
				}
			}
			y -= maxFloat(heading.Bounds().H()+issued.Bounds().H()+3, amount.Bounds().H()+10)
			return
		}
		continued := fonts.textBox("Charging Session Invoice · continued", 10.5, 178, true)
		context.DrawText(16, y, continued)
		y -= continued.Bounds().H() + 3
	}
	newPage()
	draw := func(value string, size float64, bold bool) {
		if strings.TrimSpace(value) == "" {
			return
		}
		text := fonts.textBox(value, size, 178, bold)
		if y-text.Bounds().H() < 18 {
			newPage()
		}
		context.DrawText(16, y, text)
		y -= text.Bounds().H() + 1.8
	}
	section := func(value customerInvoiceSection) {
		lines := compactInvoiceLines(value.Lines)
		if strings.TrimSpace(value.Title) == "" || len(lines) == 0 {
			return
		}
		title := fonts.textBox(value.Title, 10.5, 178, true)
		first := fonts.textBox(lines[0], 8.5, 178, false)
		if y-title.Bounds().H()-first.Bounds().H()-4 < 18 {
			newPage()
		}
		y -= 2
		context.DrawText(16, y, title)
		y -= title.Bounds().H() + 1.8
		for _, line := range lines {
			draw(line, 8.5, false)
		}
	}
	for _, value := range customerInvoiceSectionsV1(snapshot, zone) {
		section(value)
	}
	finishPage()
	if err := renderer.Close(); err != nil {
		return nil, err
	}
	return output.Bytes(), nil
}

func customerInvoiceSectionsV1(snapshot issuanceSnapshot, zone *time.Location) []customerInvoiceSection {
	sections := customerInvoiceSections(snapshot, zone)
	for index := range sections {
		if sections[index].Title == "Charging location" {
			sections[index].Lines = compactInvoiceLines([]string{snapshot.Location.HubName, snapshot.Location.HubAddress, snapshot.Location.HubState})
			break
		}
	}
	return sections
}

func renderPDFV2(snapshot issuanceSnapshot, asset models.InvoiceAsset, hasAsset bool, zone *time.Location) ([]byte, error) {
	fonts, err := newInvoiceFonts()
	if err != nil {
		return nil, err
	}
	defer fonts.latin.Destroy()
	defer fonts.bengali.Destroy()
	defer fonts.devanagari.Destroy()
	var output bytes.Buffer
	renderer := canvaspdf.New(&output, 210, 297, &canvaspdf.Options{Compress: true, SubsetFonts: true, ImageEncoding: canvas.Lossless})
	renderer.SetInfo("Charging Session Invoice "+snapshot.InvoiceNumber, "Charging session invoice", "charging,invoice", snapshot.Supplier.Name, rendererVersion)
	pages := make([]invoiceCanvasPage, 0, 2)
	var current *invoiceCanvasPage
	y := 0.0
	var logo image.Image
	if hasAsset && asset.SHA256 == snapshot.LogoSHA256 {
		logo, _, _ = image.Decode(bytes.NewReader(asset.Content))
	}
	newPage := func() {
		page := invoiceCanvasPage{document: canvas.New(210, 297)}
		page.context = canvas.NewContext(page.document)
		pages = append(pages, page)
		current = &pages[len(pages)-1]
		y = 282
		if len(pages) == 1 {
			renderInvoiceFirstHeader(current.context, fonts, snapshot, logo, zone)
			y = 210
			return
		}
		continued := fonts.textBox("Charging Session Invoice", 10.5, 120, true)
		current.context.DrawText(14, y, continued)
		identity := fonts.textBox(snapshot.InvoiceNumber+" · continued", 7.5, 70, false)
		current.context.DrawText(196-identity.Bounds().W(), y, identity)
		y -= continued.Bounds().H() + 7
	}
	newPage()
	ensure := func(height float64) {
		if y-height < invoiceFooterSafeBottom {
			newPage()
		}
	}
	sectionTitle := func(text *canvas.Text) {
		current.context.DrawText(14, y, text)
		y -= text.Bounds().H() + 3
	}
	drawTwoCards := func() {
		left := compactInvoiceLines([]string{snapshot.Customer.FullName, snapshot.Customer.Email, optionalLine("Phone", snapshot.Customer.Phone)})
		right := invoiceLocationLines(snapshot.Location)
		leftHeight := invoiceCardHeight(fonts, left, 84)
		rightHeight := invoiceCardHeight(fonts, right, 84)
		height := maxFloat(leftHeight, rightHeight)
		ensure(height)
		renderInvoiceCard(current.context, fonts, 14, y, 87, height, "Billed to", left)
		renderInvoiceCard(current.context, fonts, 109, y, 87, height, "Charging at", right)
		y -= height + 5
	}
	drawSummary := func() {
		values := []invoiceSummaryValue{
			{Label: "Started", Value: formatInvoiceTime(snapshot.Charging.StartedAt, zone)},
			{Label: "Ended", Value: optionalTimeText(snapshot.Charging.EndedAt, zone)},
			{Label: "Charging time", Value: formatDuration(snapshot.Charging.DurationSeconds)},
			{Label: "Energy delivered", Value: formatKWh(snapshot.Commercial.TotalKWh)},
		}
		height := invoiceSummaryHeight(fonts, values)
		ensure(height)
		renderInvoiceSummary(current.context, fonts, 14, y, 182, height, values)
		y -= height + 2
		secondary := joinInvoiceParts("   ·   ", invoiceChargerLines(snapshot.Location)...)
		if secondary != "" {
			line := fonts.textBox(secondary, 7.7, 182, false)
			ensure(line.Bounds().H())
			current.context.DrawText(14, y, line)
			y -= line.Bounds().H() + 5
		}
	}
	drawCharges := func() {
		rows := invoiceChargeRows(snapshot.Commercial)
		if len(rows) == 0 {
			return
		}
		title := fonts.textBox("Charges", 10, 182, true)
		headerHeight := 8.0
		firstRowHeight := invoiceChargeRowHeight(fonts, rows[0])
		ensure(title.Bounds().H() + 3 + headerHeight + firstRowHeight)
		sectionTitle(title)
		renderInvoiceTableHeader(current.context, fonts, 14, y, 182)
		y -= headerHeight
		for _, row := range rows {
			height := invoiceChargeRowHeight(fonts, row)
			ensure(height + 1)
			renderInvoiceChargeRow(current.context, fonts, 14, y, 182, height, row)
			y -= height
		}
		y -= 5
	}
	drawNote := func() {
		lines := splitInvoiceText(snapshot.InvoiceNote)
		if len(lines) == 0 {
			return
		}
		height := invoiceCardHeight(fonts, lines, 174)
		ensure(height)
		renderInvoiceCard(current.context, fonts, 14, y, 182, height, "Message from supplier", lines)
		y -= height + 5
	}
	drawDetails := func() {
		lines := invoiceSessionDetailLines(snapshot.Charging, snapshot.Commercial.Currency)
		if len(lines) == 0 {
			return
		}
		title := fonts.textBox("Session details", 10, 182, true)
		first := fonts.textBox(lines[0], 7.1, 182, false)
		ensure(title.Bounds().H() + 3 + first.Bounds().H())
		sectionTitle(title)
		current.context.DrawText(14, y, first)
		y -= first.Bounds().H() + 1.1
		for _, line := range lines[1:] {
			text := fonts.textBox(line, 7.1, 182, false)
			ensure(text.Bounds().H())
			current.context.DrawText(14, y, text)
			y -= text.Bounds().H() + 1.1
		}
	}
	drawTwoCards()
	drawSummary()
	drawCharges()
	drawNote()
	drawDetails()
	for index := range pages {
		footer := fonts.textBox(fmt.Sprintf("Issued by %s", snapshot.Supplier.Name), 6.8, 100, false)
		pages[index].context.DrawText(14, 10, footer)
		page := fonts.textBox(fmt.Sprintf("Invoice %s · Page %d of %d", snapshot.InvoiceNumber, index+1, len(pages)), 6.8, 90, false)
		pages[index].context.DrawText(196-page.Bounds().W(), 10, page)
		if index > 0 {
			renderer.NewPage(210, 297)
		}
		pages[index].document.RenderTo(renderer)
	}
	if err := renderer.Close(); err != nil {
		return nil, err
	}
	return output.Bytes(), nil
}

const invoiceFooterSafeBottom = 18.0

type invoiceCanvasPage struct {
	document *canvas.Canvas
	context  *canvas.Context
}

type invoiceSummaryValue struct{ Label, Value string }
type invoiceChargeRow struct {
	Description, Basis, Amount string
	Total                      bool
}

func renderInvoiceFirstHeader(context *canvas.Context, fonts invoiceFonts, snapshot issuanceSnapshot, logo image.Image, zone *time.Location) {
	fillInvoiceRect(context, 14, 218, 182, 64, color.RGBA{R: 246, G: 248, B: 251, A: 255})
	if logo != nil {
		context.FitImage(logo, invoiceLogoBounds(), canvas.ImageContain)
	}
	issuerX := 56.0
	issuer := fonts.textBox(snapshot.Supplier.Name, 11.5, 72, true)
	context.DrawText(issuerX, 274, issuer)
	supplier := compactInvoiceLines([]string{snapshot.Supplier.Address, joinInvoiceParts(", ", snapshot.Supplier.City, snapshot.Supplier.State, snapshot.Supplier.Pincode), optionalTextLine("GSTIN", snapshot.Supplier.GSTIN)})
	y := 274 - issuer.Bounds().H() - 1.5
	for _, line := range supplier {
		text := fonts.textBox(line, 7.2, 72, false)
		context.DrawText(issuerX, y, text)
		y -= text.Bounds().H() + 0.8
	}
	title := fonts.textBox("Charging Session Invoice", 11, 76, true)
	context.DrawText(120, 275, title)
	amount := fonts.textBox(formatMoney(snapshot.Commercial.Currency, snapshot.Commercial.TotalAmount), 16, 76, true)
	context.DrawText(120, 260, amount)
	status := invoiceHeaderStatus(snapshot.Commercial)
	if status != "" {
		badge := fonts.textBox(status, 7, 76, false)
		context.DrawText(120, 251, badge)
	}
	meta := fonts.textBox("Invoice "+snapshot.InvoiceNumber+"\nIssued "+formatInvoiceTime(snapshot.IssuedAt, zone), 7, 76, false)
	context.DrawText(120, 237, meta)
}

func invoiceLogoBounds() canvas.Rect {
	return canvas.Rect{X0: 14, Y0: 258, X1: 52, Y1: 276}
}

func fillInvoiceRect(context *canvas.Context, x, y, width, height float64, shade color.Color) {
	context.SetFillColor(shade)
	context.DrawPath(x, y, canvas.Rectangle(width, height))
}

func invoiceCardHeight(fonts invoiceFonts, lines []string, width float64) float64 {
	height := 13.0
	for _, line := range lines {
		height += fonts.textBox(line, 8, width-8, false).Bounds().H() + 1
	}
	return maxFloat(height, 27)
}

func renderInvoiceCard(context *canvas.Context, fonts invoiceFonts, x, top, width, height float64, title string, lines []string) {
	fillInvoiceRect(context, x, top-height, width, height, color.RGBA{R: 248, G: 249, B: 251, A: 255})
	heading := fonts.textBox(title, 8.3, width-8, true)
	context.DrawText(x+4, top-5, heading)
	y := top - 5 - heading.Bounds().H() - 1.2
	for _, line := range lines {
		text := fonts.textBox(line, 8, width-8, false)
		context.DrawText(x+4, y, text)
		y -= text.Bounds().H() + 1
	}
}

func invoiceSummaryHeight(fonts invoiceFonts, values []invoiceSummaryValue) float64 {
	height := 20.0
	for _, value := range values {
		height = maxFloat(height, 11+fonts.textBox(value.Value, 8.2, 39, true).Bounds().H())
	}
	return height
}

func renderInvoiceSummary(context *canvas.Context, fonts invoiceFonts, x, top, width, height float64, values []invoiceSummaryValue) {
	fillInvoiceRect(context, x, top-height, width, height, color.RGBA{R: 238, G: 244, B: 250, A: 255})
	column := width / float64(len(values))
	for index, value := range values {
		left := x + float64(index)*column + 4
		label := fonts.textBox(value.Label, 6.8, column-8, false)
		context.DrawText(left, top-5, label)
		amount := fonts.textBox(value.Value, 8.2, column-8, true)
		context.DrawText(left, top-5-label.Bounds().H()-1, amount)
	}
}

func invoiceChargeRows(commercial commercialSnapshot) []invoiceChargeRow {
	basis := joinInvoiceParts(" · ", optionalTextLine("Energy", formatKWh(commercial.TotalKWh)), invoiceTariffBasis(commercial.Tariff))
	rows := []invoiceChargeRow{{Description: "Charging energy", Basis: basis, Amount: "—"}}
	for _, tax := range []struct {
		name string
		rate *string
	}{{"CGST", commercial.Tax.CGSTRate}, {"SGST", commercial.Tax.SGSTRate}, {"IGST", commercial.Tax.IGSTRate}} {
		if tax.rate != nil && strings.TrimSpace(*tax.rate) != "" {
			rows = append(rows, invoiceChargeRow{Description: tax.name, Basis: *tax.rate + "% rate", Amount: "—"})
		}
	}
	return append(rows, invoiceChargeRow{Description: "Final total", Basis: humanValueString(commercial.SettlementStatus), Amount: formatMoney(commercial.Currency, commercial.TotalAmount), Total: true})
}

func invoiceTariffBasis(tariff invoiceTariffSnapshot) string {
	if tariff.PricePerUnit == nil || tariff.BillingUnit == nil {
		return ""
	}
	return "Rate " + *tariff.PricePerUnit + "/" + *tariff.BillingUnit
}

func invoiceChargeRowHeight(fonts invoiceFonts, row invoiceChargeRow) float64 {
	return maxFloat(8, maxFloat(fonts.textBox(row.Description, 8, 72, row.Total).Bounds().H(), fonts.textBox(row.Basis, 7.5, 66, false).Bounds().H())+3)
}

func renderInvoiceTableHeader(context *canvas.Context, fonts invoiceFonts, x, top, width float64) {
	const headerHeight = 8.0
	fillInvoiceRect(context, x, top-headerHeight, width, headerHeight, color.RGBA{R: 33, G: 56, B: 82, A: 255})
	for _, column := range []struct {
		text  string
		x     float64
		right bool
	}{{"Description", x + 3, false}, {"Basis", x + 77, false}, {"Amount", x + 179, true}} {
		label := fonts.textBoxColor(column.text, 7.2, 34, true, color.White)
		labelX := column.x
		if column.right {
			labelX -= label.Bounds().W()
		}
		context.DrawText(labelX, invoiceTextTopCentered(top, headerHeight, label), label)
	}
}

func invoiceTextTopCentered(top, height float64, text *canvas.Text) float64 {
	bounds := text.Bounds()
	return top - height/2 + (bounds.Y0+bounds.Y1)/2
}

func renderInvoiceChargeRow(context *canvas.Context, fonts invoiceFonts, x, top, width, height float64, row invoiceChargeRow) {
	if row.Total {
		fillInvoiceRect(context, x, top-height, width, height, color.RGBA{R: 238, G: 244, B: 250, A: 255})
	}
	context.SetFillColor(canvas.Black)
	context.DrawText(x+3, top-3, fonts.textBox(row.Description, 8, 72, row.Total))
	context.DrawText(x+77, top-3, fonts.textBox(row.Basis, 7.5, 64, false))
	amount := fonts.textBox(row.Amount, 8, 36, row.Total)
	context.DrawText(x+179-amount.Bounds().W(), top-3, amount)
}

func optionalTimeText(value *time.Time, zone *time.Location) string {
	if value == nil {
		return "Not recorded"
	}
	return formatInvoiceTime(*value, zone)
}

type customerInvoiceSection struct {
	Title string
	Lines []string
}

func customerInvoiceSections(snapshot issuanceSnapshot, zone *time.Location) []customerInvoiceSection {
	return []customerInvoiceSection{
		{Title: "Supplier", Lines: supplierInvoiceLines(snapshot.Supplier)},
		{Title: "Customer", Lines: compactInvoiceLines([]string{snapshot.Customer.FullName, snapshot.Customer.Email, optionalLine("Phone", snapshot.Customer.Phone)})},
		{Title: "Charging location", Lines: invoiceLocationLines(snapshot.Location)},
		{Title: "Charger and connector", Lines: invoiceChargerLines(snapshot.Location)},
		{Title: "Charging summary", Lines: invoiceChargingLines(snapshot.Charging, snapshot.Commercial, zone)},
		{Title: "Pricing and tax", Lines: commercialLines(snapshot.Commercial)},
		{Title: "Session details", Lines: invoiceSessionDetailLines(snapshot.Charging, snapshot.Commercial.Currency)},
		{Title: "Message from supplier", Lines: splitInvoiceText(snapshot.InvoiceNote)},
	}
}

func supplierInvoiceLines(supplier supplierSnapshot) []string {
	return compactInvoiceLines([]string{supplier.Name, supplier.CompanyType, optionalTextLine("GSTIN", supplier.GSTIN), supplier.Address, joinInvoiceParts(", ", supplier.City, supplier.State, supplier.Pincode)})
}

func invoiceLocationLines(location locationSnapshot) []string {
	if strings.TrimSpace(location.HubName) == "" {
		return []string{"Location unavailable"}
	}
	lines := []string{location.HubName, location.HubAddress}
	state := strings.TrimSpace(location.HubState)
	if state != "" && !strings.Contains(strings.ToLower(location.HubAddress), strings.ToLower(state)) {
		lines = append(lines, state)
	}
	return compactInvoiceLines(lines)
}

func invoiceChargerLines(location locationSnapshot) []string {
	charger := joinInvoiceParts(" · ", location.ChargerName, optionalTextLine("ID", location.ChargerCode))
	if charger == "" {
		charger = "Charger unavailable"
	}
	connector := ""
	if location.ConnectorNumber > 0 {
		connector = fmt.Sprintf("Connector %d", location.ConnectorNumber)
	}
	connector = joinInvoiceParts(" · ", connector, location.ConnectorType, optionalPower(location.RatedPowerKW))
	return compactInvoiceLines([]string{charger, connector, location.ChargerType})
}

func invoiceChargingLines(charging chargingSnapshot, commercial commercialSnapshot, zone *time.Location) []string {
	lines := []string{optionalInvoiceTime("Started", charging.StartedAt, zone), optionalInvoiceTime("Ended", charging.EndedAt, zone), "Charging time: " + formatDuration(charging.DurationSeconds), optionalTextLine("Energy delivered", formatKWh(commercial.TotalKWh))}
	return compactInvoiceLines(lines)
}

func invoiceSessionDetailLines(charging chargingSnapshot, currency string) []string {
	lines := []string{optionalTextLine("Session reference", charging.SessionID)}
	if charging.OCPPTransactionID > 0 {
		lines = append(lines, fmt.Sprintf("Charger transaction ID: %d", charging.OCPPTransactionID))
	}
	lines = append(lines, optionalMeterReadings(charging.MeterStartWh, charging.MeterStopWh), optionalLimitDescription(charging, currency), optionalLine("Requested stop", humanStopDescription(charging.RequestedStopInitiator, charging.RequestedStopReason)), optionalTextLine("Charger stop reason", humanValue(charging.OCPPStopReason)))
	return compactInvoiceLines(lines)
}

func compactInvoiceLines(lines []string) []string {
	result := make([]string, 0, len(lines))
	for _, line := range lines {
		if trimmed := strings.TrimSpace(line); trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

func splitInvoiceText(value string) []string {
	return compactInvoiceLines(strings.Split(value, "\n"))
}

func joinInvoiceParts(separator string, values ...string) string {
	return strings.Join(compactInvoiceLines(values), separator)
}

func formatInvoiceTime(value time.Time, zone *time.Location) string {
	if value.IsZero() {
		return ""
	}
	return value.In(zone).Format("02 Jan 2006, 3:04 PM MST")
}

func optionalInvoiceTime(label string, value any, zone *time.Location) string {
	switch typed := value.(type) {
	case time.Time:
		if text := formatInvoiceTime(typed, zone); text != "" {
			return label + ": " + text
		}
	case *time.Time:
		if typed != nil {
			return optionalInvoiceTime(label, *typed, zone)
		}
	}
	return ""
}

func formatDuration(seconds int64) string {
	if seconds < 60 {
		return "less than a minute"
	}
	hours, minutes := seconds/3600, (seconds%3600)/60
	if hours == 0 {
		return fmt.Sprintf("%d min", minutes)
	}
	if minutes == 0 {
		return fmt.Sprintf("%d hr", hours)
	}
	return fmt.Sprintf("%d hr %d min", hours, minutes)
}

func formatKWh(value string) string {
	if strings.TrimSpace(value) == "" {
		return ""
	}
	return value + " kWh"
}

func formatKWhFromWh(value int64) string {
	text := fmt.Sprintf("%.3f", float64(value)/1000)
	text = strings.TrimRight(strings.TrimRight(text, "0"), ".")
	return text + " kWh"
}

func formatMoney(currency, amount string) string {
	return joinInvoiceParts(" ", currency, amount)
}

func optionalPower(power float64) string {
	if power <= 0 {
		return ""
	}
	return fmt.Sprintf("%.1f kW", power)
}

func optionalMeterReadings(start int64, stop *int64) string {
	if stop == nil {
		return ""
	}
	return "Meter readings: " + formatKWhFromWh(start) + " to " + formatKWhFromWh(*stop)
}

func optionalLimitDescription(charging chargingSnapshot, currency string) string {
	if charging.LimitType == nil || strings.TrimSpace(*charging.LimitType) == "" {
		return ""
	}
	limit := humanLimitType(*charging.LimitType)
	if charging.RequestedLimitValue == nil || strings.TrimSpace(*charging.RequestedLimitValue) == "" {
		return "Selected limit: " + limit
	}
	value := *charging.RequestedLimitValue
	switch strings.TrimSpace(*charging.LimitType) {
	case "ENERGY":
		value += " kWh"
	case "TIME":
		value += " minutes"
	case "MONEY":
		value = formatMoney(currency, value)
	}
	return "Selected limit: " + limit + " (" + value + ")"
}

func humanLimitType(value string) string {
	switch strings.TrimSpace(value) {
	case "AUTO":
		return "Automatic"
	case "ENERGY":
		return "Energy limit"
	case "TIME":
		return "Time limit"
	case "MONEY":
		return "Amount limit"
	default:
		return humanValueString(value)
	}
}

func humanStopDescription(initiator, reason *string) *string {
	parts := compactInvoiceLines([]string{humanValue(initiator), humanValue(reason)})
	if len(parts) == 0 {
		return nil
	}
	value := strings.Join(parts, " — ")
	return &value
}

func humanValue(value *string) string {
	if value == nil {
		return ""
	}
	return humanValueString(*value)
}

func humanValueString(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	switch value {
	case "APP":
		return "Mobile app"
	case "CMS":
		return "Charging service"
	case "HAL":
		return "Charging network"
	case "CHARGER":
		return "Charger"
	}
	words := strings.FieldsFunc(strings.ToLower(value), func(r rune) bool { return r == '_' || r == '-' || r == ' ' })
	for index, word := range words {
		if len(word) > 0 {
			words[index] = strings.ToUpper(word[:1]) + word[1:]
		}
	}
	return strings.Join(words, " ")
}

func invoiceHeaderStatus(commercial commercialSnapshot) string {
	parts := compactInvoiceLines([]string{optionalTextLine("Settlement", humanValueString(commercial.SettlementStatus)), optionalTextLine("Payment", humanValue(commercial.PaymentMethod))})
	return strings.Join(parts, "\n")
}

func maxFloat(left, right float64) float64 {
	if left > right {
		return left
	}
	return right
}

func renderPDFLegacyGofpdf(snapshot issuanceSnapshot, asset models.InvoiceAsset, hasAsset bool, zone *time.Location) ([]byte, error) {
	pdf := gofpdf.New("P", "mm", "A4", "")
	// Keep the text stream inspectable for support and focused PDF semantics
	// tests; financial correctness is still from the immutable snapshot.
	pdf.SetCompression(false)
	pdf.SetMargins(14, 14, 14)
	pdf.SetAutoPageBreak(true, 14)
	pdf.SetCreationDate(snapshot.IssuedAt)
	pdf.SetTitle("Charging invoice "+snapshot.InvoiceNumber, true)
	pdf.SetAuthor(snapshot.Supplier.Name, true)
	pdf.AddUTF8FontFromBytes("DejaVu", "", dejavusans.TTF)
	pdf.AddUTF8FontFromBytes("DejaVu", "B", dejavusansbold.TTF)
	pdf.AddPage()
	pdf.SetFont("DejaVu", "B", 16)
	pdf.CellFormat(0, 8, "Charging Session Invoice", "", 1, "L", false, 0, "")
	pdf.SetFont("DejaVu", "", 10)
	pdf.CellFormat(0, 5, "Invoice "+snapshot.InvoiceNumber+"  |  Issued "+snapshot.IssuedAt.In(zone).Format("02 Jan 2006, 15:04 MST"), "", 1, "L", false, 0, "")
	if hasAsset && asset.SHA256 == snapshot.LogoSHA256 {
		name := "logo"
		opts := gofpdf.ImageOptions{ImageType: mimeToPDF(asset.MIMEType), ReadDpi: true}
		pdf.RegisterImageOptionsReader(name, opts, bytes.NewReader(asset.Content))
		pdf.ImageOptions(name, 165, 14, 28, 0, false, opts, 0, "")
	}
	supplierLines := []string{snapshot.Supplier.Name, snapshot.Supplier.CompanyType, optionalTextLine("GSTIN", snapshot.Supplier.GSTIN), snapshot.Supplier.Address, snapshot.Supplier.City + ", " + snapshot.Supplier.State + " " + snapshot.Supplier.Pincode}
	section(pdf, "Supplier", supplierLines)
	section(pdf, "Customer", []string{snapshot.Customer.FullName, snapshot.Customer.Email, optionalLine("Phone", snapshot.Customer.Phone)})
	section(pdf, "Charging location", []string{nonEmpty(snapshot.Location.HubName, "Charging location not recorded"), snapshot.Location.HubAddress, "Charger: " + snapshot.Location.ChargerCode + " — " + snapshot.Location.ChargerName, fmt.Sprintf("Connector: %d %s", snapshot.Location.ConnectorNumber, snapshot.Location.ConnectorType)})
	end := "Not recorded"
	if snapshot.Charging.EndedAt != nil {
		end = snapshot.Charging.EndedAt.In(zone).Format(time.RFC3339)
	}
	section(pdf, "Charging session", []string{"Session reference: " + snapshot.Charging.SessionID, fmt.Sprintf("OCPP transaction: %d", snapshot.Charging.OCPPTransactionID), "Started: " + snapshot.Charging.StartedAt.In(zone).Format(time.RFC3339), "Ended: " + end, fmt.Sprintf("Duration: %s", (time.Duration(snapshot.Charging.DurationSeconds) * time.Second).String()), fmt.Sprintf("Meter: %d Wh to %s", snapshot.Charging.MeterStartWh, optionalInt(snapshot.Charging.MeterStopWh)), "Energy delivered: " + snapshot.Commercial.TotalKWh + " kWh", optionalLine("Initial SoC", snapshot.Charging.InitialSoCPercent), optionalLine("Latest observed SoC", snapshot.Charging.LatestSoCPercent), optionalTime("SoC observed", snapshot.Charging.SoCObservedAt, zone)})
	section(pdf, "Requested plan", []string{optionalLine("Limit type", snapshot.Charging.LimitType), optionalLine("Requested limit", snapshot.Charging.RequestedLimitValue), optionalIntWithSource("Energy limit", snapshot.Charging.EnergyLimitWh, "Wh", snapshot.Charging.EnergyLimitSource), optionalIntWithSource("Time limit", snapshot.Charging.MaxDurationSeconds, "seconds", snapshot.Charging.DurationLimitSource)})
	section(pdf, "Stop provenance", []string{optionalLine("CMS/HAL requested stop initiator", snapshot.Charging.RequestedStopInitiator), optionalLine("CMS/HAL requested stop reason", snapshot.Charging.RequestedStopReason), optionalLine("Charger-reported OCPP stop reason", snapshot.Charging.OCPPStopReason)})
	section(pdf, "Commercial settlement", commercialLines(snapshot.Commercial))
	if snapshot.InvoiceNote != "" {
		section(pdf, "Note", []string{snapshot.InvoiceNote})
	}
	if pdf.Error() != nil {
		return nil, pdf.Error()
	}
	var output bytes.Buffer
	if err := pdf.Output(&output); err != nil {
		return nil, err
	}
	return output.Bytes(), nil
}

func section(pdf *gofpdf.Fpdf, title string, lines []string) {
	pdf.Ln(2)
	pdf.SetFont("DejaVu", "B", 11)
	pdf.CellFormat(0, 6, title, "", 1, "L", false, 0, "")
	pdf.SetFont("DejaVu", "", 9)
	for _, line := range lines {
		if strings.TrimSpace(line) != "" {
			pdf.MultiCell(0, 4.5, line, "", "L", false)
		}
	}
}
func optionalLine(label string, value *string) string {
	if value == nil || strings.TrimSpace(*value) == "" {
		return ""
	}
	return label + ": " + *value
}
func optionalTextLine(label, value string) string {
	if strings.TrimSpace(value) == "" {
		return ""
	}
	return label + ": " + value
}
func optionalTime(label string, value *time.Time, zone *time.Location) string {
	if value == nil {
		return ""
	}
	return label + ": " + value.In(zone).Format(time.RFC3339)
}
func optionalInt(value *int64) string {
	if value == nil {
		return "not recorded"
	}
	return fmt.Sprintf("%d Wh", *value)
}
func optionalIntWithSource(label string, value *int64, unit string, source *string) string {
	if value == nil {
		return ""
	}
	line := fmt.Sprintf("%s: %d %s", label, *value, unit)
	if source != nil && strings.TrimSpace(*source) != "" {
		line += " (" + *source + ")"
	}
	return line
}
func nonEmpty(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}
func commercialLines(commercial commercialSnapshot) []string {
	lines := []string{tariffRateLine(commercial), optionalTextLine("Tariff", humanValue(commercial.Tariff.TariffType)), taxLine(commercial.Tax), optionalTextLine("Settlement", humanValueString(commercial.SettlementStatus)), optionalTextLine("Payment method", humanValue(commercial.PaymentMethod))}
	return compactInvoiceLines(lines)
}

func tariffRateLine(commercial commercialSnapshot) string {
	if commercial.Tariff.PricePerUnit == nil || strings.TrimSpace(*commercial.Tariff.PricePerUnit) == "" {
		return ""
	}
	unit := humanBillingUnit(commercial.Tariff.BillingUnit)
	if unit == "" {
		return "Rate: " + formatMoney(commercial.Currency, *commercial.Tariff.PricePerUnit)
	}
	return "Rate: " + formatMoney(commercial.Currency, *commercial.Tariff.PricePerUnit) + " per " + unit
}

func humanBillingUnit(value *string) string {
	if value == nil {
		return ""
	}
	switch strings.ToLower(strings.TrimSpace(*value)) {
	case "kwh", "kwhs":
		return "kWh"
	case "minute", "minutes", "min":
		return "minute"
	case "session", "sessions":
		return "session"
	default:
		return humanValueString(*value)
	}
}

func taxLine(tax invoiceTaxSnapshot) string {
	parts := compactInvoiceLines([]string{optionalTextLine("CGST", percentValue(tax.CGSTRate)), optionalTextLine("SGST", percentValue(tax.SGSTRate)), optionalTextLine("IGST", percentValue(tax.IGSTRate))})
	if len(parts) == 0 {
		return ""
	}
	return "Tax: " + strings.Join(parts, ", ")
}

func percentValue(value *string) string {
	if value == nil || strings.TrimSpace(*value) == "" {
		return ""
	}
	return *value + "%"
}
func percentLine(value *string) *string {
	if value == nil {
		return nil
	}
	formatted := *value + "%"
	return &formatted
}
func mimeToPDF(mimeType string) string {
	if mimeType == "image/png" {
		return "PNG"
	}
	return "JPG"
}

func (service *Service) Summary(ctx context.Context, cpoID, customerID, sessionID uuid.UUID) (Summary, error) {
	var session models.ChargingSession
	if err := service.database.WithContext(ctx).Select("id", "status", "settlement_status").Where("cpo_id = ? AND customer_id = ? AND id = ?", cpoID, customerID, sessionID).First(&session).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return Summary{State: PublicNotAvailable}, nil
		}
		return Summary{}, err
	}
	var invoice models.ChargingSessionInvoice
	err := service.database.WithContext(ctx).Where("cpo_id = ? AND customer_id = ? AND session_id = ?", cpoID, customerID, sessionID).First(&invoice).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return summaryWithoutInvoice(session), nil
	}
	if err != nil {
		return Summary{}, err
	}
	return service.summaryFor(invoice), nil
}
func (service *Service) CPOSummary(ctx context.Context, cpoID, sessionID uuid.UUID) (CPOSummary, error) {
	var session models.ChargingSession
	if err := service.database.WithContext(ctx).Select("id", "status", "settlement_status").Where("cpo_id = ? AND id = ?", cpoID, sessionID).First(&session).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return CPOSummary{Summary: Summary{State: PublicNotAvailable}}, nil
		}
		return CPOSummary{}, err
	}
	var invoice models.ChargingSessionInvoice
	err := service.database.WithContext(ctx).Where("cpo_id = ? AND session_id = ?", cpoID, sessionID).First(&invoice).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return CPOSummary{Summary: summaryWithoutInvoice(session)}, nil
	}
	if err != nil {
		return CPOSummary{}, err
	}
	result := CPOSummary{Summary: service.summaryFor(invoice)}
	var delivery models.InvoiceDelivery
	if err := service.database.WithContext(ctx).Where("invoice_id = ?", invoice.ID).First(&delivery).Error; err == nil {
		status := delivery.Status
		result.DeliveryStatus = &status
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return CPOSummary{}, err
	}
	return result, nil
}

func (service *Service) CustomerSummaries(ctx context.Context, cpoID, customerID uuid.UUID, sessionIDs []uuid.UUID) (map[uuid.UUID]Summary, error) {
	result := make(map[uuid.UUID]Summary, len(sessionIDs))
	for _, id := range sessionIDs {
		result[id] = Summary{State: PublicNotAvailable}
	}
	if len(sessionIDs) == 0 {
		return result, nil
	}
	var sessions []models.ChargingSession
	if err := service.database.WithContext(ctx).Select("id", "status", "settlement_status").Where("cpo_id = ? AND customer_id = ? AND id IN ?", cpoID, customerID, sessionIDs).Find(&sessions).Error; err != nil {
		return nil, err
	}
	for _, session := range sessions {
		result[session.ID] = summaryWithoutInvoice(session)
	}
	var invoices []models.ChargingSessionInvoice
	if err := service.database.WithContext(ctx).Where("cpo_id = ? AND customer_id = ? AND session_id IN ?", cpoID, customerID, sessionIDs).Find(&invoices).Error; err != nil {
		return nil, err
	}
	for _, record := range invoices {
		result[record.SessionID] = service.summaryFor(record)
	}
	return result, nil
}

func (service *Service) CPOSummaries(ctx context.Context, cpoID uuid.UUID, sessionIDs []uuid.UUID) (map[uuid.UUID]CPOSummary, error) {
	result := make(map[uuid.UUID]CPOSummary, len(sessionIDs))
	for _, id := range sessionIDs {
		result[id] = CPOSummary{Summary: Summary{State: PublicNotAvailable}}
	}
	if len(sessionIDs) == 0 {
		return result, nil
	}
	var sessions []models.ChargingSession
	if err := service.database.WithContext(ctx).Select("id", "status", "settlement_status").Where("cpo_id = ? AND id IN ?", cpoID, sessionIDs).Find(&sessions).Error; err != nil {
		return nil, err
	}
	for _, session := range sessions {
		result[session.ID] = CPOSummary{Summary: summaryWithoutInvoice(session)}
	}
	var invoices []models.ChargingSessionInvoice
	if err := service.database.WithContext(ctx).Where("cpo_id = ? AND session_id IN ?", cpoID, sessionIDs).Find(&invoices).Error; err != nil {
		return nil, err
	}
	invoiceIDs := make([]uuid.UUID, 0, len(invoices))
	byInvoice := make(map[uuid.UUID]models.ChargingSessionInvoice, len(invoices))
	for _, record := range invoices {
		invoiceIDs = append(invoiceIDs, record.ID)
		byInvoice[record.ID] = record
		result[record.SessionID] = CPOSummary{Summary: service.summaryFor(record)}
	}
	if len(invoiceIDs) == 0 {
		return result, nil
	}
	var deliveries []models.InvoiceDelivery
	if err := service.database.WithContext(ctx).Where("invoice_id IN ?", invoiceIDs).Find(&deliveries).Error; err != nil {
		return nil, err
	}
	for _, delivery := range deliveries {
		if record, ok := byInvoice[delivery.InvoiceID]; ok {
			value := result[record.SessionID]
			status := delivery.Status
			value.DeliveryStatus = &status
			result[record.SessionID] = value
		}
	}
	return result, nil
}

// RecoverDelivery deliberately queues one new SMTP attempt after a CPO has
// confirmed the duplicate-delivery warning. Automatic processing never sends
// AMBIGUOUS work again. The optional auditor executes in this same durable
// transaction so the recovery action cannot exist without its CPO audit row.
func (service *Service) RecoverDelivery(ctx context.Context, cpoID, sessionID uuid.UUID, confirmDuplicateDelivery bool, auditor func(*gorm.DB, DeliveryRecovery) error) (DeliveryRecovery, error) {
	if !confirmDuplicateDelivery {
		return DeliveryRecovery{}, ErrDeliveryConfirmationRequired
	}
	if service.sender == nil {
		return DeliveryRecovery{}, ErrDeliverySenderUnavailable
	}
	// Validate the immutable artifact before a manual resend is persisted. A
	// corrupt READY artifact is transitioned terminally by open(), never rebuilt.
	download, err := service.OpenCPOSession(ctx, cpoID, sessionID)
	if err != nil {
		return DeliveryRecovery{}, err
	}
	if closeErr := download.Content.Close(); closeErr != nil {
		return DeliveryRecovery{}, fmt.Errorf("close invoice integrity check: %w", closeErr)
	}

	var recovery DeliveryRecovery
	err = service.database.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var session models.ChargingSession
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&session, "id = ? AND cpo_id = ?", sessionID, cpoID).Error; err != nil {
			return err
		}
		if session.Status != constants.SessionStatusCompleted || session.SettlementStatus != "SETTLED" {
			return ErrNotEligible
		}
		var record models.ChargingSessionInvoice
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&record, "session_id = ?", sessionID).Error; err != nil {
			return err
		}
		if record.GenerationStatus != "READY" {
			if record.GenerationStatus == "CORRUPT" {
				return ErrCorrupt
			}
			return ErrNotReady
		}
		recovery.InvoiceID = record.ID
		var delivery models.InvoiceDelivery
		deliveryErr := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&delivery, "invoice_id = ?", record.ID).Error
		now := service.now()
		if errors.Is(deliveryErr, gorm.ErrRecordNotFound) {
			snapshot, decodeErr := decodeSnapshot(record.Snapshot)
			if decodeErr != nil {
				return fmt.Errorf("decode invoice recipient: %w", decodeErr)
			}
			recipient, parseErr := mail.ParseAddress(strings.TrimSpace(snapshot.Customer.Email))
			if parseErr != nil || recipient.Address == "" {
				return ErrDeliveryUnavailable
			}
			delivery = models.InvoiceDelivery{ID: uuid.New(), InvoiceID: record.ID, Recipient: recipient.Address, Status: "PENDING", MaxAttempts: 8, AvailableAt: now, CreatedAt: now, UpdatedAt: now}
			if err := tx.Create(&delivery).Error; err != nil {
				return err
			}
			recovery.DeliveryStatus, recovery.Queued = "PENDING", true
		} else if deliveryErr != nil {
			return deliveryErr
		} else {
			recovery.PreviousStatus = delivery.Status
			switch delivery.Status {
			case "SENDING":
				return ErrDeliveryInProgress
			case "PENDING":
				recovery.DeliveryStatus = "PENDING"
			case "SENT", "FAILED", "AMBIGUOUS":
				if err := tx.Model(&models.InvoiceDelivery{}).Where("id = ? AND status = ?", delivery.ID, delivery.Status).Updates(map[string]any{"status": "PENDING", "attempts": 0, "available_at": now, "locked_at": nil, "last_error": "Manual CPO recovery queued after duplicate-delivery confirmation.", "updated_at": now}).Error; err != nil {
					return err
				}
				recovery.DeliveryStatus, recovery.Queued = "PENDING", true
			default:
				return ErrDeliveryUnavailable
			}
		}
		if auditor != nil {
			return auditor(tx, recovery)
		}
		return nil
	})
	if err != nil {
		return DeliveryRecovery{}, err
	}
	return recovery, nil
}
func (service *Service) summaryFor(invoice models.ChargingSessionInvoice) Summary {
	number := invoice.InvoiceNumber
	summary := Summary{InvoiceID: &invoice.ID, InvoiceNumber: &number, IssuedAt: &invoice.IssuedAt, ReadyAt: invoice.ReadyAt}
	switch invoice.GenerationStatus {
	case "READY":
		summary.State = PublicReady
		summary.DownloadAvailable = true
	case "FAILED", "CORRUPT":
		summary.State = PublicFailed
	default:
		summary.State = PublicPending
	}
	return summary
}

func summaryWithoutInvoice(session models.ChargingSession) Summary {
	if session.Status == constants.SessionStatusCompleted && session.SettlementStatus == "SETTLED" {
		return Summary{State: PublicPending}
	}
	return Summary{State: PublicNotAvailable}
}

func (service *Service) OpenCustomer(ctx context.Context, cpoID, customerID, invoiceID uuid.UUID) (*Download, error) {
	return service.open(ctx, "cpo_id = ? AND customer_id = ? AND id = ?", cpoID, customerID, invoiceID)
}
func (service *Service) OpenCPO(ctx context.Context, cpoID, invoiceID uuid.UUID) (*Download, error) {
	return service.open(ctx, "cpo_id = ? AND id = ?", cpoID, invoiceID)
}
func (service *Service) OpenCustomerSession(ctx context.Context, cpoID, customerID, sessionID uuid.UUID) (*Download, error) {
	return service.openSession(ctx, "cpo_id = ? AND customer_id = ? AND id = ?", cpoID, customerID, sessionID)
}
func (service *Service) OpenCPOSession(ctx context.Context, cpoID, sessionID uuid.UUID) (*Download, error) {
	return service.openSession(ctx, "cpo_id = ? AND id = ?", cpoID, sessionID)
}
func (service *Service) openSession(ctx context.Context, query string, values ...any) (*Download, error) {
	var session models.ChargingSession
	if err := service.database.WithContext(ctx).Where(query, values...).First(&session).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, os.ErrNotExist
		}
		return nil, err
	}
	if session.Status != constants.SessionStatusCompleted || session.SettlementStatus != "SETTLED" {
		return nil, ErrNotEligible
	}
	var invoice models.ChargingSessionInvoice
	if err := service.database.WithContext(ctx).Where("session_id = ?", session.ID).First(&invoice).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotReady
		}
		return nil, err
	}
	return service.open(ctx, "id = ?", invoice.ID)
}
func (service *Service) open(ctx context.Context, query string, values ...any) (*Download, error) {
	var invoice models.ChargingSessionInvoice
	if err := service.database.WithContext(ctx).Where(query, values...).First(&invoice).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, os.ErrNotExist
		}
		return nil, err
	}
	if err := invoiceDownloadStateError(invoice); err != nil {
		return nil, err
	}
	if invoice.StoragePath == nil || invoice.SHA256 == nil || invoice.FileSize == nil {
		return nil, ErrNotReady
	}
	path, err := service.safePath(*invoice.StoragePath)
	if err != nil {
		return nil, err
	}
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() {
		service.markCorrupt(ctx, invoice.ID, "invoice file is unavailable")
		return nil, ErrCorrupt
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	digest := sha256.New()
	size, readErr := io.Copy(digest, file)
	if readErr != nil || size != *invoice.FileSize || hex.EncodeToString(digest.Sum(nil)) != *invoice.SHA256 {
		file.Close()
		service.markCorrupt(ctx, invoice.ID, "invoice file failed integrity validation")
		return nil, ErrCorrupt
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		file.Close()
		return nil, err
	}
	return &Download{Content: file, Name: downloadFilename(invoice.InvoiceNumber), ModTime: info.ModTime()}, nil
}

var (
	ErrNotReady                     = errors.New("invoice not ready")
	ErrCorrupt                      = errors.New("invoice corrupt")
	ErrNotEligible                  = errors.New("session is not financially final")
	ErrDeliveryConfirmationRequired = errors.New("manual invoice delivery requires duplicate-delivery confirmation")
	ErrDeliveryInProgress           = errors.New("invoice delivery is already in progress")
	ErrDeliveryUnavailable          = errors.New("invoice delivery cannot be recovered")
	ErrDeliverySenderUnavailable    = errors.New("invoice SMTP sender is unavailable")
)

// invoiceDownloadStateError keeps failed/corrupt issuance distinct from work
// that is merely not ready. CORRUPT is durable and remains unavailable on
// every later read after the initial integrity failure.
func invoiceDownloadStateError(invoice models.ChargingSessionInvoice) error {
	switch invoice.GenerationStatus {
	case "CORRUPT", "FAILED":
		return ErrCorrupt
	case "READY":
		return nil
	default:
		return ErrNotReady
	}
}

func (service *Service) markCorrupt(ctx context.Context, invoiceID uuid.UUID, reason string) {
	now := service.now()
	_ = service.database.WithContext(ctx).Model(&models.ChargingSessionInvoice{}).Where("id = ? AND generation_status = 'READY'", invoiceID).Updates(map[string]any{"generation_status": "CORRUPT", "available_at": now, "last_generation_error": reason, "updated_at": now}).Error
}

// DeliverPending only retries failures known to occur before SMTP is invoked.
// A stale SENDING state is recorded as AMBIGUOUS because prior SMTP acceptance
// cannot be proven or disproven after a process crash.
func (service *Service) DeliverPending(ctx context.Context, limit int) error {
	for index := 0; index < limit; index++ {
		delivery, invoice, claimed, err := service.claimDelivery(ctx)
		if err != nil {
			return err
		}
		if !claimed {
			return nil
		}
		sent, err := service.deliver(ctx, delivery, invoice)
		if err != nil {
			return err
		}
		if sent {
			service.completed(ctx)
		}
	}
	return nil
}
func (service *Service) claimDelivery(ctx context.Context) (models.InvoiceDelivery, models.ChargingSessionInvoice, bool, error) {
	var delivery models.InvoiceDelivery
	result := service.database.WithContext(ctx).Raw(`UPDATE invoice_deliveries SET status = CASE WHEN status = 'SENDING' THEN 'AMBIGUOUS' ELSE 'SENDING' END, locked_at = now(), attempts = CASE WHEN status = 'SENDING' THEN attempts ELSE attempts + 1 END, updated_at = now(), last_error = CASE WHEN status = 'SENDING' THEN 'SMTP outcome became ambiguous after a stale delivery claim.' ELSE last_error END WHERE id = (SELECT id FROM invoice_deliveries WHERE (status = 'PENDING' AND available_at <= now() AND attempts < max_attempts) OR (status = 'SENDING' AND locked_at < now() - (? * interval '1 second')) ORDER BY available_at, created_at FOR UPDATE SKIP LOCKED LIMIT 1) RETURNING *`, invoiceDeliveryLock.Seconds()).Scan(&delivery)
	if result.Error != nil {
		return delivery, models.ChargingSessionInvoice{}, false, result.Error
	}
	if result.RowsAffected == 0 {
		return delivery, models.ChargingSessionInvoice{}, false, nil
	}
	if delivery.Status == "AMBIGUOUS" {
		return delivery, models.ChargingSessionInvoice{}, true, nil
	}
	var invoice models.ChargingSessionInvoice
	if err := service.database.WithContext(ctx).First(&invoice, "id = ?", delivery.InvoiceID).Error; err != nil {
		return delivery, invoice, false, err
	}
	return delivery, invoice, true, nil
}
func (service *Service) deliver(ctx context.Context, delivery models.InvoiceDelivery, invoice models.ChargingSessionInvoice) (bool, error) {
	if delivery.Status == "AMBIGUOUS" {
		return false, nil
	}
	download, err := service.openInternal(ctx, invoice)
	if err != nil {
		if errors.Is(err, ErrCorrupt) {
			service.markCorrupt(ctx, invoice.ID, "invoice artifact failed integrity validation before email delivery")
			return false, service.failDeliveryTerminal(ctx, delivery, err)
		}
		return false, service.failDeliveryBeforeSMTP(ctx, delivery, err)
	}
	defer download.Content.Close()
	content, err := io.ReadAll(download.Content)
	if err != nil {
		return false, service.failDeliveryBeforeSMTP(ctx, delivery, err)
	}
	if service.sender == nil {
		return false, service.failDeliveryBeforeSMTP(ctx, delivery, errors.New("invoice SMTP sender unavailable"))
	}
	snapshot, err := decodeSnapshot(invoice.Snapshot)
	if err != nil {
		return false, service.failDeliveryBeforeSMTP(ctx, delivery, err)
	}
	textBody := invoiceEmailText(snapshot, service.displayZone)
	prepared, err := service.sender.PrepareInvoice(delivery.Recipient, invoiceEmailSubject(snapshot), textBody, content, download.Name, invoiceIssuerDisplayName(snapshot.Supplier))
	if err != nil {
		return false, service.failDeliveryBeforeSMTP(ctx, delivery, err)
	}
	sendCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	if err := prepared(sendCtx); err != nil {
		return false, service.markAmbiguous(ctx, delivery, err)
	}
	now := service.now()
	result := service.database.WithContext(ctx).Model(&models.InvoiceDelivery{}).Where("id=? AND status='SENDING'", delivery.ID).Updates(map[string]any{"status": "SENT", "sent_at": now, "locked_at": nil, "last_error": nil, "updated_at": now})
	if result.Error != nil {
		return false, result.Error
	}
	if result.RowsAffected != 1 {
		return false, errors.New("invoice SMTP succeeded but SENT persistence is uncertain")
	}
	return true, nil
}

func invoiceEmailText(snapshot issuanceSnapshot, zone *time.Location) string {
	greeting := "Hello,"
	if strings.TrimSpace(snapshot.Customer.FullName) != "" {
		greeting = "Hello " + strings.TrimSpace(snapshot.Customer.FullName) + ","
	}
	location := invoiceLocationLines(snapshot.Location)
	chargingTime := optionalInvoiceTime("Charging time", snapshot.Charging.StartedAt, zone)
	if snapshot.Charging.EndedAt != nil {
		chargingTime += " to " + formatInvoiceTime(*snapshot.Charging.EndedAt, zone)
	}
	lines := []string{
		greeting,
		"",
		strings.TrimSpace(snapshot.Supplier.Name) + " has issued invoice " + snapshot.InvoiceNumber + " for your charging session.",
		"Final amount: " + formatMoney(snapshot.Commercial.Currency, snapshot.Commercial.TotalAmount),
		optionalTextLine("Energy delivered", formatKWh(snapshot.Commercial.TotalKWh)),
		"Charging location: " + strings.Join(location, ", "),
		chargingTime,
		"",
		"Your invoice PDF is attached.",
	}
	return strings.Join(compactInvoiceLinesPreservingParagraphs(lines), "\n")
}

func invoiceEmailSubject(snapshot issuanceSnapshot) string {
	issuer := invoiceIssuerDisplayName(snapshot.Supplier)
	if issuer == "" {
		return "Charging invoice " + snapshot.InvoiceNumber
	}
	return issuer + " charging invoice " + snapshot.InvoiceNumber
}

func invoiceIssuerDisplayName(supplier supplierSnapshot) string {
	return strings.TrimSpace(supplier.Name)
}

func compactInvoiceLinesPreservingParagraphs(lines []string) []string {
	result := make([]string, 0, len(lines))
	previousBlank := false
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			if len(result) > 0 && !previousBlank {
				result = append(result, "")
				previousBlank = true
			}
			continue
		}
		result = append(result, strings.TrimSpace(line))
		previousBlank = false
	}
	return result
}
func (service *Service) openInternal(ctx context.Context, invoice models.ChargingSessionInvoice) (*Download, error) {
	if err := invoiceDownloadStateError(invoice); err != nil {
		return nil, err
	}
	if invoice.StoragePath == nil || invoice.SHA256 == nil || invoice.FileSize == nil {
		return nil, ErrNotReady
	}
	path, err := service.safePath(*invoice.StoragePath)
	if err != nil {
		return nil, err
	}
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() {
		return nil, ErrCorrupt
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	digest := sha256.New()
	size, err := io.Copy(digest, file)
	if err != nil || size != *invoice.FileSize || hex.EncodeToString(digest.Sum(nil)) != *invoice.SHA256 {
		file.Close()
		return nil, ErrCorrupt
	}
	_, err = file.Seek(0, 0)
	if err != nil {
		file.Close()
		return nil, err
	}
	return &Download{Content: file, Name: downloadFilename(invoice.InvoiceNumber), ModTime: info.ModTime()}, nil
}
func downloadFilename(invoiceNumber string) string {
	name := strings.NewReplacer("/", "-", "\\", "-", "\r", "", "\n", "", "\x00", "").Replace(invoiceNumber)
	name = strings.Trim(name, ". ")
	if name == "" {
		name = "charging-session-invoice"
	}
	return name + ".pdf"
}
func (service *Service) failDeliveryBeforeSMTP(ctx context.Context, delivery models.InvoiceDelivery, cause error) error {
	now := service.now()
	status := "PENDING"
	available := now.Add(retryDelay(delivery.Attempts))
	if delivery.Attempts >= delivery.MaxAttempts {
		status, available = "FAILED", now
	}
	return service.database.WithContext(ctx).Model(&models.InvoiceDelivery{}).Where("id=? AND status='SENDING'", delivery.ID).Updates(map[string]any{"status": status, "available_at": available, "locked_at": nil, "last_error": boundedError(cause), "updated_at": now}).Error
}
func (service *Service) failDeliveryTerminal(ctx context.Context, delivery models.InvoiceDelivery, cause error) error {
	now := service.now()
	return service.database.WithContext(ctx).Model(&models.InvoiceDelivery{}).Where("id=? AND status='SENDING'", delivery.ID).Updates(map[string]any{"status": "FAILED", "available_at": now, "locked_at": nil, "last_error": boundedError(cause), "updated_at": now}).Error
}
func (service *Service) markAmbiguous(ctx context.Context, delivery models.InvoiceDelivery, cause error) error {
	now := service.now()
	return service.database.WithContext(ctx).Model(&models.InvoiceDelivery{}).Where("id=? AND status='SENDING'", delivery.ID).Updates(map[string]any{"status": "AMBIGUOUS", "locked_at": nil, "last_error": boundedError(cause), "updated_at": now}).Error
}
