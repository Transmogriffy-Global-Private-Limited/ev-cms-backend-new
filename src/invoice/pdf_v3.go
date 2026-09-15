package invoice

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"strings"
	"time"

	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/models"
	"github.com/tdewolff/canvas"
	canvaspdf "github.com/tdewolff/canvas/renderers/pdf"
)

var (
	invoiceInk   = color.RGBA{R: 29, G: 47, B: 65, A: 255}
	invoiceMuted = color.RGBA{R: 91, G: 106, B: 120, A: 255}
	invoiceTeal  = color.RGBA{R: 0, G: 112, B: 114, A: 255}
	invoiceTint  = color.RGBA{R: 241, G: 247, B: 248, A: 255}
	invoiceRule  = color.RGBA{R: 221, G: 230, B: 233, A: 255}
)

// V3 measures and composes one continuous page. Normal invoices use A4; only
// unusually long source text increases the page height. No text is clipped,
// truncated, or scaled down to force unbounded content into a fixed sheet.
func renderPDFV3(snapshot issuanceSnapshot, asset models.InvoiceAsset, hasAsset bool, zone *time.Location) ([]byte, error) {
	fonts, err := newInvoiceFonts()
	if err != nil {
		return nil, err
	}
	defer fonts.latin.Destroy()
	defer fonts.bengali.Destroy()
	defer fonts.devanagari.Destroy()

	var logo image.Image
	if hasAsset && asset.SHA256 == snapshot.LogoSHA256 {
		logo, _, _ = image.Decode(bytes.NewReader(asset.Content))
	}
	document, height, err := composeInvoicePageV3(fonts, snapshot, logo, zone)
	if err != nil {
		return nil, err
	}
	var output bytes.Buffer
	renderer := canvaspdf.New(&output, 210, height, &canvaspdf.Options{Compress: true, SubsetFonts: true, ImageEncoding: canvas.Lossless})
	renderer.SetInfo("Charging Session Invoice "+snapshot.InvoiceNumber, "Charging session invoice", "charging,invoice", snapshot.Supplier.Name, rendererVersion)
	document.RenderTo(renderer)
	if err := renderer.Close(); err != nil {
		return nil, err
	}
	return output.Bytes(), nil
}

func composeInvoicePageV3(fonts invoiceFonts, snapshot issuanceSnapshot, logo image.Image, zone *time.Location) (*canvas.Canvas, float64, error) {
	document := canvas.New(210, 297)
	layout := invoicePageLayout{context: canvas.NewContext(document), fonts: fonts}
	y := layout.header(snapshot, logo, zone)
	y = layout.parties(snapshot, y-7)
	y = layout.summary(snapshot, zone, y-7)
	y = layout.charges(snapshot, y-8)
	y = layout.details(snapshot.Charging, snapshot.Commercial.Currency, y-8)
	if note := strings.TrimSpace(snapshot.InvoiceNote); note != "" {
		y = layout.note(note, y-7)
	}
	// The content uses negative Y from its measured top. Translate it once into
	// the PDF page, leaving 15 mm above and at least 23 mm below the content.
	height := maxFloat(297, -y+38)
	if height > 5080 {
		return nil, 0, fmt.Errorf("invoice content exceeds supported single-page dimensions")
	}
	document.Transform(canvas.Identity.Translate(0, height-15))
	footer := invoicePageLayout{context: canvas.NewContext(document), fonts: fonts}
	footer.rule(15, 17, 180)
	footer.text("Charging Session Invoice", 15, 12, 90, 7.5, false, invoiceMuted)
	footer.textRight(snapshot.InvoiceNumber+"  ·  1 / 1", 195, 12, 85, 7.5, false, invoiceMuted)

	return document, height, nil
}

type invoicePageLayout struct {
	context *canvas.Context
	fonts   invoiceFonts
}

// Anchor the actual line box at its top edge; Canvas text origins are not tops.
func (p invoicePageLayout) text(value string, x, top, width, size float64, bold bool, shade color.Color) float64 {
	text := p.fonts.textBoxColor(value, size, width, bold, shade)
	bounds := text.Bounds()
	p.context.DrawText(x-bounds.X0, top-bounds.Y1, text)
	return bounds.H()
}

func (p invoicePageLayout) textRight(value string, right, top, width, size float64, bold bool, shade color.Color) float64 {
	text := p.fonts.textBoxColor(value, size, width, bold, shade)
	bounds := text.Bounds()
	p.context.DrawText(right-bounds.X1, top-bounds.Y1, text)
	return bounds.H()
}

func (p invoicePageLayout) rule(x, y, width float64) {
	fillInvoiceRect(p.context, x, y, width, 0.25, invoiceRule)
}

func (p invoicePageLayout) heading(value string, top float64) float64 {
	return top - p.text(value, 15, top, 180, 10, true, invoiceInk) - 3
}

// 4 mm of padding on every side of a dedicated 30 x 26 mm logo tile.
func invoiceV3LogoBounds(top float64) canvas.Rect {
	return canvas.Rect{X0: 19, Y0: top - 22, X1: 41, Y1: top - 4}
}

func (p invoicePageLayout) header(snapshot issuanceSnapshot, logo image.Image, zone *time.Location) float64 {
	x, width := 15.0, 110.0
	if logo != nil {
		fillInvoiceRect(p.context, 15, -26, 30, 26, invoiceTint)
		p.context.FitImage(logo, invoiceV3LogoBounds(0), canvas.ImageContain)
		x, width = 50, 75
	}
	left := -p.text(snapshot.Supplier.Name, x, 0, width, 12, true, invoiceInk) - 2
	address := joinInvoiceParts("\n", snapshot.Supplier.Address, joinInvoiceParts(", ", snapshot.Supplier.City, snapshot.Supplier.State, snapshot.Supplier.Pincode))
	left -= p.text(address, x, left, width, 8, false, invoiceMuted) + 2
	left -= p.text(optionalTextLine("GSTIN", snapshot.Supplier.GSTIN), x, left, width, 8, false, invoiceInk)
	right := -p.textRight("INVOICE", 195, 0, 60, 22, true, invoiceInk) - 1.5
	right -= p.textRight("EV charging session", 195, right, 60, 8, false, invoiceMuted) + 4
	right -= p.textRight(snapshot.InvoiceNumber, 195, right, 60, 9, true, invoiceInk) + 1.5
	right -= p.textRight("Issued "+formatInvoiceTime(snapshot.IssuedAt, zone), 195, right, 60, 7.5, false, invoiceMuted) + 2
	right -= p.textRight(joinInvoiceParts(" · ", humanValueString(snapshot.Commercial.SettlementStatus), humanValue(snapshot.Commercial.PaymentMethod)), 195, right, 60, 8, true, invoiceTeal)
	bottom := -maxFloat(32, maxFloat(-left, -right)) - 5
	p.rule(15, bottom, 180)
	return bottom
}

func (p invoicePageLayout) parties(snapshot issuanceSnapshot, top float64) float64 {
	left := top - p.text("BILLED TO", 15, top, 85, 7.5, true, invoiceTeal) - 2
	left -= p.text(snapshot.Customer.FullName, 15, left, 85, 10, true, invoiceInk) + 1
	left -= p.text(joinInvoiceParts("\n", snapshot.Customer.Email, optionalLine("Phone", snapshot.Customer.Phone)), 15, left, 85, 8, false, invoiceMuted)
	right := top - p.text("CHARGING LOCATION", 110, top, 85, 7.5, true, invoiceTeal) - 2
	location := invoiceLocationLines(snapshot.Location)
	if len(location) > 0 {
		right -= p.text(location[0], 110, right, 85, 10, true, invoiceInk) + 1
		right -= p.text(strings.Join(location[1:], "\n"), 110, right, 85, 8, false, invoiceMuted)
	}
	return -maxFloat(-left, -right)
}

func (p invoicePageLayout) summary(snapshot issuanceSnapshot, zone *time.Location, top float64) float64 {
	values := []invoiceSummaryValue{
		{Label: "STARTED", Value: invoiceSummaryTime(snapshot.Charging.StartedAt, zone)},
		{Label: "ENDED", Value: "Not recorded"},
		{Label: "DURATION", Value: formatDuration(snapshot.Charging.DurationSeconds)},
		{Label: "ENERGY", Value: formatKWh(snapshot.Commercial.TotalKWh)},
	}
	if snapshot.Charging.EndedAt != nil {
		values[1].Value = invoiceSummaryTime(*snapshot.Charging.EndedAt, zone)
	}
	height := 0.0
	for _, value := range values {
		height = maxFloat(height, p.fonts.textBox(value.Value, 8.5, 37, true).Bounds().H()+13)
	}
	fillInvoiceRect(p.context, 15, top-height, 180, height, invoiceTint)
	for index, value := range values {
		x := 19 + float64(index)*45
		p.text(value.Label, x, top-4, 37, 7.5, true, invoiceMuted)
		p.text(value.Value, x, top-9, 37, 8.5, true, invoiceInk)
	}
	bottom := top - height - 3
	return bottom - p.text(joinInvoiceParts("   ·   ", invoiceChargerLines(snapshot.Location)...), 15, bottom, 180, 7.5, false, invoiceMuted)
}

func invoiceSummaryTime(value time.Time, zone *time.Location) string {
	if value.IsZero() {
		return "Not recorded"
	}
	return value.In(zone).Format("02 Jan 2006\n3:04 PM MST")
}

func (p invoicePageLayout) charges(snapshot issuanceSnapshot, top float64) float64 {
	top = p.heading("Session charges", top)
	fillInvoiceRect(p.context, 15, top-8, 180, 8, invoiceInk)
	for index, title := range []string{"Description", "Basis", "Amount"} {
		label := p.fonts.textBoxColor(title, 8, 40, true, color.White)
		x := []float64{19, 92, 191 - label.Bounds().W()}[index]
		p.context.DrawText(x, invoiceTextTopCentered(top, 8, label), label)
	}
	top -= 8
	for _, row := range invoiceSessionChargeRows(snapshot) {
		size := 8.5
		if row.Total {
			size = 12
		}
		descriptionHeight := p.fonts.textBox(row.Description, size, 69, row.Total).Bounds().H()
		basisHeight := p.fonts.textBox(row.Basis, 8, 57, false).Bounds().H()
		amountHeight := p.fonts.textBox(row.Amount, size, 38, row.Total).Bounds().H()
		height := maxFloat(9, maxFloat(descriptionHeight, maxFloat(basisHeight, amountHeight))+6)
		if row.Total {
			top -= 2
			height = maxFloat(height, 15)
			fillInvoiceRect(p.context, 15, top-height, 180, height, invoiceTint)
		}
		p.text(row.Description, 19, top-(height-descriptionHeight)/2, 69, size, row.Total, invoiceInk)
		p.text(row.Basis, 92, top-(height-basisHeight)/2, 57, 8, false, invoiceMuted)
		p.textRight(row.Amount, 191, top-(height-amountHeight)/2, 38, size, row.Total, invoiceInk)
		top -= height
		if !row.Total {
			p.rule(15, top, 180)
		}
	}
	return top
}

func (p invoicePageLayout) details(snapshot chargingSnapshot, currency string, top float64) float64 {
	top = p.heading("Session details", top)
	lines := invoiceSessionDetailLines(snapshot, currency)
	// Keep the complete UUID on its own full-width line, then pair the shorter
	// details in rows; measure both cells before advancing to the next row.
	if len(lines) > 0 && snapshot.SessionID != "" {
		top -= p.text(lines[0], 15, top, 180, 7.5, false, invoiceMuted) + 2
		lines = lines[1:]
	}
	for index := 0; index < len(lines); index += 2 {
		height := p.text(lines[index], 15, top, 85, 7.5, false, invoiceMuted)
		if index+1 < len(lines) {
			height = maxFloat(height, p.text(lines[index+1], 110, top, 85, 7.5, false, invoiceMuted))
		}
		top -= height + 2
	}
	return top
}

func (p invoicePageLayout) note(value string, top float64) float64 {
	p.rule(15, top, 180)
	top -= 4
	top -= p.text("NOTE FROM SUPPLIER", 15, top, 180, 7.5, true, invoiceTeal) + 2
	return top - p.text(value, 15, top, 180, 8, false, invoiceMuted)
}
