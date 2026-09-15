package invoice

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/models"
	"github.com/tdewolff/canvas"
)

type invoicePageInspection struct {
	height float64
	texts  []string
	boxes  []canvas.Rect
	images []canvas.Rect
}

func (r *invoicePageInspection) Size() (float64, float64)                             { return 210, r.height }
func (r *invoicePageInspection) RenderPath(*canvas.Path, canvas.Style, canvas.Matrix) {}
func (r *invoicePageInspection) RenderText(text *canvas.Text, matrix canvas.Matrix) {
	r.texts = append(r.texts, text.String())
	r.boxes = append(r.boxes, text.Bounds().Transform(matrix))
}
func (r *invoicePageInspection) RenderImage(img image.Image, matrix canvas.Matrix) {
	r.images = append(r.images, canvas.Rect{X1: float64(img.Bounds().Dx()), Y1: float64(img.Bounds().Dy())}.Transform(matrix))
}

func TestInvoiceV3OnePageLayoutPreservesTextAndLogoPadding(t *testing.T) {
	fonts, err := newInvoiceFonts()
	if err != nil {
		t.Fatal(err)
	}
	defer fonts.latin.Destroy()
	defer fonts.bengali.Destroy()
	defer fonts.devanagari.Destroy()
	for _, scenario := range []string{"no-logo", "wide-logo", "tall-logo", "long-text", "rounding"} {
		t.Run(scenario, func(t *testing.T) {
			snapshot := customerPresentationSnapshot()
			var logo image.Image
			if scenario == "wide-logo" || scenario == "long-text" {
				logo = invoiceTestLogo(240, 72)
			} else if scenario == "tall-logo" {
				logo = invoiceTestLogo(72, 240)
			}
			if scenario == "long-text" {
				snapshot.Supplier.Name = strings.Repeat("বাংলা ग्राहक Charging Company ", 10)
				snapshot.Customer.FullName = strings.Repeat("গ্রাহক ग्राहक Customer ", 12)
				snapshot.Location.HubAddress = strings.Repeat("42 Riverside Road, West Bengal ", 20)
				snapshot.InvoiceNote = strings.Repeat("Keep every word of this supplier note, including the final line.\n", 90) + "END OF COMPLETE NOTE"
			}
			if scenario == "rounding" {
				snapshot.Commercial.TotalAmount = "0.07"
			}
			doc, height, err := composeInvoicePageV3(fonts, snapshot, logo, time.UTC)
			if err != nil {
				t.Fatal(err)
			}
			if scenario != "long-text" && height != 297 {
				t.Fatalf("ordinary invoice needs %v mm, want A4", height)
			}
			if scenario == "long-text" && height <= 297 {
				t.Fatal("long content did not extend the single page")
			}
			inspection := &invoicePageInspection{height: height}
			doc.RenderTo(inspection)
			allText := strings.Join(inspection.texts, "\n")
			for _, complete := range []string{snapshot.Supplier.Name, snapshot.Customer.FullName, snapshot.Location.HubAddress, snapshot.InvoiceNote, snapshot.InvoiceNumber, snapshot.Charging.SessionID, formatMoney(snapshot.Commercial.Currency, snapshot.Commercial.TotalAmount)} {
				if !strings.Contains(allText, strings.TrimSpace(complete)) {
					t.Fatalf("source text was shortened or lost: %q", complete)
				}
			}
			for index, box := range inspection.boxes {
				if box.X0 < 14.99 || box.X1 > 195.01 || box.Y0 < 5 || box.Y1 > height-14.99 {
					t.Fatalf("text outside page padding: %q %+v", inspection.texts[index], box)
				}
				for prior := 0; prior < index; prior++ {
					other := inspection.boxes[prior]
					if math.Min(box.X1, other.X1)-math.Max(box.X0, other.X0) > 0.1 && math.Min(box.Y1, other.Y1)-math.Max(box.Y0, other.Y0) > 0.1 {
						t.Fatalf("text overlaps: %q and %q", inspection.texts[index], inspection.texts[prior])
					}
				}
			}
			if logo != nil {
				if len(inspection.images) != 1 {
					t.Fatalf("logo count = %d", len(inspection.images))
				}
				box := inspection.images[0]
				padded := invoiceV3LogoBounds(height - 15)
				if box.X0 < padded.X0-0.01 || box.X1 > padded.X1+0.01 || box.Y0 < padded.Y0-0.01 || box.Y1 > padded.Y1+0.01 {
					t.Fatalf("logo escaped 4mm padding: %+v", box)
				}
				if math.Abs(box.W()/box.H()-float64(logo.Bounds().Dx())/float64(logo.Bounds().Dy())) > 0.0001 {
					t.Fatal("logo aspect ratio changed")
				}
			}
			if root := os.Getenv("INVOICE_V3_SAMPLES"); root != "" {
				var asset models.InvoiceAsset
				if logo != nil {
					var buffer bytes.Buffer
					if err := png.Encode(&buffer, logo); err != nil {
						t.Fatal(err)
					}
					asset = models.InvoiceAsset{SHA256: "test-logo", Content: buffer.Bytes()}
					snapshot.LogoSHA256 = asset.SHA256
				}
				pdf, err := renderInvoice(snapshot, rendererVersion, asset, logo != nil, time.UTC)
				if err != nil {
					t.Fatal(err)
				}
				if bytes.Count(pdf, []byte("/Type/Page/")) != 1 {
					t.Fatal("PDF output has more than one page")
				}
				if err := os.WriteFile(filepath.Join(root, scenario+".pdf"), pdf, 0600); err != nil {
					t.Fatal(err)
				}
			}
		})
	}
}

func invoiceTestLogo(width, height int) image.Image {
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			shade := invoiceTeal
			if x > width/3 && x < width*2/3 && y > height/4 && y < height*3/4 {
				shade = color.RGBA{R: 255, G: 255, B: 255, A: 255}
			}
			img.SetRGBA(x, y, shade)
		}
	}
	return img
}
