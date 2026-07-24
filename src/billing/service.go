package billing

import (
	"context"
	"errors"
	"log"
	"net/http"
	netmail "net/mail"
	"regexp"
	"strings"
	"time"

	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/auth"
	cmsmail "github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/mail"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/models"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/platformops"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var referencePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._/-]{0,119}$`)

type Service struct {
	database    *gorm.DB
	outbox      *cmsmail.Outbox
	mailEnabled bool
	events      *platformops.Service
	now         func() time.Time
}

func NewService(
	database *gorm.DB,
	outbox *cmsmail.Outbox,
	mailEnabled bool,
	events *platformops.Service,
) *Service {
	return &Service{
		database: database, outbox: outbox, mailEnabled: mailEnabled,
		events: events, now: func() time.Time { return time.Now().UTC() },
	}
}

func (service *Service) GetAccount(
	ctx context.Context,
	principal auth.Principal,
	cpoID uuid.UUID,
) (models.CPOBillingAccount, error) {
	if err := requirePlatform(principal); err != nil {
		return models.CPOBillingAccount{}, err
	}
	var record models.CPOBillingAccount
	if err := service.database.WithContext(ctx).
		First(&record, "cpo_id = ?", cpoID).Error; err != nil {
		return record, notFound(
			"billing_account_not_found",
			"CPO billing account was not found.",
			err,
		)
	}
	return record, nil
}

func (service *Service) SetAccount(
	ctx context.Context,
	principal auth.Principal,
	cpoID uuid.UUID,
	request BillingAccountRequest,
) (models.CPOBillingAccount, error) {
	if err := requirePlatform(principal); err != nil {
		return models.CPOBillingAccount{}, err
	}
	request.LegalName = strings.TrimSpace(request.LegalName)
	request.BillingEmail = strings.ToLower(strings.TrimSpace(request.BillingEmail))
	request.Currency = strings.ToUpper(strings.TrimSpace(request.Currency))
	if request.TaxID != nil {
		value := strings.TrimSpace(*request.TaxID)
		request.TaxID = &value
	}
	if request.BillingAddress == nil {
		request.BillingAddress = models.JSONB{}
	}
	if request.LegalName == "" || len(request.LegalName) > 255 {
		return models.CPOBillingAccount{}, invalid(
			"legal_name",
			"legal_name is required and must not exceed 255 characters.",
		)
	}
	address, err := netmail.ParseAddress(request.BillingEmail)
	if err != nil || !strings.EqualFold(address.Address, request.BillingEmail) ||
		len(request.BillingEmail) > 320 {
		return models.CPOBillingAccount{}, invalid(
			"billing_email",
			"billing_email must be a valid address.",
		)
	}
	if len(request.Currency) != 3 {
		return models.CPOBillingAccount{}, invalid(
			"currency",
			"currency must be a three-letter uppercase code.",
		)
	}
	if request.TaxID != nil && len(*request.TaxID) > 50 {
		return models.CPOBillingAccount{}, invalid(
			"tax_id",
			"tax_id must not exceed 50 characters.",
		)
	}
	now := service.now()
	record := models.CPOBillingAccount{
		ID: uuid.New(), CPOID: cpoID, LegalName: request.LegalName,
		BillingEmail: request.BillingEmail, TaxID: request.TaxID,
		Currency: request.Currency, BillingAddress: request.BillingAddress,
		CreatedBy: principal.UserID, CreatedAt: now, UpdatedAt: now,
	}
	err = service.database.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var cpoCount int64
		if err := tx.Model(&models.CPO{}).Where("id = ?", cpoID).
			Count(&cpoCount).Error; err != nil {
			return err
		}
		if cpoCount == 0 {
			return notFound("cpo_not_found", "CPO was not found.", gorm.ErrRecordNotFound)
		}
		if err := tx.Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "cpo_id"}},
			DoUpdates: clause.Assignments(map[string]any{
				"legal_name": request.LegalName, "billing_email": request.BillingEmail,
				"tax_id": request.TaxID, "currency": request.Currency,
				"billing_address": request.BillingAddress, "updated_at": now,
			}),
		}).Create(&record).Error; err != nil {
			return mapWriteError(err, "billing_account_conflict")
		}
		if err := tx.Where("cpo_id = ?", cpoID).First(&record).Error; err != nil {
			return err
		}
		if err := writeAudit(
			tx, principal.UserID, "CPO_BILLING_ACCOUNT_SET",
			"CPO", cpoID, models.JSONB{"currency": record.Currency}, now,
		); err != nil {
			return err
		}
		return service.emit(
			tx, principal.UserID, "platform.invoice.billing_account_updated",
			"CPO", cpoID.String(), models.JSONB{"currency": record.Currency},
		)
	})
	return record, err
}

func (service *Service) CreateInvoice(
	ctx context.Context,
	principal auth.Principal,
	cpoID uuid.UUID,
	request CreateInvoiceRequest,
) (InvoiceView, error) {
	if err := requirePlatform(principal); err != nil {
		return InvoiceView{}, err
	}
	request.InvoiceNumber = strings.TrimSpace(request.InvoiceNumber)
	request.IdempotencyKey = strings.TrimSpace(request.IdempotencyKey)
	if request.ExternalReference != nil {
		value := strings.TrimSpace(*request.ExternalReference)
		request.ExternalReference = &value
	}
	if !referencePattern.MatchString(request.InvoiceNumber) ||
		request.IdempotencyKey == "" || len(request.IdempotencyKey) > 120 ||
		len(request.Lines) == 0 || len(request.Lines) > 500 {
		return InvoiceView{}, invalid(
			"invoice",
			"invoice_number, idempotency_key, and 1 through 500 lines are required.",
		)
	}
	if request.PeriodStartsAt != nil && request.PeriodEndsAt != nil &&
		!request.PeriodEndsAt.After(*request.PeriodStartsAt) {
		return InvoiceView{}, invalid(
			"period",
			"period_ends_at must be after period_starts_at.",
		)
	}
	now := service.now()
	var invoice models.PlatformInvoice
	var lines []models.PlatformInvoiceLine
	err := service.database.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		existing, ok, err := service.idempotentInvoice(
			tx, principal.UserID, request.IdempotencyKey, cpoID,
		)
		if err != nil {
			return err
		}
		if ok {
			invoice = existing
			return tx.Where("invoice_id = ?", invoice.ID).
				Order("line_number").Find(&lines).Error
		}
		var account models.CPOBillingAccount
		if err := tx.First(&account, "cpo_id = ?", cpoID).Error; err != nil {
			return notFound(
				"billing_account_not_found",
				"Create the CPO billing account before an invoice.",
				err,
			)
		}
		if request.SubscriptionID != nil {
			var count int64
			if err := tx.Model(&models.CPOSubscription{}).
				Where("id = ? AND cpo_id = ?", *request.SubscriptionID, cpoID).
				Count(&count).Error; err != nil {
				return err
			}
			if count == 0 {
				return notFound(
					"subscription_not_found",
					"Subscription does not belong to this CPO.",
					gorm.ErrRecordNotFound,
				)
			}
		}
		var subtotal, tax int64
		lines = make([]models.PlatformInvoiceLine, 0, len(request.Lines))
		for index, input := range request.Lines {
			input.Description = strings.TrimSpace(input.Description)
			if input.Description == "" || len(input.Description) > 500 ||
				input.Quantity <= 0 || input.UnitAmountMinor < 0 ||
				input.TaxMinor < 0 {
				return invalid(
					"lines",
					"Every line requires description, positive quantity, and non-negative exact amounts.",
				)
			}
			lineSubtotal, overflow := multiplyExact(
				input.Quantity,
				input.UnitAmountMinor,
			)
			if overflow || lineSubtotal > maxMoney-input.TaxMinor {
				return invalid("lines", "Invoice line amount is too large.")
			}
			if input.Metadata == nil {
				input.Metadata = models.JSONB{}
			}
			if subtotal > maxMoney-lineSubtotal ||
				tax > maxMoney-input.TaxMinor {
				return invalid("lines", "Invoice total is too large.")
			}
			subtotal += lineSubtotal
			tax += input.TaxMinor
			lines = append(lines, models.PlatformInvoiceLine{
				ID: uuid.New(), LineNumber: index + 1,
				Description: input.Description, Quantity: input.Quantity,
				UnitAmountMinor: input.UnitAmountMinor,
				SubtotalMinor:   lineSubtotal, TaxMinor: input.TaxMinor,
				TotalMinor: lineSubtotal + input.TaxMinor,
				Metadata:   input.Metadata, CreatedAt: now,
			})
		}
		if subtotal > maxMoney-tax {
			return invalid("lines", "Invoice total is too large.")
		}
		total := subtotal + tax
		invoice = models.PlatformInvoice{
			ID: uuid.New(), InvoiceNumber: request.InvoiceNumber, CPOID: cpoID,
			BillingAccountID: account.ID, SubscriptionID: request.SubscriptionID,
			Currency: account.Currency, Status: "DRAFT",
			SubtotalMinor: subtotal, TaxMinor: tax, TotalMinor: total,
			DueMinor: total, PeriodStartsAt: request.PeriodStartsAt,
			PeriodEndsAt:      request.PeriodEndsAt,
			ExternalReference: request.ExternalReference,
			IdempotencyKey:    request.IdempotencyKey, CreatedBy: principal.UserID,
			CreatedAt: now, UpdatedAt: now,
		}
		if err := tx.Create(&invoice).Error; err != nil {
			return mapWriteError(err, "invoice_conflict")
		}
		for index := range lines {
			lines[index].InvoiceID = invoice.ID
		}
		if err := tx.Create(&lines).Error; err != nil {
			return mapWriteError(err, "invoice_conflict")
		}
		if err := writeAudit(
			tx, principal.UserID, "PLATFORM_INVOICE_CREATED",
			"PLATFORM_INVOICE", invoice.ID,
			models.JSONB{
				"cpo_id": cpoID, "invoice_number": invoice.InvoiceNumber,
				"currency": invoice.Currency, "total_minor": invoice.TotalMinor,
			}, now,
		); err != nil {
			return err
		}
		return service.emit(
			tx, principal.UserID, "platform.invoice.created",
			"PLATFORM_INVOICE", invoice.ID.String(),
			models.JSONB{
				"cpo_id": cpoID, "status": invoice.Status,
				"total_minor": invoice.TotalMinor, "currency": invoice.Currency,
			},
		)
	})
	return InvoiceView{Invoice: invoice, Lines: lines}, err
}

func (service *Service) IssueInvoice(
	ctx context.Context,
	principal auth.Principal,
	invoiceID uuid.UUID,
	request IssueInvoiceRequest,
) (InvoiceView, error) {
	if err := requirePlatform(principal); err != nil {
		return InvoiceView{}, err
	}
	request.Reason = strings.TrimSpace(request.Reason)
	now := service.now()
	if !request.DueAt.After(now) || request.Reason == "" ||
		len(request.Reason) > 500 {
		return InvoiceView{}, invalid(
			"issue",
			"due_at must be future and a reason is required.",
		)
	}
	var invoice models.PlatformInvoice
	err := service.database.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			First(&invoice, "id = ?", invoiceID).Error; err != nil {
			return notFound("invoice_not_found", "Invoice was not found.", err)
		}
		if invoice.Status != "DRAFT" {
			if invoice.Status == "ISSUED" {
				return nil
			}
			return conflict(
				"invalid_invoice_transition",
				"Only a draft invoice can be issued.",
			)
		}
		invoice.Status = "ISSUED"
		invoice.IssuedAt = &now
		due := request.DueAt.UTC()
		invoice.DueAt = &due
		invoice.UpdatedAt = now
		if err := tx.Save(&invoice).Error; err != nil {
			return err
		}
		if err := writeAudit(
			tx, principal.UserID, "PLATFORM_INVOICE_ISSUED",
			"PLATFORM_INVOICE", invoice.ID,
			models.JSONB{
				"cpo_id": invoice.CPOID, "due_at": due,
				"reason": request.Reason,
			}, now,
		); err != nil {
			return err
		}
		if err := service.emit(
			tx, principal.UserID, "platform.invoice.issued",
			"PLATFORM_INVOICE", invoice.ID.String(),
			models.JSONB{
				"cpo_id": invoice.CPOID, "status": invoice.Status,
				"due_at": due,
			},
		); err != nil {
			return err
		}
		if !service.mailEnabled || service.outbox == nil {
			return nil
		}
		var account models.CPOBillingAccount
		if err := tx.First(&account, "id = ?", invoice.BillingAccountID).Error; err != nil {
			return err
		}
		return service.outbox.EnqueueMessage(
			tx,
			account.BillingEmail,
			"CPO_PLATFORM_INVOICE_ISSUED",
			cmsmail.MessagePayload{
				RecipientName:     account.LegalName,
				CPOID:             invoice.CPOID.String(),
				InvoiceNumber:     invoice.InvoiceNumber,
				InvoiceCurrency:   invoice.Currency,
				InvoiceTotalMinor: invoice.TotalMinor,
				InvoiceDueAt:      due,
			},
		)
	})
	if err != nil {
		return InvoiceView{}, err
	}
	return service.GetInvoice(ctx, principal, invoice.ID)
}

func (service *Service) VoidInvoice(
	ctx context.Context,
	principal auth.Principal,
	invoiceID uuid.UUID,
	request VoidInvoiceRequest,
) (InvoiceView, error) {
	if err := requirePlatform(principal); err != nil {
		return InvoiceView{}, err
	}
	request.Reason = strings.TrimSpace(request.Reason)
	if request.Reason == "" || len(request.Reason) > 500 {
		return InvoiceView{}, invalid(
			"reason",
			"reason is required and must not exceed 500 characters.",
		)
	}
	now := service.now()
	var invoice models.PlatformInvoice
	err := service.database.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			First(&invoice, "id = ?", invoiceID).Error; err != nil {
			return notFound("invoice_not_found", "Invoice was not found.", err)
		}
		if invoice.Status == "VOID" {
			return nil
		}
		if invoice.Status == "DRAFT" || invoice.PaidMinor > 0 {
			return conflict(
				"invalid_invoice_transition",
				"Only an issued unpaid invoice can be voided.",
			)
		}
		invoice.Status = "VOID"
		invoice.VoidedAt = &now
		invoice.VoidReason = &request.Reason
		invoice.UpdatedAt = now
		if err := tx.Save(&invoice).Error; err != nil {
			return err
		}
		if err := writeAudit(
			tx, principal.UserID, "PLATFORM_INVOICE_VOIDED",
			"PLATFORM_INVOICE", invoice.ID,
			models.JSONB{
				"cpo_id": invoice.CPOID, "reason": request.Reason,
			}, now,
		); err != nil {
			return err
		}
		return service.emit(
			tx, principal.UserID, "platform.invoice.voided",
			"PLATFORM_INVOICE", invoice.ID.String(),
			models.JSONB{"cpo_id": invoice.CPOID, "status": invoice.Status},
		)
	})
	if err != nil {
		return InvoiceView{}, err
	}
	return service.GetInvoice(ctx, principal, invoice.ID)
}

func (service *Service) GetInvoice(
	ctx context.Context,
	principal auth.Principal,
	invoiceID uuid.UUID,
) (InvoiceView, error) {
	if err := requirePlatform(principal); err != nil {
		return InvoiceView{}, err
	}
	var invoice models.PlatformInvoice
	if err := service.database.WithContext(ctx).
		First(&invoice, "id = ?", invoiceID).Error; err != nil {
		return InvoiceView{}, notFound(
			"invoice_not_found",
			"Invoice was not found.",
			err,
		)
	}
	var lines []models.PlatformInvoiceLine
	if err := service.database.WithContext(ctx).
		Where("invoice_id = ?", invoiceID).Order("line_number").
		Find(&lines).Error; err != nil {
		return InvoiceView{}, err
	}
	return InvoiceView{Invoice: invoice, Lines: lines}, nil
}

func (service *Service) ListInvoices(
	ctx context.Context,
	principal auth.Principal,
	cpoID uuid.UUID,
) ([]models.PlatformInvoice, error) {
	if err := requirePlatform(principal); err != nil {
		return nil, err
	}
	var records []models.PlatformInvoice
	if err := service.database.WithContext(ctx).Where("cpo_id = ?", cpoID).
		Order("created_at DESC, id DESC").Limit(500).
		Find(&records).Error; err != nil {
		return nil, err
	}
	return records, nil
}

func (service *Service) RecordPayment(
	ctx context.Context,
	principal auth.Principal,
	cpoID uuid.UUID,
	request RecordPaymentRequest,
) (PaymentView, error) {
	if err := requirePlatform(principal); err != nil {
		return PaymentView{}, err
	}
	request.PaymentReference = strings.TrimSpace(request.PaymentReference)
	request.Currency = strings.ToUpper(strings.TrimSpace(request.Currency))
	request.Method = strings.ToUpper(strings.TrimSpace(request.Method))
	request.Notes = strings.TrimSpace(request.Notes)
	request.IdempotencyKey = strings.TrimSpace(request.IdempotencyKey)
	if request.ExternalReference != nil {
		value := strings.TrimSpace(*request.ExternalReference)
		request.ExternalReference = &value
	}
	if !referencePattern.MatchString(request.PaymentReference) ||
		len(request.Currency) != 3 || request.AmountMinor <= 0 ||
		request.Method == "" || len(request.Method) > 50 ||
		len(request.Notes) > 1000 || request.IdempotencyKey == "" ||
		len(request.IdempotencyKey) > 120 ||
		request.OccurredAt.IsZero() || request.OccurredAt.After(service.now()) ||
		len(request.Allocations) > 500 {
		return PaymentView{}, invalid(
			"payment",
			"Payment metadata, past occurred_at, and idempotency key are required.",
		)
	}
	seen := map[uuid.UUID]struct{}{}
	var allocated int64
	for _, input := range request.Allocations {
		if input.InvoiceID == uuid.Nil || input.AmountMinor <= 0 {
			return PaymentView{}, invalid(
				"allocations",
				"Every allocation requires an invoice and positive amount.",
			)
		}
		if _, exists := seen[input.InvoiceID]; exists {
			return PaymentView{}, invalid(
				"allocations",
				"An invoice may appear only once per payment.",
			)
		}
		seen[input.InvoiceID] = struct{}{}
		if allocated > maxMoney-input.AmountMinor {
			return PaymentView{}, invalid("allocations", "Allocated amount is too large.")
		}
		allocated += input.AmountMinor
	}
	if allocated > request.AmountMinor {
		return PaymentView{}, invalid(
			"allocations",
			"Allocated amount exceeds the payment amount.",
		)
	}
	now := service.now()
	var payment models.PlatformPayment
	var allocations []models.PlatformPaymentAllocation
	err := service.database.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		existing, ok, err := service.idempotentPayment(
			tx, principal.UserID, request.IdempotencyKey, cpoID,
		)
		if err != nil {
			return err
		}
		if ok {
			payment = existing
			return tx.Where("payment_id = ?", payment.ID).
				Order("created_at, id").Find(&allocations).Error
		}
		var cpoCount int64
		if err := tx.Model(&models.CPO{}).Where("id = ?", cpoID).
			Count(&cpoCount).Error; err != nil {
			return err
		}
		if cpoCount == 0 {
			return notFound("cpo_not_found", "CPO was not found.", gorm.ErrRecordNotFound)
		}
		payment = models.PlatformPayment{
			ID: uuid.New(), PaymentReference: request.PaymentReference,
			CPOID: cpoID, Currency: request.Currency,
			AmountMinor: request.AmountMinor, AllocatedMinor: allocated,
			Status: "RECORDED", Method: request.Method,
			ExternalReference: request.ExternalReference,
			OccurredAt:        request.OccurredAt.UTC(), Notes: request.Notes,
			IdempotencyKey: request.IdempotencyKey, CreatedBy: principal.UserID,
			CreatedAt: now, UpdatedAt: now,
		}
		if err := tx.Create(&payment).Error; err != nil {
			return mapWriteError(err, "payment_conflict")
		}
		allocations = make([]models.PlatformPaymentAllocation, 0, len(request.Allocations))
		for _, input := range request.Allocations {
			var invoice models.PlatformInvoice
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
				First(&invoice, "id = ?", input.InvoiceID).Error; err != nil {
				return notFound("invoice_not_found", "Allocated invoice was not found.", err)
			}
			if invoice.CPOID != cpoID || invoice.Currency != request.Currency {
				return conflict(
					"allocation_scope_mismatch",
					"Payment and invoice CPO/currency must match.",
				)
			}
			if invoice.Status != "ISSUED" &&
				invoice.Status != "PARTIALLY_PAID" &&
				invoice.Status != "OVERDUE" {
				return conflict(
					"invalid_invoice_transition",
					"Payment can be allocated only to an open issued invoice.",
				)
			}
			if input.AmountMinor > invoice.DueMinor {
				return conflict(
					"allocation_exceeds_invoice_due",
					"Allocation exceeds invoice due amount.",
				)
			}
			invoice.PaidMinor += input.AmountMinor
			invoice.DueMinor -= input.AmountMinor
			if invoice.DueMinor == 0 {
				invoice.Status = "PAID"
			} else if invoice.DueAt != nil && !invoice.DueAt.After(now) {
				invoice.Status = "OVERDUE"
			} else {
				invoice.Status = "PARTIALLY_PAID"
			}
			invoice.UpdatedAt = now
			if err := tx.Save(&invoice).Error; err != nil {
				return err
			}
			allocations = append(allocations, models.PlatformPaymentAllocation{
				ID: uuid.New(), PaymentID: payment.ID,
				InvoiceID: invoice.ID, AmountMinor: input.AmountMinor,
				CreatedBy: principal.UserID, CreatedAt: now,
			})
		}
		if len(allocations) > 0 {
			if err := tx.Create(&allocations).Error; err != nil {
				return mapWriteError(err, "payment_conflict")
			}
		}
		if err := writeAudit(
			tx, principal.UserID, "PLATFORM_PAYMENT_RECORDED",
			"PLATFORM_PAYMENT", payment.ID,
			models.JSONB{
				"cpo_id": cpoID, "currency": payment.Currency,
				"amount_minor":    payment.AmountMinor,
				"allocated_minor": payment.AllocatedMinor,
			}, now,
		); err != nil {
			return err
		}
		return service.emit(
			tx, principal.UserID, "platform.invoice.payment_recorded",
			"PLATFORM_PAYMENT", payment.ID.String(),
			models.JSONB{
				"cpo_id": cpoID, "amount_minor": payment.AmountMinor,
				"allocated_minor": payment.AllocatedMinor,
			},
		)
	})
	return PaymentView{Payment: payment, Allocations: allocations}, err
}

func (service *Service) GetPayment(
	ctx context.Context,
	principal auth.Principal,
	paymentID uuid.UUID,
) (PaymentView, error) {
	if err := requirePlatform(principal); err != nil {
		return PaymentView{}, err
	}
	var payment models.PlatformPayment
	if err := service.database.WithContext(ctx).
		First(&payment, "id = ?", paymentID).Error; err != nil {
		return PaymentView{}, notFound(
			"payment_not_found",
			"Payment was not found.",
			err,
		)
	}
	var allocations []models.PlatformPaymentAllocation
	if err := service.database.WithContext(ctx).Where("payment_id = ?", paymentID).
		Order("created_at, id").Find(&allocations).Error; err != nil {
		return PaymentView{}, err
	}
	return PaymentView{Payment: payment, Allocations: allocations}, nil
}

func (service *Service) VoidPayment(
	ctx context.Context,
	principal auth.Principal,
	paymentID uuid.UUID,
	request VoidPaymentRequest,
) (PaymentView, error) {
	if err := requirePlatform(principal); err != nil {
		return PaymentView{}, err
	}
	request.Reason = strings.TrimSpace(request.Reason)
	if request.Reason == "" || len(request.Reason) > 500 {
		return PaymentView{}, invalid(
			"reason",
			"reason is required and must not exceed 500 characters.",
		)
	}
	now := service.now()
	var payment models.PlatformPayment
	err := service.database.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			First(&payment, "id = ?", paymentID).Error; err != nil {
			return notFound("payment_not_found", "Payment was not found.", err)
		}
		if payment.Status == "VOID" {
			return nil
		}
		var allocations []models.PlatformPaymentAllocation
		if err := tx.Where("payment_id = ?", payment.ID).
			Order("created_at, id").Find(&allocations).Error; err != nil {
			return err
		}
		for _, allocation := range allocations {
			var invoice models.PlatformInvoice
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
				First(&invoice, "id = ?", allocation.InvoiceID).Error; err != nil {
				return err
			}
			if invoice.PaidMinor < allocation.AmountMinor ||
				invoice.Status == "VOID" {
				return conflict(
					"payment_reversal_conflict",
					"Payment allocations cannot be safely reversed.",
				)
			}
			invoice.PaidMinor -= allocation.AmountMinor
			invoice.DueMinor += allocation.AmountMinor
			if invoice.DueAt != nil && !invoice.DueAt.After(now) {
				invoice.Status = "OVERDUE"
			} else if invoice.PaidMinor > 0 {
				invoice.Status = "PARTIALLY_PAID"
			} else {
				invoice.Status = "ISSUED"
			}
			invoice.UpdatedAt = now
			if err := tx.Save(&invoice).Error; err != nil {
				return err
			}
		}
		payment.Status = "VOID"
		payment.VoidedAt = &now
		payment.VoidReason = &request.Reason
		payment.UpdatedAt = now
		if err := tx.Save(&payment).Error; err != nil {
			return err
		}
		if err := writeAudit(
			tx, principal.UserID, "PLATFORM_PAYMENT_VOIDED",
			"PLATFORM_PAYMENT", payment.ID,
			models.JSONB{
				"cpo_id": payment.CPOID, "reason": request.Reason,
				"reversed_allocated_minor": payment.AllocatedMinor,
			}, now,
		); err != nil {
			return err
		}
		return service.emit(
			tx, principal.UserID, "platform.invoice.payment_voided",
			"PLATFORM_PAYMENT", payment.ID.String(),
			models.JSONB{"cpo_id": payment.CPOID, "status": payment.Status},
		)
	})
	if err != nil {
		return PaymentView{}, err
	}
	return service.GetPayment(ctx, principal, payment.ID)
}

func (service *Service) MarkOverdue(ctx context.Context) error {
	for {
		processed := false
		err := service.database.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			now := service.now()
			var invoice models.PlatformInvoice
			result := tx.Clauses(clause.Locking{
				Strength: "UPDATE",
				Options:  "SKIP LOCKED",
			}).Where(
				"status IN ? AND due_at <= ? AND due_minor > 0",
				[]string{"ISSUED", "PARTIALLY_PAID"},
				now,
			).Order("due_at, id").First(&invoice)
			if errors.Is(result.Error, gorm.ErrRecordNotFound) {
				return nil
			}
			if result.Error != nil {
				return result.Error
			}
			processed = true
			invoice.Status = "OVERDUE"
			invoice.UpdatedAt = now
			if err := tx.Save(&invoice).Error; err != nil {
				return err
			}
			if err := writeAudit(
				tx, invoice.CreatedBy, "PLATFORM_INVOICE_MARKED_OVERDUE",
				"PLATFORM_INVOICE", invoice.ID,
				models.JSONB{
					"cpo_id":      invoice.CPOID,
					"executed_by": "billing-maintenance",
				}, now,
			); err != nil {
				return err
			}
			return service.emit(
				tx, invoice.CreatedBy, "platform.invoice.overdue",
				"PLATFORM_INVOICE", invoice.ID.String(),
				models.JSONB{"cpo_id": invoice.CPOID, "status": invoice.Status},
			)
		})
		if err != nil {
			return err
		}
		if !processed {
			return nil
		}
	}
}

func (service *Service) RunMaintenance(
	ctx context.Context,
	instanceKey string,
	every time.Duration,
) {
	const workerName = "billing-maintenance"
	ticker := time.NewTicker(every)
	defer ticker.Stop()
	for {
		if service.events != nil {
			if err := service.events.Heartbeat(ctx, workerName, instanceKey); err != nil &&
				ctx.Err() == nil {
				log.Printf("record billing maintenance heartbeat: %v", err)
			}
		}
		if err := service.MarkOverdue(ctx); err != nil && ctx.Err() == nil {
			log.Printf("mark overdue platform invoices: %v", err)
		} else if service.events != nil {
			if err := service.events.JobCompleted(ctx, workerName, instanceKey); err != nil &&
				ctx.Err() == nil {
				log.Printf("record billing maintenance completion: %v", err)
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (service *Service) ListPayments(
	ctx context.Context,
	principal auth.Principal,
	cpoID uuid.UUID,
) ([]models.PlatformPayment, error) {
	if err := requirePlatform(principal); err != nil {
		return nil, err
	}
	var records []models.PlatformPayment
	if err := service.database.WithContext(ctx).Where("cpo_id = ?", cpoID).
		Order("created_at DESC, id DESC").Limit(500).
		Find(&records).Error; err != nil {
		return nil, err
	}
	return records, nil
}

func (service *Service) Timeline(
	ctx context.Context,
	principal auth.Principal,
	cpoID uuid.UUID,
) (TimelineResponse, error) {
	if err := requirePlatform(principal); err != nil {
		return TimelineResponse{}, err
	}
	response := TimelineResponse{
		Invoices: []models.PlatformInvoice{},
		Payments: []models.PlatformPayment{},
	}
	var account models.CPOBillingAccount
	result := service.database.WithContext(ctx).First(&account, "cpo_id = ?", cpoID)
	if result.Error == nil {
		response.Account = &account
	} else if !errors.Is(result.Error, gorm.ErrRecordNotFound) {
		return response, result.Error
	}
	if err := service.database.WithContext(ctx).Where("cpo_id = ?", cpoID).
		Order("created_at DESC, id DESC").Limit(500).
		Find(&response.Invoices).Error; err != nil {
		return response, err
	}
	if err := service.database.WithContext(ctx).Where("cpo_id = ?", cpoID).
		Order("created_at DESC, id DESC").Limit(500).
		Find(&response.Payments).Error; err != nil {
		return response, err
	}
	return response, nil
}

const maxMoney int64 = 9_000_000_000_000_000

func multiplyExact(left, right int64) (int64, bool) {
	if left < 0 || right < 0 || (left != 0 && right > maxMoney/left) {
		return 0, true
	}
	return left * right, false
}

func (service *Service) idempotentInvoice(
	tx *gorm.DB,
	actor uuid.UUID,
	key string,
	cpoID uuid.UUID,
) (models.PlatformInvoice, bool, error) {
	var record models.PlatformInvoice
	result := tx.Where("created_by = ? AND idempotency_key = ?", actor, key).
		First(&record)
	if errors.Is(result.Error, gorm.ErrRecordNotFound) {
		return record, false, nil
	}
	if result.Error != nil {
		return record, false, result.Error
	}
	if record.CPOID != cpoID {
		return record, false, conflict(
			"idempotency_conflict",
			"Idempotency key was used for another CPO.",
		)
	}
	return record, true, nil
}

func (service *Service) idempotentPayment(
	tx *gorm.DB,
	actor uuid.UUID,
	key string,
	cpoID uuid.UUID,
) (models.PlatformPayment, bool, error) {
	var record models.PlatformPayment
	result := tx.Where("created_by = ? AND idempotency_key = ?", actor, key).
		First(&record)
	if errors.Is(result.Error, gorm.ErrRecordNotFound) {
		return record, false, nil
	}
	if result.Error != nil {
		return record, false, result.Error
	}
	if record.CPOID != cpoID {
		return record, false, conflict(
			"idempotency_conflict",
			"Idempotency key was used for another CPO.",
		)
	}
	return record, true, nil
}

func (service *Service) emit(
	tx *gorm.DB,
	actor uuid.UUID,
	eventType string,
	resourceType string,
	resourceID string,
	data models.JSONB,
) error {
	if service.events == nil {
		return nil
	}
	_, err := service.events.Emit(tx, platformops.EventInput{
		Type: eventType, ActorUserID: &actor, ResourceType: resourceType,
		ResourceID: &resourceID, Data: data,
	})
	return err
}

func writeAudit(
	tx *gorm.DB,
	actor uuid.UUID,
	action string,
	entity string,
	entityID uuid.UUID,
	details models.JSONB,
	now time.Time,
) error {
	return tx.Create(&models.AuditLog{
		ID: uuid.New(), UserID: &actor, Action: action, Entity: entity,
		EntityID: &entityID, Details: details, CreatedAt: now,
	}).Error
}

func requirePlatform(principal auth.Principal) error {
	if principal.Scope != "PLATFORM" {
		return &auth.APIError{
			Status: http.StatusForbidden, Code: "permission_denied",
			Message: "Platform superadmin access is required.",
		}
	}
	return nil
}

func invalid(field, message string) error {
	return &auth.APIError{
		Status: http.StatusBadRequest, Code: "invalid_" + field,
		Message: message,
	}
}

func conflict(code, message string) error {
	return &auth.APIError{Status: http.StatusConflict, Code: code, Message: message}
}

func notFound(code, message string, err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return &auth.APIError{Status: http.StatusNotFound, Code: code, Message: message}
	}
	return err
}

func mapWriteError(err error, code string) error {
	var pgError *pgconn.PgError
	if errors.As(err, &pgError) &&
		(pgError.Code == "23505" ||
			pgError.Code == "23514" ||
			pgError.Code == "23503") {
		return conflict(code, "The billing operation conflicts with current state.")
	}
	return err
}
