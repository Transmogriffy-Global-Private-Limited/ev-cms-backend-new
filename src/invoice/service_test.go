package invoice

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/config"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/models"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

func TestIndianFinancialYear(t *testing.T) {
	zone := time.FixedZone("IST", 5*60*60+30*60)
	if got := indianFinancialYear(time.Date(2026, time.March, 31, 23, 59, 0, 0, zone), zone); got != "25-26" {
		t.Fatalf("March financial year = %q", got)
	}
	if got := indianFinancialYear(time.Date(2026, time.April, 1, 0, 0, 0, 0, zone), zone); got != "26-27" {
		t.Fatalf("April financial year = %q", got)
	}
	if got := invoiceNumber("26-27", 1); got != "INV/26-27/000001" {
		t.Fatalf("invoice number = %q", got)
	}
	if got := invoiceNumber("26-27", maxInvoiceSerial); got != "INV/26-27/999999" {
		t.Fatalf("last invoice number = %q", got)
	}
}

func TestPublishIsDeterministicAndRefusesDifferentExistingBytes(t *testing.T) {
	root := t.TempDir()
	service := &Service{storageRoot: root}
	record := models.ChargingSessionInvoice{ID: uuid.New(), CPOID: uuid.New(), FinancialYear: "26-27"}
	firstPath, firstSize, firstHash, err := service.publish(record, []byte("%PDF-immutable"))
	if err != nil {
		t.Fatalf("publish initial artifact: %v", err)
	}
	secondPath, secondSize, secondHash, err := service.publish(record, []byte("%PDF-immutable"))
	if err != nil {
		t.Fatalf("adopt matching crash-recovery artifact: %v", err)
	}
	if firstPath != secondPath || firstSize != secondSize || firstHash != secondHash {
		t.Fatalf("matching publication was not deterministic: (%q,%d,%q) then (%q,%d,%q)", firstPath, firstSize, firstHash, secondPath, secondSize, secondHash)
	}
	if _, _, _, err := service.publish(record, []byte("%PDF-different")); err == nil {
		t.Fatal("different bytes overwrote a canonical artifact")
	}
	stored, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(firstPath)))
	if err != nil {
		t.Fatalf("read canonical artifact: %v", err)
	}
	if string(stored) != "%PDF-immutable" {
		t.Fatalf("canonical artifact changed: %q", stored)
	}
}

func TestDownloadFilenameCannotContainPathSeparators(t *testing.T) {
	name := downloadFilename("INV/26-27/000001\\..\r\n")
	if strings.ContainsAny(name, "\\/\r\n") || !strings.HasSuffix(name, ".pdf") {
		t.Fatalf("unsafe download filename: %q", name)
	}
}

func TestControlledLogoFileRejectsPathOutsidePrivateRoot(t *testing.T) {
	if _, _, err := controlledLogoFile(filepath.Join(t.TempDir(), "logo.png")); err == nil {
		t.Fatal("accepted logo outside controlled private root")
	}
}

func TestUnsupportedRendererVersionFailsClosed(t *testing.T) {
	_, err := renderInvoice(issuanceSnapshot{SchemaVersion: snapshotVersion}, "unknown", models.InvoiceAsset{}, false, time.UTC)
	if err == nil || !strings.Contains(err.Error(), "unsupported invoice renderer") {
		t.Fatalf("unsupported renderer error = %v", err)
	}
	if errors.Is(err, ErrNotReady) {
		t.Fatal("renderer failure was misclassified as a pending invoice")
	}
}

func TestRenderPDFUsesUnicodeAndIssuanceSnapshot(t *testing.T) {
	now := time.Date(2026, time.April, 2, 10, 0, 0, 0, time.UTC)
	limitType, energySource := "ENERGY", "CUSTOMER"
	energy := int64(1000)
	snapshot := issuanceSnapshot{SchemaVersion: snapshotVersion, InvoiceNumber: "INV/26-27/000001", IssuedAt: now, Supplier: supplierSnapshot{Name: "CPO Café", GSTIN: "27ABCDE1234F1Z5", Address: "1 Test Road", City: "Mumbai", State: "Maharashtra", Pincode: "400001"}, Customer: customerSnapshot{FullName: "Zoë ग्राहक বাংলা", Email: "zoe@example.test"}, Location: locationSnapshot{HubName: "Central", ChargerCode: "abc123", ChargerName: "Fast charger", ConnectorNumber: 1, ConnectorType: "CCS2"}, Charging: chargingSnapshot{SessionID: "00000000-0000-0000-0000-000000000001", OCPPTransactionID: 42, StartedAt: now.Add(-time.Hour), EndedAt: &now, MeterStartWh: 100, MeterStopWh: int64Pointer(1100), DurationSeconds: 3600, LimitType: &limitType, EnergyLimitWh: &energy, EnergyLimitSource: &energySource}, Commercial: commercialSnapshot{TotalKWh: "1.000", TotalAmount: "12.50", Currency: "INR", SettlementStatus: "SETTLED", Tariff: invoiceTariffSnapshot{PricePerUnit: stringPointer("10.00")}, Tax: invoiceTaxSnapshot{CGSTRate: stringPointer("9.00")}}, InvoiceNote: "Thank you"}
	pdf, err := renderInvoice(snapshot, rendererVersion, models.InvoiceAsset{}, false, time.UTC)
	if err != nil {
		t.Fatalf("render PDF: %v", err)
	}
	if !bytes.HasPrefix(pdf, []byte("%PDF-")) {
		t.Fatal("output is not PDF")
	}
	if len(pdf) < 1024 || strings.Contains(string(pdf), "Final amount: INR 99.99") {
		t.Fatal("PDF did not contain the expected rendered document")
	}
}

func TestInvoiceFontsShapeIndicText(t *testing.T) {
	fonts, err := newInvoiceFonts()
	if err != nil {
		t.Fatalf("load invoice fonts: %v", err)
	}
	defer fonts.latin.Destroy()
	defer fonts.bengali.Destroy()
	defer fonts.devanagari.Destroy()

	for _, text := range []string{"বাংলা চার্জিং", "ग्राहक चार्जिंग"} {
		shaped := fonts.textBox(text, 11, 120, false)
		if shaped == nil || shaped.Bounds().W() <= 0 || shaped.Bounds().H() <= 0 {
			t.Fatalf("Indic text did not form a drawable shaped run: %q", text)
		}
	}
}

func TestNewServiceRejectsInvalidInvoiceConfiguration(t *testing.T) {
	for name, cfg := range map[string]config.Invoice{
		"blank storage root":   {StorageRoot: " ", WorkerPoll: time.Second, BatchSize: 1},
		"zero worker interval": {StorageRoot: t.TempDir(), BatchSize: 1},
		"zero batch size":      {StorageRoot: t.TempDir(), WorkerPoll: time.Second},
		"oversized batch":      {StorageRoot: t.TempDir(), WorkerPoll: time.Second, BatchSize: 101},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := NewService(&gorm.DB{}, cfg, time.UTC); err == nil {
				t.Fatal("NewService accepted invalid invoice configuration")
			}
		})
	}
}

func TestGenerationFailureStatusExhaustsFinalAttempt(t *testing.T) {
	if got := generationFailureStatus(maxGenerationAttempts - 1); got != "PENDING" {
		t.Fatalf("attempt %d status = %q, want PENDING", maxGenerationAttempts-1, got)
	}
	if got := generationFailureStatus(maxGenerationAttempts); got != "FAILED" {
		t.Fatalf("attempt %d status = %q, want FAILED", maxGenerationAttempts, got)
	}
}

func TestEnsureInvoiceBatchContinuesAfterOneFailure(t *testing.T) {
	first, poison, last := uuid.New(), uuid.New(), uuid.New()
	var calls []uuid.UUID
	err := ensureInvoiceBatch(context.Background(), []uuid.UUID{first, poison, last}, func(_ context.Context, sessionID uuid.UUID) (models.ChargingSessionInvoice, error) {
		calls = append(calls, sessionID)
		if sessionID == poison {
			return models.ChargingSessionInvoice{}, errors.New("poison historical session")
		}
		return models.ChargingSessionInvoice{}, nil
	})
	if err == nil || !strings.Contains(err.Error(), "1 charging-session invoice issuances failed") {
		t.Fatalf("batch error = %v, want truthful aggregate failure", err)
	}
	if len(calls) != 3 || calls[0] != first || calls[1] != poison || calls[2] != last {
		t.Fatalf("EnsureInvoice calls = %v, want all sessions in bounded batch", calls)
	}
}

func TestInvoiceDownloadStateDispatchesTerminalFailuresAsUnavailable(t *testing.T) {
	for status, want := range map[string]error{
		"PENDING": ErrNotReady,
		"FAILED":  ErrCorrupt,
		"CORRUPT": ErrCorrupt,
		"READY":   nil,
	} {
		if got := invoiceDownloadStateError(models.ChargingSessionInvoice{GenerationStatus: status}); !errors.Is(got, want) {
			t.Fatalf("status %s error = %v, want %v", status, got, want)
		}
	}
}

func TestSummaryWithoutInvoiceTreatsFinalSessionAsPending(t *testing.T) {
	if got := summaryWithoutInvoice(models.ChargingSession{Status: "COMPLETED", SettlementStatus: "SETTLED"}); got.State != PublicPending {
		t.Fatalf("final session summary = %s, want PENDING", got.State)
	}
	if got := summaryWithoutInvoice(models.ChargingSession{Status: "COMPLETED", SettlementStatus: "PENDING"}); got.State != PublicNotAvailable {
		t.Fatalf("unsettled session summary = %s, want NOT_AVAILABLE", got.State)
	}
}

func TestLogoAssetFromContentRejectsTruncatedPNG(t *testing.T) {
	if _, ok := logoAssetFromContent(truncatedPNG(t)); ok {
		t.Fatal("truncated PNG was accepted as an invoice snapshot logo")
	}
}

func truncatedPNG(t *testing.T) []byte {
	t.Helper()
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, image.NewRGBA(image.Rect(0, 0, 1, 1))); err != nil {
		t.Fatalf("encode PNG: %v", err)
	}
	data := encoded.Bytes()
	chunk := bytes.Index(data, []byte("IDAT"))
	if chunk < 0 {
		t.Fatal("test PNG lacks IDAT")
	}
	truncated := append([]byte(nil), data[:chunk+4]...)
	if _, _, err := image.DecodeConfig(bytes.NewReader(truncated)); err != nil {
		t.Fatalf("truncated PNG must pass DecodeConfig: %v", err)
	}
	if _, _, err := image.Decode(bytes.NewReader(truncated)); err == nil {
		t.Fatal("truncated PNG unexpectedly fully decoded")
	}
	return truncated
}

func int64Pointer(value int64) *int64    { return &value }
func stringPointer(value string) *string { return &value }
