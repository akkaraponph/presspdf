// Example: Thai Tax Invoice / Receipt (ใบกำกับภาษี/ใบเสร็จรับเงิน)
//
// Demonstrates a realistic single-page A4 business document with:
//   - Bilingual (Thai/English) labels rendered with the bundled Sarabun font
//   - Built-in Thai word segmentation for proper line wrapping
//   - PNG → JPEG conversion at runtime so an existing PNG asset can be
//     embedded via folio.Document.RegisterImage (which only accepts JPEG)
//   - Filled header bar, two-column party block, line-items table with
//     zebra striping, totals block with VAT 7%, and signature lines
//
// Run from the repo root:
//
//	go run ./examples/invoice
package main

import (
	"bytes"
	"fmt"
	"image/jpeg"
	"image/png"
	"os"
	"strconv"
	"strings"

	"github.com/akkaraponph/folio"
	"github.com/akkaraponph/folio/fonts/sarabun"
	"github.com/akkaraponph/folio/thai"
)

// --- Page geometry (A4, mm) ---

const (
	pageW   = 210.0
	pageH   = 297.0
	lMargin = 15.0
	tMargin = 15.0
	rMargin = 15.0
)

// --- Brand palette ---

var (
	navy      = [3]int{17, 38, 77}
	white     = [3]int{255, 255, 255}
	bodyText  = [3]int{30, 30, 35}
	mutedText = [3]int{110, 110, 120}
	rule      = [3]int{200, 200, 210}
	zebra     = [3]int{245, 247, 252}
)

// --- Domain types ---

type item struct {
	desc      string
	qty       float64
	unitPrice float64
}

func (it item) amount() float64 { return it.qty * it.unitPrice }

// writer bundles the document, current page, and shared geometry so the
// drawing helpers stay terse.
type writer struct {
	doc       *folio.Document
	page      *folio.Page
	bodyWidth float64
}

func main() {
	doc := folio.New(folio.WithCompression(true))
	doc.SetTitle("ใบกำกับภาษี / Tax Invoice")
	doc.SetAuthor("Folio Studio")
	doc.SetCreator("folio examples/invoice")
	doc.SetMargins(lMargin, tMargin, rMargin)

	if err := sarabun.Register(doc); err != nil {
		fail("register sarabun: %v", err)
	}

	// Plug in the built-in Thai word segmenter so MultiCell wraps on word boundaries.
	thai.Setup(doc)

	// Register the company logo. RegisterImage only accepts JPEG, but the
	// in-repo asset is PNG, so convert it on the fly.
	logo, err := loadLogoAsJPEG("assets/logo-folio.png")
	if err != nil {
		fail("load logo: %v", err)
	}
	if err := doc.RegisterImage("logo", bytes.NewReader(logo)); err != nil {
		fail("register logo: %v", err)
	}

	w := &writer{
		doc:       doc,
		bodyWidth: pageW - lMargin - rMargin,
	}
	w.page = doc.AddPage(folio.A4)

	items := []item{
		{"ออกแบบโลโก้และอัตลักษณ์", 1, 4500.00},
		{"พัฒนาเว็บไซต์ Landing Page", 1, 6500.00},
		{"ดูแลโซเชียลมีเดีย 1 เดือน", 1, 2500.00},
		{"ถ่ายภาพสินค้า", 20, 80.00},
		{"ค่าจัดส่งและติดตั้ง", 1, 1200.00},
	}

	w.drawHeaderBar()
	w.drawTitleAndMeta("INV-2026-0042", "10 เม.ย. 2026", "24 เม.ย. 2026")
	w.drawPartyBlock()
	w.drawItemsTable(items)
	w.drawTotals(items)
	w.drawNotes()
	w.drawSignatures()

	out := "/tmp/folio_thai_invoice.pdf"
	if err := doc.Save(out); err != nil {
		fail("save: %v", err)
	}
	fmt.Printf("Thai invoice PDF saved to %s\n", out)
}

// --- Section drawers ---

func (w *writer) drawHeaderBar() {
	const barH = 22.0
	y := tMargin

	// Navy background
	w.doc.SetFillColor(navy[0], navy[1], navy[2])
	w.page.Rect(lMargin, y, w.bodyWidth, barH, "F")

	// Logo on the left of the bar
	w.page.DrawImageRect("logo", lMargin+3, y+2, 18, 18)

	// Company name + tagline on the right of the logo
	w.doc.SetTextColor(white[0], white[1], white[2])
	w.doc.SetFont("sarabun", "B", 16)
	w.page.SetXY(lMargin+25, y+4)
	w.page.Cell(w.bodyWidth-25, 8, "Folio Studio Co., Ltd.", "", "L", false, 1)

	w.doc.SetFont("sarabun", "", 10)
	w.page.SetX(lMargin + 25)
	w.page.Cell(w.bodyWidth-25, 6,
		"บริการออกแบบและพัฒนาดิจิทัล / Digital Design & Development",
		"", "L", false, 1)

	w.page.SetY(y + barH + 4)
}

func (w *writer) drawTitleAndMeta(invNo, date, due string) {
	// Centered bilingual title (two short lines).
	w.doc.SetFont("sarabun", "B", 14)
	w.doc.SetTextColor(navy[0], navy[1], navy[2])
	w.page.SetX(lMargin)
	w.page.Cell(w.bodyWidth, 7, "ใบกำกับภาษี / TAX INVOICE", "", "C", false, 1)
	w.page.SetX(lMargin)
	w.page.Cell(w.bodyWidth, 6, "ใบเสร็จรับเงิน / RECEIPT", "", "C", false, 1)

	// Thin rule below title
	w.page.SetY(w.page.GetY() + 1)
	w.doc.SetDrawColor(rule[0], rule[1], rule[2])
	w.doc.SetLineWidth(0.3)
	w.page.Line(lMargin, w.page.GetY(), lMargin+w.bodyWidth, w.page.GetY())
	w.page.SetY(w.page.GetY() + 3)

	// Right-aligned meta block (3 rows)
	const (
		labelW = 38.0
		valueW = 50.0
		metaW  = labelW + valueW
	)
	metaX := lMargin + w.bodyWidth - metaW
	rows := [][2]string{
		{"เลขที่ / No.", invNo},
		{"วันที่ / Date", date},
		{"ครบกำหนด / Due", due},
	}
	w.doc.SetTextColor(bodyText[0], bodyText[1], bodyText[2])
	for _, r := range rows {
		w.page.SetX(metaX)
		w.doc.SetFont("sarabun", "B", 10)
		w.page.Cell(labelW, 6, r[0]+":", "", "L", false, 0)
		w.doc.SetFont("sarabun", "", 10)
		w.page.Cell(valueW, 6, r[1], "", "L", false, 1)
	}
	w.page.SetY(w.page.GetY() + 3)
}

func (w *writer) drawPartyBlock() {
	startY := w.page.GetY()
	colW := (w.bodyWidth - 5) / 2 // 87.5mm columns, 5mm gutter
	const blockH = 38.0

	w.drawPartyColumn(lMargin, startY, colW, blockH,
		"ผู้ขาย / Seller",
		"Folio Studio Co., Ltd.",
		"123/45 ซอยสุขุมวิท 21\nแขวงคลองเตยเหนือ เขตวัฒนา\nกรุงเทพฯ 10110",
		"0-1055-12345-67-8",
		"+66 2 123 4567",
		"hello@folio.studio",
	)
	w.drawPartyColumn(lMargin+colW+5, startY, colW, blockH,
		"ผู้ซื้อ / Buyer",
		"บริษัท ตัวอย่าง จำกัด",
		"999 ถนนพระราม 9\nแขวงห้วยขวาง เขตห้วยขวาง\nกรุงเทพฯ 10310",
		"0-9988-77665-54-3",
		"+66 2 987 6543",
		"billing@example.co.th",
	)

	w.page.SetY(startY + blockH + 5)
}

func (w *writer) drawPartyColumn(x, y, cw, ch float64, header, name, addr, taxID, phone, email string) {
	// Outer box
	w.doc.SetDrawColor(rule[0], rule[1], rule[2])
	w.doc.SetLineWidth(0.3)
	w.page.Rect(x, y, cw, ch, "D")

	// Header band
	w.doc.SetFillColor(zebra[0], zebra[1], zebra[2])
	w.page.Rect(x, y, cw, 6, "F")
	w.page.Line(x, y+6, x+cw, y+6)

	w.doc.SetFont("sarabun", "B", 10)
	w.doc.SetTextColor(navy[0], navy[1], navy[2])
	w.page.SetXY(x+2, y)
	w.page.Cell(cw-4, 6, header, "", "L", false, 1)

	// Body
	w.doc.SetFont("sarabun", "B", 10)
	w.doc.SetTextColor(bodyText[0], bodyText[1], bodyText[2])
	w.page.SetXY(x+2, y+7)
	w.page.Cell(cw-4, 5, name, "", "L", false, 1)

	w.doc.SetFont("sarabun", "", 9)
	w.doc.SetTextColor(mutedText[0], mutedText[1], mutedText[2])
	w.page.SetX(x + 2)
	w.page.MultiCell(cw-4, 4.2, addr, "", "L", false)

	w.doc.SetTextColor(bodyText[0], bodyText[1], bodyText[2])
	for _, line := range []string{
		"เลขผู้เสียภาษี / Tax ID: " + taxID,
		"โทร / Tel: " + phone,
		"อีเมล / Email: " + email,
	} {
		w.page.SetX(x + 2)
		w.page.Cell(cw-4, 4.5, line, "", "L", false, 1)
	}
}

func (w *writer) drawItemsTable(items []item) {
	colW := []float64{12, 78, 18, 32, 40} // sums to 180 = bodyWidth
	headers := []string{
		"#",
		"รายการ / Description",
		"จำนวน / Qty",
		"ราคา/หน่วย / Unit",
		"จำนวนเงิน / Amount",
	}
	aligns := []string{"C", "L", "R", "R", "R"}

	// Header row — navy fill, white bold text
	const headerH = 9.0
	w.doc.SetFont("sarabun", "B", 10)
	w.doc.SetFillColor(navy[0], navy[1], navy[2])
	w.doc.SetTextColor(white[0], white[1], white[2])
	w.doc.SetDrawColor(navy[0], navy[1], navy[2])
	startY := w.page.GetY()
	w.page.SetXY(lMargin, startY)
	for i, h := range headers {
		w.page.Cell(colW[i], headerH, h, "1", aligns[i], true, 0)
	}
	w.page.SetXY(lMargin, startY+headerH)

	// Body rows — alternating zebra fill, body text
	const rowH = 9.0
	w.doc.SetFont("sarabun", "", 10)
	w.doc.SetTextColor(bodyText[0], bodyText[1], bodyText[2])
	w.doc.SetDrawColor(rule[0], rule[1], rule[2])

	for i, it := range items {
		rowY := w.page.GetY()
		if i%2 == 0 {
			w.doc.SetFillColor(zebra[0], zebra[1], zebra[2])
		} else {
			w.doc.SetFillColor(white[0], white[1], white[2])
		}

		w.page.SetX(lMargin)
		w.page.Cell(colW[0], rowH, strconv.Itoa(i+1), "1", "C", true, 0)
		w.page.Cell(colW[1], rowH, it.desc, "1", "L", true, 0)
		w.page.Cell(colW[2], rowH, formatQty(it.qty), "1", "R", true, 0)
		w.page.Cell(colW[3], rowH, formatTHB(it.unitPrice), "1", "R", true, 0)
		w.page.Cell(colW[4], rowH, formatTHB(it.amount()), "1", "R", true, 0)
		w.page.SetXY(lMargin, rowY+rowH)
	}

	w.page.SetY(w.page.GetY() + 4)
}

func (w *writer) drawTotals(items []item) {
	var subtotal float64
	for _, it := range items {
		subtotal += it.amount()
	}
	vat := subtotal * 0.07
	total := subtotal + vat

	const (
		blockW = 90.0
		labelW = 55.0
		valueW = blockW - labelW
		rowH   = 7.0
	)
	blockX := lMargin + w.bodyWidth - blockW

	rows := []struct {
		label string
		value float64
		grand bool
	}{
		{"รวม / Subtotal", subtotal, false},
		{"ภาษีมูลค่าเพิ่ม 7% / VAT 7%", vat, false},
		{"รวมทั้งสิ้น / Grand Total", total, true},
	}

	w.doc.SetDrawColor(rule[0], rule[1], rule[2])
	for _, r := range rows {
		if r.grand {
			w.doc.SetFillColor(navy[0], navy[1], navy[2])
			w.doc.SetTextColor(white[0], white[1], white[2])
			w.doc.SetFont("sarabun", "B", 11)
			w.doc.SetDrawColor(navy[0], navy[1], navy[2])
		} else {
			w.doc.SetFillColor(zebra[0], zebra[1], zebra[2])
			w.doc.SetTextColor(bodyText[0], bodyText[1], bodyText[2])
			w.doc.SetFont("sarabun", "", 10)
			w.doc.SetDrawColor(rule[0], rule[1], rule[2])
		}
		w.page.SetX(blockX)
		w.page.Cell(labelW, rowH, r.label, "1", "L", true, 0)
		w.page.Cell(valueW, rowH, formatTHB(r.value), "1", "R", true, 1)
	}

	w.doc.SetTextColor(bodyText[0], bodyText[1], bodyText[2])
	w.page.SetY(w.page.GetY() + 5)
}

func (w *writer) drawNotes() {
	w.doc.SetFont("sarabun", "B", 10)
	w.doc.SetTextColor(navy[0], navy[1], navy[2])
	w.page.SetX(lMargin)
	w.page.Cell(w.bodyWidth, 6, "หมายเหตุ / Notes", "", "L", false, 1)

	w.doc.SetFont("sarabun", "", 9)
	w.doc.SetTextColor(bodyText[0], bodyText[1], bodyText[2])
	w.page.SetX(lMargin)
	notes := "กรุณาชำระเงินภายใน 14 วันนับจากวันที่ในใบแจ้งหนี้ โดยโอนเข้าบัญชี " +
		"ธนาคารกสิกรไทย สาขาสีลม เลขที่บัญชี 123-4-56789-0 " +
		"ชื่อบัญชี บริษัท โฟลิโอ สตูดิโอ จำกัด " +
		"เมื่อชำระเงินแล้วโปรดส่งหลักฐานการโอนมาที่ billing@folio.studio / " +
		"Please remit payment within 14 days of the invoice date and email " +
		"the transfer slip to billing@folio.studio."
	w.page.MultiCell(w.bodyWidth, 4.8, notes, "", "J", false)
}

func (w *writer) drawSignatures() {
	// Anchor signatures near the bottom of the page so they always sit
	// at the same vertical position regardless of how tall the notes ran.
	const sigBlockY = pageH - 32
	if w.page.GetY() < sigBlockY {
		w.page.SetY(sigBlockY)
	}

	const sigW = 70.0
	leftX := lMargin + 10
	rightX := lMargin + w.bodyWidth - sigW - 10
	lineY := w.page.GetY() + 12

	w.doc.SetDrawColor(bodyText[0], bodyText[1], bodyText[2])
	w.doc.SetLineWidth(0.3)
	w.page.Line(leftX, lineY, leftX+sigW, lineY)
	w.page.Line(rightX, lineY, rightX+sigW, lineY)

	w.doc.SetFont("sarabun", "", 9)
	w.doc.SetTextColor(mutedText[0], mutedText[1], mutedText[2])

	w.page.SetXY(leftX, lineY+1)
	w.page.Cell(sigW, 5, "ผู้ส่งของ / Issued by", "", "C", false, 0)
	w.page.SetXY(rightX, lineY+1)
	w.page.Cell(sigW, 5, "ผู้รับของ / Received by", "", "C", false, 1)

	w.page.SetX(leftX)
	w.page.Cell(sigW, 5, "วันที่ / Date: ___________", "", "C", false, 0)
	w.page.SetX(rightX)
	w.page.Cell(sigW, 5, "วันที่ / Date: ___________", "", "C", false, 1)
}

// --- Helpers ---

// loadLogoAsJPEG decodes a PNG file from disk and re-encodes it as JPEG so
// it can be embedded via folio.Document.RegisterImage (which currently
// only accepts JPEG).
func loadLogoAsJPEG(path string) ([]byte, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	img, err := png.Decode(bytes.NewReader(raw))
	if err != nil {
		return nil, fmt.Errorf("decode png: %w", err)
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 90}); err != nil {
		return nil, fmt.Errorf("encode jpeg: %w", err)
	}
	return buf.Bytes(), nil
}

// formatTHB formats an amount as a Thai Baht string with thousands
// separators, e.g. 17441.00 → "฿17,441.00".
func formatTHB(v float64) string {
	neg := v < 0
	if neg {
		v = -v
	}
	s := strconv.FormatFloat(v, 'f', 2, 64)
	parts := strings.SplitN(s, ".", 2)
	intPart := parts[0]

	var b strings.Builder
	n := len(intPart)
	for i := range n {
		if i > 0 && (n-i)%3 == 0 {
			b.WriteByte(',')
		}
		b.WriteByte(intPart[i])
	}
	sign := ""
	if neg {
		sign = "-"
	}
	return sign + "฿" + b.String() + "." + parts[1]
}

// formatQty drops the decimal portion when the quantity is a whole number.
func formatQty(q float64) string {
	if q == float64(int64(q)) {
		return strconv.FormatInt(int64(q), 10)
	}
	return strconv.FormatFloat(q, 'f', 2, 64)
}

func fail(format string, a ...any) {
	fmt.Fprintf(os.Stderr, "error: "+format+"\n", a...)
	os.Exit(1)
}
