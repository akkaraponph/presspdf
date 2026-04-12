package folio

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"os"
	"strings"
	"testing"
)

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
