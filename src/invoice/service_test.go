package invoice

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/config"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/constants"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/models"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/tdewolff/canvas"
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

func TestCanvasRendererVersionsRemainDispatchable(t *testing.T) {
	snapshot := customerPresentationSnapshot()
	snapshot.Location.HubAddress = "42 River Road, West Bengal"
	legacyLocation := customerInvoiceSectionsV1(snapshot, time.UTC)[2].Lines
	currentLocation := customerInvoiceSections(snapshot, time.UTC)[2].Lines
	if len(legacyLocation) != 3 || len(currentLocation) != 2 {
		t.Fatalf("renderer-version location projection changed: v1=%q v2=%q", legacyLocation, currentLocation)
	}
	legacy, err := renderInvoice(snapshot, legacyCanvasRenderer, models.InvoiceAsset{}, false, time.UTC)
	if err != nil {
		t.Fatalf("render recorded v1 invoice: %v", err)
	}
	current, err := renderInvoice(snapshot, rendererVersion, models.InvoiceAsset{}, false, time.UTC)
	if err != nil {
		t.Fatalf("render current v3 invoice: %v", err)
	}
	if !bytes.HasPrefix(legacy, []byte("%PDF-")) || !bytes.HasPrefix(current, []byte("%PDF-")) {
		t.Fatal("one of the supported Canvas renderer versions did not produce a PDF")
	}
	if bytes.Equal(legacy, current) {
		t.Fatal("redesigned v3 renderer unexpectedly produced the v1 artifact")
	}
	v2, err := renderInvoice(snapshot, legacyCanvasRendererV2, models.InvoiceAsset{}, false, time.UTC)
	if err != nil || !bytes.HasPrefix(v2, []byte("%PDF-")) || bytes.Equal(v2, current) {
		t.Fatalf("recorded v2 renderer was not preserved: %v", err)
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

func TestBuildSnapshotUsesHubAsLocationAndKeepsChargerSeparate(t *testing.T) {
	zone := time.FixedZone("IST", 5*60*60+30*60)
	now := time.Date(2026, time.September, 14, 9, 30, 0, 0, time.UTC)
	end := now.Add(75 * time.Minute)
	phone, paymentMethod := "+919999999999", "WALLET"
	requested := decimal.RequireFromString("45")
	session := models.ChargingSession{
		ID:               uuid.New(),
		TransactionID:    42,
		StartTime:        now,
		EndTime:          &end,
		MeterStartWh:     1000,
		MeterStopWh:      int64Pointer(6250),
		TotalKWh:         decimal.RequireFromString("5.250"),
		TotalAmount:      decimal.RequireFromString("126.00"),
		Currency:         "INR",
		SettlementStatus: "SETTLED",
		Customer:         models.Customer{ID: uuid.New(), FullName: "প্রিয়া शर्मा", Email: "priya@example.test", Phone: &phone},
		Charger: models.Charger{ChargerID: "CP0042", ChargerName: "Riverside DC", ChargerType: "Fast charger", Hub: &models.Hub{
			Name: "নদী তীর Hub", Address: "42 River Road", State: constants.IndianState("West Bengal"),
		}},
		Connector:   models.Connector{ConnectorNumber: 2, ConnectorType: "CCS2", ConnectorTotalCapacity: 60},
		StartIntent: &models.ChargingStartIntent{LimitType: constants.ChargingLimitTypeTime, RequestedLimitValue: &requested},
		Payment:     &models.Payment{PaymentMethod: paymentMethod},
	}
	snapshot := buildSnapshot(uuid.New(), session, models.CPO{BusinessName: "Example Charge", Address: "1 Supplier Street", City: "Kolkata", State: constants.IndianState("West Bengal"), Pincode: "700001"}, models.Settings{}, end, "26-27", 1)
	if snapshot.Location.HubName != "নদী তীর Hub" || snapshot.Location.HubAddress != "42 River Road" || snapshot.Location.HubState != "West Bengal" {
		t.Fatalf("Hub location snapshot = %+v", snapshot.Location)
	}
	if snapshot.Location.ChargerName != "Riverside DC" || snapshot.Location.ChargerCode != "CP0042" || snapshot.Location.ConnectorNumber != 2 {
		t.Fatalf("charger identity was not kept separate: %+v", snapshot.Location)
	}
	if snapshot.Commercial.PaymentMethod == nil || *snapshot.Commercial.PaymentMethod != "WALLET" {
		t.Fatalf("payment method snapshot = %v", snapshot.Commercial.PaymentMethod)
	}
	if location := invoiceLocationLines(snapshot.Location); len(location) != 3 || location[0] != "নদী তীর Hub" || location[1] != "42 River Road" || location[2] != "West Bengal" {
		t.Fatalf("customer location presentation = %q", location)
	}
	email := invoiceEmailText(snapshot, zone)
	for _, want := range []string{"নদী তীর Hub", "42 River Road", snapshot.InvoiceNumber} {
		if !strings.Contains(email, want) {
			t.Fatalf("email omitted customer-facing value %q: %s", want, email)
		}
	}
	if strings.Contains(email, "Riverside DC") || strings.Contains(email, "immutable invoice") {
		t.Fatalf("email leaked charger fallback or backend wording: %s", email)
	}
}

func TestInvoiceIssuanceHydrationFreezesAllAuthoritativeCustomerInputs(t *testing.T) {
	now := time.Date(2026, time.September, 14, 10, 0, 0, 0, time.UTC)
	end := now.Add(time.Hour)
	cpoID, customerID, chargerID, connectorID, hubID, intentID := uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()
	requested := decimal.RequireFromString("30")
	inputs := invoiceIssuanceInputs{
		Session:     models.ChargingSession{ID: uuid.New(), CPOID: cpoID, CustomerID: customerID, ChargerID: chargerID, ConnectorID: connectorID, StartIntentID: &intentID, TransactionID: 73, StartTime: now, EndTime: &end, MeterStartWh: 1000, MeterStopWh: int64Pointer(3000), TotalKWh: decimal.RequireFromString("2"), TotalAmount: decimal.RequireFromString("40"), Currency: "INR", SettlementStatus: "SETTLED", TariffSnapshot: models.JSONB{"price_per_unit": "20.00", "units": "kWh"}, TaxSnapshot: models.JSONB{"cgst_rate": "9"}},
		Customer:    models.Customer{ID: customerID, CPOID: cpoID, FullName: "Customer Name", Email: "customer@example.test"},
		Connector:   models.Connector{ID: connectorID, CPOID: cpoID, ChargerID: chargerID, ConnectorNumber: 1, ConnectorType: "CCS2", ConnectorTotalCapacity: 47},
		Charger:     models.Charger{ID: chargerID, CPOID: cpoID, HubID: &hubID, ChargerID: "CH0001", ChargerName: "Station One", ChargerType: "DC"},
		Hub:         &models.Hub{ID: hubID, CPOID: cpoID, Name: "Authoritative Hub", Address: "1 Charging Road", State: constants.IndianState("West Bengal")},
		StartIntent: &models.ChargingStartIntent{ID: intentID, CPOID: cpoID, CustomerID: customerID, ChargerID: chargerID, ConnectorID: connectorID, LimitType: constants.ChargingLimitTypeTime, RequestedLimitValue: &requested},
		Payment:     &models.Payment{ID: uuid.New(), CPOID: cpoID, SessionID: uuid.Nil, PaymentMethod: "WALLET"},
		CPO:         models.CPO{ID: cpoID, BusinessName: "Issuer Charge Private Limited", CompanyType: constants.CPOCompanyTypeCompany, GSTIN: "27ABCDE1234F1Z5", Address: "10 Issuer Street", City: "Kolkata", State: constants.IndianState("West Bengal"), Pincode: "700001"},
	}
	inputs.Payment.SessionID = inputs.Session.ID
	if err := inputs.validate(); err != nil {
		t.Fatalf("valid issuance hydration rejected: %v", err)
	}
	snapshot := buildSnapshot(uuid.New(), inputs.snapshotSession(), inputs.CPO, inputs.Settings, end, "26-27", 1)
	if snapshot.Customer.FullName != "Customer Name" || snapshot.Location.HubName != "Authoritative Hub" || snapshot.Location.ChargerName != "Station One" || snapshot.Location.ConnectorType != "CCS2" {
		t.Fatalf("authoritative hydration was not frozen: %+v", snapshot)
	}
	if snapshot.Charging.LimitType == nil || *snapshot.Charging.LimitType != "TIME" || snapshot.Commercial.PaymentMethod == nil || *snapshot.Commercial.PaymentMethod != "WALLET" {
		t.Fatalf("start intent or payment was not frozen: %+v", snapshot)
	}
}

func TestInvoiceIssuanceHydrationRejectsRelationalMismatches(t *testing.T) {
	valid := validInvoiceIssuanceInputs()
	for name, mutate := range map[string]func(*invoiceIssuanceInputs){
		"missing charger":   func(inputs *invoiceIssuanceInputs) { inputs.Charger = models.Charger{} },
		"customer tenant":   func(inputs *invoiceIssuanceInputs) { inputs.Customer.CPOID = uuid.New() },
		"connector tenant":  func(inputs *invoiceIssuanceInputs) { inputs.Connector.CPOID = uuid.New() },
		"connector charger": func(inputs *invoiceIssuanceInputs) { inputs.Connector.ChargerID = uuid.New() },
		"charger tenant":    func(inputs *invoiceIssuanceInputs) { inputs.Charger.CPOID = uuid.New() },
		"hub tenant":        func(inputs *invoiceIssuanceInputs) { inputs.Hub.CPOID = uuid.New() },
		"start intent":      func(inputs *invoiceIssuanceInputs) { inputs.StartIntent.CPOID = uuid.New() },
		"payment":           func(inputs *invoiceIssuanceInputs) { inputs.Payment.CPOID = uuid.New() },
	} {
		t.Run(name, func(t *testing.T) {
			inputs := valid
			mutate(&inputs)
			if err := inputs.validate(); err == nil {
				t.Fatal("mismatched issuance inputs were accepted")
			}
		})
	}
}

func TestInvoiceIssuanceHydrationAllowsHublessChargerWithExplicitFallback(t *testing.T) {
	inputs := validInvoiceIssuanceInputs()
	inputs.Charger.HubID = nil
	inputs.Hub = nil
	if err := inputs.validate(); err != nil {
		t.Fatalf("hubless charger rejected: %v", err)
	}
	snapshot := buildSnapshot(uuid.New(), inputs.snapshotSession(), inputs.CPO, inputs.Settings, time.Now().UTC(), "26-27", 1)
	location := invoiceLocationLines(snapshot.Location)
	if snapshot.Location.ChargerName == "" || snapshot.Location.HubName != "" || len(location) != 1 || location[0] != "Location unavailable" {
		t.Fatalf("hubless charging location snapshot = %+v, rendered %q", snapshot.Location, location)
	}
}

func validInvoiceIssuanceInputs() invoiceIssuanceInputs {
	cpoID, customerID, chargerID, connectorID, hubID, intentID, sessionID := uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()
	return invoiceIssuanceInputs{
		Session:     models.ChargingSession{ID: sessionID, CPOID: cpoID, CustomerID: customerID, ChargerID: chargerID, ConnectorID: connectorID, StartIntentID: &intentID},
		Customer:    models.Customer{ID: customerID, CPOID: cpoID},
		Connector:   models.Connector{ID: connectorID, CPOID: cpoID, ChargerID: chargerID},
		Charger:     models.Charger{ID: chargerID, CPOID: cpoID, HubID: &hubID, ChargerID: "CH0001", ChargerName: "Station One"},
		Hub:         &models.Hub{ID: hubID, CPOID: cpoID},
		StartIntent: &models.ChargingStartIntent{ID: intentID, CPOID: cpoID, CustomerID: customerID, ChargerID: chargerID, ConnectorID: connectorID},
		Payment:     &models.Payment{ID: uuid.New(), CPOID: cpoID, SessionID: sessionID},
		CPO:         models.CPO{ID: cpoID},
	}
}

func TestCustomerInvoicePresentationOmitsBlankLabelsAndUsesHubFallback(t *testing.T) {
	snapshot := customerPresentationSnapshot()
	snapshot.Location = locationSnapshot{ChargerName: "Do not use as location"}
	snapshot.Customer.Email = ""
	snapshot.Customer.Phone = nil
	snapshot.Commercial.PaymentMethod = nil
	snapshot.Charging.OCPPStopReason = nil
	sections := customerInvoiceSections(snapshot, time.UTC)
	var location []string
	for _, section := range sections {
		if section.Title == "Charging location" {
			location = section.Lines
		}
		for _, line := range section.Lines {
			if strings.TrimSpace(line) == "" || strings.HasSuffix(strings.TrimSpace(line), ":") {
				t.Fatalf("blank customer-facing label in %q: %q", section.Title, line)
			}
			for _, forbidden := range []string{"Phone:", "Payment method:", "Charger stop reason:", "CMS/HAL", "OCPP"} {
				if strings.Contains(line, forbidden) {
					t.Fatalf("customer presentation contains %q in %q", forbidden, line)
				}
			}
		}
	}
	if len(location) != 1 || location[0] != "Location unavailable" {
		t.Fatalf("hubless location = %q, want explicit fallback", location)
	}
	if email := invoiceEmailText(snapshot, time.UTC); !strings.Contains(email, "Charging location: Location unavailable") || strings.Contains(email, "Do not use as location") {
		t.Fatalf("hubless email location = %s", email)
	}
}

func TestRenderPDFCustomerPresentationFitsOnePage(t *testing.T) {
	pdf, err := renderInvoice(customerPresentationSnapshot(), rendererVersion, models.InvoiceAsset{}, false, time.UTC)
	if err != nil {
		t.Fatalf("render one-page invoice: %v", err)
	}
	if pages := bytes.Count(pdf, []byte("/Type/Page/")); pages != 1 {
		t.Fatalf("one-page invoice rendered %d pages", pages)
	}
}

func TestRenderPDFRemoteStopDetailsFitOnFirstPage(t *testing.T) {
	snapshot := customerPresentationSnapshot()
	snapshot.InvoiceNumber = "INV/26-27/000075"
	phone := "+91 98765 43210"
	remote := "Remote"
	snapshot.Customer = customerSnapshot{FullName: "Anubhab Dey", Email: "anubhab@example.test", Phone: &phone}
	snapshot.Supplier.Address = "Unit 405, Horizon Plaza, 14 Riverside Avenue"
	snapshot.Location = locationSnapshot{HubName: "Riverside Mobility Hub", HubAddress: "42 River Road, Near Eastern Bypass", HubState: "West Bengal", ChargerCode: "CP0042", ChargerName: "Riverside DC", ChargerType: "Fast charger", ConnectorNumber: 2, ConnectorType: "CCS2", RatedPowerKW: 60}
	snapshot.Charging.OCPPStopReason = &remote
	pdf, err := renderInvoice(snapshot, rendererVersion, models.InvoiceAsset{}, false, time.UTC)
	if err != nil {
		t.Fatalf("render invoice with final remote stop: %v", err)
	}
	if pages := bytes.Count(pdf, []byte("/Type/Page/")); pages != 1 {
		t.Fatalf("fitting final stop detail rendered %d pages, want 1", pages)
	}
}

func TestInvoiceChargeHeaderUsesWhiteBoldMetricCenteredLabels(t *testing.T) {
	fonts, err := newInvoiceFonts()
	if err != nil {
		t.Fatalf("load invoice fonts: %v", err)
	}
	defer fonts.latin.Destroy()
	defer fonts.bengali.Destroy()
	defer fonts.devanagari.Destroy()

	for _, value := range []string{"Description", "Basis", "Amount"} {
		label := fonts.textBoxColor(value, 7.2, 34, true, color.White)
		face := label.MostCommonFontFace()
		if face == nil || !face.Fill.IsColor() || face.Fill.Color != canvas.White || face.Style != canvas.FontBold {
			t.Fatalf("header label %q face = %#v, want white bold", value, face)
		}
		top := 180.0
		labelTop := invoiceTextTopCentered(top, 8, label)
		ink := label.OutlineBounds().Translate(0, labelTop)
		if gapAbove, gapBelow := top-ink.Y1, ink.Y0-(top-8); gapAbove < 0 || gapBelow < 0 || gapAbove-gapBelow > 0.000001 || gapBelow-gapAbove > 0.000001 {
			t.Fatalf("header label %q visible gaps: above=%v below=%v", value, gapAbove, gapBelow)
		}
	}
}

func TestRenderPDFCustomerPresentationKeepsLongUnicodeContentOnOnePage(t *testing.T) {
	snapshot := customerPresentationSnapshot()
	snapshot.Supplier.Name = strings.Repeat("দীর্ঘ সরবরাহকারী नाम ", 18)
	snapshot.Customer.FullName = strings.Repeat("দীর্ঘ গ্রাহক नाम ", 18)
	snapshot.Location.HubName = strings.Repeat("দীর্ঘ হাব स्थान ", 18)
	snapshot.Location.HubAddress = strings.Repeat("42 দীর্ঘ রাস্তা मार्ग ", 30)
	snapshot.InvoiceNote = strings.Repeat("আপনার চার্জিং অভিজ্ঞতার জন্য धन्यवाद।\n", 90)
	pdf, err := renderInvoice(snapshot, rendererVersion, models.InvoiceAsset{}, false, time.UTC)
	if err != nil {
		t.Fatalf("render long single-page invoice: %v", err)
	}
	if pages := bytes.Count(pdf, []byte("/Type/Page/")); pages != 1 {
		t.Fatalf("long Unicode invoice rendered %d pages, want one page", pages)
	}
	if path := os.Getenv("INVOICE_OVERFLOW_SAMPLE_PDF"); path != "" {
		if err := os.WriteFile(path, pdf, 0600); err != nil {
			t.Fatalf("write overflow inspection sample: %v", err)
		}
	}
}

func TestInvoicePDFPresentationCommercialRowsAndLocationAreTruthful(t *testing.T) {
	snapshot := customerPresentationSnapshot()
	snapshot.Location.HubAddress = "42 River Road, West Bengal"
	if lines := invoiceLocationLines(snapshot.Location); len(lines) != 2 || lines[1] != "42 River Road, West Bengal" {
		t.Fatalf("location duplicated state: %q", lines)
	}
	rows := invoiceChargeRows(snapshot.Commercial)
	if len(rows) != 5 || rows[0].Amount != "INR 105.00" || rows[1].Description != "CGST" || rows[1].Amount != "INR 9.45" || rows[2].Description != "SGST" || rows[2].Amount != "INR 9.45" || !rows[4].Total || rows[4].Amount != "INR 123.90" {
		t.Fatalf("CGST/SGST charge presentation = %+v", rows)
	}
	snapshot.Commercial.Tax = invoiceTaxSnapshot{CGSTRate: stringPointer("0"), SGSTRate: stringPointer("0"), IGSTRate: stringPointer("18")}
	rows = invoiceChargeRows(snapshot.Commercial)
	if len(rows) != 5 || rows[3].Description != "IGST" || rows[3].Amount != "INR 18.90" {
		t.Fatalf("IGST charge presentation = %+v", rows)
	}
}

func TestInvoicePDFLogoBoundsStayWithinHeaderAndPage(t *testing.T) {
	bounds := invoiceLogoBounds()
	if bounds.W() > 40 || bounds.H() > 18 || bounds.X0 < 14 || bounds.Y0 < 0 || bounds.X1 > 196 || bounds.Y1 > 297 {
		t.Fatalf("logo bounds are not safely contained: %+v", bounds)
	}
}

func TestRenderInvoiceSampleWhenRequested(t *testing.T) {
	path := os.Getenv("INVOICE_SAMPLE_PDF")
	if path == "" {
		t.Skip("set INVOICE_SAMPLE_PDF to generate a local visual-inspection artifact")
	}
	pdf, err := renderInvoice(customerPresentationSnapshot(), rendererVersion, models.InvoiceAsset{}, false, time.UTC)
	if err != nil {
		t.Fatalf("render inspection sample: %v", err)
	}
	if err := os.WriteFile(path, pdf, 0600); err != nil {
		t.Fatalf("write inspection sample: %v", err)
	}
}

func customerPresentationSnapshot() issuanceSnapshot {
	now := time.Date(2026, time.September, 14, 10, 0, 0, 0, time.UTC)
	end := now.Add(80 * time.Minute)
	limit, requested, payment, initiator, reason, stop := "TIME", "45", "WALLET", "APP", "customer_requested", "Local"
	energy, duration := int64(5250), int64(2700)
	return issuanceSnapshot{
		SchemaVersion: snapshotVersion,
		InvoiceNumber: "INV/26-27/000001",
		IssuedAt:      end,
		Supplier:      supplierSnapshot{Name: "Example Charge", CompanyType: "Private Limited", GSTIN: "27ABCDE1234F1Z5", Address: "1 Supplier Street", City: "Kolkata", State: "West Bengal", Pincode: "700001"},
		Customer:      customerSnapshot{FullName: "Priya Das", Email: "priya@example.test"},
		Location:      locationSnapshot{HubName: "Riverside Hub", HubAddress: "42 River Road", HubState: "West Bengal", ChargerCode: "CP0042", ChargerName: "Riverside DC", ChargerType: "Fast charger", ConnectorNumber: 2, ConnectorType: "CCS2", RatedPowerKW: 60},
		Charging:      chargingSnapshot{SessionID: "00000000-0000-0000-0000-000000000001", OCPPTransactionID: 42, StartedAt: now, EndedAt: &end, DurationSeconds: 4800, MeterStartWh: 1000, MeterStopWh: int64Pointer(6250), LimitType: &limit, RequestedLimitValue: &requested, EnergyLimitWh: &energy, MaxDurationSeconds: &duration, RequestedStopInitiator: &initiator, RequestedStopReason: &reason, OCPPStopReason: &stop},
		Commercial:    commercialSnapshot{TotalKWh: "5.250", TotalAmount: "123.90", Currency: "INR", SettlementStatus: "SETTLED", PaymentMethod: &payment, Tariff: invoiceTariffSnapshot{BillingUnit: stringPointer("kwh"), PricePerUnit: stringPointer("20.00"), TariffType: stringPointer("fixed"), PriceType: stringPointer("energy")}, Tax: invoiceTaxSnapshot{CGSTRate: stringPointer("9"), SGSTRate: stringPointer("9"), IGSTRate: stringPointer("0")}},
		InvoiceNote:   "Thank you for charging with us.",
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

func TestSnapshotLogoCurrentAndLegacyStorageFreezeTheSameAsset(t *testing.T) {
	root := t.TempDir()
	t.Chdir(root)
	content := validInvoiceLogoPNG(t)
	if err := os.MkdirAll(filepath.Join("uploads", "invoice-logos"), 0750); err != nil {
		t.Fatalf("create current logo root: %v", err)
	}
	currentPath := filepath.Join("uploads", "invoice-logos", "current.png")
	if err := os.WriteFile(currentPath, content, 0600); err != nil {
		t.Fatalf("write current logo: %v", err)
	}
	currentAsset, currentOK, currentMigration, err := snapshotLogo(&currentPath)
	if err != nil || !currentOK || currentMigration != nil {
		t.Fatalf("current configured logo = (%v, %v, %v), want frozen unchanged asset", currentOK, currentMigration, err)
	}

	legacyPath := filepath.Join("uploads", uuid.NewString()+".legacy-upload")
	if err := os.WriteFile(legacyPath, content, 0600); err != nil {
		t.Fatalf("write legacy logo: %v", err)
	}
	legacyAsset, legacyOK, legacyMigration, err := snapshotLogo(&legacyPath)
	if err != nil || !legacyOK || legacyMigration == nil {
		t.Fatalf("legacy configured logo = (%v, %v, %v), want safely imported frozen asset", legacyOK, legacyMigration, err)
	}
	if currentAsset.hash != legacyAsset.hash || currentAsset.mimeType != legacyAsset.mimeType {
		t.Fatalf("legacy import changed frozen logo asset: current=%+v legacy=%+v", currentAsset, legacyAsset)
	}
	if _, info, err := controlledLogoFile(*legacyMigration); err != nil || info.Size() != int64(len(content)) {
		t.Fatalf("legacy logo did not import into controlled storage: info=%v err=%v", info, err)
	}
	snapshot := customerPresentationSnapshot()
	snapshot.LogoSHA256 = legacyAsset.hash
	pdf, err := renderInvoice(snapshot, rendererVersion, models.InvoiceAsset{MIMEType: legacyAsset.mimeType, SHA256: legacyAsset.hash, Content: legacyAsset.content}, true, time.UTC)
	withoutLogo, withoutErr := renderInvoice(customerPresentationSnapshot(), rendererVersion, models.InvoiceAsset{}, false, time.UTC)
	if err != nil || withoutErr != nil || len(pdf) <= len(withoutLogo) {
		t.Fatalf("frozen CPO logo was not rendered: err=%v", err)
	}
}

func TestSnapshotLogoRejectsInvalidConfiguredPathsAndPreservesFrozenAsset(t *testing.T) {
	root := t.TempDir()
	t.Chdir(root)
	if err := os.MkdirAll(filepath.Join("uploads", "invoice-logos"), 0750); err != nil {
		t.Fatalf("create current logo root: %v", err)
	}
	malformed := filepath.Join("uploads", "invoice-logos", "malformed.png")
	if err := os.WriteFile(malformed, []byte("not an image"), 0600); err != nil {
		t.Fatalf("write malformed logo: %v", err)
	}
	missing := filepath.Join("uploads", "invoice-logos", "missing.png")
	outside := filepath.Join(root, "outside.png")
	if err := os.WriteFile(outside, validInvoiceLogoPNG(t), 0600); err != nil {
		t.Fatalf("write outside logo: %v", err)
	}
	for _, path := range []string{malformed, missing, outside} {
		if _, ok, _, err := snapshotLogo(&path); err == nil || ok {
			t.Fatalf("invalid configured logo %q = (ok=%v, err=%v), want distinguishable failure", path, ok, err)
		}
	}
	if asset, ok, migrated, err := snapshotLogo(nil); err != nil || ok || migrated != nil || asset.hash != "" {
		t.Fatalf("no configured logo = (%+v, %v, %v, %v), want allowed absence", asset, ok, migrated, err)
	}
	first, ok := logoAssetFromContent(validInvoiceLogoPNG(t))
	if !ok {
		t.Fatal("create first frozen logo asset")
	}
	second, ok := logoAssetFromContent(validInvoiceLogoPNGWithSize(t, 5, 3))
	if !ok || second.hash == first.hash {
		t.Fatal("a later valid CPO logo did not produce a distinct asset")
	}
	snapshot := customerPresentationSnapshot()
	snapshot.LogoSHA256 = first.hash
	frozenPDF, frozenErr := renderInvoice(snapshot, rendererVersion, models.InvoiceAsset{MIMEType: first.mimeType, SHA256: first.hash, Content: first.content}, true, time.UTC)
	laterPDF, laterErr := renderInvoice(snapshot, rendererVersion, models.InvoiceAsset{MIMEType: second.mimeType, SHA256: second.hash, Content: second.content}, true, time.UTC)
	if frozenErr != nil || laterErr != nil || len(frozenPDF) <= len(laterPDF) {
		t.Fatal("later CPO logo could alter the already-frozen invoice asset")
	}

	target := filepath.Join("uploads", "invoice-logos", "link.png")
	if err := os.Symlink(outside, target); err != nil {
		t.Skipf("symlink creation unavailable in this environment: %v", err)
	}
	if _, ok, _, err := snapshotLogo(&target); err == nil || ok {
		t.Fatalf("symlinked configured logo = (ok=%v, err=%v), want rejection", ok, err)
	}
}

func TestInvoiceIssuerIdentityUsesCPOWithoutPlatformBranding(t *testing.T) {
	snapshot := customerPresentationSnapshot()
	snapshot.Supplier = supplierSnapshot{Name: "CPO Sunrise Mobility", CompanyType: "Company", GSTIN: "27ABCDE1234F1Z5", Address: "5 Issuer Lane", City: "Kolkata", State: "West Bengal", Pincode: "700001"}
	sections := customerInvoiceSections(snapshot, time.UTC)
	if len(sections) == 0 || sections[0].Title != "Supplier" || !strings.Contains(strings.Join(sections[0].Lines, "\n"), "CPO Sunrise Mobility") {
		t.Fatalf("PDF supplier section does not use CPO identity: %+v", sections)
	}
	email := invoiceEmailText(snapshot, time.UTC)
	subject := invoiceEmailSubject(snapshot)
	for _, value := range []string{"CPO Sunrise Mobility", "Riverside Hub", "42 River Road"} {
		if !strings.Contains(email, value) {
			t.Fatalf("CPO email omitted %q: %s", value, email)
		}
	}
	if !strings.Contains(subject, "CPO Sunrise Mobility") || invoiceIssuerDisplayName(snapshot.Supplier) != "CPO Sunrise Mobility" {
		t.Fatalf("invoice subject/sender do not identify the CPO: subject=%q sender=%q", subject, invoiceIssuerDisplayName(snapshot.Supplier))
	}
	for _, forbidden := range []string{"TransEV", "CMS", "platform", "Powered by"} {
		if strings.Contains(email, forbidden) || strings.Contains(subject, forbidden) {
			t.Fatalf("customer invoice identity leaked infrastructure wording %q", forbidden)
		}
	}
}

func validInvoiceLogoPNG(t *testing.T) []byte {
	return validInvoiceLogoPNGWithSize(t, 4, 3)
}

func validInvoiceLogoPNGWithSize(t *testing.T, width, height int) []byte {
	t.Helper()
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, image.NewRGBA(image.Rect(0, 0, width, height))); err != nil {
		t.Fatalf("encode invoice logo: %v", err)
	}
	return encoded.Bytes()
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
