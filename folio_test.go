package folio

import (
	"bytes"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	crand "crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

var randReader = crand.Reader

func TestEmptyPage(t *testing.T) {
	doc := New()
	doc.AddPage(A4)

	var buf bytes.Buffer
	_, err := doc.WriteTo(&buf)
	if err != nil {
		t.Fatal(err)
	}
	s := buf.String()

	// Verify PDF structure
	if !strings.HasPrefix(s, "%PDF-1.4") {
		t.Error("missing PDF header")
	}
	if !strings.Contains(s, "/Type /Page") {
		t.Error("missing Page object")
	}
	if !strings.Contains(s, "/Type /Pages") {
		t.Error("missing Pages root")
	}
	if !strings.Contains(s, "/Type /Catalog") {
		t.Error("missing Catalog")
	}
	if !strings.Contains(s, "xref") {
		t.Error("missing xref")
	}
	if !strings.Contains(s, "%%EOF") {
		t.Error("missing EOF marker")
	}
}

func TestTextAt(t *testing.T) {
	doc := New(WithCompression(false))
	doc.SetFont("helvetica", "", 16)
	page := doc.AddPage(A4)
	page.TextAt(40, 60, "Hello Folio")

	var buf bytes.Buffer
	_, err := doc.WriteTo(&buf)
	if err != nil {
		t.Fatal(err)
	}
	s := buf.String()

	// Should contain text operators
	if !strings.Contains(s, "BT") {
		t.Error("missing BT operator")
	}
	if !strings.Contains(s, "Hello Folio") {
		t.Error("missing text content")
	}
	if !strings.Contains(s, "Tj") {
		t.Error("missing Tj operator")
	}
	if !strings.Contains(s, "ET") {
		t.Error("missing ET operator")
	}
}

func TestTargetAPI(t *testing.T) {
	doc := New(WithCompression(false))
	doc.SetTitle("Invoice")
	doc.SetAuthor("Akkarapon")
	doc.SetFont("helvetica", "", 16)

	page := doc.AddPage(A4)
	page.TextAt(40, 60, "Hello Folio")
	page.Line(40, 70, 200, 70)

	var buf bytes.Buffer
	_, err := doc.WriteTo(&buf)
	if err != nil {
		t.Fatal(err)
	}
	s := buf.String()

	if !strings.Contains(s, "/Title (Invoice)") {
		t.Error("missing title")
	}
	if !strings.Contains(s, "/Author (Akkarapon)") {
		t.Error("missing author")
	}
	if !strings.Contains(s, "Hello Folio") {
		t.Error("missing text")
	}
	// Line should have moveto/lineto/stroke
	if !strings.Contains(s, " m\n") {
		t.Error("missing line moveto")
	}
	if !strings.Contains(s, " l\n") {
		t.Error("missing line lineto")
	}
}

func TestMultiplePages(t *testing.T) {
	doc := New(WithCompression(false))
	doc.SetFont("courier", "", 12)

	p1 := doc.AddPage(A4)
	p1.TextAt(20, 20, "Page 1")

	p2 := doc.AddPage(Letter)
	p2.TextAt(20, 20, "Page 2")

	var buf bytes.Buffer
	_, err := doc.WriteTo(&buf)
	if err != nil {
		t.Fatal(err)
	}
	s := buf.String()

	if strings.Count(s, "/Type /Page\n") != 2 {
		t.Errorf("expected 2 Page objects, got %d", strings.Count(s, "/Type /Page\n"))
	}
	if !strings.Contains(s, "/Count 2") {
		t.Error("Pages count should be 2")
	}
	if !strings.Contains(s, "Page 1") || !strings.Contains(s, "Page 2") {
		t.Error("missing page text content")
	}
}

func TestRect(t *testing.T) {
	doc := New(WithCompression(false))
	page := doc.AddPage(A4)
	page.Rect(20, 50, 170, 100, "D")

	var buf bytes.Buffer
	_, err := doc.WriteTo(&buf)
	if err != nil {
		t.Fatal(err)
	}
	s := buf.String()

	if !strings.Contains(s, "re\nS") {
		t.Error("missing rect + stroke operators")
	}
}

func TestRectFill(t *testing.T) {
	doc := New(WithCompression(false))
	doc.SetFillColor(255, 0, 0)
	page := doc.AddPage(A4)
	page.Rect(20, 50, 170, 100, "F")

	var buf bytes.Buffer
	_, err := doc.WriteTo(&buf)
	if err != nil {
		t.Fatal(err)
	}
	s := buf.String()

	if !strings.Contains(s, "re\nf") {
		t.Error("missing rect + fill operators")
	}
}

func TestFontDedup(t *testing.T) {
	doc := New()
	doc.SetFont("helvetica", "", 12)
	doc.SetFont("helvetica", "", 16) // same font, different size
	doc.AddPage(A4)

	var buf bytes.Buffer
	_, err := doc.WriteTo(&buf)
	if err != nil {
		t.Fatal(err)
	}
	s := buf.String()

	// Should only have one font object
	if strings.Count(s, "/BaseFont /Helvetica") != 1 {
		t.Error("font should be deduplicated")
	}
}

func TestNoPages(t *testing.T) {
	doc := New()
	var buf bytes.Buffer
	_, err := doc.WriteTo(&buf)
	if err == nil {
		t.Error("expected error for no pages")
	}
}

func TestTextColor(t *testing.T) {
	doc := New(WithCompression(false))
	doc.SetFont("helvetica", "", 12)
	doc.SetTextColor(255, 0, 0)
	page := doc.AddPage(A4)
	page.TextAt(20, 20, "Red text")

	var buf bytes.Buffer
	_, err := doc.WriteTo(&buf)
	if err != nil {
		t.Fatal(err)
	}
	s := buf.String()

	if !strings.Contains(s, "1.000 0.000 0.000 rg") {
		t.Error("missing red fill color for text")
	}
}

func TestGetStringWidth(t *testing.T) {
	doc := New()
	doc.SetFont("helvetica", "", 12)
	page := doc.AddPage(A4)

	w := page.GetStringWidth("Hello")
	if w <= 0 {
		t.Fatalf("GetStringWidth(Hello) = %f, want > 0", w)
	}
	// Helvetica 12pt "Hello" should be roughly 9-15mm
	if w < 5 || w > 20 {
		t.Errorf("GetStringWidth(Hello) = %f, seems unreasonable", w)
	}
}

func TestEscapeParens(t *testing.T) {
	doc := New(WithCompression(false))
	doc.SetFont("helvetica", "", 12)
	page := doc.AddPage(A4)
	page.TextAt(20, 20, "test (parens) here")

	var buf bytes.Buffer
	_, err := doc.WriteTo(&buf)
	if err != nil {
		t.Fatal(err)
	}
	s := buf.String()

	if !strings.Contains(s, `test \(parens\) here`) {
		t.Error("parentheses not escaped in text")
	}
}

func TestCell(t *testing.T) {
	doc := New(WithCompression(false))
	doc.SetFont("helvetica", "", 12)
	page := doc.AddPage(A4)
	page.SetXY(10, 10)
	page.Cell(50, 10, "Cell 1", "1", "L", false, 0)
	page.Cell(50, 10, "Cell 2", "", "C", false, 1)

	var buf bytes.Buffer
	_, err := doc.WriteTo(&buf)
	if err != nil {
		t.Fatal(err)
	}
	s := buf.String()
	if !strings.Contains(s, "Cell 1") {
		t.Error("missing Cell 1 text")
	}
	if !strings.Contains(s, "Cell 2") {
		t.Error("missing Cell 2 text")
	}
}

func TestMultiCell(t *testing.T) {
	doc := New(WithCompression(false))
	doc.SetFont("helvetica", "", 12)
	page := doc.AddPage(A4)
	page.SetXY(10, 10)
	page.MultiCell(80, 6, "This is a long text that should be wrapped into multiple lines when it exceeds the width.", "", "L", false)

	var buf bytes.Buffer
	_, err := doc.WriteTo(&buf)
	if err != nil {
		t.Fatal(err)
	}
	// Should have produced output with text operators
	s := buf.String()
	if !strings.Contains(s, "Tj") {
		t.Error("missing text output from MultiCell")
	}
}

func TestMultiCellNewlines(t *testing.T) {
	doc := New(WithCompression(false))
	doc.SetFont("helvetica", "", 12)
	page := doc.AddPage(A4)
	page.SetXY(10, 10)
	page.MultiCell(0, 6, "Line 1\nLine 2\nLine 3", "", "L", false)

	var buf bytes.Buffer
	_, err := doc.WriteTo(&buf)
	if err != nil {
		t.Fatal(err)
	}
	s := buf.String()
	if !strings.Contains(s, "Line 1") || !strings.Contains(s, "Line 2") || !strings.Contains(s, "Line 3") {
		t.Error("missing newline-separated text")
	}
}

func testJPEG(w, h int) []byte {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.RGBA{R: uint8(x % 256), G: uint8(y % 256), B: 128, A: 255})
		}
	}
	var buf bytes.Buffer
	jpeg.Encode(&buf, img, &jpeg.Options{Quality: 50})
	return buf.Bytes()
}

func TestJPEGImage(t *testing.T) {
	doc := New(WithCompression(false))
	doc.SetFont("helvetica", "", 12)

	jpgData := testJPEG(100, 80)
	err := doc.RegisterImage("photo", bytes.NewReader(jpgData))
	if err != nil {
		t.Fatal(err)
	}

	page := doc.AddPage(A4)
	page.TextAt(20, 20, "Image below:")
	page.DrawImageRect("photo", 20, 30, 60, 48)

	var buf bytes.Buffer
	_, err = doc.WriteTo(&buf)
	if err != nil {
		t.Fatal(err)
	}
	s := buf.String()

	if !strings.Contains(s, "/Type /XObject") {
		t.Error("missing XObject")
	}
	if !strings.Contains(s, "/Subtype /Image") {
		t.Error("missing /Subtype /Image")
	}
	if !strings.Contains(s, "/Filter /DCTDecode") {
		t.Error("missing DCTDecode filter")
	}
	if !strings.Contains(s, "/Im1 Do") {
		t.Error("missing image draw operator")
	}
}

func TestImageDedup(t *testing.T) {
	doc := New(WithCompression(false))

	jpgData := testJPEG(50, 50)
	doc.RegisterImage("img1", bytes.NewReader(jpgData))
	doc.RegisterImage("img2", bytes.NewReader(jpgData)) // same data

	page := doc.AddPage(A4)
	page.DrawImageRect("img1", 10, 10, 30, 30)
	page.DrawImageRect("img2", 50, 10, 30, 30)

	var buf bytes.Buffer
	_, err := doc.WriteTo(&buf)
	if err != nil {
		t.Fatal(err)
	}
	s := buf.String()

	// Should only have one image XObject
	count := strings.Count(s, "/Type /XObject")
	if count != 1 {
		t.Errorf("expected 1 image XObject, got %d", count)
	}
}

func TestImageNotRegistered(t *testing.T) {
	doc := New()
	page := doc.AddPage(A4)
	page.DrawImageRect("nonexistent", 10, 10, 30, 30)

	_, err := doc.WriteTo(&bytes.Buffer{})
	if err == nil {
		t.Error("expected error for unregistered image")
	}
}

func TestBytes(t *testing.T) {
	doc := New()
	doc.AddPage(A4)

	b, err := doc.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	if len(b) == 0 {
		t.Error("Bytes() returned empty")
	}
	if !strings.HasPrefix(string(b), "%PDF") {
		t.Error("Bytes() not a valid PDF")
	}
}

func loadTTFFont(t *testing.T) []byte {
	t.Helper()
	paths := []string{
		"/System/Library/Fonts/Supplemental/Georgia.ttf",
		"/System/Library/Fonts/Supplemental/Trebuchet MS Italic.ttf",
		"/usr/share/fonts/truetype/dejavu/DejaVuSans.ttf",
	}
	for _, p := range paths {
		data, err := os.ReadFile(p)
		if err == nil {
			return data
		}
	}
	t.Skip("no suitable TTF font found on this system")
	return nil
}

func TestUTF8Font(t *testing.T) {
	data := loadTTFFont(t)

	doc := New(WithCompression(false))
	err := doc.AddUTF8Font("georgia", "", data)
	if err != nil {
		t.Fatalf("AddUTF8Font: %v", err)
	}

	doc.SetFont("georgia", "", 14)
	page := doc.AddPage(A4)
	page.TextAt(20, 30, "Hello UTF-8 World")

	var buf bytes.Buffer
	_, err = doc.WriteTo(&buf)
	if err != nil {
		t.Fatalf("WriteTo: %v", err)
	}
	s := buf.String()

	// Verify CIDFont Type2 structure
	if !strings.Contains(s, "/Subtype /Type0") {
		t.Error("missing Type0 font")
	}
	if !strings.Contains(s, "/Subtype /CIDFontType2") {
		t.Error("missing CIDFontType2")
	}
	if !strings.Contains(s, "/Encoding /Identity-H") {
		t.Error("missing Identity-H encoding")
	}
	if !strings.Contains(s, "/Type /FontDescriptor") {
		t.Error("missing FontDescriptor")
	}
	if !strings.Contains(s, "/FontFile2") {
		t.Error("missing FontFile2")
	}
	if !strings.Contains(s, "/CIDToGIDMap") {
		t.Error("missing CIDToGIDMap")
	}
	if !strings.Contains(s, "CIDInit") {
		t.Error("missing ToUnicode CMap")
	}

	// Text should be hex-encoded, not ASCII
	if !strings.Contains(s, "Tj") {
		t.Error("missing Tj operator")
	}
	// Hex string should use angle brackets
	if !strings.Contains(s, "<") {
		t.Error("missing hex string in text output")
	}

	// Verify PDF structure
	if !strings.HasPrefix(s, "%PDF-1.4") {
		t.Error("missing PDF header")
	}
	if !strings.Contains(s, "%%EOF") {
		t.Error("missing EOF marker")
	}
}

func TestUTF8FontWidth(t *testing.T) {
	data := loadTTFFont(t)

	doc := New(WithCompression(false))
	doc.AddUTF8Font("georgia", "", data)
	doc.SetFont("georgia", "", 12)
	page := doc.AddPage(A4)

	w := page.GetStringWidth("Hello")
	if w <= 0 {
		t.Fatalf("GetStringWidth(Hello) = %f, want > 0", w)
	}
	if w < 5 || w > 20 {
		t.Errorf("GetStringWidth(Hello) = %f, seems unreasonable", w)
	}
}

func TestUTF8MultiCell(t *testing.T) {
	data := loadTTFFont(t)

	doc := New(WithCompression(false))
	doc.AddUTF8Font("georgia", "", data)
	doc.SetFont("georgia", "", 10)
	page := doc.AddPage(A4)
	page.SetXY(10, 10)
	page.MultiCell(80, 6, "This is a long UTF-8 text that should wrap into multiple lines when it exceeds the width.", "", "L", false)

	var buf bytes.Buffer
	_, err := doc.WriteTo(&buf)
	if err != nil {
		t.Fatalf("WriteTo: %v", err)
	}
	s := buf.String()
	if !strings.Contains(s, "Tj") {
		t.Error("missing text output from UTF-8 MultiCell")
	}
}

func TestMixedFonts(t *testing.T) {
	data := loadTTFFont(t)

	doc := New(WithCompression(false))
	doc.AddUTF8Font("georgia", "", data)

	doc.SetFont("helvetica", "", 12)
	page := doc.AddPage(A4)
	page.TextAt(20, 20, "Core font text")

	doc.SetFont("georgia", "", 12)
	page.TextAt(20, 40, "UTF-8 font text")

	var buf bytes.Buffer
	_, err := doc.WriteTo(&buf)
	if err != nil {
		t.Fatalf("WriteTo: %v", err)
	}
	s := buf.String()

	// Should have both core and TTF font objects
	if !strings.Contains(s, "/Subtype /Type1") {
		t.Error("missing Type1 font (core)")
	}
	if !strings.Contains(s, "/Subtype /Type0") {
		t.Error("missing Type0 font (TTF)")
	}
	// Core font text uses parentheses, TTF uses hex
	if !strings.Contains(s, "(Core font text)") {
		t.Error("missing core font text with parenthesis encoding")
	}
}

func TestSetFontSize(t *testing.T) {
	doc := New(WithCompression(false))
	doc.SetFont("helvetica", "", 12)

	p1 := doc.AddPage(A4)
	w1 := p1.GetStringWidth("Hello")

	doc.SetFontSize(24)
	p2 := doc.AddPage(A4)
	w2 := p2.GetStringWidth("Hello")

	if w2 <= w1 {
		t.Errorf("expected larger width at 24pt (%f) than 12pt (%f)", w2, w1)
	}
	if doc.GetFontSize() != 24 {
		t.Errorf("GetFontSize() = %f, want 24", doc.GetFontSize())
	}
}

func TestSetFontStyle(t *testing.T) {
	doc := New(WithCompression(false))
	doc.SetFont("helvetica", "", 12)
	page := doc.AddPage(A4)
	page.TextAt(20, 20, "Normal")

	doc.SetFontStyle("B")
	page.TextAt(20, 40, "Bold")

	if doc.GetFontStyle() != "B" {
		t.Errorf("GetFontStyle() = %q, want %q", doc.GetFontStyle(), "B")
	}
	if doc.GetFontFamily() != "helvetica" {
		t.Errorf("GetFontFamily() = %q, want %q", doc.GetFontFamily(), "helvetica")
	}

	var buf bytes.Buffer
	_, err := doc.WriteTo(&buf)
	if err != nil {
		t.Fatal(err)
	}
	s := buf.String()
	if !strings.Contains(s, "(Normal)") {
		t.Error("missing Normal text")
	}
	if !strings.Contains(s, "(Bold)") {
		t.Error("missing Bold text")
	}
	// Should have both Helvetica and Helvetica-Bold
	if !strings.Contains(s, "/BaseFont /Helvetica\n") {
		t.Error("missing Helvetica font")
	}
	if !strings.Contains(s, "/BaseFont /Helvetica-Bold") {
		t.Error("missing Helvetica-Bold font")
	}
}

func TestPageSetFontSize(t *testing.T) {
	doc := New(WithCompression(false))
	doc.SetFont("helvetica", "", 12)
	page := doc.AddPage(A4)

	page.SetFontSize(20)
	if page.GetFontSize() != 20 {
		t.Errorf("page.GetFontSize() = %f, want 20", page.GetFontSize())
	}
	// Document font size should be unchanged
	if doc.GetFontSize() != 12 {
		t.Errorf("doc.GetFontSize() = %f, want 12 (unchanged)", doc.GetFontSize())
	}
}

func TestPageSetFontStyle(t *testing.T) {
	doc := New(WithCompression(false))
	doc.SetFont("helvetica", "", 12)
	page := doc.AddPage(A4)

	page.TextAt(20, 20, "Regular")
	page.SetFontStyle("I")
	page.TextAt(20, 40, "Italic")

	if page.GetFontStyle() != "I" {
		t.Errorf("page.GetFontStyle() = %q, want %q", page.GetFontStyle(), "I")
	}
	if page.GetFontFamily() != "helvetica" {
		t.Errorf("page.GetFontFamily() = %q, want %q", page.GetFontFamily(), "helvetica")
	}

	var buf bytes.Buffer
	_, err := doc.WriteTo(&buf)
	if err != nil {
		t.Fatal(err)
	}
	s := buf.String()
	if !strings.Contains(s, "(Regular)") {
		t.Error("missing Regular text")
	}
	if !strings.Contains(s, "(Italic)") {
		t.Error("missing Italic text")
	}
}

func TestSetFontSizeNoFont(t *testing.T) {
	doc := New()
	doc.SetFontSize(12)
	if doc.Err() == nil {
		t.Error("expected error when setting size with no font")
	}
}

func TestSetFontStyleNoFont(t *testing.T) {
	doc := New()
	doc.SetFontStyle("B")
	if doc.Err() == nil {
		t.Error("expected error when setting style with no font")
	}
}

func TestAddUTF8FontFromFile(t *testing.T) {
	paths := []string{
		"/System/Library/Fonts/Supplemental/Georgia.ttf",
		"/usr/share/fonts/truetype/dejavu/DejaVuSans.ttf",
	}
	var fontPath string
	for _, p := range paths {
		if _, err := os.Stat(p); err == nil {
			fontPath = p
			break
		}
	}
	if fontPath == "" {
		t.Skip("no suitable TTF font found on this system")
	}

	doc := New(WithCompression(false))
	err := doc.AddUTF8FontFromFile("testfont", "", fontPath)
	if err != nil {
		t.Fatalf("AddUTF8FontFromFile: %v", err)
	}
	doc.SetFont("testfont", "", 14)
	page := doc.AddPage(A4)
	page.TextAt(20, 30, "From file")

	var buf bytes.Buffer
	_, err = doc.WriteTo(&buf)
	if err != nil {
		t.Fatalf("WriteTo: %v", err)
	}
	if !strings.Contains(buf.String(), "/Subtype /Type0") {
		t.Error("missing Type0 font")
	}
}

// --- Auto page break tests ---

func TestAutoPageBreak(t *testing.T) {
	doc := New(WithCompression(false))
	doc.SetFont("helvetica", "", 12)
	doc.SetAutoPageBreak(true, 15)
	page := doc.AddPage(A4)

	// A4 = 297mm height. Top margin 10mm, bottom margin 15mm.
	// Available: 272mm. Cell height: 10mm. 27 cells fit on page 1.
	// Cell 28 triggers a break → page 2.
	for i := 0; i < 30; i++ {
		page.Cell(0, 10, fmt.Sprintf("Line %d", i+1), "", "L", false, 1)
	}

	if n := doc.PageCount(); n != 2 {
		t.Errorf("expected 2 pages, got %d", n)
	}

	var buf bytes.Buffer
	_, err := doc.WriteTo(&buf)
	if err != nil {
		t.Fatal(err)
	}
	s := buf.String()
	if !strings.Contains(s, "/Count 2") {
		t.Error("PDF should have 2 pages")
	}
}

func TestAutoPageBreakMultiplePages(t *testing.T) {
	doc := New(WithCompression(false))
	doc.SetFont("helvetica", "", 12)
	doc.SetAutoPageBreak(true, 15)
	page := doc.AddPage(A4)

	// 100 cells × 10mm. ~27 cells/page → 4 pages.
	for i := 0; i < 100; i++ {
		page.Cell(0, 10, "X", "", "L", false, 1)
	}

	if n := doc.PageCount(); n != 4 {
		t.Errorf("expected 4 pages, got %d", n)
	}

	var buf bytes.Buffer
	_, err := doc.WriteTo(&buf)
	if err != nil {
		t.Fatal(err)
	}
}

func TestAutoPageBreakForwarding(t *testing.T) {
	doc := New()
	doc.SetFont("helvetica", "", 12)
	doc.SetAutoPageBreak(true, 15)
	page := doc.AddPage(A4)

	// Fill page 1 to trigger a break.
	for i := 0; i < 30; i++ {
		page.Cell(0, 10, "X", "", "L", false, 1)
	}

	// The original page variable should forward to the active page.
	// After 30 cells (27 on page 1, 3 on page 2), cursor should be
	// near the top of page 2.
	y := page.GetY()
	if y < 20 || y > 50 {
		t.Errorf("expected cursor near top of page 2, got y=%f", y)
	}

	// CurrentPage should be page 2.
	if doc.CurrentPage() == nil {
		t.Fatal("CurrentPage should not be nil")
	}
	if doc.PageCount() != 2 {
		t.Errorf("expected 2 pages, got %d", doc.PageCount())
	}
}

func TestAutoPageBreakDisabled(t *testing.T) {
	doc := New(WithCompression(false))
	doc.SetFont("helvetica", "", 12)
	// Auto page break NOT enabled (default).
	page := doc.AddPage(A4)

	for i := 0; i < 50; i++ {
		page.Cell(0, 10, "X", "", "L", false, 1)
	}

	// All content on one page — no automatic breaks.
	if n := doc.PageCount(); n != 1 {
		t.Errorf("expected 1 page (no auto break), got %d", n)
	}
}

func TestAutoPageBreakMultiCell(t *testing.T) {
	doc := New(WithCompression(false))
	doc.SetFont("helvetica", "", 12)
	doc.SetAutoPageBreak(true, 15)
	page := doc.AddPage(A4)

	// Write a very long text via MultiCell. Each line is ~6mm tall.
	// With 272mm available, ~45 lines fit per page.
	long := strings.Repeat("This is a line of text that will be wrapped. ", 120)
	page.MultiCell(0, 6, long, "", "L", false)

	if n := doc.PageCount(); n < 2 {
		t.Errorf("expected at least 2 pages from long MultiCell, got %d", n)
	}

	var buf bytes.Buffer
	_, err := doc.WriteTo(&buf)
	if err != nil {
		t.Fatal(err)
	}
}

func TestPageBreakTrigger(t *testing.T) {
	doc := New()
	doc.SetAutoPageBreak(true, 20)
	page := doc.AddPage(A4)

	// A4 height in mm ≈ 297. Trigger = 297 - 20 = 277.
	trigger := page.PageBreakTrigger()
	if trigger < 276 || trigger > 278 {
		t.Errorf("expected trigger ~277, got %f", trigger)
	}
}

func TestCurrentPageAndPageCount(t *testing.T) {
	doc := New()
	if doc.PageCount() != 0 {
		t.Errorf("expected 0 pages initially, got %d", doc.PageCount())
	}

	p1 := doc.AddPage(A4)
	if doc.PageCount() != 1 {
		t.Errorf("expected 1 page, got %d", doc.PageCount())
	}
	if doc.CurrentPage() != p1 {
		t.Error("CurrentPage should be p1")
	}

	p2 := doc.AddPage(A4)
	if doc.PageCount() != 2 {
		t.Errorf("expected 2 pages, got %d", doc.PageCount())
	}
	if doc.CurrentPage() != p2 {
		t.Error("CurrentPage should be p2")
	}
}

// --- Header/footer tests ---

func TestHeaderFooter(t *testing.T) {
	doc := New(WithCompression(false))
	doc.SetFont("helvetica", "", 12)

	headerCalls := 0
	footerCalls := 0

	doc.SetHeaderFunc(func(p *Page) {
		headerCalls++
		doc.SetFont("helvetica", "I", 8)
		p.Cell(0, 5, "Document Header", "", "C", false, 1)
	})
	doc.SetFooterFunc(func(p *Page) {
		footerCalls++
		p.SetY(-15)
		doc.SetFont("helvetica", "I", 8)
		p.Cell(0, 10, fmt.Sprintf("Page %d", doc.PageNo()), "", "C", false, 0)
	})

	doc.AddPage(A4)
	doc.AddPage(A4)

	var buf bytes.Buffer
	_, err := doc.WriteTo(&buf)
	if err != nil {
		t.Fatal(err)
	}

	if headerCalls != 2 {
		t.Errorf("expected 2 header calls, got %d", headerCalls)
	}
	// Footer: page 1 footer called during AddPage(page 2), page 2 footer
	// called during WriteTo → closeDoc.
	if footerCalls != 2 {
		t.Errorf("expected 2 footer calls, got %d", footerCalls)
	}

	s := buf.String()
	if !strings.Contains(s, "Document Header") {
		t.Error("missing header text in PDF")
	}
}

func TestHeaderFooterPageNo(t *testing.T) {
	doc := New(WithCompression(false))
	doc.SetFont("helvetica", "", 12)

	pageNos := []int{}
	doc.SetFooterFunc(func(p *Page) {
		pageNos = append(pageNos, doc.PageNo())
	})

	doc.AddPage(A4)
	doc.AddPage(A4)
	doc.AddPage(A4)

	doc.WriteTo(&bytes.Buffer{})

	// Footer for page 1 is called during AddPage(2) → PageNo()=1 at that point?
	// Actually: footer on page 1 is called before page 2 is added, so
	// len(pages) = 1 → PageNo() = 1.
	// Footer on page 2 is called before page 3 is added → PageNo() = 2.
	// Footer on page 3 is called during closeDoc → PageNo() = 3.
	if len(pageNos) != 3 {
		t.Fatalf("expected 3 footer calls, got %d", len(pageNos))
	}
	for i, pn := range pageNos {
		if pn != i+1 {
			t.Errorf("footer call %d: expected PageNo=%d, got %d", i, i+1, pn)
		}
	}
}

func TestHeaderFooterWithAutoBreak(t *testing.T) {
	doc := New(WithCompression(false))
	doc.SetFont("helvetica", "", 12)
	doc.SetAutoPageBreak(true, 20)

	headerCalls := 0
	footerCalls := 0

	doc.SetHeaderFunc(func(p *Page) {
		headerCalls++
		doc.SetFont("helvetica", "B", 10)
		p.Cell(0, 8, "HEADER", "", "C", false, 1)
	})
	doc.SetFooterFunc(func(p *Page) {
		footerCalls++
		p.SetY(-20)
		doc.SetFont("helvetica", "I", 8)
		p.Cell(0, 10, fmt.Sprintf("- %d -", doc.PageNo()), "", "C", false, 0)
	})

	page := doc.AddPage(A4)
	// Write enough cells to trigger automatic page breaks.
	for i := 0; i < 60; i++ {
		page.Cell(0, 10, fmt.Sprintf("Row %d", i+1), "", "L", false, 1)
	}

	var buf bytes.Buffer
	_, err := doc.WriteTo(&buf)
	if err != nil {
		t.Fatal(err)
	}

	// With header (8mm) + top margin (10mm) + bottom margin (20mm),
	// available per page ≈ 297 - 10 - 20 = 267, minus header 8 = 259mm.
	// 259/10 ≈ 25 cells per page. 60 cells → 3 pages.
	if doc.PageCount() < 2 {
		t.Errorf("expected at least 2 pages, got %d", doc.PageCount())
	}
	if headerCalls != doc.PageCount() {
		t.Errorf("expected %d header calls, got %d", doc.PageCount(), headerCalls)
	}
	if footerCalls != doc.PageCount() {
		t.Errorf("expected %d footer calls, got %d", doc.PageCount(), footerCalls)
	}
}

func TestHeaderFooterStateRestore(t *testing.T) {
	doc := New(WithCompression(false))
	doc.SetFont("helvetica", "", 14)
	doc.SetTextColor(0, 0, 0)

	doc.SetHeaderFunc(func(p *Page) {
		// Header uses a different font and color.
		doc.SetFont("courier", "B", 8)
		doc.SetTextColor(255, 0, 0)
		p.Cell(0, 5, "RED COURIER HEADER", "", "L", false, 1)
	})

	page := doc.AddPage(A4)

	// After header, document state should be restored.
	if doc.GetFontFamily() != "helvetica" {
		t.Errorf("font family not restored: got %q", doc.GetFontFamily())
	}
	if doc.GetFontSize() != 14 {
		t.Errorf("font size not restored: got %f", doc.GetFontSize())
	}

	page.Cell(0, 10, "Body text", "", "L", false, 1)

	var buf bytes.Buffer
	_, err := doc.WriteTo(&buf)
	if err != nil {
		t.Fatal(err)
	}
}

func TestFooterOnLastPage(t *testing.T) {
	doc := New(WithCompression(false))
	doc.SetFont("helvetica", "", 12)

	footerPages := []int{}
	doc.SetFooterFunc(func(p *Page) {
		footerPages = append(footerPages, doc.PageNo())
	})

	doc.AddPage(A4)
	// Only one page — footer should still be called during WriteTo.

	doc.WriteTo(&bytes.Buffer{})

	if len(footerPages) != 1 {
		t.Fatalf("expected 1 footer call, got %d", len(footerPages))
	}
	if footerPages[0] != 1 {
		t.Errorf("footer should report page 1, got %d", footerPages[0])
	}
}

func TestNegativeSetY(t *testing.T) {
	doc := New()
	page := doc.AddPage(A4)

	page.SetY(-15)
	// A4 height ≈ 297mm. -15 → 297 - 15 = 282.
	y := page.GetY()
	if y < 281 || y > 283 {
		t.Errorf("expected y ≈ 282, got %f", y)
	}
}

func TestPageWidthHeight(t *testing.T) {
	doc := New()
	page := doc.AddPage(A4)

	// A4 in mm: 210 × 297
	w := page.Width()
	h := page.Height()
	if w < 209 || w > 211 {
		t.Errorf("expected width ≈ 210, got %f", w)
	}
	if h < 296 || h > 298 {
		t.Errorf("expected height ≈ 297, got %f", h)
	}
}

// --- PNG support tests ---

// testPNG creates a PNG image in memory.
func testPNG(w, h int, alpha bool) []byte {
	var buf bytes.Buffer
	if alpha {
		img := image.NewNRGBA(image.Rect(0, 0, w, h))
		for y := range h {
			for x := range w {
				img.SetNRGBA(x, y, color.NRGBA{
					R: uint8(x % 256),
					G: uint8(y % 256),
					B: 100,
					A: uint8((x + y) % 256), // varying alpha
				})
			}
		}
		png.Encode(&buf, img)
	} else {
		img := image.NewRGBA(image.Rect(0, 0, w, h))
		for y := range h {
			for x := range w {
				img.Set(x, y, color.RGBA{
					R: uint8(x % 256),
					G: uint8(y % 256),
					B: 100,
					A: 255,
				})
			}
		}
		png.Encode(&buf, img)
	}
	return buf.Bytes()
}

func testGrayPNG(w, h int) []byte {
	img := image.NewGray(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			img.SetGray(x, y, color.Gray{Y: uint8((x + y) % 256)})
		}
	}
	var buf bytes.Buffer
	png.Encode(&buf, img)
	return buf.Bytes()
}

func TestPNGImage(t *testing.T) {
	doc := New(WithCompression(false))
	doc.SetFont("helvetica", "", 12)

	pngData := testPNG(80, 60, false)
	err := doc.RegisterImage("photo", bytes.NewReader(pngData))
	if err != nil {
		t.Fatal(err)
	}

	page := doc.AddPage(A4)
	page.TextAt(20, 20, "PNG below:")
	page.DrawImageRect("photo", 20, 30, 60, 45)

	var buf bytes.Buffer
	_, err = doc.WriteTo(&buf)
	if err != nil {
		t.Fatal(err)
	}
	s := buf.String()

	if !strings.Contains(s, "/Type /XObject") {
		t.Error("missing XObject")
	}
	if !strings.Contains(s, "/Subtype /Image") {
		t.Error("missing /Subtype /Image")
	}
	if !strings.Contains(s, "/Filter /FlateDecode") {
		t.Error("expected FlateDecode filter for PNG")
	}
	if !strings.Contains(s, "/ColorSpace /DeviceRGB") {
		t.Error("expected DeviceRGB color space")
	}
	if !strings.Contains(s, "/Im1 Do") {
		t.Error("missing image draw operator")
	}
}

func TestPNGWithAlpha(t *testing.T) {
	doc := New(WithCompression(false))
	doc.SetFont("helvetica", "", 12)

	pngData := testPNG(40, 40, true)
	err := doc.RegisterImage("alpha", bytes.NewReader(pngData))
	if err != nil {
		t.Fatal(err)
	}

	page := doc.AddPage(A4)
	page.DrawImageRect("alpha", 20, 20, 30, 30)

	var buf bytes.Buffer
	_, err = doc.WriteTo(&buf)
	if err != nil {
		t.Fatal(err)
	}
	s := buf.String()

	// Should have an SMask reference for the alpha channel.
	if !strings.Contains(s, "/SMask") {
		t.Error("missing SMask for RGBA PNG")
	}
	// Two XObjects: the SMask and the main image.
	count := strings.Count(s, "/Subtype /Image")
	if count != 2 {
		t.Errorf("expected 2 image XObjects (main + smask), got %d", count)
	}
}

func TestPNGGrayscale(t *testing.T) {
	doc := New(WithCompression(false))
	doc.SetFont("helvetica", "", 12)

	pngData := testGrayPNG(50, 50)
	err := doc.RegisterImage("gray", bytes.NewReader(pngData))
	if err != nil {
		t.Fatal(err)
	}

	page := doc.AddPage(A4)
	page.DrawImageRect("gray", 10, 10, 40, 40)

	var buf bytes.Buffer
	_, err = doc.WriteTo(&buf)
	if err != nil {
		t.Fatal(err)
	}
	s := buf.String()

	if !strings.Contains(s, "/ColorSpace /DeviceGray") {
		t.Error("expected DeviceGray for grayscale PNG")
	}
	if strings.Contains(s, "/SMask") {
		t.Error("grayscale PNG should not have SMask")
	}
}

func TestPNGAutoDetect(t *testing.T) {
	doc := New(WithCompression(false))

	// Register PNG via RegisterImage (auto-detect).
	pngData := testPNG(20, 20, false)
	err := doc.RegisterImage("auto-png", bytes.NewReader(pngData))
	if err != nil {
		t.Fatal(err)
	}

	// Register JPEG via RegisterImage (auto-detect).
	jpgData := testJPEG(20, 20)
	err = doc.RegisterImage("auto-jpg", bytes.NewReader(jpgData))
	if err != nil {
		t.Fatal(err)
	}

	page := doc.AddPage(A4)
	page.DrawImageRect("auto-png", 10, 10, 20, 20)
	page.DrawImageRect("auto-jpg", 40, 10, 20, 20)

	var buf bytes.Buffer
	_, err = doc.WriteTo(&buf)
	if err != nil {
		t.Fatal(err)
	}
	s := buf.String()

	if !strings.Contains(s, "/Filter /FlateDecode") {
		t.Error("PNG should use FlateDecode")
	}
	if !strings.Contains(s, "/Filter /DCTDecode") {
		t.Error("JPEG should use DCTDecode")
	}
}

func TestPNGDedup(t *testing.T) {
	doc := New(WithCompression(false))

	pngData := testPNG(30, 30, false)
	doc.RegisterImage("img1", bytes.NewReader(pngData))
	doc.RegisterImage("img2", bytes.NewReader(pngData)) // same data

	page := doc.AddPage(A4)
	page.DrawImageRect("img1", 10, 10, 20, 20)
	page.DrawImageRect("img2", 40, 10, 20, 20)

	var buf bytes.Buffer
	_, err := doc.WriteTo(&buf)
	if err != nil {
		t.Fatal(err)
	}
	s := buf.String()

	count := strings.Count(s, "/Type /XObject")
	if count != 1 {
		t.Errorf("expected 1 image XObject (dedup), got %d", count)
	}
}

// --- Table helper tests ---

func TestTableBasic(t *testing.T) {
	doc := New(WithCompression(false))
	doc.SetFont("helvetica", "", 10)
	page := doc.AddPage(A4)

	tbl := NewTable(doc, page)
	tbl.SetWidths(30, 100, 50)
	tbl.SetAligns("C", "L", "R")
	tbl.SetRowHeight(8)

	tbl.Header("#", "Item", "Price")
	tbl.Row("1", "Widget", "9.99")
	tbl.Row("2", "Gadget", "19.99")
	tbl.Row("3", "Doohickey", "4.50")

	var buf bytes.Buffer
	_, err := doc.WriteTo(&buf)
	if err != nil {
		t.Fatal(err)
	}
	s := buf.String()

	for _, text := range []string{"Widget", "Gadget", "Doohickey", "9.99", "19.99"} {
		if !strings.Contains(s, text) {
			t.Errorf("missing table text: %s", text)
		}
	}
}

func TestTableHeaderStyle(t *testing.T) {
	doc := New(WithCompression(false))
	doc.SetFont("helvetica", "", 10)
	page := doc.AddPage(A4)

	tbl := NewTable(doc, page)
	tbl.SetWidths(60, 120)
	tbl.SetHeaderStyle(CellStyle{
		FontFamily: "helvetica", FontStyle: "B", FontSize: 11,
		TextColor: [3]int{255, 255, 255},
		FillColor: [3]int{0, 0, 128},
		Fill:      true,
	})
	tbl.SetBodyStyle(CellStyle{
		FontFamily: "helvetica", FontSize: 10,
		TextColor: [3]int{30, 30, 30},
	})

	tbl.Header("Name", "Value")
	tbl.Row("Alpha", "100")
	tbl.Row("Beta", "200")

	var buf bytes.Buffer
	_, err := doc.WriteTo(&buf)
	if err != nil {
		t.Fatal(err)
	}
	s := buf.String()
	if !strings.Contains(s, "Alpha") || !strings.Contains(s, "Beta") {
		t.Error("missing table body text")
	}
}

func TestTableAlternateRows(t *testing.T) {
	doc := New(WithCompression(false))
	doc.SetFont("helvetica", "", 10)
	page := doc.AddPage(A4)

	tbl := NewTable(doc, page)
	tbl.SetWidths(90, 90)
	tbl.SetAlternateRows([3]int{240, 240, 240}, [3]int{255, 255, 255})

	tbl.Header("Key", "Value")
	for i := range 6 {
		tbl.Row(fmt.Sprintf("key%d", i), fmt.Sprintf("val%d", i))
	}

	var buf bytes.Buffer
	_, err := doc.WriteTo(&buf)
	if err != nil {
		t.Fatal(err)
	}
	s := buf.String()

	// Alternate row fills produce rg color operators.
	// Even rows: 240/255 ≈ 0.941; odd rows: 1.000.
	if !strings.Contains(s, "0.941 0.941 0.941 rg") {
		t.Error("missing even-row fill color")
	}
	if !strings.Contains(s, "1.000 1.000 1.000 rg") {
		t.Error("missing odd-row fill color")
	}
}

func TestTableAutoPageBreakWithHeaderRepeat(t *testing.T) {
	doc := New(WithCompression(false))
	doc.SetFont("helvetica", "", 10)
	doc.SetAutoPageBreak(true, 15)
	page := doc.AddPage(A4)

	tbl := NewTable(doc, page)
	tbl.SetWidths(30, 100, 50)
	tbl.SetAligns("C", "L", "R")
	tbl.SetRowHeight(10)

	tbl.Header("#", "Description", "Amount")

	// 50 rows × 10mm = 500mm. With A4 (297mm), top margin (10mm),
	// bottom margin (15mm), header (10mm) — available ≈ 262mm per page.
	// 26 rows per page → ~2 pages needed.
	for i := range 50 {
		tbl.Row(fmt.Sprintf("%d", i+1), "Line item", "100.00")
	}

	if doc.PageCount() < 2 {
		t.Errorf("expected at least 2 pages, got %d", doc.PageCount())
	}

	var buf bytes.Buffer
	_, err := doc.WriteTo(&buf)
	if err != nil {
		t.Fatal(err)
	}
	s := buf.String()

	// Header text "Description" should appear more than once (repeated).
	count := strings.Count(s, "Description")
	if count < 2 {
		t.Errorf("expected header repeated on page break, found %d occurrences", count)
	}
}

func TestTableNoRepeatHeader(t *testing.T) {
	doc := New(WithCompression(false))
	doc.SetFont("helvetica", "", 10)
	doc.SetAutoPageBreak(true, 15)
	page := doc.AddPage(A4)

	tbl := NewTable(doc, page)
	tbl.SetWidths(90, 90)
	tbl.SetRowHeight(10)
	tbl.SetRepeatHeader(false)

	tbl.Header("Col A", "Col B")
	for i := range 50 {
		tbl.Row(fmt.Sprintf("a%d", i), fmt.Sprintf("b%d", i))
	}

	var buf bytes.Buffer
	_, err := doc.WriteTo(&buf)
	if err != nil {
		t.Fatal(err)
	}
	s := buf.String()

	// Header should appear exactly once.
	if strings.Count(s, "Col A") != 1 {
		t.Errorf("expected header once (no repeat), got %d", strings.Count(s, "Col A"))
	}
}

func TestTableNoBorder(t *testing.T) {
	doc := New(WithCompression(false))
	doc.SetFont("helvetica", "", 10)
	page := doc.AddPage(A4)

	tbl := NewTable(doc, page)
	tbl.SetWidths(90, 90)
	tbl.SetBorder("")

	tbl.Header("A", "B")
	tbl.Row("1", "2")

	var buf bytes.Buffer
	_, err := doc.WriteTo(&buf)
	if err != nil {
		t.Fatal(err)
	}
	// No border means no rect stroke in cells.
	// Just verify it produces valid PDF.
	s := buf.String()
	if !strings.HasPrefix(s, "%PDF-1.4") {
		t.Error("invalid PDF")
	}
}

// --- Link tests ---

func TestLinkURL(t *testing.T) {
	doc := New(WithCompression(false))
	doc.SetFont("helvetica", "", 12)
	page := doc.AddPage(A4)

	page.TextAt(20, 30, "Click here")
	page.LinkURL(20, 25, 50, 10, "https://example.com")

	var buf bytes.Buffer
	_, err := doc.WriteTo(&buf)
	if err != nil {
		t.Fatal(err)
	}
	s := buf.String()

	if !strings.Contains(s, "/Subtype /Link") {
		t.Error("missing /Subtype /Link annotation")
	}
	if !strings.Contains(s, "/S /URI") {
		t.Error("missing URI action")
	}
	if !strings.Contains(s, "https://example.com") {
		t.Error("missing URL in annotation")
	}
	if !strings.Contains(s, "/Annots") {
		t.Error("missing /Annots array in page dict")
	}
	if !strings.Contains(s, "/Border [0 0 0]") {
		t.Error("missing /Border [0 0 0] (no visible border)")
	}
}

func TestLinkAnchorInternal(t *testing.T) {
	doc := New(WithCompression(false))
	doc.SetFont("helvetica", "", 12)

	page1 := doc.AddPage(A4)
	page1.TextAt(20, 30, "Go to chapter 2")
	page1.LinkAnchor(20, 25, 80, 10, "chapter2")

	page2 := doc.AddPage(A4)
	page2.AddAnchor("chapter2")
	page2.TextAt(20, 30, "Chapter 2 starts here")

	var buf bytes.Buffer
	_, err := doc.WriteTo(&buf)
	if err != nil {
		t.Fatal(err)
	}
	s := buf.String()

	if !strings.Contains(s, "/Subtype /Link") {
		t.Error("missing /Subtype /Link annotation")
	}
	if !strings.Contains(s, "/Dest") {
		t.Error("missing /Dest for internal link")
	}
	if !strings.Contains(s, "/XYZ") {
		t.Error("missing /XYZ destination type")
	}
}

func TestLinkMultipleOnPage(t *testing.T) {
	doc := New(WithCompression(false))
	doc.SetFont("helvetica", "", 12)
	page := doc.AddPage(A4)

	page.TextAt(20, 30, "Link 1")
	page.LinkURL(20, 25, 40, 10, "https://one.example.com")

	page.TextAt(20, 50, "Link 2")
	page.LinkURL(20, 45, 40, 10, "https://two.example.com")

	var buf bytes.Buffer
	_, err := doc.WriteTo(&buf)
	if err != nil {
		t.Fatal(err)
	}
	s := buf.String()

	// Two link annotations on the same page.
	count := strings.Count(s, "/Subtype /Link")
	if count != 2 {
		t.Errorf("expected 2 link annotations, got %d", count)
	}
	if !strings.Contains(s, "https://one.example.com") {
		t.Error("missing first URL")
	}
	if !strings.Contains(s, "https://two.example.com") {
		t.Error("missing second URL")
	}
}

func TestLinkNoAnnotsWhenNoLinks(t *testing.T) {
	doc := New(WithCompression(false))
	doc.SetFont("helvetica", "", 12)
	page := doc.AddPage(A4)
	page.TextAt(20, 30, "No links here")

	var buf bytes.Buffer
	_, err := doc.WriteTo(&buf)
	if err != nil {
		t.Fatal(err)
	}
	s := buf.String()

	if strings.Contains(s, "/Annots") {
		t.Error("/Annots should not appear when page has no links")
	}
}

func TestLinkRectCoordinates(t *testing.T) {
	doc := New(WithCompression(false))
	doc.SetFont("helvetica", "", 12)
	page := doc.AddPage(A4)

	// Place a link at known coordinates.
	page.LinkURL(10, 20, 50, 10, "https://example.com")

	var buf bytes.Buffer
	_, err := doc.WriteTo(&buf)
	if err != nil {
		t.Fatal(err)
	}
	s := buf.String()

	// k = 72/25.4 ≈ 2.8346
	// A4 height in mm ≈ 297.0 (841.89 pt / k)
	// x1 = 10 * k ≈ 28.35
	// y1 = (297 - 30) * k ≈ 756.85 (bottom edge: y + h = 20 + 10 = 30)
	// x2 = 60 * k ≈ 170.08
	// y2 = (297 - 20) * k ≈ 785.20 (top edge: y = 20)
	if !strings.Contains(s, "/Rect [") {
		t.Error("missing /Rect in annotation")
	}
}

func TestLinkURLEscaping(t *testing.T) {
	doc := New(WithCompression(false))
	doc.SetFont("helvetica", "", 12)
	page := doc.AddPage(A4)

	// URL with parentheses that need escaping in PDF strings.
	page.LinkURL(10, 10, 50, 10, "https://example.com/path?q=a(b)")

	var buf bytes.Buffer
	_, err := doc.WriteTo(&buf)
	if err != nil {
		t.Fatal(err)
	}
	s := buf.String()

	// The parentheses in the URL should be escaped.
	if !strings.Contains(s, `\(b\)`) {
		t.Error("URL parentheses should be escaped in PDF string")
	}
}

func TestLinkWithAutoPageBreak(t *testing.T) {
	doc := New(WithCompression(false))
	doc.SetFont("helvetica", "", 12)
	doc.SetAutoPageBreak(true, 15)

	page := doc.AddPage(A4)

	// Fill page 1 with cells.
	for i := 0; i < 28; i++ {
		page.Cell(0, 10, fmt.Sprintf("Line %d", i+1), "", "L", false, 1)
	}

	// After auto break, page should forward to page 2.
	// Adding a link should go on page 2 via the forwarding pointer.
	page.LinkURL(10, page.GetY(), 50, 10, "https://page2.example.com")

	var buf bytes.Buffer
	_, err := doc.WriteTo(&buf)
	if err != nil {
		t.Fatal(err)
	}
	s := buf.String()

	if doc.PageCount() != 2 {
		t.Errorf("expected 2 pages, got %d", doc.PageCount())
	}
	if !strings.Contains(s, "https://page2.example.com") {
		t.Error("missing link URL on page 2")
	}
}

// --- Text decoration tests ---

func TestUnderline(t *testing.T) {
	doc := New(WithCompression(false))
	doc.SetFont("helvetica", "", 12)
	page := doc.AddPage(A4)

	doc.SetUnderline(true)
	page.Cell(0, 10, "Underlined text", "", "L", false, 1)
	doc.SetUnderline(false)
	page.Cell(0, 10, "Normal text", "", "L", false, 1)

	var buf bytes.Buffer
	_, err := doc.WriteTo(&buf)
	if err != nil {
		t.Fatal(err)
	}
	s := buf.String()

	// Underline draws a filled rect ("re" + "f" operators).
	// The underlined cell should produce a rect fill for the decoration.
	// Count "re" occurrences: normal text has no decoration rect.
	if !strings.Contains(s, "Underlined text") {
		t.Error("missing underlined text")
	}
	if !strings.Contains(s, "Normal text") {
		t.Error("missing normal text")
	}
}

func TestStrikethrough(t *testing.T) {
	doc := New(WithCompression(false))
	doc.SetFont("helvetica", "", 12)
	page := doc.AddPage(A4)

	doc.SetStrikethrough(true)
	page.Cell(0, 10, "Struck text", "", "L", false, 1)
	doc.SetStrikethrough(false)

	var buf bytes.Buffer
	_, err := doc.WriteTo(&buf)
	if err != nil {
		t.Fatal(err)
	}
	s := buf.String()

	if !strings.Contains(s, "Struck text") {
		t.Error("missing strikethrough text")
	}
}

func TestUnderlineTextAt(t *testing.T) {
	doc := New(WithCompression(false))
	doc.SetFont("helvetica", "", 12)
	page := doc.AddPage(A4)

	doc.SetUnderline(true)
	page.TextAt(20, 30, "Underlined via TextAt")

	var buf bytes.Buffer
	_, err := doc.WriteTo(&buf)
	if err != nil {
		t.Fatal(err)
	}
	s := buf.String()

	if !strings.Contains(s, "Underlined via TextAt") {
		t.Error("missing text")
	}
	// The underline should produce a rect + fill in the content stream.
	// "re" for rect, "f" for fill — but we need to check for the
	// decoration rect specifically. The text color state block (q/Q)
	// will contain both the text and the decoration rect.
}

func TestDecorationProducesRect(t *testing.T) {
	// Verify that underline/strikethrough actually produce rect+fill
	// operations in the content stream beyond what borders produce.
	doc := New(WithCompression(false))
	doc.SetFont("helvetica", "", 12)
	page := doc.AddPage(A4)

	// No decoration, no border, no fill — baseline.
	page.Cell(100, 10, "Plain", "", "L", false, 1)

	var buf1 bytes.Buffer
	doc.WriteTo(&buf1)
	plainRects := strings.Count(buf1.String(), " re\n")

	// Now with underline.
	doc2 := New(WithCompression(false))
	doc2.SetFont("helvetica", "", 12)
	page2 := doc2.AddPage(A4)
	doc2.SetUnderline(true)
	page2.Cell(100, 10, "Underlined", "", "L", false, 1)

	var buf2 bytes.Buffer
	doc2.WriteTo(&buf2)
	underlineRects := strings.Count(buf2.String(), " re\n")

	if underlineRects <= plainRects {
		t.Errorf("underline should produce more rects: plain=%d underline=%d",
			plainRects, underlineRects)
	}
}

func TestDecorationStateRestore(t *testing.T) {
	// Decoration state should be saved/restored around header/footer.
	doc := New(WithCompression(false))
	doc.SetFont("helvetica", "", 12)
	doc.SetUnderline(true)

	doc.SetHeaderFunc(func(p *Page) {
		doc.SetUnderline(false)
		doc.SetStrikethrough(true)
		p.Cell(0, 5, "Header", "", "C", false, 1)
	})

	doc.AddPage(A4)

	// After header, underline should be restored to true, strikethrough to false.
	if !doc.underline {
		t.Error("underline should be restored to true after header")
	}
	if doc.strikethrough {
		t.Error("strikethrough should be restored to false after header")
	}
}

func TestUnderlineAndStrikethroughTogether(t *testing.T) {
	doc := New(WithCompression(false))
	doc.SetFont("helvetica", "", 12)
	page := doc.AddPage(A4)

	doc.SetUnderline(true)
	doc.SetStrikethrough(true)
	page.Cell(0, 10, "Both decorations", "", "L", false, 1)

	var buf bytes.Buffer
	_, err := doc.WriteTo(&buf)
	if err != nil {
		t.Fatal(err)
	}
	s := buf.String()

	if !strings.Contains(s, "Both decorations") {
		t.Error("missing text")
	}
	// Both decorations produce two rects within the same graphics state.
	// Count decoration rects: should be 2 (underline + strikethrough).
	// No border/fill rects since border="" and fill=false.
	rectCount := strings.Count(s, " re\n")
	if rectCount < 2 {
		t.Errorf("expected at least 2 decoration rects, got %d", rectCount)
	}
}

// --- Write() inline text flow tests ---

func TestWriteBasic(t *testing.T) {
	doc := New(WithCompression(false))
	doc.SetFont("helvetica", "", 12)
	page := doc.AddPage(A4)

	page.Write(6, "Hello ")
	doc.SetFontStyle("B")
	page.Write(6, "bold")
	doc.SetFontStyle("")
	page.Write(6, " world.")

	var buf bytes.Buffer
	_, err := doc.WriteTo(&buf)
	if err != nil {
		t.Fatal(err)
	}
	s := buf.String()

	for _, text := range []string{"Hello ", "bold", " world."} {
		if !strings.Contains(s, text) {
			t.Errorf("missing text: %q", text)
		}
	}
}

func TestWriteCursorAdvances(t *testing.T) {
	doc := New(WithCompression(false))
	doc.SetFont("helvetica", "", 12)
	page := doc.AddPage(A4)

	x0 := page.GetX()
	page.Write(6, "Hello")
	x1 := page.GetX()

	if x1 <= x0 {
		t.Errorf("cursor should advance: x0=%f x1=%f", x0, x1)
	}

	// Write more on the same line — cursor continues.
	page.Write(6, " World")
	x2 := page.GetX()
	if x2 <= x1 {
		t.Errorf("cursor should continue advancing: x1=%f x2=%f", x1, x2)
	}

	// Y should not have changed (no wrapping on short text).
	y := page.GetY()
	if y != doc.tMargin {
		t.Errorf("y should still be at top margin (%f), got %f", doc.tMargin, y)
	}
}

func TestWriteNewlineHandling(t *testing.T) {
	doc := New(WithCompression(false))
	doc.SetFont("helvetica", "", 12)
	page := doc.AddPage(A4)

	y0 := page.GetY()
	page.Write(6, "Line one\nLine two")
	y1 := page.GetY()

	if y1 <= y0 {
		t.Errorf("newline should advance y: y0=%f y1=%f", y0, y1)
	}
	// After newline, x should be at left margin.
	if page.GetX() != doc.lMargin {
		// Actually, after "Line two", x should be past left margin.
		// But x was set to lMargin at the newline, then advanced by "Line two" width.
		if page.GetX() <= doc.lMargin {
			t.Errorf("x should be past left margin after 'Line two'")
		}
	}
}

func TestWriteWordWrap(t *testing.T) {
	doc := New(WithCompression(false))
	doc.SetFont("helvetica", "", 12)
	page := doc.AddPage(A4)

	// A4 width = 210mm, margins 10mm each → 190mm available.
	// Write enough text to force word wrapping.
	long := strings.Repeat("word ", 60) // ~300 short words
	y0 := page.GetY()
	page.Write(6, long)
	y1 := page.GetY()

	if y1 <= y0 {
		t.Errorf("wrapping should advance y: y0=%f y1=%f", y0, y1)
	}

	var buf bytes.Buffer
	_, err := doc.WriteTo(&buf)
	if err != nil {
		t.Fatal(err)
	}
}

func TestWriteWithPageBreak(t *testing.T) {
	doc := New(WithCompression(false))
	doc.SetFont("helvetica", "", 12)
	doc.SetAutoPageBreak(true, 15)
	page := doc.AddPage(A4)

	// Write enough text to overflow to page 2.
	// A4 available height: 297 - 10 - 15 = 272mm. At 6mm per line, ~45 lines fit.
	// Each repetition is ~40 chars. At 12pt Helvetica (~2mm/char), ~95 chars/line,
	// so ~2.4 reps/line. Need ~45 lines → ~108 reps. Use 200 for margin.
	long := strings.Repeat("Testing page breaks with the Write method works well. ", 200)
	page.Write(6, long)

	if doc.PageCount() < 2 {
		t.Errorf("expected at least 2 pages, got %d", doc.PageCount())
	}

	var buf bytes.Buffer
	_, err := doc.WriteTo(&buf)
	if err != nil {
		t.Fatal(err)
	}
}

func TestWriteMixedStyles(t *testing.T) {
	// The primary use case: rich inline text with mixed formatting.
	doc := New(WithCompression(false))
	doc.SetFont("helvetica", "", 12)
	page := doc.AddPage(A4)

	page.Write(6, "This is ")
	doc.SetFontStyle("B")
	page.Write(6, "bold")
	doc.SetFontStyle("I")
	page.Write(6, " italic")
	doc.SetFontStyle("")
	page.Write(6, " and back to normal.")

	var buf bytes.Buffer
	_, err := doc.WriteTo(&buf)
	if err != nil {
		t.Fatal(err)
	}
	s := buf.String()

	// Should reference multiple font objects (regular, bold, italic).
	if !strings.Contains(s, "/F1") {
		t.Error("missing font reference F1")
	}
	if !strings.Contains(s, "/F2") {
		t.Error("missing font reference F2 (bold)")
	}
	if !strings.Contains(s, "/F3") {
		t.Error("missing font reference F3 (italic)")
	}
}

func TestWriteUnderline(t *testing.T) {
	doc := New(WithCompression(false))
	doc.SetFont("helvetica", "", 12)
	page := doc.AddPage(A4)

	page.Write(6, "Normal ")
	doc.SetUnderline(true)
	page.Write(6, "underlined")
	doc.SetUnderline(false)
	page.Write(6, " normal again.")

	var buf bytes.Buffer
	_, err := doc.WriteTo(&buf)
	if err != nil {
		t.Fatal(err)
	}
	s := buf.String()

	if !strings.Contains(s, "underlined") {
		t.Error("missing underlined text")
	}
}

func TestWriteEmptyString(t *testing.T) {
	doc := New(WithCompression(false))
	doc.SetFont("helvetica", "", 12)
	page := doc.AddPage(A4)

	x0 := page.GetX()
	page.Write(6, "")
	x1 := page.GetX()

	if x0 != x1 {
		t.Errorf("empty Write should not move cursor: x0=%f x1=%f", x0, x1)
	}
}

func TestWriteStartMidLine(t *testing.T) {
	// Start mid-line with Cell, then continue with Write.
	doc := New(WithCompression(false))
	doc.SetFont("helvetica", "", 12)
	page := doc.AddPage(A4)

	page.Cell(40, 6, "Label:", "", "L", false, 0) // cursor moves right by 40mm
	x0 := page.GetX()
	page.Write(6, "value text here")
	x1 := page.GetX()

	if x1 <= x0 {
		t.Errorf("Write after Cell should continue: x0=%f x1=%f", x0, x1)
	}
}

// --- Transform tests ---

func TestRotate(t *testing.T) {
	doc := New(WithCompression(false))
	doc.SetFont("helvetica", "", 40)
	page := doc.AddPage(A4)

	page.TransformBegin()
	page.Rotate(45, 105, 148.5) // rotate 45° around page center
	page.TextAt(60, 148.5, "WATERMARK")
	page.TransformEnd()

	var buf bytes.Buffer
	_, err := doc.WriteTo(&buf)
	if err != nil {
		t.Fatal(err)
	}
	s := buf.String()

	// Should contain cm operator with rotation matrix values.
	if !strings.Contains(s, " cm\n") {
		t.Error("missing cm operator")
	}
	// q/Q pair for transform begin/end.
	if !strings.Contains(s, "q\n") {
		t.Error("missing q (save state)")
	}
	if !strings.Contains(s, "Q\n") {
		t.Error("missing Q (restore state)")
	}
	if !strings.Contains(s, "WATERMARK") {
		t.Error("missing text")
	}
}

func TestScale(t *testing.T) {
	doc := New(WithCompression(false))
	doc.SetFont("helvetica", "", 12)
	page := doc.AddPage(A4)

	page.TransformBegin()
	page.Scale(2, 2, 105, 148.5) // scale 2x around page center
	page.TextAt(90, 148, "Scaled text")
	page.TransformEnd()

	var buf bytes.Buffer
	_, err := doc.WriteTo(&buf)
	if err != nil {
		t.Fatal(err)
	}
	s := buf.String()

	if !strings.Contains(s, " cm\n") {
		t.Error("missing cm operator")
	}
	if !strings.Contains(s, "Scaled text") {
		t.Error("missing text")
	}
}

func TestSkew(t *testing.T) {
	doc := New(WithCompression(false))
	doc.SetFont("helvetica", "", 12)
	page := doc.AddPage(A4)

	page.TransformBegin()
	page.Skew(15, 0, 105, 148.5) // horizontal skew
	page.TextAt(80, 148, "Skewed")
	page.TransformEnd()

	var buf bytes.Buffer
	_, err := doc.WriteTo(&buf)
	if err != nil {
		t.Fatal(err)
	}
	s := buf.String()

	if !strings.Contains(s, " cm\n") {
		t.Error("missing cm operator")
	}
	if !strings.Contains(s, "Skewed") {
		t.Error("missing text")
	}
}

func TestTranslate(t *testing.T) {
	doc := New(WithCompression(false))
	doc.SetFont("helvetica", "", 12)
	page := doc.AddPage(A4)

	page.TransformBegin()
	page.Translate(50, 50) // shift everything 50mm right and down
	page.TextAt(10, 10, "Translated")
	page.TransformEnd()

	var buf bytes.Buffer
	_, err := doc.WriteTo(&buf)
	if err != nil {
		t.Fatal(err)
	}
	s := buf.String()

	if !strings.Contains(s, " cm\n") {
		t.Error("missing cm operator")
	}
	if !strings.Contains(s, "Translated") {
		t.Error("missing text")
	}
}

func TestTransformRotatedImage(t *testing.T) {
	doc := New(WithCompression(false))
	doc.SetFont("helvetica", "", 12)

	// Register a test image.
	jpgData := testJPEG(60, 40)
	doc.RegisterImage("photo", bytes.NewReader(jpgData))

	page := doc.AddPage(A4)

	page.TransformBegin()
	page.Rotate(30, 80, 100) // rotate image 30° around (80,100)
	page.DrawImageRect("photo", 50, 80, 60, 40)
	page.TransformEnd()

	var buf bytes.Buffer
	_, err := doc.WriteTo(&buf)
	if err != nil {
		t.Fatal(err)
	}
	s := buf.String()

	// Should have two cm operators: one from the transform and one from DrawImage.
	cmCount := strings.Count(s, " cm\n")
	if cmCount < 2 {
		t.Errorf("expected at least 2 cm operators (transform + image), got %d", cmCount)
	}
}

func TestTransformChained(t *testing.T) {
	// Multiple transforms applied in sequence.
	doc := New(WithCompression(false))
	doc.SetFont("helvetica", "", 12)
	page := doc.AddPage(A4)

	page.TransformBegin()
	page.Translate(20, 20)
	page.Rotate(15, 0, 0)
	page.TextAt(10, 10, "Chained")
	page.TransformEnd()

	var buf bytes.Buffer
	_, err := doc.WriteTo(&buf)
	if err != nil {
		t.Fatal(err)
	}
	s := buf.String()

	// Two cm operators (translate + rotate).
	cmCount := strings.Count(s, " cm\n")
	if cmCount < 2 {
		t.Errorf("expected at least 2 cm operators, got %d", cmCount)
	}
	if !strings.Contains(s, "Chained") {
		t.Error("missing text")
	}
}

func TestTransformNested(t *testing.T) {
	// Nested transform blocks.
	doc := New(WithCompression(false))
	doc.SetFont("helvetica", "", 12)
	page := doc.AddPage(A4)

	page.TransformBegin()
	page.Rotate(30, 105, 148.5)
	page.TextAt(80, 148, "Outer")

	page.TransformBegin()
	page.Scale(1.5, 1.5, 105, 148.5)
	page.TextAt(80, 160, "Inner")
	page.TransformEnd()

	page.TransformEnd()

	var buf bytes.Buffer
	_, err := doc.WriteTo(&buf)
	if err != nil {
		t.Fatal(err)
	}
	s := buf.String()

	if !strings.Contains(s, "Outer") || !strings.Contains(s, "Inner") {
		t.Error("missing text from nested transforms")
	}

	// Two q/Q pairs.
	qCount := strings.Count(s, "q\n")
	bigQCount := strings.Count(s, "Q\n")
	if qCount < 2 {
		t.Errorf("expected at least 2 q operators, got %d", qCount)
	}
	if bigQCount < 2 {
		t.Errorf("expected at least 2 Q operators, got %d", bigQCount)
	}
}

func TestRotateMatrixValues(t *testing.T) {
	// Verify the cm matrix values for a known rotation.
	doc := New(WithCompression(false))
	doc.SetFont("helvetica", "", 12)
	page := doc.AddPage(A4)

	page.TransformBegin()
	// 90° rotation around origin (0, 0) in user units.
	// In PDF coords, origin (0,0) is bottom-left corner.
	// cos(90°) = 0, sin(90°) = 1
	// Matrix: a=0, b=1, c=-1, d=0, e=cx(1-0)+cy*1, f=cy(1-0)-cx*1
	// With cx=0, cy=(297)*k (top of page in PDF = origin in user coords):
	// e=0+cy, f=cy-0 = cy
	page.Rotate(90, 0, 0)
	page.TransformEnd()

	var buf bytes.Buffer
	doc.WriteTo(&buf)
	s := buf.String()

	// Should contain "0.00000 1.00000 -1.00000 0.00000" for cos/sin values.
	if !strings.Contains(s, "0.00000 1.00000 -1.00000 0.00000") {
		t.Errorf("unexpected rotation matrix in output:\n%s", s)
	}
}

func TestTranslateDirection(t *testing.T) {
	// Translate should produce correct PDF coordinates.
	doc := New(WithCompression(false))
	doc.SetFont("helvetica", "", 12)
	page := doc.AddPage(A4)

	page.TransformBegin()
	page.Translate(10, 20)
	page.TransformEnd()

	var buf bytes.Buffer
	doc.WriteTo(&buf)
	s := buf.String()

	// Translate(10, 20) in mm:
	// e = 10 * k ≈ 28.35
	// f = -20 * k ≈ -56.69 (positive ty = downward = negative PDF y)
	if !strings.Contains(s, "1.00000 0.00000 0.00000 1.00000") {
		t.Error("translate should have identity rotation/scale components")
	}
}

// --- Alpha transparency tests ---

func TestSetAlpha(t *testing.T) {
	doc := New(WithCompression(false))
	doc.SetFont("helvetica", "", 12)
	page := doc.AddPage(A4)

	doc.SetAlpha(0.5)
	page.TextAt(20, 30, "Semi-transparent text")

	var buf bytes.Buffer
	_, err := doc.WriteTo(&buf)
	if err != nil {
		t.Fatal(err)
	}
	s := buf.String()

	// Should have ExtGState object with ca and CA.
	if !strings.Contains(s, "/Type /ExtGState") {
		t.Error("missing ExtGState object")
	}
	if !strings.Contains(s, "/ca 0.500") {
		t.Error("missing fill opacity /ca")
	}
	if !strings.Contains(s, "/CA 0.500") {
		t.Error("missing stroke opacity /CA")
	}
	// Resource dict should reference ExtGState.
	if !strings.Contains(s, "/ExtGState") {
		t.Error("missing /ExtGState in resource dict")
	}
	// Content stream should have gs operator.
	if !strings.Contains(s, "/GS1 gs") {
		t.Error("missing gs operator in content stream")
	}
}

func TestAlphaMultipleValues(t *testing.T) {
	doc := New(WithCompression(false))
	doc.SetFont("helvetica", "", 12)
	page := doc.AddPage(A4)

	doc.SetAlpha(0.3)
	page.TextAt(20, 30, "Light")
	doc.SetAlpha(0.7)
	page.TextAt(20, 50, "Medium")
	doc.SetAlpha(1.0)
	page.TextAt(20, 70, "Opaque")

	var buf bytes.Buffer
	_, err := doc.WriteTo(&buf)
	if err != nil {
		t.Fatal(err)
	}
	s := buf.String()

	// Three distinct alpha states.
	gsCount := strings.Count(s, "/Type /ExtGState")
	if gsCount != 3 {
		t.Errorf("expected 3 ExtGState objects, got %d", gsCount)
	}
	// Three gs operator calls.
	if !strings.Contains(s, "/GS1 gs") || !strings.Contains(s, "/GS2 gs") || !strings.Contains(s, "/GS3 gs") {
		t.Error("missing gs operator calls for different alpha values")
	}
}

func TestAlphaDedup(t *testing.T) {
	doc := New(WithCompression(false))
	doc.SetFont("helvetica", "", 12)
	page := doc.AddPage(A4)

	doc.SetAlpha(0.5)
	page.TextAt(20, 30, "First")
	doc.SetAlpha(0.5) // same value — should reuse
	page.TextAt(20, 50, "Second")

	var buf bytes.Buffer
	_, err := doc.WriteTo(&buf)
	if err != nil {
		t.Fatal(err)
	}
	s := buf.String()

	// Only one ExtGState object (dedup).
	gsCount := strings.Count(s, "/Type /ExtGState")
	if gsCount != 1 {
		t.Errorf("expected 1 ExtGState (dedup), got %d", gsCount)
	}
}

func TestAlphaNoExtGStateWhenNotUsed(t *testing.T) {
	doc := New(WithCompression(false))
	doc.SetFont("helvetica", "", 12)
	page := doc.AddPage(A4)
	page.TextAt(20, 30, "Fully opaque by default")

	var buf bytes.Buffer
	_, err := doc.WriteTo(&buf)
	if err != nil {
		t.Fatal(err)
	}
	s := buf.String()

	if strings.Contains(s, "/ExtGState") {
		t.Error("/ExtGState should not appear when alpha is never set")
	}
}

func TestAlphaWatermarkPattern(t *testing.T) {
	// Realistic watermark: rotated semi-transparent text.
	doc := New(WithCompression(false))
	doc.SetFont("helvetica", "B", 60)
	page := doc.AddPage(A4)

	page.TransformBegin()
	page.Rotate(45, 105, 148.5)
	doc.SetAlpha(0.2)
	doc.SetTextColor(200, 200, 200)
	page.TextAt(30, 148.5, "DRAFT")
	doc.SetAlpha(1.0)
	page.TransformEnd()

	// Draw normal content on top.
	doc.SetTextColor(0, 0, 0)
	doc.SetFont("helvetica", "", 12)
	page.TextAt(20, 30, "Normal content")

	var buf bytes.Buffer
	_, err := doc.WriteTo(&buf)
	if err != nil {
		t.Fatal(err)
	}
	s := buf.String()

	if !strings.Contains(s, "DRAFT") {
		t.Error("missing watermark text")
	}
	if !strings.Contains(s, "Normal content") {
		t.Error("missing normal content")
	}
	if !strings.Contains(s, "/ca 0.200") {
		t.Error("missing 0.2 opacity")
	}
	if !strings.Contains(s, " cm\n") {
		t.Error("missing rotation transform")
	}
}

func TestAlphaStateRestore(t *testing.T) {
	// Alpha should be saved/restored around header/footer callbacks.
	doc := New(WithCompression(false))
	doc.SetFont("helvetica", "", 12)
	doc.SetAlpha(0.8)

	doc.SetHeaderFunc(func(p *Page) {
		doc.SetAlpha(0.3)
		p.Cell(0, 5, "Faded header", "", "C", false, 1)
	})

	doc.AddPage(A4)

	// After header, alpha should be restored to 0.8.
	if doc.currentAlpha != 0.8 {
		t.Errorf("alpha should be restored to 0.8 after header, got %f", doc.currentAlpha)
	}
}

func TestAlphaOnNewPage(t *testing.T) {
	doc := New(WithCompression(false))
	doc.SetFont("helvetica", "", 12)
	doc.SetAlpha(0.5)
	doc.AddPage(A4)

	var buf bytes.Buffer
	_, err := doc.WriteTo(&buf)
	if err != nil {
		t.Fatal(err)
	}
	s := buf.String()

	// Alpha should be applied to the first page.
	if !strings.Contains(s, "/GS1 gs") {
		t.Error("alpha should be applied to new page")
	}
}

func TestAlphaClamp(t *testing.T) {
	doc := New(WithCompression(false))
	doc.SetFont("helvetica", "", 12)
	doc.AddPage(A4)

	doc.SetAlpha(-0.5)
	if doc.currentAlpha != 0 {
		t.Errorf("negative alpha should be clamped to 0, got %f", doc.currentAlpha)
	}
	doc.SetAlpha(1.5)
	if doc.currentAlpha != 1 {
		t.Errorf("alpha > 1 should be clamped to 1, got %f", doc.currentAlpha)
	}
}

// --- Circle/Ellipse + Dash pattern tests ---

func TestCircleStroke(t *testing.T) {
	doc := New(WithCompression(false))
	page := doc.AddPage(A4)

	page.Circle(105, 148.5, 40, "D")

	var buf bytes.Buffer
	_, err := doc.WriteTo(&buf)
	if err != nil {
		t.Fatal(err)
	}
	s := buf.String()

	// Circle is 4 Bézier curves: 4 "c" operators + 1 "m" (moveto) + 1 "S" (stroke).
	if strings.Count(s, " c\n") != 4 {
		t.Errorf("expected 4 cubic Bézier curves, got %d", strings.Count(s, " c\n"))
	}
	if !strings.Contains(s, " m\n") {
		t.Error("missing moveto operator")
	}
	if !strings.Contains(s, "S\n") {
		t.Error("missing stroke operator")
	}
}

func TestCircleFill(t *testing.T) {
	doc := New(WithCompression(false))
	page := doc.AddPage(A4)

	doc.SetFillColor(255, 0, 0)
	page.Circle(50, 50, 20, "F")

	var buf bytes.Buffer
	_, err := doc.WriteTo(&buf)
	if err != nil {
		t.Fatal(err)
	}
	s := buf.String()

	if !strings.Contains(s, "f\n") {
		t.Error("missing fill operator")
	}
}

func TestCircleFillStroke(t *testing.T) {
	doc := New(WithCompression(false))
	page := doc.AddPage(A4)

	doc.SetFillColor(200, 200, 255)
	page.Circle(80, 80, 30, "DF")

	var buf bytes.Buffer
	_, err := doc.WriteTo(&buf)
	if err != nil {
		t.Fatal(err)
	}
	s := buf.String()

	if !strings.Contains(s, "B\n") {
		t.Error("missing fill+stroke operator")
	}
}

func TestEllipse(t *testing.T) {
	doc := New(WithCompression(false))
	page := doc.AddPage(A4)

	page.Ellipse(105, 148.5, 60, 30, "D")

	var buf bytes.Buffer
	_, err := doc.WriteTo(&buf)
	if err != nil {
		t.Fatal(err)
	}
	s := buf.String()

	if strings.Count(s, " c\n") != 4 {
		t.Errorf("expected 4 Bézier curves for ellipse, got %d", strings.Count(s, " c\n"))
	}
}

func TestDashPattern(t *testing.T) {
	doc := New(WithCompression(false))
	page := doc.AddPage(A4)

	// Set a dash pattern: 5mm dash, 3mm gap.
	page.SetDashPattern([]float64{5, 3}, 0)
	page.Line(20, 50, 190, 50)

	var buf bytes.Buffer
	_, err := doc.WriteTo(&buf)
	if err != nil {
		t.Fatal(err)
	}
	s := buf.String()

	// Should contain the d operator with a dash array.
	if !strings.Contains(s, "] ") || !strings.Contains(s, " d\n") {
		t.Error("missing dash pattern (d operator)")
	}
}

func TestDashPatternSolid(t *testing.T) {
	doc := New(WithCompression(false))
	page := doc.AddPage(A4)

	// Set dashed, then reset to solid.
	page.SetDashPattern([]float64{3, 2}, 0)
	page.Line(20, 40, 190, 40)
	page.SetDashPattern(nil, 0)
	page.Line(20, 50, 190, 50)

	var buf bytes.Buffer
	_, err := doc.WriteTo(&buf)
	if err != nil {
		t.Fatal(err)
	}
	s := buf.String()

	// Two d operators: one dashed, one solid (empty array).
	dCount := strings.Count(s, " d\n")
	if dCount != 2 {
		t.Errorf("expected 2 dash operators (set + reset), got %d", dCount)
	}
	// The reset should produce "[] 0.00 d".
	if !strings.Contains(s, "[] 0.00 d") {
		t.Error("missing solid line reset (empty dash array)")
	}
}

func TestDashedCircle(t *testing.T) {
	// Combine dash pattern with circle.
	doc := New(WithCompression(false))
	page := doc.AddPage(A4)

	page.SetDashPattern([]float64{2, 2}, 0)
	page.Circle(105, 148.5, 50, "D")

	var buf bytes.Buffer
	_, err := doc.WriteTo(&buf)
	if err != nil {
		t.Fatal(err)
	}
	s := buf.String()

	if !strings.Contains(s, " d\n") {
		t.Error("missing dash pattern")
	}
	if strings.Count(s, " c\n") != 4 {
		t.Error("missing Bézier curves for circle")
	}
}

func TestDashPatternPhase(t *testing.T) {
	doc := New(WithCompression(false))
	page := doc.AddPage(A4)

	// Phase = 2mm offset into the pattern.
	page.SetDashPattern([]float64{5, 3}, 2)
	page.Line(20, 60, 190, 60)

	var buf bytes.Buffer
	_, err := doc.WriteTo(&buf)
	if err != nil {
		t.Fatal(err)
	}
	s := buf.String()

	// Phase should be non-zero in the output.
	// k ≈ 2.835, so phase = 2 * 2.835 ≈ 5.67
	if strings.Contains(s, "] 0.00 d") {
		t.Error("phase should be non-zero")
	}
}

// --- Typography: Character Spacing ---

func TestSetCharSpacing(t *testing.T) {
	doc := New(WithCompression(false))
	doc.SetFont("helvetica", "", 12)
	doc.SetCharSpacing(2.5)
	page := doc.AddPage(A4)
	page.TextAt(10, 20, "Spaced")

	b, err := doc.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	if !strings.Contains(s, "2.50 Tc") {
		t.Error("expected Tc operator for char spacing")
	}
}

func TestCharSpacingZeroReset(t *testing.T) {
	doc := New(WithCompression(false))
	doc.SetFont("helvetica", "", 12)
	doc.SetCharSpacing(3.0)
	doc.AddPage(A4)
	doc.SetCharSpacing(0)

	b, err := doc.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	if !strings.Contains(s, "0.00 Tc") {
		t.Error("expected Tc reset to 0")
	}
}

func TestCharSpacingCarriedToNewPage(t *testing.T) {
	doc := New(WithCompression(false))
	doc.SetFont("helvetica", "", 12)
	doc.AddPage(A4)
	doc.SetCharSpacing(1.5)
	doc.AddPage(A4) // second page should inherit char spacing

	b, err := doc.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	// Count occurrences of "1.50 Tc" — should appear at least twice
	// (once on page 1 when set, once on page 2 carried over).
	if strings.Count(s, "1.50 Tc") < 2 {
		t.Error("char spacing should carry to new page")
	}
}

// --- Typography: Word Spacing ---

func TestSetWordSpacing(t *testing.T) {
	doc := New(WithCompression(false))
	doc.SetFont("helvetica", "", 12)
	doc.SetWordSpacing(4.0)
	page := doc.AddPage(A4)
	page.TextAt(10, 20, "Hello World")

	b, err := doc.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	if !strings.Contains(s, "4.00 Tw") {
		t.Error("expected Tw operator for word spacing")
	}
}

func TestWordSpacingCarriedToNewPage(t *testing.T) {
	doc := New(WithCompression(false))
	doc.SetFont("helvetica", "", 12)
	doc.AddPage(A4)
	doc.SetWordSpacing(2.0)
	doc.AddPage(A4)

	b, err := doc.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	if strings.Count(s, "2.00 Tw") < 2 {
		t.Error("word spacing should carry to new page")
	}
}

// --- Typography: Text Rise ---

func TestSetTextRise(t *testing.T) {
	doc := New(WithCompression(false))
	doc.SetFont("helvetica", "", 12)
	page := doc.AddPage(A4)

	doc.SetTextRise(5.0) // superscript
	page.TextAt(10, 20, "sup")
	doc.SetTextRise(0)

	b, err := doc.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	if !strings.Contains(s, "5.00 Ts") {
		t.Error("expected Ts operator for text rise")
	}
}

func TestTextRiseNegative(t *testing.T) {
	doc := New(WithCompression(false))
	doc.SetFont("helvetica", "", 12)
	page := doc.AddPage(A4)

	doc.SetTextRise(-3.0) // subscript
	page.TextAt(10, 20, "sub")

	b, err := doc.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	if !strings.Contains(s, "-3.00 Ts") {
		t.Error("expected negative Ts for subscript")
	}
}

func TestTextRiseCarriedToNewPage(t *testing.T) {
	doc := New(WithCompression(false))
	doc.SetFont("helvetica", "", 12)
	doc.AddPage(A4)
	doc.SetTextRise(4.0)
	doc.AddPage(A4)

	b, err := doc.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	if strings.Count(s, "4.00 Ts") < 2 {
		t.Error("text rise should carry to new page")
	}
}

// --- Typography: Text Rotation ---

func TestTextRotatedAt(t *testing.T) {
	doc := New(WithCompression(false))
	doc.SetFont("helvetica", "", 14)
	page := doc.AddPage(A4)
	page.TextRotatedAt(50, 100, 45, "Rotated!")

	b, err := doc.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	// Should contain q (save), cm (matrix), Tj (text), Q (restore).
	if !strings.Contains(s, " cm\n") {
		t.Error("expected cm operator for rotation matrix")
	}
	if !strings.Contains(s, "(Rotated!) Tj") {
		t.Error("expected text to be drawn")
	}
}

func TestTextRotatedAtZero(t *testing.T) {
	doc := New(WithCompression(false))
	doc.SetFont("helvetica", "", 12)
	page := doc.AddPage(A4)
	page.TextRotatedAt(10, 20, 0, "NoRotation")

	b, err := doc.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	// With 0 degrees, cos=1, sin=0, so matrix is identity-like.
	if !strings.Contains(s, "(NoRotation) Tj") {
		t.Error("expected text output even with zero rotation")
	}
}

func TestTextRotatedAt90(t *testing.T) {
	doc := New(WithCompression(false))
	doc.SetFont("helvetica", "", 12)
	page := doc.AddPage(A4)
	page.TextRotatedAt(50, 50, 90, "Vertical")

	b, err := doc.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	// At 90°, cos≈0, sin≈1 → matrix has b≈1.00000 and c≈-1.00000
	if !strings.Contains(s, "1.00000") {
		t.Error("expected sin(90)=1 in rotation matrix")
	}
}

// --- Typography: Rich Text ---

func TestRichTextBold(t *testing.T) {
	doc := New(WithCompression(false))
	doc.SetFont("helvetica", "", 12)
	page := doc.AddPage(A4)
	page.RichText(6, "Normal <b>bold</b> text")

	b, err := doc.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	// Must contain both regular and bold font references.
	if !strings.Contains(s, "/F1") {
		t.Error("expected regular font F1")
	}
	if !strings.Contains(s, "/F2") {
		t.Error("expected bold font F2")
	}
}

func TestRichTextItalic(t *testing.T) {
	doc := New(WithCompression(false))
	doc.SetFont("helvetica", "", 12)
	page := doc.AddPage(A4)
	page.RichText(6, "Normal <i>italic</i> text")

	b, err := doc.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	if !strings.Contains(s, "(Normal ) Tj") {
		t.Error("expected normal text segment")
	}
	if !strings.Contains(s, "(italic) Tj") {
		t.Error("expected italic text segment")
	}
}

func TestRichTextUnderline(t *testing.T) {
	doc := New(WithCompression(false))
	doc.SetFont("helvetica", "", 12)
	page := doc.AddPage(A4)
	page.RichText(6, "Normal <u>underlined</u> text")

	b, err := doc.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	// Underlined text produces a filled rect for decoration.
	if !strings.Contains(s, "(underlined) Tj") {
		t.Error("expected underlined text segment")
	}
	// Should have decoration rect (re + f operators).
	reCount := strings.Count(s, " re\n")
	if reCount < 1 {
		t.Error("expected decoration rectangle for underline")
	}
}

func TestRichTextStrikethrough(t *testing.T) {
	doc := New(WithCompression(false))
	doc.SetFont("helvetica", "", 12)
	page := doc.AddPage(A4)
	page.RichText(6, "Normal <s>struck</s> text")

	b, err := doc.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	if !strings.Contains(s, "(struck) Tj") {
		t.Error("expected strikethrough text segment")
	}
}

func TestRichTextNested(t *testing.T) {
	doc := New(WithCompression(false))
	doc.SetFont("helvetica", "", 12)
	page := doc.AddPage(A4)
	page.RichText(6, "<b><i>bold-italic</i></b>")

	b, err := doc.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	// Should register helvetica BI variant.
	if !strings.Contains(s, "(bold-italic) Tj") {
		t.Error("expected bold-italic text")
	}
}

func TestRichTextRestoresState(t *testing.T) {
	doc := New(WithCompression(false))
	doc.SetFont("helvetica", "", 12)
	doc.SetUnderline(false)
	page := doc.AddPage(A4)
	page.RichText(6, "<b>bold</b>")

	// After RichText, font style and decoration should be restored.
	if doc.fontStyle != "" {
		t.Errorf("expected empty style after RichText, got %q", doc.fontStyle)
	}
	if doc.underline {
		t.Error("underline should be restored to false")
	}
}

func TestRichTextUnknownTag(t *testing.T) {
	doc := New(WithCompression(false))
	doc.SetFont("helvetica", "", 12)
	page := doc.AddPage(A4)
	page.RichText(6, "Hello <xyz>world")

	b, err := doc.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	// Unknown tags should be output as literal text.
	if !strings.Contains(s, "(Hello ) Tj") {
		t.Error("expected Hello text before unknown tag")
	}
}

// --- Typography: State save/restore ---

func TestTypographyStateSaveRestore(t *testing.T) {
	doc := New(WithCompression(false))
	doc.SetFont("helvetica", "", 12)
	doc.SetCharSpacing(2.0)
	doc.SetWordSpacing(3.0)
	doc.SetTextRise(1.5)

	saved := doc.saveDocState()
	doc.SetCharSpacing(0)
	doc.SetWordSpacing(0)
	doc.SetTextRise(0)

	if doc.charSpacing != 0 {
		t.Error("charSpacing should be 0 after reset")
	}

	doc.restoreDocState(saved)
	if doc.charSpacing != 2.0 {
		t.Errorf("charSpacing not restored: got %f", doc.charSpacing)
	}
	if doc.wordSpacing != 3.0 {
		t.Errorf("wordSpacing not restored: got %f", doc.wordSpacing)
	}
	if doc.textRise != 1.5 {
		t.Errorf("textRise not restored: got %f", doc.textRise)
	}
}

// --- Drawing & Graphics: Arc ---

func TestArcStroke(t *testing.T) {
	doc := New(WithCompression(false))
	doc.SetFont("helvetica", "", 12)
	page := doc.AddPage(A4)
	page.Arc(100, 100, 30, 30, 0, 90, "D")

	b, err := doc.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	// Should contain a move-to, curve-to, and stroke.
	if !strings.Contains(s, " m\n") {
		t.Error("expected m operator for arc start")
	}
	if !strings.Contains(s, " c\n") {
		t.Error("expected c operator for Bézier curve")
	}
	if !strings.Contains(s, "S\n") {
		t.Error("expected S (stroke) operator")
	}
}

func TestArcFill(t *testing.T) {
	doc := New(WithCompression(false))
	doc.SetFont("helvetica", "", 12)
	page := doc.AddPage(A4)
	page.Arc(100, 100, 30, 20, 0, 180, "F")

	b, err := doc.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	// Filled arc should close path (h) and fill (f).
	if !strings.Contains(s, "h\n") {
		t.Error("expected h (close path) for filled arc")
	}
	if !strings.Contains(s, "f\n") {
		t.Error("expected f (fill) for filled arc")
	}
}

func TestArcFullCircle(t *testing.T) {
	doc := New(WithCompression(false))
	doc.SetFont("helvetica", "", 12)
	page := doc.AddPage(A4)
	page.Arc(100, 100, 25, 25, 0, 360, "D")

	b, err := doc.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	// Full circle arc should have 4 curve segments.
	curveCount := strings.Count(s, " c\n")
	if curveCount < 4 {
		t.Errorf("expected at least 4 Bézier curves for 360° arc, got %d", curveCount)
	}
}

func TestArcElliptical(t *testing.T) {
	doc := New(WithCompression(false))
	doc.SetFont("helvetica", "", 12)
	page := doc.AddPage(A4)
	page.Arc(100, 100, 40, 20, 45, 270, "DF")

	b, err := doc.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	if !strings.Contains(s, " c\n") {
		t.Error("expected Bézier curves for elliptical arc")
	}
	if !strings.Contains(s, "B\n") {
		t.Error("expected B (fill+stroke) for DF style")
	}
}

// --- Drawing & Graphics: Clipping ---

func TestClipRect(t *testing.T) {
	doc := New(WithCompression(false))
	doc.SetFont("helvetica", "", 12)
	page := doc.AddPage(A4)

	page.TransformBegin()
	page.ClipRect(10, 10, 100, 50)
	page.Rect(0, 0, 200, 200, "F")
	page.TransformEnd()

	b, err := doc.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	if !strings.Contains(s, "W\n") {
		t.Error("expected W (clip) operator")
	}
	if !strings.Contains(s, "n\n") {
		t.Error("expected n (end path) after clip")
	}
}

func TestClipCircle(t *testing.T) {
	doc := New(WithCompression(false))
	doc.SetFont("helvetica", "", 12)
	page := doc.AddPage(A4)

	page.TransformBegin()
	page.ClipCircle(100, 100, 30)
	page.Rect(70, 70, 60, 60, "F")
	page.TransformEnd()

	b, err := doc.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	// Should have Bézier curves for circular clip and W operator.
	if !strings.Contains(s, " c\n") {
		t.Error("expected Bézier curves for circular clip path")
	}
	if !strings.Contains(s, "W\n") {
		t.Error("expected W (clip) operator")
	}
}

func TestClipEllipse(t *testing.T) {
	doc := New(WithCompression(false))
	doc.SetFont("helvetica", "", 12)
	page := doc.AddPage(A4)

	page.TransformBegin()
	page.ClipEllipse(100, 100, 40, 20)
	page.TextAt(80, 100, "Clipped text")
	page.TransformEnd()

	b, err := doc.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	if !strings.Contains(s, "W\n") {
		t.Error("expected W (clip) operator for ellipse")
	}
	if !strings.Contains(s, "(Clipped text) Tj") {
		t.Error("expected clipped text output")
	}
}

func TestClipPreservesState(t *testing.T) {
	doc := New(WithCompression(false))
	doc.SetFont("helvetica", "", 12)
	page := doc.AddPage(A4)

	page.TransformBegin()
	page.ClipRect(10, 10, 50, 50)
	page.TransformEnd()

	// After TransformEnd, clip should be released. Drawing should still work.
	page.TextAt(10, 10, "After clip")

	b, err := doc.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	// q/Q wrapping should be present.
	qCount := strings.Count(s, "q\n")
	bigQCount := strings.Count(s, "Q\n")
	if qCount < 1 || bigQCount < 1 {
		t.Error("expected q/Q save/restore around clip")
	}
}

// --- Drawing & Graphics: Linear Gradient ---

func TestLinearGradient(t *testing.T) {
	doc := New(WithCompression(false))
	doc.SetFont("helvetica", "", 12)
	page := doc.AddPage(A4)

	page.LinearGradient(10, 10, 100, 50, 10, 10, 110, 10,
		GradientStop(0, 255, 0, 0),
		GradientStop(1, 0, 0, 255),
	)

	b, err := doc.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	if !strings.Contains(s, "/Sh1 sh") {
		t.Error("expected sh operator for shading")
	}
	if !strings.Contains(s, "/ShadingType 2") {
		t.Error("expected ShadingType 2 (axial) for linear gradient")
	}
	if !strings.Contains(s, "/Shading") {
		t.Error("expected /Shading in resource dict")
	}
}

func TestLinearGradientMultiStop(t *testing.T) {
	doc := New(WithCompression(false))
	doc.SetFont("helvetica", "", 12)
	page := doc.AddPage(A4)

	page.LinearGradient(10, 10, 100, 50, 10, 10, 110, 10,
		GradientStop(0, 255, 0, 0),
		GradientStop(0.5, 0, 255, 0),
		GradientStop(1, 0, 0, 255),
	)

	b, err := doc.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	// Multi-stop gradient should use FunctionType 3 (stitching).
	if !strings.Contains(s, "/FunctionType 3") {
		t.Error("expected FunctionType 3 (stitching) for multi-stop gradient")
	}
	if !strings.Contains(s, "/Bounds") {
		t.Error("expected /Bounds for stitching function")
	}
}

// --- Drawing & Graphics: Radial Gradient ---

func TestRadialGradient(t *testing.T) {
	doc := New(WithCompression(false))
	doc.SetFont("helvetica", "", 12)
	page := doc.AddPage(A4)

	page.RadialGradient(10, 10, 100, 100, 60, 60, 50,
		GradientStop(0, 255, 255, 255),
		GradientStop(1, 0, 0, 128),
	)

	b, err := doc.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	if !strings.Contains(s, "/Sh1 sh") {
		t.Error("expected sh operator for radial shading")
	}
	if !strings.Contains(s, "/ShadingType 3") {
		t.Error("expected ShadingType 3 (radial)")
	}
}

func TestMultipleGradients(t *testing.T) {
	doc := New(WithCompression(false))
	doc.SetFont("helvetica", "", 12)
	page := doc.AddPage(A4)

	page.LinearGradient(10, 10, 90, 40, 10, 10, 100, 10,
		GradientStop(0, 255, 0, 0),
		GradientStop(1, 0, 255, 0),
	)
	page.RadialGradient(10, 60, 90, 90, 55, 105, 45,
		GradientStop(0, 255, 255, 0),
		GradientStop(1, 0, 128, 255),
	)

	b, err := doc.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	if !strings.Contains(s, "/Sh1 sh") {
		t.Error("expected Sh1")
	}
	if !strings.Contains(s, "/Sh2 sh") {
		t.Error("expected Sh2")
	}
	if !strings.Contains(s, "/Sh1") && !strings.Contains(s, "/Sh2") {
		t.Error("expected both shadings in resource dict")
	}
}

func TestGradientClipsToRect(t *testing.T) {
	doc := New(WithCompression(false))
	doc.SetFont("helvetica", "", 12)
	page := doc.AddPage(A4)

	page.LinearGradient(20, 20, 80, 40, 20, 20, 100, 20,
		GradientStop(0, 0, 0, 0),
		GradientStop(1, 255, 255, 255),
	)

	b, err := doc.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	// Gradient should be clipped to a rect: q ... re W n ... sh ... Q.
	if !strings.Contains(s, "W\n") {
		t.Error("expected W (clip) operator for gradient clipping rect")
	}
	if !strings.Contains(s, "n\n") {
		t.Error("expected n (end path) after clip")
	}
}

// --- Landscape Orientation ---

func TestLandscapeMethod(t *testing.T) {
	ls := A4.Landscape()
	if ls.WidthPt != A4.HeightPt || ls.HeightPt != A4.WidthPt {
		t.Errorf("Landscape should swap width/height: got %.2f x %.2f", ls.WidthPt, ls.HeightPt)
	}
}

func TestLandscapePresets(t *testing.T) {
	if A4Landscape.WidthPt != A4.HeightPt {
		t.Error("A4Landscape width should equal A4 height")
	}
	if LetterLandscape.HeightPt != Letter.WidthPt {
		t.Error("LetterLandscape height should equal Letter width")
	}
}

func TestLandscapePageMediaBox(t *testing.T) {
	doc := New(WithCompression(false))
	doc.SetFont("helvetica", "", 12)
	doc.AddPage(A4Landscape)

	b, err := doc.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	// A4 landscape: width=841.89, height=595.28
	if !strings.Contains(s, "/MediaBox [0 0 841.89 595.28]") {
		t.Error("expected landscape MediaBox with swapped dimensions")
	}
}

func TestLandscapePageDimensions(t *testing.T) {
	doc := New(WithCompression(false))
	doc.SetFont("helvetica", "", 12)
	page := doc.AddPage(A4Landscape)

	// In mm (default unit), width should be ~297 and height ~210
	w := page.Width()
	h := page.Height()
	if w < 296 || w > 298 {
		t.Errorf("landscape page width should be ~297mm, got %.2f", w)
	}
	if h < 209 || h > 211 {
		t.Errorf("landscape page height should be ~210mm, got %.2f", h)
	}
}

func TestMixedOrientationPages(t *testing.T) {
	doc := New(WithCompression(false))
	doc.SetFont("helvetica", "", 12)
	doc.AddPage(A4)
	doc.AddPage(A4Landscape)
	doc.AddPage(A4)

	b, err := doc.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	// Should have both portrait and landscape MediaBoxes.
	portraitBox := "/MediaBox [0 0 595.28 841.89]"
	landscapeBox := "/MediaBox [0 0 841.89 595.28]"
	if strings.Count(s, portraitBox) != 2 {
		t.Errorf("expected 2 portrait pages, got %d", strings.Count(s, portraitBox))
	}
	if strings.Count(s, landscapeBox) != 1 {
		t.Errorf("expected 1 landscape page, got %d", strings.Count(s, landscapeBox))
	}
}

func TestDoubleLandscapeIsPortrait(t *testing.T) {
	back := A4.Landscape().Landscape()
	if back.WidthPt != A4.WidthPt || back.HeightPt != A4.HeightPt {
		t.Error("double Landscape() should return to portrait")
	}
}

// --- Bookmarks/Outlines ---

func TestBookmarkSingle(t *testing.T) {
	doc := New(WithCompression(false))
	doc.SetFont("helvetica", "", 12)
	page := doc.AddPage(A4)
	doc.AddBookmark("Chapter 1", 0)
	page.TextAt(10, 20, "Chapter 1 content")

	b, err := doc.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	if !strings.Contains(s, "/Type /Outlines") {
		t.Error("expected /Type /Outlines root")
	}
	if !strings.Contains(s, "/Title (Chapter 1)") {
		t.Error("expected bookmark title")
	}
	if !strings.Contains(s, "/Outlines") {
		t.Error("expected /Outlines reference in catalog")
	}
	if !strings.Contains(s, "/PageMode /UseOutlines") {
		t.Error("expected /PageMode /UseOutlines in catalog")
	}
}

func TestBookmarkNested(t *testing.T) {
	doc := New(WithCompression(false))
	doc.SetFont("helvetica", "", 12)
	doc.AddPage(A4)
	doc.AddBookmark("Chapter 1", 0)
	doc.AddBookmark("Section 1.1", 1)
	doc.AddBookmark("Section 1.2", 1)
	doc.AddBookmark("Chapter 2", 0)

	b, err := doc.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	if !strings.Contains(s, "/Title (Chapter 1)") {
		t.Error("expected Chapter 1 bookmark")
	}
	if !strings.Contains(s, "/Title (Section 1.1)") {
		t.Error("expected Section 1.1 bookmark")
	}
	if !strings.Contains(s, "/Title (Chapter 2)") {
		t.Error("expected Chapter 2 bookmark")
	}
	// Nested items should have /Parent, /First, /Last links.
	if !strings.Contains(s, "/First") {
		t.Error("expected /First link in parent bookmark")
	}
	if !strings.Contains(s, "/Last") {
		t.Error("expected /Last link in parent bookmark")
	}
}

func TestBookmarkMultiPage(t *testing.T) {
	doc := New(WithCompression(false))
	doc.SetFont("helvetica", "", 12)
	doc.AddPage(A4)
	doc.AddBookmark("Page 1", 0)
	doc.AddPage(A4)
	doc.AddBookmark("Page 2", 0)

	b, err := doc.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	if !strings.Contains(s, "/Title (Page 1)") {
		t.Error("expected Page 1 bookmark")
	}
	if !strings.Contains(s, "/Title (Page 2)") {
		t.Error("expected Page 2 bookmark")
	}
	// Both should have /Dest entries pointing to different pages.
	destCount := strings.Count(s, "/Dest [")
	if destCount != 2 {
		t.Errorf("expected 2 /Dest entries, got %d", destCount)
	}
}

func TestBookmarkDestination(t *testing.T) {
	doc := New(WithCompression(false))
	doc.SetFont("helvetica", "", 12)
	page := doc.AddPage(A4)
	page.SetY(50) // move cursor to Y=50mm
	doc.AddBookmark("At Y=50", 0)

	b, err := doc.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	// Should have /Dest with /XYZ and a Y coordinate.
	if !strings.Contains(s, "/XYZ 0") {
		t.Error("expected /XYZ destination type")
	}
}

func TestBookmarkSiblingLinks(t *testing.T) {
	doc := New(WithCompression(false))
	doc.SetFont("helvetica", "", 12)
	doc.AddPage(A4)
	doc.AddBookmark("First", 0)
	doc.AddBookmark("Second", 0)
	doc.AddBookmark("Third", 0)

	b, err := doc.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	// Middle item should have both /Prev and /Next.
	if !strings.Contains(s, "/Prev") {
		t.Error("expected /Prev link for sibling navigation")
	}
	if !strings.Contains(s, "/Next") {
		t.Error("expected /Next link for sibling navigation")
	}
}

func TestBookmarkCount(t *testing.T) {
	doc := New(WithCompression(false))
	doc.SetFont("helvetica", "", 12)
	doc.AddPage(A4)
	doc.AddBookmark("Parent", 0)
	doc.AddBookmark("Child 1", 1)
	doc.AddBookmark("Child 2", 1)

	b, err := doc.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	// Outline root count should be 3 (1 parent + 2 children).
	if !strings.Contains(s, "/Count 3") {
		t.Error("expected /Count 3 for outline root (1 parent + 2 children)")
	}
}

func TestNoBookmarks(t *testing.T) {
	doc := New(WithCompression(false))
	doc.SetFont("helvetica", "", 12)
	doc.AddPage(A4)

	b, err := doc.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	// No bookmarks — should not have outlines or UseOutlines.
	if strings.Contains(s, "/Type /Outlines") {
		t.Error("should not have outlines when no bookmarks added")
	}
	if strings.Contains(s, "/PageMode /UseOutlines") {
		t.Error("should not have /PageMode when no bookmarks")
	}
}

// --- SVG Path Import ---

func TestSVGPathTriangle(t *testing.T) {
	doc := New(WithCompression(false))
	doc.SetFont("helvetica", "", 12)
	page := doc.AddPage(A4)
	page.SVGPath(10, 10, 1.0, "M 0 0 L 100 0 L 50 100 Z", "D")

	b, err := doc.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	if !strings.Contains(s, " m\n") {
		t.Error("expected m (moveto) operator")
	}
	if !strings.Contains(s, " l\n") {
		t.Error("expected l (lineto) operator")
	}
	if !strings.Contains(s, "h\n") {
		t.Error("expected h (closepath) operator")
	}
	if !strings.Contains(s, "S\n") {
		t.Error("expected S (stroke) operator")
	}
}

func TestSVGPathRelative(t *testing.T) {
	doc := New(WithCompression(false))
	doc.SetFont("helvetica", "", 12)
	page := doc.AddPage(A4)
	// Relative commands: move to 0,0 then line by +50,+0, +0,+50, close.
	page.SVGPath(10, 10, 1.0, "m 0 0 l 50 0 l 0 50 z", "F")

	b, err := doc.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	if !strings.Contains(s, " l\n") {
		t.Error("expected lineto from relative l commands")
	}
	if !strings.Contains(s, "f\n") {
		t.Error("expected f (fill) operator")
	}
}

func TestSVGPathCubicBezier(t *testing.T) {
	doc := New(WithCompression(false))
	doc.SetFont("helvetica", "", 12)
	page := doc.AddPage(A4)
	page.SVGPath(10, 10, 1.0, "M 0 0 C 25 50 75 50 100 0", "D")

	b, err := doc.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	if !strings.Contains(s, " c\n") {
		t.Error("expected c (curveto) operator for cubic Bézier")
	}
}

func TestSVGPathQuadraticBezier(t *testing.T) {
	doc := New(WithCompression(false))
	doc.SetFont("helvetica", "", 12)
	page := doc.AddPage(A4)
	// Q command should be promoted to cubic C in PDF.
	page.SVGPath(10, 10, 1.0, "M 0 0 Q 50 100 100 0", "D")

	b, err := doc.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	// Should use cubic bezier (c), not quadratic.
	if !strings.Contains(s, " c\n") {
		t.Error("expected c (curveto) from promoted quadratic Q command")
	}
}

func TestSVGPathHV(t *testing.T) {
	doc := New(WithCompression(false))
	doc.SetFont("helvetica", "", 12)
	page := doc.AddPage(A4)
	// H and V are horizontal/vertical line shortcuts.
	page.SVGPath(10, 10, 1.0, "M 0 0 H 100 V 50 H 0 Z", "D")

	b, err := doc.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	// Should have 3 lineto operations (H, V, H) + close.
	lineCount := strings.Count(s, " l\n")
	if lineCount < 3 {
		t.Errorf("expected at least 3 lineto from H/V commands, got %d", lineCount)
	}
}

func TestSVGPathArc(t *testing.T) {
	doc := New(WithCompression(false))
	doc.SetFont("helvetica", "", 12)
	page := doc.AddPage(A4)
	// SVG arc command.
	page.SVGPath(10, 10, 1.0, "M 0 50 A 50 50 0 1 1 100 50", "D")

	b, err := doc.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	// Arc should be decomposed into cubic Bézier curves.
	if !strings.Contains(s, " c\n") {
		t.Error("expected c operators from arc decomposition")
	}
}

func TestSVGPathScale(t *testing.T) {
	doc := New(WithCompression(false))
	doc.SetFont("helvetica", "", 12)
	page := doc.AddPage(A4)
	// Scale factor 0.5 — all coordinates should be halved.
	page.SVGPath(10, 10, 0.5, "M 0 0 L 200 0", "D")

	b, err := doc.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	if err != nil {
		t.Fatal(err)
	}
	_ = string(b)
	// Test that it compiles and produces valid PDF (no assertion on exact coords).
}

func TestSVGPathFillStroke(t *testing.T) {
	doc := New(WithCompression(false))
	doc.SetFont("helvetica", "", 12)
	page := doc.AddPage(A4)
	page.SVGPath(10, 10, 1.0, "M 0 0 L 50 0 L 25 50 Z", "DF")

	b, err := doc.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	if !strings.Contains(s, "B\n") {
		t.Error("expected B (fill+stroke) operator for DF style")
	}
}

func TestSVGPathSmoothCubic(t *testing.T) {
	doc := New(WithCompression(false))
	doc.SetFont("helvetica", "", 12)
	page := doc.AddPage(A4)
	// S is smooth cubic (reflects previous control point).
	page.SVGPath(10, 10, 1.0, "M 0 0 C 10 20 40 20 50 0 S 90 -20 100 0", "D")

	b, err := doc.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	// Should have 2 cubic curves (C + S).
	curveCount := strings.Count(s, " c\n")
	if curveCount < 2 {
		t.Errorf("expected at least 2 curves from C+S, got %d", curveCount)
	}
}

func TestSVGPathEmptyString(t *testing.T) {
	doc := New(WithCompression(false))
	doc.SetFont("helvetica", "", 12)
	page := doc.AddPage(A4)
	page.SVGPath(10, 10, 1.0, "", "D")

	_, err := doc.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	// Should not crash on empty path.
}

// === Page Templates (F4) ===

func TestTemplateBasic(t *testing.T) {
	doc := New(WithCompression(false))
	doc.SetFont("helvetica", "", 12)

	tpl := doc.BeginTemplate(A4)
	tpl.Rect(10, 10, 100, 50, "D")
	name := doc.EndTemplate()

	page := doc.AddPage(A4)
	page.UseTemplate(name, 0, 0, A4.WidthPt/doc.k, A4.HeightPt/doc.k)

	b, err := doc.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)

	if !strings.Contains(s, "/Type /XObject") {
		t.Error("missing XObject type in template")
	}
	if !strings.Contains(s, "/Subtype /Form") {
		t.Error("missing Form subtype in template")
	}
	if !strings.Contains(s, "/BBox [0 0") {
		t.Error("missing BBox in template")
	}
	if !strings.Contains(s, fmt.Sprintf("/%s Do", name)) {
		t.Error("missing Do operator for template")
	}
}

func TestTemplateInResourceDict(t *testing.T) {
	doc := New(WithCompression(false))
	doc.SetFont("helvetica", "", 12)

	tpl := doc.BeginTemplate(A4)
	tpl.SetFillColorRGB(255, 0, 0)
	tpl.Rect(0, 0, 50, 50, "F")
	name := doc.EndTemplate()

	page := doc.AddPage(A4)
	page.UseTemplate(name, 10, 10, 50, 50)

	b, err := doc.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)

	// Template should be in the XObject resource dict.
	if !strings.Contains(s, "/"+name) {
		t.Errorf("template %q not referenced in resource dict", name)
	}
}

func TestTemplateText(t *testing.T) {
	doc := New(WithCompression(false))
	doc.SetFont("helvetica", "", 12)

	tpl := doc.BeginTemplate(A4)
	tpl.TextAt(10, 20, "Template Text")
	name := doc.EndTemplate()

	page := doc.AddPage(A4)
	page.UseTemplate(name, 0, 0, 210, 297)

	b, err := doc.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)

	if !strings.Contains(s, "(Template Text)") {
		t.Error("template should contain text")
	}
}

func TestTemplateWithOwnFont(t *testing.T) {
	doc := New(WithCompression(false))
	doc.SetFont("helvetica", "", 12)

	tpl := doc.BeginTemplate(A4)
	tpl.SetFont("courier", "", 14)
	tpl.TextAt(10, 20, "Courier in template")
	name := doc.EndTemplate()

	page := doc.AddPage(A4)
	page.UseTemplate(name, 0, 0, 210, 297)

	b, err := doc.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)

	if !strings.Contains(s, "/BaseFont /Courier") {
		t.Error("template should register Courier font")
	}
	if !strings.Contains(s, "(Courier in template)") {
		t.Error("template should contain text")
	}
}

func TestTemplateMultipleStamps(t *testing.T) {
	doc := New(WithCompression(false))
	doc.SetFont("helvetica", "", 12)

	tpl := doc.BeginTemplate(PageSize{WidthPt: 100, HeightPt: 50})
	tpl.Rect(0, 0, 100/doc.k, 50/doc.k, "D")
	name := doc.EndTemplate()

	page := doc.AddPage(A4)
	// Stamp same template three times at different positions.
	page.UseTemplate(name, 10, 10, 100/doc.k, 50/doc.k)
	page.UseTemplate(name, 10, 80, 100/doc.k, 50/doc.k)
	page.UseTemplate(name, 10, 150, 100/doc.k, 50/doc.k)

	b, err := doc.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)

	doCount := strings.Count(s, fmt.Sprintf("/%s Do", name))
	if doCount != 3 {
		t.Errorf("expected 3 Do operators, got %d", doCount)
	}
}

func TestTemplateMultipleTemplates(t *testing.T) {
	doc := New(WithCompression(false))
	doc.SetFont("helvetica", "", 12)

	tpl1 := doc.BeginTemplate(A4)
	tpl1.SetFillColorRGB(255, 0, 0)
	tpl1.Rect(0, 0, 50, 50, "F")
	name1 := doc.EndTemplate()

	tpl2 := doc.BeginTemplate(A4)
	tpl2.SetFillColorRGB(0, 0, 255)
	tpl2.Rect(0, 0, 50, 50, "F")
	name2 := doc.EndTemplate()

	if name1 == name2 {
		t.Error("templates should have unique names")
	}

	page := doc.AddPage(A4)
	page.UseTemplate(name1, 10, 10, 50, 50)
	page.UseTemplate(name2, 70, 10, 50, 50)

	b, err := doc.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)

	if !strings.Contains(s, fmt.Sprintf("/%s Do", name1)) {
		t.Errorf("missing Do for %s", name1)
	}
	if !strings.Contains(s, fmt.Sprintf("/%s Do", name2)) {
		t.Errorf("missing Do for %s", name2)
	}
}

func TestTemplateNotFound(t *testing.T) {
	doc := New(WithCompression(false))
	doc.SetFont("helvetica", "", 12)
	page := doc.AddPage(A4)
	page.UseTemplate("nonexistent", 0, 0, 100, 100)

	_, err := doc.Bytes()
	if err == nil {
		t.Fatal("expected error for nonexistent template")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected 'not found' error, got: %v", err)
	}
}

func TestTemplateDrawing(t *testing.T) {
	doc := New(WithCompression(false))
	doc.SetFont("helvetica", "", 12)

	tpl := doc.BeginTemplate(A4)
	tpl.SetDrawColorRGB(0, 128, 0)
	tpl.SetLineWidth(2)
	tpl.Line(10, 10, 100, 100)
	tpl.SetFillColorRGB(200, 200, 200)
	tpl.Rect(20, 20, 60, 40, "F")
	name := doc.EndTemplate()

	page := doc.AddPage(A4)
	page.UseTemplate(name, 0, 0, 210, 297)

	b, err := doc.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)

	// Should have stroke color, fill color, line width, and rect/line ops.
	if !strings.Contains(s, "RG") {
		t.Error("missing stroke color in template")
	}
	if !strings.Contains(s, "rg") {
		t.Error("missing fill color in template")
	}
	if !strings.Contains(s, " w\n") {
		t.Error("missing line width in template")
	}
}

func TestTemplateOnMultiplePages(t *testing.T) {
	doc := New(WithCompression(false))
	doc.SetFont("helvetica", "", 12)

	// Create a header template.
	tpl := doc.BeginTemplate(PageSize{WidthPt: A4.WidthPt, HeightPt: 50})
	tpl.TextAt(10, 10, "Page Header")
	name := doc.EndTemplate()

	// Stamp on multiple pages.
	for i := 0; i < 3; i++ {
		page := doc.AddPage(A4)
		page.UseTemplate(name, 0, 0, A4.WidthPt/doc.k, 50/doc.k)
	}

	b, err := doc.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)

	doCount := strings.Count(s, fmt.Sprintf("/%s Do", name))
	if doCount != 3 {
		t.Errorf("expected 3 Do operators across pages, got %d", doCount)
	}

	// Only one Form XObject should exist.
	formCount := strings.Count(s, "/Subtype /Form")
	if formCount != 1 {
		t.Errorf("expected 1 Form XObject, got %d", formCount)
	}
}

func TestTemplateEndWithNoTemplates(t *testing.T) {
	doc := New(WithCompression(false))
	name := doc.EndTemplate()
	if name != "" {
		t.Errorf("expected empty string from EndTemplate with no templates, got %q", name)
	}
}

// === Table of Contents (F5) ===

func TestTOCBasic(t *testing.T) {
	doc := New(WithCompression(false))
	doc.SetFont("helvetica", "", 12)

	// Create content pages.
	ch1 := doc.AddPage(A4)
	ch1.TextAt(20, 30, "Chapter 1 content")
	ch1Y := 30.0

	ch2 := doc.AddPage(A4)
	ch2.TextAt(20, 30, "Chapter 2 content")
	ch2Y := 30.0

	// Build TOC.
	toc := NewTOC(doc)
	toc.Add("Chapter 1", 0, ch1, ch1Y)
	toc.Add("Chapter 2", 0, ch2, ch2Y)

	// Insert TOC on a new page (page 3, but typically you'd insert before).
	tocPage := doc.AddPage(A4)
	toc.Render(tocPage, 6)

	b, err := doc.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)

	if !strings.Contains(s, "(Chapter 1)") {
		t.Error("TOC should contain 'Chapter 1' text")
	}
	if !strings.Contains(s, "(Chapter 2)") {
		t.Error("TOC should contain 'Chapter 2' text")
	}
	// Should have page numbers.
	if !strings.Contains(s, "(1)") {
		t.Error("TOC should contain page number 1")
	}
	if !strings.Contains(s, "(2)") {
		t.Error("TOC should contain page number 2")
	}
}

func TestTOCWithNestedLevels(t *testing.T) {
	doc := New(WithCompression(false))
	doc.SetFont("helvetica", "", 12)

	p1 := doc.AddPage(A4)
	p1.TextAt(20, 30, "Part 1")

	p2 := doc.AddPage(A4)
	p2.TextAt(20, 30, "Section 1.1")

	p3 := doc.AddPage(A4)
	p3.TextAt(20, 30, "Section 1.2")

	toc := NewTOC(doc)
	toc.Add("Part 1", 0, p1, 30)
	toc.Add("Section 1.1", 1, p2, 30)
	toc.Add("Section 1.2", 1, p3, 30)

	tocPage := doc.AddPage(A4)
	toc.Render(tocPage, 6)

	b, err := doc.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)

	if !strings.Contains(s, "(Part 1)") {
		t.Error("missing Part 1 in TOC")
	}
	if !strings.Contains(s, "(Section 1.1)") {
		t.Error("missing Section 1.1 in TOC")
	}
}

func TestTOCCreatesBookmarks(t *testing.T) {
	doc := New(WithCompression(false))
	doc.SetFont("helvetica", "", 12)

	p1 := doc.AddPage(A4)
	p2 := doc.AddPage(A4)

	toc := NewTOC(doc)
	toc.Add("Chapter 1", 0, p1, 10)
	toc.Add("Chapter 2", 0, p2, 10)

	tocPage := doc.AddPage(A4)
	toc.Render(tocPage, 6)

	b, err := doc.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)

	// Should produce outline objects.
	if !strings.Contains(s, "/Type /Outlines") {
		t.Error("TOC should create PDF outlines")
	}
	if !strings.Contains(s, "/Title (Chapter 1)") {
		t.Error("missing outline title for Chapter 1")
	}
	if !strings.Contains(s, "/Title (Chapter 2)") {
		t.Error("missing outline title for Chapter 2")
	}
}

func TestTOCCreatesLinks(t *testing.T) {
	doc := New(WithCompression(false))
	doc.SetFont("helvetica", "", 12)

	p1 := doc.AddPage(A4)
	p2 := doc.AddPage(A4)

	toc := NewTOC(doc)
	toc.Add("Chapter 1", 0, p1, 20)
	toc.Add("Chapter 2", 0, p2, 20)

	tocPage := doc.AddPage(A4)
	toc.Render(tocPage, 6)

	b, err := doc.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)

	// Should have link annotations with /Dest.
	if !strings.Contains(s, "/Subtype /Link") {
		t.Error("TOC should create link annotations")
	}
	if !strings.Contains(s, "/Dest [") {
		t.Error("TOC links should have /Dest destinations")
	}
}

func TestTOCWithOffset(t *testing.T) {
	doc := New(WithCompression(false))
	doc.SetFont("helvetica", "", 12)

	p1 := doc.AddPage(A4)

	toc := NewTOC(doc)
	toc.Add("Chapter 1", 0, p1, 10)

	tocPage := doc.AddPage(A4)
	toc.RenderWithPageNums(tocPage, 6, 5) // offset by 5

	b, err := doc.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)

	// Page number should be 1+5 = 6.
	if !strings.Contains(s, "(6)") {
		t.Error("expected page number 6 with offset 5")
	}
}

func TestTOCDotLeaders(t *testing.T) {
	doc := New(WithCompression(false))
	doc.SetFont("helvetica", "", 12)

	p1 := doc.AddPage(A4)

	toc := NewTOC(doc)
	toc.Add("Short", 0, p1, 10)

	tocPage := doc.AddPage(A4)
	toc.Render(tocPage, 6)

	b, err := doc.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)

	// Should contain dots in the output.
	dotCount := strings.Count(s, "(...") + strings.Count(s, "(..") + strings.Count(s, "(.")
	if dotCount == 0 {
		// Check for dot sequences in parenthesized strings.
		if !strings.Contains(s, "..") {
			t.Error("TOC should contain dot leaders")
		}
	}
}

func TestTOCEmptyEntries(t *testing.T) {
	doc := New(WithCompression(false))
	doc.SetFont("helvetica", "", 12)

	toc := NewTOC(doc)

	tocPage := doc.AddPage(A4)
	toc.Render(tocPage, 6)

	_, err := doc.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	// Should not crash with empty TOC.
}

func TestTOCNoFont(t *testing.T) {
	doc := New(WithCompression(false))

	p1 := doc.AddPage(A4)

	toc := NewTOC(doc)
	toc.Add("Chapter 1", 0, p1, 10)

	tocPage := doc.AddPage(A4)
	toc.Render(tocPage, 6)

	_, err := doc.Bytes()
	if err == nil {
		t.Fatal("expected error when no font is set")
	}
}

func TestTemplateConcatMatrix(t *testing.T) {
	doc := New(WithCompression(false))
	doc.SetFont("helvetica", "", 12)

	tpl := doc.BeginTemplate(PageSize{WidthPt: 200, HeightPt: 100})
	tpl.Rect(0, 0, 200/doc.k, 100/doc.k, "D")
	name := doc.EndTemplate()

	page := doc.AddPage(A4)
	// Stamp at scaled size (half).
	page.UseTemplate(name, 20, 30, 100/doc.k, 50/doc.k)

	b, err := doc.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)

	// Should contain cm (concat matrix) operator for scaling/positioning.
	if !strings.Contains(s, " cm\n") {
		t.Error("missing cm operator for template transform")
	}
	// Should be wrapped in q/Q save/restore.
	if !strings.Contains(s, "q\n") {
		t.Error("missing q (save state) for template")
	}
	if !strings.Contains(s, "Q\n") {
		t.Error("missing Q (restore state) for template")
	}
}

// === Multi-Column Layout (F6) ===

func TestColumnLayoutBasic(t *testing.T) {
	doc := New(WithCompression(false))
	doc.SetFont("helvetica", "", 12)
	page := doc.AddPage(A4)

	cols := NewColumnLayout(doc, page, 2, 5)
	cols.Begin()

	// Column 1.
	page.Cell(0, 5, "Left column", "", "L", false, 2)

	cols.NextColumn()

	// Column 2.
	page.Cell(0, 5, "Right column", "", "L", false, 2)

	cols.End()

	b, err := doc.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)

	if !strings.Contains(s, "(Left column)") {
		t.Error("missing left column text")
	}
	if !strings.Contains(s, "(Right column)") {
		t.Error("missing right column text")
	}
}

func TestColumnLayoutThreeColumns(t *testing.T) {
	doc := New(WithCompression(false))
	doc.SetFont("helvetica", "", 10)
	page := doc.AddPage(A4)

	cols := NewColumnLayout(doc, page, 3, 3)
	cols.Begin()

	page.Cell(0, 5, "Col1", "", "L", false, 2)
	cols.NextColumn()
	page.Cell(0, 5, "Col2", "", "L", false, 2)
	cols.NextColumn()
	page.Cell(0, 5, "Col3", "", "L", false, 2)

	cols.End()

	b, err := doc.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)

	if !strings.Contains(s, "(Col1)") {
		t.Error("missing Col1")
	}
	if !strings.Contains(s, "(Col2)") {
		t.Error("missing Col2")
	}
	if !strings.Contains(s, "(Col3)") {
		t.Error("missing Col3")
	}
}

func TestColumnLayoutResetsY(t *testing.T) {
	doc := New(WithCompression(false))
	doc.SetFont("helvetica", "", 12)
	page := doc.AddPage(A4)

	cols := NewColumnLayout(doc, page, 2, 5)
	cols.Begin()

	startY := page.GetY()
	page.Cell(0, 10, "Line 1", "", "L", false, 2)
	page.Cell(0, 10, "Line 2", "", "L", false, 2)

	cols.NextColumn()
	// Y should reset to the start position.
	if page.GetY() != startY {
		t.Errorf("Y after NextColumn = %f, want %f", page.GetY(), startY)
	}

	cols.End()

	_, err := doc.Bytes()
	if err != nil {
		t.Fatal(err)
	}
}

func TestColumnLayoutRestoresMargins(t *testing.T) {
	doc := New(WithCompression(false))
	doc.SetFont("helvetica", "", 12)
	page := doc.AddPage(A4)

	origLMargin := doc.lMargin
	origRMargin := doc.rMargin

	cols := NewColumnLayout(doc, page, 2, 5)
	cols.Begin()

	// Margins should be different during column mode.
	if doc.lMargin == origLMargin && doc.rMargin == origRMargin {
		// At least one margin should change for column 0.
	}

	cols.End()

	// After End, margins should be restored.
	if doc.lMargin != origLMargin {
		t.Errorf("lMargin after End = %f, want %f", doc.lMargin, origLMargin)
	}
	if doc.rMargin != origRMargin {
		t.Errorf("rMargin after End = %f, want %f", doc.rMargin, origRMargin)
	}
}

func TestColumnLayoutNewPageWrap(t *testing.T) {
	doc := New(WithCompression(false))
	doc.SetFont("helvetica", "", 12)
	page := doc.AddPage(A4)

	cols := NewColumnLayout(doc, page, 2, 5)
	cols.Begin()

	page.Cell(0, 5, "Col1", "", "L", false, 2)
	cols.NextColumn()
	page.Cell(0, 5, "Col2", "", "L", false, 2)

	// Wrap to new page.
	cols.NextColumn()
	if cols.CurrentColumn() != 0 {
		t.Errorf("column after wrap = %d, want 0", cols.CurrentColumn())
	}
	if doc.PageCount() != 2 {
		t.Errorf("page count after wrap = %d, want 2", doc.PageCount())
	}

	cols.End()

	_, err := doc.Bytes()
	if err != nil {
		t.Fatal(err)
	}
}

func TestColumnLayoutCurrentColumn(t *testing.T) {
	doc := New(WithCompression(false))
	doc.SetFont("helvetica", "", 12)
	page := doc.AddPage(A4)

	cols := NewColumnLayout(doc, page, 3, 5)
	cols.Begin()

	if cols.CurrentColumn() != 0 {
		t.Errorf("initial column = %d, want 0", cols.CurrentColumn())
	}

	cols.NextColumn()
	if cols.CurrentColumn() != 1 {
		t.Errorf("after first next = %d, want 1", cols.CurrentColumn())
	}

	cols.NextColumn()
	if cols.CurrentColumn() != 2 {
		t.Errorf("after second next = %d, want 2", cols.CurrentColumn())
	}

	cols.End()

	_, err := doc.Bytes()
	if err != nil {
		t.Fatal(err)
	}
}

func TestColumnLayoutPage(t *testing.T) {
	doc := New(WithCompression(false))
	doc.SetFont("helvetica", "", 12)
	page := doc.AddPage(A4)

	cols := NewColumnLayout(doc, page, 2, 5)
	cols.Begin()

	origPage := cols.Page()
	cols.NextColumn()
	cols.NextColumn() // wraps to new page
	newPage := cols.Page()

	if origPage == newPage {
		t.Error("Page() should return new page after wrap")
	}

	cols.End()

	_, err := doc.Bytes()
	if err != nil {
		t.Fatal(err)
	}
}

// === Barcode/QR Code (F7) ===

func TestBarcode128Basic(t *testing.T) {
	doc := New(WithCompression(false))
	doc.SetFont("helvetica", "", 12)
	page := doc.AddPage(A4)

	page.Barcode128(20, 30, 80, 20, "ABC-123")

	b, err := doc.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)

	if !strings.Contains(s, "f\n") {
		t.Error("barcode should contain fill operations")
	}
	if !strings.Contains(s, "re\n") {
		t.Error("barcode should contain rect operations")
	}
}

func TestBarcode128NumericCodeC(t *testing.T) {
	doc := New(WithCompression(false))
	doc.SetFont("helvetica", "", 12)
	page := doc.AddPage(A4)

	page.Barcode128(20, 30, 80, 20, "123456")

	b, err := doc.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)

	if !strings.Contains(s, "f\n") {
		t.Error("barcode should contain fill operations")
	}
}

func TestBarcode128EmptyData(t *testing.T) {
	doc := New(WithCompression(false))
	doc.SetFont("helvetica", "", 12)
	page := doc.AddPage(A4)

	page.Barcode128(20, 30, 80, 20, "")

	_, err := doc.Bytes()
	if err == nil {
		t.Fatal("expected error for empty barcode data")
	}
}

func TestBarcodeEAN13(t *testing.T) {
	doc := New(WithCompression(false))
	doc.SetFont("helvetica", "", 12)
	page := doc.AddPage(A4)

	page.BarcodeEAN13(20, 30, 60, 20, "590123412345")

	b, err := doc.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)

	if !strings.Contains(s, "f\n") {
		t.Error("EAN-13 should contain fill operations")
	}
}

func TestBarcodeEAN13With13Digits(t *testing.T) {
	doc := New(WithCompression(false))
	doc.SetFont("helvetica", "", 12)
	page := doc.AddPage(A4)

	page.BarcodeEAN13(20, 30, 60, 20, "5901234123457")

	_, err := doc.Bytes()
	if err != nil {
		t.Fatal(err)
	}
}

func TestBarcodeEAN13InvalidDigits(t *testing.T) {
	doc := New(WithCompression(false))
	doc.SetFont("helvetica", "", 12)
	page := doc.AddPage(A4)

	page.BarcodeEAN13(20, 30, 60, 20, "ABC")

	_, err := doc.Bytes()
	if err == nil {
		t.Fatal("expected error for non-numeric EAN-13 data")
	}
}

func TestQRCodeBasic(t *testing.T) {
	doc := New(WithCompression(false))
	doc.SetFont("helvetica", "", 12)
	page := doc.AddPage(A4)

	page.QRCode(20, 30, 50, "Hello World", 0)

	b, err := doc.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)

	rectCount := strings.Count(s, " re\n")
	if rectCount < 10 {
		t.Errorf("QR code should have many rects, got %d", rectCount)
	}
}

func TestQRCodeMediumEC(t *testing.T) {
	doc := New(WithCompression(false))
	doc.SetFont("helvetica", "", 12)
	page := doc.AddPage(A4)

	page.QRCode(20, 30, 50, "Test data", 1)

	_, err := doc.Bytes()
	if err != nil {
		t.Fatal(err)
	}
}

func TestQRCodeLongData(t *testing.T) {
	doc := New(WithCompression(false))
	doc.SetFont("helvetica", "", 12)
	page := doc.AddPage(A4)

	data := strings.Repeat("A", 200)
	page.QRCode(20, 30, 60, data, 0)

	_, err := doc.Bytes()
	if err != nil {
		t.Fatal(err)
	}
}

func TestQRCodeTooLong(t *testing.T) {
	doc := New(WithCompression(false))
	doc.SetFont("helvetica", "", 12)
	page := doc.AddPage(A4)

	data := strings.Repeat("A", 500)
	page.QRCode(20, 30, 60, data, 0)

	_, err := doc.Bytes()
	if err == nil {
		t.Fatal("expected error for data too long")
	}
}

func TestQRCodeInvalidEC(t *testing.T) {
	doc := New(WithCompression(false))
	doc.SetFont("helvetica", "", 12)
	page := doc.AddPage(A4)

	page.QRCode(20, 30, 50, "test", 5)

	_, err := doc.Bytes()
	if err == nil {
		t.Fatal("expected error for invalid EC level")
	}
}

// === AutoTable (F8) ===

func TestAutoTableFromStructs(t *testing.T) {
	type Item struct {
		Name  string
		Price string
		Qty   int
	}
	data := []Item{
		{"Widget", "9.99", 5},
		{"Gadget", "24.50", 2},
	}

	doc := New(WithCompression(false))
	doc.SetFont("helvetica", "", 10)
	page := doc.AddPage(A4)

	at := AutoTableFromStructs(doc, page, data)
	at.Render()

	b, err := doc.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)

	// Headers should appear.
	if !strings.Contains(s, "(Name)") {
		t.Error("missing header 'Name'")
	}
	if !strings.Contains(s, "(Price)") {
		t.Error("missing header 'Price'")
	}
	if !strings.Contains(s, "(Qty)") {
		t.Error("missing header 'Qty'")
	}
	// Data should appear.
	if !strings.Contains(s, "(Widget)") {
		t.Error("missing data 'Widget'")
	}
	if !strings.Contains(s, "(Gadget)") {
		t.Error("missing data 'Gadget'")
	}
}

func TestAutoTableFromJSON(t *testing.T) {
	jsonData := []byte(`[
		{"id": 1, "name": "Alice", "score": 95},
		{"id": 2, "name": "Bob", "score": 87}
	]`)

	doc := New(WithCompression(false))
	doc.SetFont("helvetica", "", 10)
	page := doc.AddPage(A4)

	at, err := AutoTableFromJSON(doc, page, jsonData)
	if err != nil {
		t.Fatal(err)
	}
	at.Render()

	b, err := doc.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)

	if !strings.Contains(s, "(Alice)") {
		t.Error("missing data 'Alice'")
	}
	if !strings.Contains(s, "(Bob)") {
		t.Error("missing data 'Bob'")
	}
}

func TestAutoTableInvalidJSON(t *testing.T) {
	doc := New(WithCompression(false))
	doc.SetFont("helvetica", "", 10)
	page := doc.AddPage(A4)

	_, err := AutoTableFromJSON(doc, page, []byte("invalid"))
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestAutoTableAutoFitWidths(t *testing.T) {
	type Row struct {
		Short    string
		LongName string
	}
	data := []Row{
		{"A", "This is a longer text"},
		{"B", "Another long text"},
	}

	doc := New(WithCompression(false))
	doc.SetFont("helvetica", "", 10)
	page := doc.AddPage(A4)

	at := AutoTableFromStructs(doc, page, data)
	at.AutoFitWidths(190)
	at.Render()

	_, err := doc.Bytes()
	if err != nil {
		t.Fatal(err)
	}
}

func TestAutoTableEmptySlice(t *testing.T) {
	type Item struct{ Name string }
	var data []Item

	doc := New(WithCompression(false))
	doc.SetFont("helvetica", "", 10)
	page := doc.AddPage(A4)

	at := AutoTableFromStructs(doc, page, data)
	at.Render()

	_, err := doc.Bytes()
	if err != nil {
		t.Fatal(err)
	}
}

func TestAutoTableNotSlice(t *testing.T) {
	doc := New(WithCompression(false))
	doc.SetFont("helvetica", "", 10)
	page := doc.AddPage(A4)

	AutoTableFromStructs(doc, page, "not a slice")

	_, err := doc.Bytes()
	if err == nil {
		t.Fatal("expected error for non-slice input")
	}
}

func TestAutoTableWithStyles(t *testing.T) {
	type Item struct {
		Name  string
		Value string
	}
	data := []Item{{"X", "1"}, {"Y", "2"}}

	doc := New(WithCompression(false))
	doc.SetFont("helvetica", "", 10)
	page := doc.AddPage(A4)

	at := AutoTableFromStructs(doc, page, data)
	at.SetHeaderStyle(CellStyle{
		FontFamily: "helvetica",
		FontStyle:  "B",
		FontSize:   11,
		Fill:       true,
		FillColor:  [3]int{200, 200, 200},
	})
	at.SetRowHeight(7)
	at.SetBorder("1")
	at.Render()

	_, err := doc.Bytes()
	if err != nil {
		t.Fatal(err)
	}
}

// === Password Protection (F9) ===

func TestEncryptionBasic(t *testing.T) {
	doc := New(WithCompression(false))
	doc.SetFont("helvetica", "", 12)
	doc.SetProtection("user", "owner", PermAll)
	page := doc.AddPage(A4)
	page.TextAt(20, 30, "Secret content")

	b, err := doc.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)

	if !strings.Contains(s, "/Filter /Standard") {
		t.Error("missing /Filter /Standard in encrypt dict")
	}
	if !strings.Contains(s, "/V 1") {
		t.Error("missing /V 1 in encrypt dict")
	}
	if !strings.Contains(s, "/R 2") {
		t.Error("missing /R 2 in encrypt dict")
	}
	if !strings.Contains(s, "/O <") {
		t.Error("missing /O hash in encrypt dict")
	}
	if !strings.Contains(s, "/U <") {
		t.Error("missing /U hash in encrypt dict")
	}
	if !strings.Contains(s, "/Encrypt") {
		t.Error("missing /Encrypt in trailer")
	}
	if !strings.Contains(s, "/ID [") {
		t.Error("missing /ID in trailer")
	}
}

func TestEncryptionOwnerOnly(t *testing.T) {
	doc := New(WithCompression(false))
	doc.SetFont("helvetica", "", 12)
	doc.SetProtection("", "owner123", PermPrint|PermCopy)
	page := doc.AddPage(A4)
	page.TextAt(20, 30, "Open without password")

	b, err := doc.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)

	if !strings.Contains(s, "/Filter /Standard") {
		t.Error("missing encryption filter")
	}
}

func TestEncryptionStreamEncrypted(t *testing.T) {
	doc := New(WithCompression(false))
	doc.SetFont("helvetica", "", 12)
	doc.SetProtection("pass", "owner", PermAll)
	page := doc.AddPage(A4)
	page.TextAt(20, 30, "Encrypted text")

	b, err := doc.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)

	// The plaintext "Encrypted text" should NOT appear in the raw PDF
	// because streams are RC4-encrypted.
	if strings.Contains(s, "(Encrypted text)") {
		t.Error("plaintext should not appear in encrypted PDF streams")
	}
}

func TestEncryptionPermissions(t *testing.T) {
	doc := New(WithCompression(false))
	doc.SetFont("helvetica", "", 12)
	doc.SetProtection("", "owner", PermPrint) // only print
	doc.AddPage(A4)

	b, err := doc.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)

	if !strings.Contains(s, "/P ") {
		t.Error("missing /P permissions in encrypt dict")
	}
}

func TestEncryptionDoesNotBreakStructure(t *testing.T) {
	doc := New(WithCompression(false))
	doc.SetFont("helvetica", "", 12)
	doc.SetProtection("test", "test", PermAll)
	page := doc.AddPage(A4)
	page.TextAt(20, 30, "Hello")

	b, err := doc.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)

	// Basic PDF structure should still be present.
	if !strings.HasPrefix(s, "%PDF-1.4") {
		t.Error("missing PDF header")
	}
	if !strings.Contains(s, "/Type /Page") {
		t.Error("missing Page object")
	}
	if !strings.Contains(s, "/Type /Catalog") {
		t.Error("missing Catalog")
	}
	if !strings.Contains(s, "%%EOF") {
		t.Error("missing EOF marker")
	}
}

func TestNoEncryptionDefault(t *testing.T) {
	doc := New(WithCompression(false))
	doc.SetFont("helvetica", "", 12)
	page := doc.AddPage(A4)
	page.TextAt(20, 30, "No encryption")

	b, err := doc.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)

	if strings.Contains(s, "/Filter /Standard") {
		t.Error("should not have encryption when not set")
	}
	if strings.Contains(s, "/Encrypt") {
		t.Error("should not have /Encrypt in trailer when not set")
	}
}

// === AcroForms (F10) ===

func TestFormTextField(t *testing.T) {
	doc := New(WithCompression(false))
	doc.SetFont("helvetica", "", 12)
	page := doc.AddPage(A4)

	page.FormTextField("name", 20, 30, 100, 20)

	b, err := doc.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)

	if !strings.Contains(s, "/Subtype /Widget") {
		t.Error("missing Widget annotation")
	}
	if !strings.Contains(s, "/FT /Tx") {
		t.Error("missing text field type")
	}
	if !strings.Contains(s, "/AcroForm") {
		t.Error("missing AcroForm in catalog")
	}
}

func TestFormTextFieldWithDefault(t *testing.T) {
	doc := New(WithCompression(false))
	doc.SetFont("helvetica", "", 12)
	page := doc.AddPage(A4)

	page.FormTextField("email", 20, 30, 100, 20,
		WithDefaultValue("test@example.com"),
		WithMaxLen(50),
	)

	b, err := doc.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)

	if !strings.Contains(s, "/V (test@example.com)") {
		t.Error("missing default value")
	}
	if !strings.Contains(s, "/MaxLen 50") {
		t.Error("missing MaxLen")
	}
}

func TestFormCheckbox(t *testing.T) {
	doc := New(WithCompression(false))
	doc.SetFont("helvetica", "", 12)
	page := doc.AddPage(A4)

	page.FormCheckbox("agree", 20, 30, 12, true)

	b, err := doc.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)

	if !strings.Contains(s, "/FT /Btn") {
		t.Error("missing button field type")
	}
	if !strings.Contains(s, "/V /Yes") {
		t.Error("missing checked value")
	}
}

func TestFormCheckboxUnchecked(t *testing.T) {
	doc := New(WithCompression(false))
	doc.SetFont("helvetica", "", 12)
	page := doc.AddPage(A4)

	page.FormCheckbox("agree", 20, 30, 12, false)

	b, err := doc.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)

	if !strings.Contains(s, "/V /Off") {
		t.Error("unchecked checkbox should have /V /Off")
	}
}

func TestFormDropdown(t *testing.T) {
	doc := New(WithCompression(false))
	doc.SetFont("helvetica", "", 12)
	page := doc.AddPage(A4)

	page.FormDropdown("country", 20, 30, 100, 20, []string{"US", "UK", "TH"})

	b, err := doc.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)

	if !strings.Contains(s, "/FT /Ch") {
		t.Error("missing choice field type")
	}
	if !strings.Contains(s, "/Opt [") {
		t.Error("missing options")
	}
}

func TestFormMultipleFields(t *testing.T) {
	doc := New(WithCompression(false))
	doc.SetFont("helvetica", "", 12)
	page := doc.AddPage(A4)

	page.FormTextField("name", 20, 30, 100, 20)
	page.FormCheckbox("agree", 20, 60, 12, true)
	page.FormDropdown("role", 20, 80, 100, 20, []string{"Admin", "User"})

	b, err := doc.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)

	if !strings.Contains(s, "/FT /Tx") {
		t.Error("missing text field")
	}
	if !strings.Contains(s, "/FT /Btn") {
		t.Error("missing button field")
	}
	if !strings.Contains(s, "/FT /Ch") {
		t.Error("missing choice field")
	}
	// Should have 3 fields in AcroForm.
	if strings.Count(s, "/Subtype /Widget") != 3 {
		t.Errorf("expected 3 widget annotations, got %d", strings.Count(s, "/Subtype /Widget"))
	}
}

func TestFormNoFields(t *testing.T) {
	doc := New(WithCompression(false))
	doc.SetFont("helvetica", "", 12)
	doc.AddPage(A4)

	b, err := doc.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)

	if strings.Contains(s, "/AcroForm") {
		t.Error("should not have AcroForm when no fields exist")
	}
}

func TestFormAppearanceStream(t *testing.T) {
	doc := New(WithCompression(false))
	doc.SetFont("helvetica", "", 12)
	page := doc.AddPage(A4)

	page.FormTextField("test", 20, 30, 100, 20)

	b, err := doc.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)

	// Should have appearance stream (Form XObject for the field).
	if !strings.Contains(s, "/AP <<") {
		t.Error("missing appearance dictionary")
	}
}

// === Digital Signatures (F11) ===

func TestSignatureBasic(t *testing.T) {
	doc := New(WithCompression(false))
	doc.SetFont("helvetica", "", 12)
	page := doc.AddPage(A4)
	page.TextAt(20, 30, "Signed document")

	// Generate a self-signed cert for testing.
	cert, key := generateTestCert(t)

	doc.Sign(cert, key, page, 20, 250, 100, 30, SignOptions{
		Name:     "Test Signer",
		Reason:   "Testing",
		Location: "Bangkok",
	})

	b, err := doc.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)

	if !strings.Contains(s, "/Type /Sig") {
		t.Error("missing signature value object")
	}
	if !strings.Contains(s, "/Filter /Adobe.PPKLite") {
		t.Error("missing signature filter")
	}
	if !strings.Contains(s, "/SubFilter /adbe.pkcs7.detached") {
		t.Error("missing signature sub-filter")
	}
	if !strings.Contains(s, "/FT /Sig") {
		t.Error("missing signature field type")
	}
	if !strings.Contains(s, "(Test Signer)") {
		t.Error("missing signer name")
	}
	if !strings.Contains(s, "(Testing)") {
		t.Error("missing reason")
	}
	if !strings.Contains(s, "/ByteRange") {
		t.Error("missing ByteRange")
	}
}

func TestSignatureInAcroForm(t *testing.T) {
	doc := New(WithCompression(false))
	doc.SetFont("helvetica", "", 12)
	page := doc.AddPage(A4)

	cert, key := generateTestCert(t)
	doc.Sign(cert, key, page, 10, 250, 80, 25, SignOptions{Name: "Signer"})

	b, err := doc.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)

	if !strings.Contains(s, "/AcroForm") {
		t.Error("signature field should be in AcroForm")
	}
}

func TestNoSignatureDefault(t *testing.T) {
	doc := New(WithCompression(false))
	doc.SetFont("helvetica", "", 12)
	doc.AddPage(A4)

	b, err := doc.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)

	if strings.Contains(s, "/Type /Sig") {
		t.Error("should not have signature when not configured")
	}
}

func TestSignatureWithFormFields(t *testing.T) {
	doc := New(WithCompression(false))
	doc.SetFont("helvetica", "", 12)
	page := doc.AddPage(A4)

	page.FormTextField("name", 20, 30, 100, 20)

	cert, key := generateTestCert(t)
	doc.Sign(cert, key, page, 20, 250, 100, 30, SignOptions{Name: "Signer"})

	b, err := doc.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)

	// Should have both form field and signature.
	if !strings.Contains(s, "/FT /Tx") {
		t.Error("missing text field")
	}
	if !strings.Contains(s, "/FT /Sig") {
		t.Error("missing signature field")
	}
}

// generateTestCert creates a self-signed certificate for testing.
func generateTestCert(t *testing.T) (*x509.Certificate, crypto.Signer) {
	t.Helper()

	keyData, err := ecdsa.GenerateKey(elliptic.P256(), randReader)
	if err != nil {
		t.Fatal(err)
	}

	template := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "Test"},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
	}

	certDER, err := x509.CreateCertificate(randReader, template, template, &keyData.PublicKey, keyData)
	if err != nil {
		t.Fatal(err)
	}

	cert, err := x509.ParseCertificate(certDER)
	if err != nil {
		t.Fatal(err)
	}

	return cert, keyData
}

// --------------- F15: HTML-to-PDF ---------------

func TestHTMLParagraph(t *testing.T) {
	doc := New(WithCompression(false))
	doc.SetFont("helvetica", "", 12)
	p := doc.AddPage(A4)

	p.HTML("<p>Hello World</p>")

	data, err := doc.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	out := string(data)
	if !strings.Contains(out, "(Hello World)") {
		t.Error("expected paragraph text in output")
	}
}

func TestHTMLHeadings(t *testing.T) {
	doc := New(WithCompression(false))
	doc.SetFont("helvetica", "", 12)
	p := doc.AddPage(A4)

	p.HTML("<h1>Title</h1><h2>Subtitle</h2>")

	data, err := doc.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	out := string(data)
	if !strings.Contains(out, "(Title)") {
		t.Error("expected h1 text")
	}
	if !strings.Contains(out, "(Subtitle)") {
		t.Error("expected h2 text")
	}
}

func TestHTMLBoldItalic(t *testing.T) {
	doc := New(WithCompression(false))
	doc.SetFont("helvetica", "", 12)
	p := doc.AddPage(A4)

	p.HTML("<p><b>Bold</b> and <i>Italic</i></p>")

	data, err := doc.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	out := string(data)
	if !strings.Contains(out, "(Bold)") {
		t.Error("expected bold text")
	}
	if !strings.Contains(out, "(Italic)") {
		t.Error("expected italic text")
	}
}

func TestHTMLUnderline(t *testing.T) {
	doc := New(WithCompression(false))
	doc.SetFont("helvetica", "", 12)
	p := doc.AddPage(A4)

	p.HTML("<p><u>Underlined</u></p>")

	data, err := doc.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	out := string(data)
	if !strings.Contains(out, "(Underlined)") {
		t.Error("expected underlined text")
	}
}

func TestHTMLLineBreak(t *testing.T) {
	doc := New(WithCompression(false))
	doc.SetFont("helvetica", "", 12)
	p := doc.AddPage(A4)

	p.HTML("<p>Line one<br/>Line two</p>")

	data, err := doc.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	out := string(data)
	if !strings.Contains(out, "(Line one)") {
		t.Error("expected first line")
	}
	if !strings.Contains(out, "(Line two)") {
		t.Error("expected second line")
	}
}

func TestHTMLHorizontalRule(t *testing.T) {
	doc := New(WithCompression(false))
	doc.SetFont("helvetica", "", 12)
	p := doc.AddPage(A4)

	p.HTML("<p>Above</p><hr/><p>Below</p>")

	data, err := doc.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	out := string(data)
	if !strings.Contains(out, " l\nS\n") {
		t.Error("expected line stroke for hr")
	}
}

func TestHTMLUnorderedList(t *testing.T) {
	doc := New(WithCompression(false))
	doc.SetFont("helvetica", "", 12)
	p := doc.AddPage(A4)

	p.HTML("<ul><li>Apple</li><li>Banana</li></ul>")

	data, err := doc.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	out := string(data)
	if !strings.Contains(out, "(Apple)") {
		t.Error("expected list item Apple")
	}
	if !strings.Contains(out, "(Banana)") {
		t.Error("expected list item Banana")
	}
}

func TestHTMLOrderedList(t *testing.T) {
	doc := New(WithCompression(false))
	doc.SetFont("helvetica", "", 12)
	p := doc.AddPage(A4)

	p.HTML("<ol><li>First</li><li>Second</li></ol>")

	data, err := doc.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	out := string(data)
	if !strings.Contains(out, "(First)") {
		t.Error("expected ordered list item")
	}
}

func TestHTMLLink(t *testing.T) {
	doc := New(WithCompression(false))
	doc.SetFont("helvetica", "", 12)
	p := doc.AddPage(A4)

	p.HTML(`<p>Visit <a href="https://example.com">Example</a></p>`)

	data, err := doc.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	out := string(data)
	if !strings.Contains(out, "(Example)") {
		t.Error("expected link text")
	}
	if !strings.Contains(out, "/URI (https://example.com)") {
		t.Error("expected URI annotation")
	}
}

func TestHTMLTable(t *testing.T) {
	doc := New(WithCompression(false))
	doc.SetFont("helvetica", "", 12)
	p := doc.AddPage(A4)

	p.HTML(`<table>
		<tr><td>Name</td><td>Age</td></tr>
		<tr><td>Alice</td><td>30</td></tr>
	</table>`)

	data, err := doc.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	out := string(data)
	if !strings.Contains(out, "(Name)") {
		t.Error("expected table header Name")
	}
	if !strings.Contains(out, "(Alice)") {
		t.Error("expected table cell Alice")
	}
}

func TestHTMLInlineColor(t *testing.T) {
	doc := New(WithCompression(false))
	doc.SetFont("helvetica", "", 12)
	p := doc.AddPage(A4)

	p.HTML(`<p style="color: red">Red text</p>`)

	data, err := doc.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	out := string(data)
	if !strings.Contains(out, "(Red text)") {
		t.Error("expected colored text")
	}
	// Red color: 1.000 0.000 0.000 rg
	if !strings.Contains(out, "1.000 0.000 0.000 rg") {
		t.Error("expected red text color")
	}
}

func TestHTMLParser(t *testing.T) {
	nodes := parseHTML("<p>Hello <b>World</b></p>")

	if len(nodes) != 1 {
		t.Fatalf("expected 1 node, got %d", len(nodes))
	}
	if nodes[0].tag != "p" {
		t.Errorf("expected <p>, got <%s>", nodes[0].tag)
	}
	if len(nodes[0].children) != 2 {
		t.Fatalf("expected 2 children, got %d", len(nodes[0].children))
	}
	if nodes[0].children[0].text != "Hello " {
		t.Errorf("expected text 'Hello ', got %q", nodes[0].children[0].text)
	}
	if nodes[0].children[1].tag != "b" {
		t.Errorf("expected <b>, got <%s>", nodes[0].children[1].tag)
	}
}

func TestHTMLEntities(t *testing.T) {
	nodes := parseHTML("<p>&amp; &lt; &gt;</p>")
	if len(nodes) == 0 || len(nodes[0].children) == 0 {
		t.Fatal("expected parsed content")
	}
	text := nodes[0].children[0].text
	if text != "& < >" {
		t.Errorf("expected '& < >', got %q", text)
	}
}

func TestCSSColorParser(t *testing.T) {
	tests := []struct {
		input    string
		r, g, b  int
		ok       bool
	}{
		{"red", 255, 0, 0, true},
		{"#ff0000", 255, 0, 0, true},
		{"#f00", 255, 0, 0, true},
		{"invalid", 0, 0, 0, false},
	}
	for _, tc := range tests {
		r, g, b, ok := parseCSSColor(tc.input)
		if ok != tc.ok || r != tc.r || g != tc.g || b != tc.b {
			t.Errorf("parseCSSColor(%q) = (%d,%d,%d,%v), want (%d,%d,%d,%v)",
				tc.input, r, g, b, ok, tc.r, tc.g, tc.b, tc.ok)
		}
	}
}

// --------------- F14: Markdown Renderer ---------------

func TestMarkdownHeadings(t *testing.T) {
	doc := New(WithCompression(false))
	doc.SetFont("helvetica", "", 12)
	p := doc.AddPage(A4)

	p.Markdown("# Heading 1\n\n## Heading 2\n\n### Heading 3")

	data, err := doc.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	out := string(data)
	if !strings.Contains(out, "(Heading 1)") {
		t.Error("expected Heading 1 in output")
	}
	if !strings.Contains(out, "(Heading 2)") {
		t.Error("expected Heading 2 in output")
	}
	if !strings.Contains(out, "(Heading 3)") {
		t.Error("expected Heading 3 in output")
	}
}

func TestMarkdownBoldItalic(t *testing.T) {
	doc := New(WithCompression(false))
	doc.SetFont("helvetica", "", 12)
	p := doc.AddPage(A4)

	p.Markdown("This is **bold** and *italic* text.")

	data, err := doc.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	out := string(data)
	if !strings.Contains(out, "(bold)") {
		t.Error("expected bold text in output")
	}
	if !strings.Contains(out, "(italic)") {
		t.Error("expected italic text in output")
	}
}

func TestMarkdownCode(t *testing.T) {
	doc := New(WithCompression(false))
	doc.SetFont("helvetica", "", 12)
	p := doc.AddPage(A4)

	p.Markdown("Use `fmt.Println` to print.")

	data, err := doc.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	out := string(data)
	if !strings.Contains(out, "(fmt.Println)") {
		t.Error("expected code text in output")
	}
}

func TestMarkdownUnorderedList(t *testing.T) {
	doc := New(WithCompression(false))
	doc.SetFont("helvetica", "", 12)
	p := doc.AddPage(A4)

	p.Markdown("- Item one\n- Item two\n- Item three")

	data, err := doc.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	out := string(data)
	if !strings.Contains(out, "(Item one)") {
		t.Error("expected list item one")
	}
	if !strings.Contains(out, "(Item three)") {
		t.Error("expected list item three")
	}
}

func TestMarkdownOrderedList(t *testing.T) {
	doc := New(WithCompression(false))
	doc.SetFont("helvetica", "", 12)
	p := doc.AddPage(A4)

	p.Markdown("1. First\n2. Second\n3. Third")

	data, err := doc.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	out := string(data)
	if !strings.Contains(out, "(First)") {
		t.Error("expected ordered list item")
	}
}

func TestMarkdownHorizontalRule(t *testing.T) {
	doc := New(WithCompression(false))
	doc.SetFont("helvetica", "", 12)
	p := doc.AddPage(A4)

	p.Markdown("Above\n\n---\n\nBelow")

	data, err := doc.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	out := string(data)
	if !strings.Contains(out, "(Above)") {
		t.Error("expected text above rule")
	}
	if !strings.Contains(out, "(Below)") {
		t.Error("expected text below rule")
	}
	// Horizontal rule uses line operator
	if !strings.Contains(out, " l\nS\n") {
		t.Error("expected line stroke for horizontal rule")
	}
}

func TestMarkdownLink(t *testing.T) {
	doc := New(WithCompression(false))
	doc.SetFont("helvetica", "", 12)
	p := doc.AddPage(A4)

	p.Markdown("Visit [Folio](https://example.com) now.")

	data, err := doc.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	out := string(data)
	if !strings.Contains(out, "(Folio)") {
		t.Error("expected link text in output")
	}
	if !strings.Contains(out, "/URI (https://example.com)") {
		t.Error("expected URI annotation")
	}
}

func TestMarkdownWithBookmarks(t *testing.T) {
	doc := New(WithCompression(false))
	doc.SetFont("helvetica", "", 12)
	p := doc.AddPage(A4)

	p.Markdown("# Chapter 1\n\nContent.\n\n## Section 1.1", WithBookmarks())

	data, err := doc.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	out := string(data)
	// Bookmarks generate /Type /Outlines
	if !strings.Contains(out, "/Type /Outlines") {
		t.Error("expected outline tree for bookmarks")
	}
	if !strings.Contains(out, "(Chapter 1)") {
		t.Error("expected Chapter 1 bookmark title")
	}
}

func TestMDBlockParser(t *testing.T) {
	md := "# Title\n\nParagraph text.\n\n- a\n- b\n\n1. one\n2. two\n\n---\n"
	blocks := parseMDBlocks(md)

	expected := []string{"h1", "p", "ul", "ol", "hr"}
	if len(blocks) != len(expected) {
		t.Fatalf("expected %d blocks, got %d", len(expected), len(blocks))
	}
	for i, b := range blocks {
		if b.kind != expected[i] {
			t.Errorf("block %d: expected kind %q, got %q", i, expected[i], b.kind)
		}
	}
}

func TestMDInlineParser(t *testing.T) {
	inlines := parseInline("Hello **bold** and *italic* with `code` and [link](url)")

	kinds := []string{}
	for _, inl := range inlines {
		kinds = append(kinds, inl.kind)
	}

	expected := []string{"text", "bold", "text", "italic", "text", "code", "text", "link"}
	if len(kinds) != len(expected) {
		t.Fatalf("expected %d inlines, got %d: %v", len(expected), len(kinds), kinds)
	}
	for i, k := range kinds {
		if k != expected[i] {
			t.Errorf("inline %d: expected %q, got %q", i, expected[i], k)
		}
	}

	// Check link details.
	link := inlines[len(inlines)-1]
	if link.text != "link" || link.url != "url" {
		t.Errorf("expected link text='link' url='url', got text=%q url=%q", link.text, link.url)
	}
}

// --------------- F13: Fluent Builder API ---------------

func TestTextBuilder(t *testing.T) {
	doc := New(WithCompression(false))
	doc.SetFont("helvetica", "", 12)
	p := doc.AddPage(A4)

	p.Text("Hello Builder").At(20, 30).Draw()

	data, err := doc.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	out := string(data)
	if !strings.Contains(out, "(Hello Builder)") {
		t.Error("expected text in output")
	}
}

func TestTextBuilderBoldColor(t *testing.T) {
	doc := New(WithCompression(false))
	doc.SetFont("helvetica", "", 12)
	p := doc.AddPage(A4)

	p.Text("Bold Red").Bold().Color(255, 0, 0).At(10, 10).Draw()

	data, err := doc.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	out := string(data)
	if !strings.Contains(out, "(Bold Red)") {
		t.Error("expected text in output")
	}
	// Font should be restored to regular after Draw
	if doc.fontStyle != "" {
		t.Errorf("expected font style restored to empty, got %q", doc.fontStyle)
	}
}

func TestTextBuilderSize(t *testing.T) {
	doc := New(WithCompression(false))
	doc.SetFont("helvetica", "", 12)
	p := doc.AddPage(A4)

	p.Text("Big").Size(24).At(10, 10).Draw()

	// Font size should be restored
	if doc.fontSizePt != 12 {
		t.Errorf("expected font size restored to 12, got %.1f", doc.fontSizePt)
	}
}

func TestShapeBuilderRect(t *testing.T) {
	doc := New(WithCompression(false))
	p := doc.AddPage(A4)

	p.Shape().Rect(10, 10, 50, 30).FillColor(255, 0, 0).Fill().Draw()

	data, err := doc.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	out := string(data)
	// Should contain a filled rect (f operator)
	if !strings.Contains(out, " re\nf\n") {
		t.Error("expected filled rectangle in output")
	}
}

func TestShapeBuilderCircle(t *testing.T) {
	doc := New(WithCompression(false))
	p := doc.AddPage(A4)

	p.Shape().Circle(50, 50, 20).StrokeColor(0, 0, 255).Stroke().Draw()

	data, err := doc.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	out := string(data)
	// Circle uses cubic bezier curves (c operator)
	if !strings.Contains(out, " c\n") {
		t.Error("expected bezier curve in output (circle)")
	}
}

func TestShapeBuilderLine(t *testing.T) {
	doc := New(WithCompression(false))
	p := doc.AddPage(A4)

	p.Shape().Line(10, 10, 100, 100).LineWidth(2).Draw()

	data, err := doc.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	out := string(data)
	if !strings.Contains(out, " l\n") {
		t.Error("expected line operator in output")
	}
	// Line width should be restored
	if doc.lineWidth != 0.2 {
		t.Errorf("expected line width restored to 0.2, got %f", doc.lineWidth)
	}
}

func TestShapeBuilderFillStroke(t *testing.T) {
	doc := New(WithCompression(false))
	p := doc.AddPage(A4)

	p.Shape().Rect(10, 10, 50, 30).
		FillColor(200, 200, 200).
		StrokeColor(0, 0, 0).
		FillStroke().
		Draw()

	data, err := doc.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	out := string(data)
	// Should contain fill+stroke rect (B operator)
	if !strings.Contains(out, " re\nB\n") {
		t.Error("expected fill+stroke rectangle in output")
	}
}

func TestShapeBuilderEllipse(t *testing.T) {
	doc := New(WithCompression(false))
	p := doc.AddPage(A4)

	p.Shape().Ellipse(50, 50, 30, 20).Fill().Draw()

	data, err := doc.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	out := string(data)
	if !strings.Contains(out, " c\n") {
		t.Error("expected bezier curves in output (ellipse)")
	}
}

// --------------- F12: PDF/A Compliance ---------------

func TestPDFABasic1b(t *testing.T) {
	doc := New(WithPDFA("1b"), WithCompression(false))
	doc.SetTitle("PDF/A Test")
	doc.SetAuthor("Test Author")

	// PDF/A requires embedded fonts — use a TTF font.
	sarabunData, err := os.ReadFile("fonts/sarabun/Sarabun-Regular.ttf")
	if err != nil {
		t.Skip("Skipping: Sarabun-Regular.ttf not available")
	}
	if err := doc.AddUTF8Font("sarabun", "", sarabunData); err != nil {
		t.Fatal(err)
	}
	doc.SetFont("sarabun", "", 14)

	p := doc.AddPage(A4)
	p.SetXY(10, 10)
	p.Cell(0, 10, "PDF/A-1b Document", "", "", false, 0)

	data, err := doc.Bytes()
	if err != nil {
		t.Fatal(err)
	}

	out := string(data)

	// Check PDF version
	if !strings.HasPrefix(out, "%PDF-1.4") {
		t.Error("expected %PDF-1.4 header for PDF/A-1b")
	}

	// Check XMP metadata
	if !strings.Contains(out, "/Type /Metadata") {
		t.Error("expected /Type /Metadata in output")
	}
	if !strings.Contains(out, "<pdfaid:part>1</pdfaid:part>") {
		t.Error("expected pdfaid:part=1 in XMP")
	}
	if !strings.Contains(out, "<pdfaid:conformance>B</pdfaid:conformance>") {
		t.Error("expected pdfaid:conformance=B in XMP")
	}

	// Check OutputIntent
	if !strings.Contains(out, "/Type /OutputIntent") {
		t.Error("expected /Type /OutputIntent")
	}
	if !strings.Contains(out, "/S /GTS_PDFA1") {
		t.Error("expected /S /GTS_PDFA1")
	}
	if !strings.Contains(out, "/OutputConditionIdentifier (sRGB)") {
		t.Error("expected sRGB output condition")
	}

	// Check MarkInfo
	if !strings.Contains(out, "/MarkInfo <</Marked true>>") {
		t.Error("expected /MarkInfo in catalog")
	}

	// Check OutputIntents in catalog
	if !strings.Contains(out, "/OutputIntents [") {
		t.Error("expected /OutputIntents array in catalog")
	}
}

func TestPDFA2b(t *testing.T) {
	doc := New(WithPDFA("2b"), WithCompression(false))
	doc.SetTitle("PDF/A-2b Test")

	sarabunData, err := os.ReadFile("fonts/sarabun/Sarabun-Regular.ttf")
	if err != nil {
		t.Skip("Skipping: Sarabun-Regular.ttf not available")
	}
	if err := doc.AddUTF8Font("sarabun", "", sarabunData); err != nil {
		t.Fatal(err)
	}
	doc.SetFont("sarabun", "", 12)

	p := doc.AddPage(A4)
	p.SetXY(10, 10)
	p.Cell(0, 10, "PDF/A-2b", "", "", false, 0)

	data, err := doc.Bytes()
	if err != nil {
		t.Fatal(err)
	}

	out := string(data)
	if !strings.HasPrefix(out, "%PDF-1.7") {
		t.Error("expected %PDF-1.7 header for PDF/A-2b")
	}
	if !strings.Contains(out, "<pdfaid:part>2</pdfaid:part>") {
		t.Error("expected pdfaid:part=2 in XMP")
	}
}

func TestPDFARejectsCoreFont(t *testing.T) {
	doc := New(WithPDFA("1b"))
	doc.SetFont("helvetica", "", 12)
	p := doc.AddPage(A4)
	p.SetXY(10, 10)
	p.Cell(0, 10, "Should fail", "", "", false, 0)

	_, err := doc.Bytes()
	if err == nil {
		t.Fatal("expected error for core font in PDF/A mode")
	}
	if !strings.Contains(err.Error(), "PDF/A") {
		t.Errorf("error should mention PDF/A, got: %v", err)
	}
}

func TestPDFA1bRejectsTransparency(t *testing.T) {
	doc := New(WithPDFA("1b"), WithCompression(false))

	sarabunData, err := os.ReadFile("fonts/sarabun/Sarabun-Regular.ttf")
	if err != nil {
		t.Skip("Skipping: Sarabun-Regular.ttf not available")
	}
	if err := doc.AddUTF8Font("sarabun", "", sarabunData); err != nil {
		t.Fatal(err)
	}
	doc.SetFont("sarabun", "", 12)

	doc.SetAlpha(0.5) // transparency — forbidden in PDF/A-1b
	p := doc.AddPage(A4)
	p.SetXY(10, 10)
	p.Cell(0, 10, "Transparent", "", "", false, 0)

	_, err = doc.Bytes()
	if err == nil {
		t.Fatal("expected error for transparency in PDF/A-1b")
	}
	if !strings.Contains(err.Error(), "transparency") {
		t.Errorf("error should mention transparency, got: %v", err)
	}
}

func TestPDFAInvalidLevel(t *testing.T) {
	doc := New(WithPDFA("3a")) // unsupported
	if doc.Err() == nil {
		t.Fatal("expected error for unsupported PDF/A level")
	}
}

func TestNoPDFADefault(t *testing.T) {
	doc := New(WithCompression(false))
	doc.SetFont("helvetica", "", 12)
	p := doc.AddPage(A4)
	p.SetXY(10, 10)
	p.Cell(0, 10, "Normal PDF", "", "", false, 0)

	data, err := doc.Bytes()
	if err != nil {
		t.Fatal(err)
	}

	out := string(data)
	if strings.Contains(out, "/Type /Metadata") {
		t.Error("non-PDF/A document should not have XMP metadata")
	}
	if strings.Contains(out, "/OutputIntents") {
		t.Error("non-PDF/A document should not have OutputIntents")
	}
}

func TestICCProfileStructure(t *testing.T) {
	profile := buildSRGBICCProfile()

	// Check size
	if len(profile) != 456 {
		t.Errorf("expected profile size 456, got %d", len(profile))
	}

	// Check header signature
	if string(profile[36:40]) != "acsp" {
		t.Error("expected 'acsp' signature at offset 36")
	}

	// Check device class
	if string(profile[12:16]) != "mntr" {
		t.Error("expected 'mntr' device class")
	}

	// Check color space
	if string(profile[16:20]) != "RGB " {
		t.Error("expected 'RGB ' color space")
	}
}

// --- PDF-to-Image Conversion Tests ---

func hasRenderer() bool {
	_, err := FindTool("pdftoppm", "mutool", "gs")
	return err == nil
}

func createTestPDF(t *testing.T) string {
	t.Helper()
	doc := New()
	p := doc.AddPage(A4)
	p.SetFont("Helvetica", "", 14)
	p.TextAt(50, 50, "Page 1")
	p2 := doc.AddPage(A4)
	p2.SetFont("Helvetica", "", 14)
	p2.TextAt(50, 50, "Page 2")
	p3 := doc.AddPage(A4)
	p3.SetFont("Helvetica", "", 14)
	p3.TextAt(50, 50, "Page 3")

	tmp := filepath.Join(t.TempDir(), "test.pdf")
	f, err := os.Create(tmp)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := doc.WriteTo(f); err != nil {
		f.Close()
		t.Fatal(err)
	}
	f.Close()
	return tmp
}

func TestConvertToImages_PNG(t *testing.T) {
	if !hasRenderer() {
		t.Skip("no PDF renderer available")
	}
	pdf := createTestPDF(t)
	outDir := filepath.Join(t.TempDir(), "out")

	paths, err := ConvertToImages(pdf, outDir)
	if err != nil {
		t.Fatalf("ConvertToImages: %v", err)
	}
	if len(paths) != 3 {
		t.Fatalf("expected 3 images, got %d", len(paths))
	}
	for _, p := range paths {
		if !strings.HasSuffix(p, ".png") {
			t.Errorf("expected .png file, got %s", p)
		}
		info, err := os.Stat(p)
		if err != nil {
			t.Errorf("stat %s: %v", p, err)
		} else if info.Size() == 0 {
			t.Errorf("image %s is empty", p)
		}
	}
}

func TestConvertToImages_JPEG(t *testing.T) {
	if !hasRenderer() {
		t.Skip("no PDF renderer available")
	}
	pdf := createTestPDF(t)
	outDir := filepath.Join(t.TempDir(), "out")

	paths, err := ConvertToImages(pdf, outDir, WithFormat(JPEG))
	if err != nil {
		t.Fatalf("ConvertToImages JPEG: %v", err)
	}
	if len(paths) != 3 {
		t.Fatalf("expected 3 images, got %d", len(paths))
	}
	for _, p := range paths {
		if !strings.HasSuffix(p, ".jpeg") && !strings.HasSuffix(p, ".jpg") {
			t.Errorf("expected .jpeg file, got %s", p)
		}
	}
}

func TestConvertToImages_WithDPI(t *testing.T) {
	if !hasRenderer() {
		t.Skip("no PDF renderer available")
	}
	pdf := createTestPDF(t)

	// Low DPI
	outLow := filepath.Join(t.TempDir(), "low")
	pathsLow, err := ConvertToImages(pdf, outLow, WithDPI(72))
	if err != nil {
		t.Fatalf("ConvertToImages DPI 72: %v", err)
	}
	lowInfo, _ := os.Stat(pathsLow[0])

	// High DPI
	outHigh := filepath.Join(t.TempDir(), "high")
	pathsHigh, err := ConvertToImages(pdf, outHigh, WithDPI(300))
	if err != nil {
		t.Fatalf("ConvertToImages DPI 300: %v", err)
	}
	highInfo, _ := os.Stat(pathsHigh[0])

	if highInfo.Size() <= lowInfo.Size() {
		t.Errorf("300 DPI image (%d bytes) should be larger than 72 DPI (%d bytes)",
			highInfo.Size(), lowInfo.Size())
	}
}

func TestConvertToImages_WithPages(t *testing.T) {
	if !hasRenderer() {
		t.Skip("no PDF renderer available")
	}
	pdf := createTestPDF(t)
	outDir := filepath.Join(t.TempDir(), "out")

	paths, err := ConvertToImages(pdf, outDir, WithPages(2))
	if err != nil {
		t.Fatalf("ConvertToImages page 2: %v", err)
	}
	if len(paths) != 1 {
		t.Fatalf("expected 1 image for single page, got %d", len(paths))
	}
}

func TestConvertPage_ReturnsImage(t *testing.T) {
	if !hasRenderer() {
		t.Skip("no PDF renderer available")
	}
	pdf := createTestPDF(t)

	img, err := ConvertPage(pdf, 1)
	if err != nil {
		t.Fatalf("ConvertPage: %v", err)
	}
	bounds := img.Bounds()
	if bounds.Dx() == 0 || bounds.Dy() == 0 {
		t.Error("image has zero dimensions")
	}
}

func TestConvertPage_JPEG(t *testing.T) {
	if !hasRenderer() {
		t.Skip("no PDF renderer available")
	}
	pdf := createTestPDF(t)

	img, err := ConvertPage(pdf, 1, WithFormat(JPEG))
	if err != nil {
		t.Fatalf("ConvertPage JPEG: %v", err)
	}
	bounds := img.Bounds()
	if bounds.Dx() == 0 || bounds.Dy() == 0 {
		t.Error("image has zero dimensions")
	}
}

func TestConvertPage_WithDPI(t *testing.T) {
	if !hasRenderer() {
		t.Skip("no PDF renderer available")
	}
	pdf := createTestPDF(t)

	img72, err := ConvertPage(pdf, 1, WithDPI(72))
	if err != nil {
		t.Fatalf("ConvertPage DPI 72: %v", err)
	}
	img300, err := ConvertPage(pdf, 1, WithDPI(300))
	if err != nil {
		t.Fatalf("ConvertPage DPI 300: %v", err)
	}
	if img300.Bounds().Dx() <= img72.Bounds().Dx() {
		t.Errorf("300 DPI image (%dx%d) should be larger than 72 DPI (%dx%d)",
			img300.Bounds().Dx(), img300.Bounds().Dy(),
			img72.Bounds().Dx(), img72.Bounds().Dy())
	}
}

func TestConvert_NoRenderer(t *testing.T) {
	// Test FindTool with names that definitely don't exist.
	_, err := FindTool("nonexistent-tool-xyz")
	if err == nil {
		t.Error("expected error for nonexistent tool")
	}
	var tnf *ToolNotFoundError
	if !errors.As(err, &tnf) {
		t.Errorf("expected *ToolNotFoundError, got %T: %v", err, err)
	}
}

func TestFindTool_FirstAvailable(t *testing.T) {
	// FindTool should return the first available tool.
	tool, err := FindTool("nonexistent-xyz", "sh")
	if err != nil {
		t.Skipf("sh not on PATH: %v", err)
	}
	if tool.Name != "sh" {
		t.Errorf("expected tool name 'sh', got %q", tool.Name)
	}
	if tool.Path == "" {
		t.Error("expected non-empty path")
	}
}

func TestExternalTool_Run(t *testing.T) {
	tool, err := FindTool("echo")
	if err != nil {
		t.Skip("echo not on PATH")
	}
	out, err := tool.Run("hello", "world")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got := strings.TrimSpace(string(out)); got != "hello world" {
		t.Errorf("expected 'hello world', got %q", got)
	}
}

func TestExternalTool_RunError(t *testing.T) {
	tool, err := FindTool("false")
	if err != nil {
		t.Skip("false not on PATH")
	}
	_, err = tool.Run()
	if err == nil {
		t.Error("expected error from 'false' command")
	}
	var te *ToolError
	if !errors.As(err, &te) {
		t.Errorf("expected *ToolError, got %T", err)
	}
}

func TestTempDir_CreateAndCleanup(t *testing.T) {
	dir, cleanup, err := TempDir("folio-test-*")
	if err != nil {
		t.Fatal(err)
	}
	// Directory should exist.
	if _, err := os.Stat(dir); err != nil {
		t.Fatalf("temp dir should exist: %v", err)
	}
	cleanup()
	// Directory should be removed.
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Error("temp dir should be removed after cleanup")
	}
}

func TestCollectFiles(t *testing.T) {
	dir := t.TempDir()
	// Create some files.
	os.WriteFile(filepath.Join(dir, "page-01.png"), []byte("img"), 0o644)
	os.WriteFile(filepath.Join(dir, "page-02.png"), []byte("img"), 0o644)
	os.WriteFile(filepath.Join(dir, "page-01.jpg"), []byte("img"), 0o644)
	os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("txt"), 0o644)

	pngs, err := CollectFiles(dir, ".png")
	if err != nil {
		t.Fatal(err)
	}
	if len(pngs) != 2 {
		t.Errorf("expected 2 .png files, got %d", len(pngs))
	}

	jpgs, err := CollectFiles(dir, ".jpg")
	if err != nil {
		t.Fatal(err)
	}
	if len(jpgs) != 1 {
		t.Errorf("expected 1 .jpg file, got %d", len(jpgs))
	}
}

func TestConvert_InvalidPDF(t *testing.T) {
	if !hasRenderer() {
		t.Skip("no PDF renderer available")
	}
	// Create a file that is not a valid PDF.
	tmp := filepath.Join(t.TempDir(), "bad.pdf")
	os.WriteFile(tmp, []byte("not a pdf"), 0o644)
	outDir := filepath.Join(t.TempDir(), "out")

	_, err := ConvertToImages(tmp, outDir)
	if err == nil {
		t.Error("expected error for invalid PDF")
	}
}

func TestConvert_NonexistentFile(t *testing.T) {
	if !hasRenderer() {
		t.Skip("no PDF renderer available")
	}
	outDir := filepath.Join(t.TempDir(), "out")
	_, err := ConvertToImages("/nonexistent/file.pdf", outDir)
	if err == nil {
		t.Error("expected error for nonexistent file")
	}
}

// --- Split PDF tests (pure Go, no external tools) ---

func TestSplitPDF_AllPages(t *testing.T) {
	pdf := createTestPDF(t)
	outDir := filepath.Join(t.TempDir(), "out")

	paths, err := SplitPDF(pdf, outDir)
	if err != nil {
		t.Fatalf("SplitPDF: %v", err)
	}
	if len(paths) != 3 {
		t.Fatalf("expected 3 files, got %d", len(paths))
	}
	for _, p := range paths {
		if !strings.HasSuffix(p, ".pdf") {
			t.Errorf("expected .pdf file, got %s", p)
		}
		info, err := os.Stat(p)
		if err != nil {
			t.Errorf("stat %s: %v", p, err)
		} else if info.Size() == 0 {
			t.Errorf("file %s is empty", p)
		}
	}
}

func TestSplitPDF_WithRanges(t *testing.T) {
	pdf := createTestPDF(t)
	outDir := filepath.Join(t.TempDir(), "out")

	paths, err := SplitPDF(pdf, outDir,
		WithRanges(
			PageRange{From: 1, To: 1},
			PageRange{From: 2, To: 3},
		),
	)
	if err != nil {
		t.Fatalf("SplitPDF with ranges: %v", err)
	}
	if len(paths) != 2 {
		t.Fatalf("expected 2 files, got %d", len(paths))
	}
	for _, p := range paths {
		info, err := os.Stat(p)
		if err != nil {
			t.Errorf("stat %s: %v", p, err)
		} else if info.Size() == 0 {
			t.Errorf("file %s is empty", p)
		}
	}
}

func TestSplitPDF_SinglePageRange(t *testing.T) {
	pdf := createTestPDF(t)
	outDir := filepath.Join(t.TempDir(), "out")

	paths, err := SplitPDF(pdf, outDir,
		WithRanges(PageRange{From: 2, To: 2}),
	)
	if err != nil {
		t.Fatalf("SplitPDF single page range: %v", err)
	}
	if len(paths) != 1 {
		t.Fatalf("expected 1 file, got %d", len(paths))
	}
}

func TestSplitPDF_NonexistentFile(t *testing.T) {
	outDir := filepath.Join(t.TempDir(), "out")
	_, err := SplitPDF("/nonexistent/file.pdf", outDir)
	if err == nil {
		t.Error("expected error for nonexistent file")
	}
}

func TestSplitPDF_InvalidRange(t *testing.T) {
	pdf := createTestPDF(t)
	outDir := filepath.Join(t.TempDir(), "out")

	_, err := SplitPDF(pdf, outDir,
		WithRanges(PageRange{From: 0, To: 1}),
	)
	if err == nil {
		t.Error("expected error for invalid range (From=0)")
	}

	_, err = SplitPDF(pdf, outDir,
		WithRanges(PageRange{From: 1, To: 99}),
	)
	if err == nil {
		t.Error("expected error for out-of-bounds range")
	}
}

func TestSplitPDF_RoundTrip(t *testing.T) {
	// Split a PDF, then split one of the results again.
	pdf := createTestPDF(t)
	dir1 := filepath.Join(t.TempDir(), "split1")

	paths, err := SplitPDF(pdf, dir1)
	if err != nil {
		t.Fatalf("first split: %v", err)
	}

	// Split the first single-page PDF again (should produce 1 file).
	dir2 := filepath.Join(t.TempDir(), "split2")
	paths2, err := SplitPDF(paths[0], dir2)
	if err != nil {
		t.Fatalf("second split: %v", err)
	}
	if len(paths2) != 1 {
		t.Fatalf("expected 1 file from re-split, got %d", len(paths2))
	}
	info, _ := os.Stat(paths2[0])
	if info.Size() == 0 {
		t.Error("re-split file is empty")
	}
}

// --- Merge PDF tests (pure Go, no external tools) ---

func TestMergePDF_TwoFiles(t *testing.T) {
	pdf1 := createTestPDF(t) // 3 pages
	pdf2 := createTestPDF(t) // 3 pages
	out := filepath.Join(t.TempDir(), "merged.pdf")

	if err := MergePDF(out, pdf1, pdf2); err != nil {
		t.Fatalf("MergePDF: %v", err)
	}

	info, err := os.Stat(out)
	if err != nil {
		t.Fatalf("stat merged: %v", err)
	}
	if info.Size() == 0 {
		t.Fatal("merged PDF is empty")
	}

	// Verify the merged PDF can be re-split into 6 pages.
	splitDir := filepath.Join(t.TempDir(), "split")
	pages, err := SplitPDF(out, splitDir)
	if err != nil {
		t.Fatalf("split merged PDF: %v", err)
	}
	if len(pages) != 6 {
		t.Fatalf("expected 6 pages from merge of 3+3, got %d", len(pages))
	}
}

func TestMergePDF_SingleFile(t *testing.T) {
	pdf := createTestPDF(t)
	out := filepath.Join(t.TempDir(), "merged.pdf")

	if err := MergePDF(out, pdf); err != nil {
		t.Fatalf("MergePDF single: %v", err)
	}

	splitDir := filepath.Join(t.TempDir(), "split")
	pages, err := SplitPDF(out, splitDir)
	if err != nil {
		t.Fatalf("split merged: %v", err)
	}
	if len(pages) != 3 {
		t.Fatalf("expected 3 pages, got %d", len(pages))
	}
}

func TestMergePDF_SplitThenMerge(t *testing.T) {
	// Split a 3-page PDF, then merge the individual pages back.
	pdf := createTestPDF(t)
	splitDir := filepath.Join(t.TempDir(), "split")

	parts, err := SplitPDF(pdf, splitDir)
	if err != nil {
		t.Fatalf("split: %v", err)
	}

	out := filepath.Join(t.TempDir(), "reassembled.pdf")
	if err := MergePDF(out, parts...); err != nil {
		t.Fatalf("merge split pages: %v", err)
	}

	// Re-split to verify page count.
	verifyDir := filepath.Join(t.TempDir(), "verify")
	pages, err := SplitPDF(out, verifyDir)
	if err != nil {
		t.Fatalf("verify split: %v", err)
	}
	if len(pages) != 3 {
		t.Fatalf("expected 3 pages after split+merge, got %d", len(pages))
	}
}

func TestMergePDF_NoInputs(t *testing.T) {
	out := filepath.Join(t.TempDir(), "merged.pdf")
	if err := MergePDF(out); err == nil {
		t.Error("expected error for no inputs")
	}
}

func TestMergePDF_NonexistentFile(t *testing.T) {
	out := filepath.Join(t.TempDir(), "merged.pdf")
	if err := MergePDF(out, "/nonexistent/file.pdf"); err == nil {
		t.Error("expected error for nonexistent file")
	}
}

// --- Watermark PDF tests ---

func TestWatermarkPDF_Text(t *testing.T) {
	pdf := createTestPDF(t)
	out := filepath.Join(t.TempDir(), "watermarked.pdf")

	err := WatermarkPDF(pdf, out, WatermarkText("DRAFT"))
	if err != nil {
		t.Fatalf("WatermarkPDF text: %v", err)
	}
	info, _ := os.Stat(out)
	if info.Size() == 0 {
		t.Fatal("watermarked PDF is empty")
	}

	// Verify the result can be re-split (valid PDF).
	splitDir := filepath.Join(t.TempDir(), "split")
	pages, err := SplitPDF(out, splitDir)
	if err != nil {
		t.Fatalf("split watermarked: %v", err)
	}
	if len(pages) != 3 {
		t.Fatalf("expected 3 pages, got %d", len(pages))
	}
}

func TestWatermarkPDF_Template(t *testing.T) {
	pdf := createTestPDF(t)
	out := filepath.Join(t.TempDir(), "wm.pdf")

	err := WatermarkPDF(pdf, out, WatermarkTemplate("confidential"))
	if err != nil {
		t.Fatalf("WatermarkPDF template: %v", err)
	}
	info, _ := os.Stat(out)
	if info.Size() == 0 {
		t.Fatal("watermarked PDF is empty")
	}
}

func TestWatermarkPDF_Pattern(t *testing.T) {
	pdf := createTestPDF(t)
	out := filepath.Join(t.TempDir(), "wm.pdf")

	err := WatermarkPDF(pdf, out,
		WatermarkText("COPY"),
		WatermarkPattern(150, 150),
		WatermarkFontSize(36),
		WatermarkOpacity(0.15),
	)
	if err != nil {
		t.Fatalf("WatermarkPDF pattern: %v", err)
	}
	info, _ := os.Stat(out)
	if info.Size() == 0 {
		t.Fatal("watermarked PDF is empty")
	}
}

func TestWatermarkPDF_CustomPosition(t *testing.T) {
	pdf := createTestPDF(t)
	out := filepath.Join(t.TempDir(), "wm.pdf")

	err := WatermarkPDF(pdf, out,
		WatermarkText("TOP-RIGHT"),
		WatermarkPosition(500, 800),
		WatermarkRotation(0),
		WatermarkFontSize(24),
		WatermarkColor(255, 0, 0),
	)
	if err != nil {
		t.Fatalf("WatermarkPDF position: %v", err)
	}
	info, _ := os.Stat(out)
	if info.Size() == 0 {
		t.Fatal("watermarked PDF is empty")
	}
}

func TestWatermarkPDF_ImageJPEG(t *testing.T) {
	pdf := createTestPDF(t)

	// Create a small JPEG test image.
	imgPath := filepath.Join(t.TempDir(), "logo.jpg")
	img := image.NewRGBA(image.Rect(0, 0, 50, 50))
	for y := 0; y < 50; y++ {
		for x := 0; x < 50; x++ {
			img.Set(x, y, image.White)
		}
	}
	f, _ := os.Create(imgPath)
	jpeg.Encode(f, img, nil)
	f.Close()

	out := filepath.Join(t.TempDir(), "wm.pdf")
	err := WatermarkPDF(pdf, out,
		WatermarkImage(imgPath),
		WatermarkOpacity(0.2),
		WatermarkScale(0.5),
	)
	if err != nil {
		t.Fatalf("WatermarkPDF image: %v", err)
	}
	info, _ := os.Stat(out)
	if info.Size() == 0 {
		t.Fatal("watermarked PDF is empty")
	}
}

func TestWatermarkPDF_ImagePNG(t *testing.T) {
	pdf := createTestPDF(t)

	// Create a small PNG test image with alpha.
	imgPath := filepath.Join(t.TempDir(), "logo.png")
	img := image.NewRGBA(image.Rect(0, 0, 30, 30))
	for y := 0; y < 30; y++ {
		for x := 0; x < 30; x++ {
			img.SetRGBA(x, y, color.RGBA{R: 0, G: 0, B: 0, A: 128})
		}
	}
	f, _ := os.Create(imgPath)
	png.Encode(f, img)
	f.Close()

	out := filepath.Join(t.TempDir(), "wm.pdf")
	err := WatermarkPDF(pdf, out, WatermarkImage(imgPath))
	if err != nil {
		t.Fatalf("WatermarkPDF PNG: %v", err)
	}
	info, _ := os.Stat(out)
	if info.Size() == 0 {
		t.Fatal("watermarked PDF is empty")
	}
}

func TestWatermarkPDF_NoContent(t *testing.T) {
	pdf := createTestPDF(t)
	out := filepath.Join(t.TempDir(), "wm.pdf")

	err := WatermarkPDF(pdf, out)
	if err == nil {
		t.Error("expected error when no text or image provided")
	}
}

func TestWatermarkPDF_NonexistentInput(t *testing.T) {
	out := filepath.Join(t.TempDir(), "wm.pdf")
	err := WatermarkPDF("/nonexistent/file.pdf", out, WatermarkText("X"))
	if err == nil {
		t.Error("expected error for nonexistent file")
	}
}

func TestWatermarkPDF_TemplateOverride(t *testing.T) {
	pdf := createTestPDF(t)
	out := filepath.Join(t.TempDir(), "wm.pdf")

	// Use template, then override opacity and rotation.
	err := WatermarkPDF(pdf, out,
		WatermarkTemplate("draft"),
		WatermarkOpacity(0.5),
		WatermarkRotation(30),
	)
	if err != nil {
		t.Fatalf("WatermarkPDF template+override: %v", err)
	}
	info, _ := os.Stat(out)
	if info.Size() == 0 {
		t.Fatal("watermarked PDF is empty")
	}
}

// --- ImagesToPDF tests ---

func createTestJPEGs(t *testing.T, count int) []string {
	t.Helper()
	var paths []string
	for i := 0; i < count; i++ {
		p := filepath.Join(t.TempDir(), fmt.Sprintf("img%d.jpg", i))
		img := image.NewRGBA(image.Rect(0, 0, 200, 150))
		for y := 0; y < 150; y++ {
			for x := 0; x < 200; x++ {
				img.Set(x, y, color.RGBA{R: uint8(i * 80), G: 100, B: 200, A: 255})
			}
		}
		f, _ := os.Create(p)
		jpeg.Encode(f, img, nil)
		f.Close()
		paths = append(paths, p)
	}
	return paths
}

func TestImagesToPDF_MultipleJPEGs(t *testing.T) {
	imgs := createTestJPEGs(t, 3)
	out := filepath.Join(t.TempDir(), "photos.pdf")

	err := ImagesToPDF(out, imgs)
	if err != nil {
		t.Fatalf("ImagesToPDF: %v", err)
	}
	info, _ := os.Stat(out)
	if info.Size() == 0 {
		t.Fatal("output PDF is empty")
	}

	// Verify page count by splitting.
	splitDir := filepath.Join(t.TempDir(), "split")
	pages, err := SplitPDF(out, splitDir)
	if err != nil {
		t.Fatalf("split: %v", err)
	}
	if len(pages) != 3 {
		t.Fatalf("expected 3 pages, got %d", len(pages))
	}
}

func TestImagesToPDF_PNG(t *testing.T) {
	p := filepath.Join(t.TempDir(), "test.png")
	img := image.NewRGBA(image.Rect(0, 0, 100, 100))
	for y := 0; y < 100; y++ {
		for x := 0; x < 100; x++ {
			img.Set(x, y, color.RGBA{R: 255, G: 0, B: 0, A: 200})
		}
	}
	f, _ := os.Create(p)
	png.Encode(f, img)
	f.Close()

	out := filepath.Join(t.TempDir(), "png.pdf")
	err := ImagesToPDF(out, []string{p})
	if err != nil {
		t.Fatalf("ImagesToPDF PNG: %v", err)
	}
	info, _ := os.Stat(out)
	if info.Size() == 0 {
		t.Fatal("output PDF is empty")
	}
}

func TestImagesToPDF_FixedPageSize(t *testing.T) {
	imgs := createTestJPEGs(t, 2)
	out := filepath.Join(t.TempDir(), "a4.pdf")

	err := ImagesToPDF(out, imgs,
		ImagePageSize(A4),
		ImageMargin(36),
	)
	if err != nil {
		t.Fatalf("ImagesToPDF fixed size: %v", err)
	}
	info, _ := os.Stat(out)
	if info.Size() == 0 {
		t.Fatal("output PDF is empty")
	}
}

func TestImagesToPDF_FitModes(t *testing.T) {
	imgs := createTestJPEGs(t, 1)

	for _, mode := range []string{"fit", "fill", "stretch"} {
		out := filepath.Join(t.TempDir(), mode+".pdf")
		err := ImagesToPDF(out, imgs,
			ImagePageSize(A4),
			ImageFit(mode),
		)
		if err != nil {
			t.Fatalf("ImagesToPDF %s: %v", mode, err)
		}
		info, _ := os.Stat(out)
		if info.Size() == 0 {
			t.Fatalf("%s: output PDF is empty", mode)
		}
	}
}

func TestImagesToPDF_CustomDPI(t *testing.T) {
	imgs := createTestJPEGs(t, 1)

	out72 := filepath.Join(t.TempDir(), "72dpi.pdf")
	err := ImagesToPDF(out72, imgs, ImageDPI(72))
	if err != nil {
		t.Fatalf("72 DPI: %v", err)
	}

	out300 := filepath.Join(t.TempDir(), "300dpi.pdf")
	err = ImagesToPDF(out300, imgs, ImageDPI(300))
	if err != nil {
		t.Fatalf("300 DPI: %v", err)
	}

	// Higher DPI = smaller page = smaller file.
	info72, _ := os.Stat(out72)
	info300, _ := os.Stat(out300)
	if info300.Size() >= info72.Size() {
		t.Errorf("300 DPI file (%d) should be smaller than 72 DPI (%d)",
			info300.Size(), info72.Size())
	}
}

func TestImagesToPDF_NoImages(t *testing.T) {
	out := filepath.Join(t.TempDir(), "empty.pdf")
	err := ImagesToPDF(out, nil)
	if err == nil {
		t.Error("expected error for no images")
	}
}

func TestImagesToPDF_BadFormat(t *testing.T) {
	out := filepath.Join(t.TempDir(), "bad.pdf")
	err := ImagesToPDF(out, []string{"file.bmp"})
	if err == nil {
		t.Error("expected error for unsupported format")
	}
}

func TestImagesToPDF_NonexistentImage(t *testing.T) {
	out := filepath.Join(t.TempDir(), "bad.pdf")
	err := ImagesToPDF(out, []string{"/nonexistent/photo.jpg"})
	if err == nil {
		t.Error("expected error for nonexistent image")
	}
}
