// Example: Folio Feature Showcase
//
// Generates one PDF per feature (v0.16–v0.30) into examples/pdf/.
// Run from the repo root:
//
//	go run ./examples/showcase
package main

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"fmt"
	"math/big"
	"os"
	"time"

	"github.com/akkaraponph/folio"
	"github.com/akkaraponph/folio/fonts/sarabun"
)

const outDir = "examples/pdf"

func main() {
	demos := []struct {
		name string
		fn   func() error
	}{
		{"01_landscape", demoLandscape},
		{"02_bookmarks", demoBookmarks},
		{"03_svg_path", demoSVGPath},
		{"04_template", demoTemplate},
		{"05_toc", demoTOC},
		{"06_columns", demoColumns},
		{"07_barcode_qr", demoBarcodeQR},
		{"08_auto_table", demoAutoTable},
		{"09_encryption", demoEncryption},
		{"10_acroforms", demoAcroForms},
		{"11_signature", demoSignature},
		{"12_pdfa", demoPDFA},
		{"13_fluent_api", demoFluentAPI},
		{"14_markdown", demoMarkdown},
		{"15_html", demoHTML},
	}

	for _, d := range demos {
		path := outDir + "/" + d.name + ".pdf"
		if err := d.fn(); err != nil {
			fmt.Fprintf(os.Stderr, "FAIL %s: %v\n", d.name, err)
			continue
		}
		fmt.Printf("OK  %s\n", path)
	}
}

func save(doc *folio.Document, name string) error {
	return doc.Save(outDir + "/" + name + ".pdf")
}

// --- 01: Landscape Orientation ---

func demoLandscape() error {
	doc := folio.New()
	doc.SetFont("helvetica", "B", 24)
	p := doc.AddPage(folio.A4.Landscape())

	doc.SetTextColor(50, 50, 150)
	p.TextAt(30, 30, "Landscape A4 Page")
	doc.SetFont("helvetica", "", 14)
	doc.SetTextColor(0, 0, 0)
	p.TextAt(30, 45, fmt.Sprintf("Page size: %.0f x %.0f points",
		folio.A4Landscape.WidthPt, folio.A4Landscape.HeightPt))

	// Draw a wide table to show the benefit
	doc.SetFont("helvetica", "", 11)
	tbl := folio.NewTable(doc, p)
	p.SetXY(30, 65)
	tbl.SetWidths(120, 120, 120, 120, 120, 120)
	tbl.Header("Month", "Revenue", "Expenses", "Profit", "Growth", "Notes")
	tbl.Row("January", "$12,500", "$8,200", "$4,300", "+5%", "Strong start")
	tbl.Row("February", "$11,800", "$7,900", "$3,900", "-3%", "Seasonal dip")
	tbl.Row("March", "$14,200", "$9,100", "$5,100", "+12%", "Q1 close")

	return save(doc, "01_landscape")
}

// --- 02: Bookmarks/Outlines ---

func demoBookmarks() error {
	doc := folio.New()
	doc.SetFont("helvetica", "", 12)

	chapters := []string{"Introduction", "Getting Started", "Advanced Topics", "API Reference"}
	for _, ch := range chapters {
		p := doc.AddPage(folio.A4)
		doc.SetFont("helvetica", "B", 20)
		doc.AddBookmark(ch, 0)
		p.TextAt(20, 25, ch)

		doc.SetFont("helvetica", "", 12)
		p.SetXY(20, 40)
		p.MultiCell(170, 6, "This is the content of the \""+ch+"\" chapter. "+
			"Open the PDF sidebar to see the bookmark navigation panel. "+
			"Each chapter is a separate page with a level-0 bookmark.", "", "L", false)

		// Sub-bookmark
		p.SetXY(20, 70)
		doc.SetFont("helvetica", "B", 14)
		doc.AddBookmark("Overview of "+ch, 1)
		p.TextAt(20, 70, "Overview of "+ch)
		doc.SetFont("helvetica", "", 12)
		p.SetXY(20, 80)
		p.MultiCell(170, 6, "This subsection has a level-1 bookmark, nested under the chapter.", "", "L", false)
	}

	return save(doc, "02_bookmarks")
}

// --- 03: SVG Path Import ---

func demoSVGPath() error {
	doc := folio.New()
	doc.SetFont("helvetica", "B", 18)
	p := doc.AddPage(folio.A4)
	p.TextAt(20, 20, "SVG Path Import")

	doc.SetFont("helvetica", "", 11)
	p.TextAt(20, 32, "Shapes drawn from SVG path data strings:")

	// Star shape
	doc.SetDrawColor(200, 50, 50)
	doc.SetFillColor(255, 200, 50)
	doc.SetLineWidth(0.5)
	star := "M 50 0 L 61 35 L 98 35 L 68 57 L 79 91 L 50 70 L 21 91 L 32 57 L 2 35 L 39 35 Z"
	p.SVGPath(20, 45, 0.6, star, "DF")

	// Heart shape
	doc.SetFillColor(220, 30, 60)
	heart := "M 10 30 A 20 20 0 0 1 50 30 A 20 20 0 0 1 90 30 Q 90 60 50 90 Q 10 60 10 30 Z"
	p.SVGPath(90, 45, 0.5, heart, "F")

	// Arrow
	doc.SetDrawColor(0, 100, 200)
	doc.SetLineWidth(0.8)
	arrow := "M 0 20 L 60 20 L 60 0 L 100 30 L 60 60 L 60 40 L 0 40 Z"
	p.SVGPath(20, 110, 0.5, arrow, "D")

	return save(doc, "03_svg_path")
}

// --- 04: Page Templates ---

func demoTemplate() error {
	doc := folio.New()
	doc.SetFont("helvetica", "", 12)

	// Create a letterhead template
	tpl := doc.BeginTemplate(folio.A4)
	tpl.SetFillColorRGB(30, 60, 120)
	tpl.Rect(0, 0, 210, 25, "F")
	tpl.SetFont("helvetica", "B", 16)
	tpl.TextAt(15, 10, "ACME Corporation")
	tpl.SetFont("helvetica", "", 9)
	tpl.TextAt(15, 17, "123 Business Ave, Suite 100 | Tel: +1 555-0100 | www.acme.example.com")

	// Footer bar
	tpl.SetFillColorRGB(230, 230, 235)
	tpl.Rect(0, 280, 210, 17, "F")
	tpl.SetFont("helvetica", "", 8)
	tpl.TextAt(15, 285, "Confidential - ACME Corporation")
	tplName := doc.EndTemplate()

	// Use template on multiple pages
	for i := 1; i <= 3; i++ {
		p := doc.AddPage(folio.A4)
		p.UseTemplate(tplName, 0, 0, 210, 297)

		doc.SetFont("helvetica", "", 12)
		doc.SetTextColor(0, 0, 0)
		p.SetXY(15, 35)
		p.MultiCell(180, 6, fmt.Sprintf(
			"This is page %d of the document. The header and footer are "+
				"rendered from a reusable template (Form XObject). "+
				"The template is defined once and stamped onto each page, "+
				"keeping the PDF size small.", i), "", "L", false)
	}

	return save(doc, "04_template")
}

// --- 05: Table of Contents ---

func demoTOC() error {
	doc := folio.New()
	doc.SetFont("helvetica", "", 12)

	// Create TOC page first (will be rendered last)
	tocPage := doc.AddPage(folio.A4)
	doc.SetFont("helvetica", "B", 20)
	tocPage.TextAt(20, 20, "Table of Contents")

	toc := folio.NewTOC(doc)

	// Add content pages
	sections := []struct {
		title string
		level int
	}{
		{"Chapter 1: Basics", 0},
		{"Installing Folio", 1},
		{"Your First PDF", 1},
		{"Chapter 2: Layouts", 0},
		{"Tables", 1},
		{"Multi-Column", 1},
		{"Chapter 3: Advanced", 0},
	}

	for _, s := range sections {
		p := doc.AddPage(folio.A4)
		if s.level == 0 {
			doc.SetFont("helvetica", "B", 18)
		} else {
			doc.SetFont("helvetica", "B", 14)
		}
		p.TextAt(20, 25, s.title)
		toc.Add(s.title, s.level, p, 25)

		doc.SetFont("helvetica", "", 12)
		p.SetXY(20, 40)
		p.MultiCell(170, 6, "Content for: "+s.title, "", "L", false)
	}

	// Render the TOC on the first page, below the title
	doc.SetFont("helvetica", "", 11)
	tocPage.SetXY(20, 35)
	toc.RenderWithPageNums(tocPage, 6, -1)

	return save(doc, "05_toc")
}

// --- 06: Multi-Column Layout ---

func demoColumns() error {
	doc := folio.New()
	doc.SetFont("helvetica", "B", 18)
	p := doc.AddPage(folio.A4)
	p.TextAt(20, 15, "Multi-Column Layout")

	doc.SetFont("helvetica", "", 10)
	p.SetXY(10, 30)

	cols := folio.NewColumnLayout(doc, p, 3, 5)
	cols.Begin()

	p.MultiCell(0, 5, "Column 1: Lorem ipsum dolor sit amet, "+
		"consectetur adipiscing elit. Sed do eiusmod tempor "+
		"incididunt ut labore et dolore magna aliqua. Ut enim "+
		"ad minim veniam.", "", "J", false)

	cols.NextColumn()
	p.MultiCell(0, 5, "Column 2: Duis aute irure dolor in "+
		"reprehenderit in voluptate velit esse cillum dolore "+
		"eu fugiat nulla pariatur. Excepteur sint occaecat "+
		"cupidatat non proident.", "", "J", false)

	cols.NextColumn()
	p.MultiCell(0, 5, "Column 3: Sunt in culpa qui officia "+
		"deserunt mollit anim id est laborum. Sed ut perspiciatis "+
		"unde omnis iste natus error sit voluptatem accusantium.", "", "J", false)

	cols.End()

	return save(doc, "06_columns")
}

// --- 07: Barcode & QR Code ---

func demoBarcodeQR() error {
	doc := folio.New()
	doc.SetFont("helvetica", "B", 18)
	p := doc.AddPage(folio.A4)
	p.TextAt(20, 20, "Barcode & QR Code")

	// Code 128
	doc.SetFont("helvetica", "B", 12)
	p.TextAt(20, 38, "Code 128:")
	p.Barcode128(20, 42, 80, 20, "FOLIO-2026")
	doc.SetFont("helvetica", "", 9)
	p.TextAt(20, 64, "FOLIO-2026")

	// EAN-13
	doc.SetFont("helvetica", "B", 12)
	p.TextAt(20, 80, "EAN-13:")
	p.BarcodeEAN13(20, 84, 60, 20, "4901234567894")
	doc.SetFont("helvetica", "", 9)
	p.TextAt(20, 106, "4901234567894")

	// QR Code (Thai tax invoice URL)
	doc.SetFont("helvetica", "B", 12)
	p.TextAt(20, 125, "QR Code (Tax Invoice):")
	p.QRCode(20, 130, 40, "https://tax.rd.go.th/verify?id=0105563000001&ref=INV2026042", 1)
	doc.SetFont("helvetica", "", 8)
	p.TextAt(20, 175, "Scan to verify tax invoice")

	// Another QR code
	doc.SetFont("helvetica", "B", 12)
	p.TextAt(120, 125, "QR Code (PromptPay):")
	p.QRCode(120, 130, 40, "00020101021130370016A000000677010111011300661234567890253049999", 1)
	doc.SetFont("helvetica", "", 8)
	p.TextAt(120, 175, "PromptPay QR")

	return save(doc, "07_barcode_qr")
}

// --- 08: Auto Table from JSON ---

func demoAutoTable() error {
	doc := folio.New()
	doc.SetFont("helvetica", "B", 18)
	p := doc.AddPage(folio.A4)
	p.TextAt(20, 20, "Auto Table from Structs & JSON")

	// From structs
	type Employee struct {
		Name       string
		Department string
		Salary     string
	}
	employees := []Employee{
		{"Alice Johnson", "Engineering", "$95,000"},
		{"Bob Smith", "Marketing", "$72,000"},
		{"Charlie Lee", "Engineering", "$88,000"},
		{"Diana Chen", "Design", "$78,000"},
		{"Evan Park", "Marketing", "$69,000"},
	}

	doc.SetFont("helvetica", "B", 13)
	p.TextAt(20, 35, "From Structs:")
	doc.SetFont("helvetica", "", 10)
	p.SetXY(20, 40)
	at := folio.AutoTableFromStructs(doc, p, employees)
	at.SetHeaderStyle(folio.CellStyle{
		FillColor: [3]int{40, 60, 120},
		TextColor: [3]int{255, 255, 255},
		FontStyle: "B",
	})
	at.Render()

	// From JSON
	jsonData := []byte(`[
		{"Product": "Widget A", "Price": "$12.99", "Stock": "142"},
		{"Product": "Widget B", "Price": "$24.50", "Stock": "87"},
		{"Product": "Gadget X", "Price": "$49.99", "Stock": "23"}
	]`)

	doc.SetFont("helvetica", "B", 13)
	p.SetXY(20, p.GetY()+15)
	p.TextAt(20, p.GetY(), "From JSON:")
	doc.SetFont("helvetica", "", 10)
	p.SetY(p.GetY() + 5)
	p.SetX(20)
	jt, err := folio.AutoTableFromJSON(doc, p, jsonData)
	if err != nil {
		return err
	}
	jt.Render()

	return save(doc, "08_auto_table")
}

// --- 09: Password Protection ---

func demoEncryption() error {
	doc := folio.New()
	doc.SetFont("helvetica", "B", 18)
	p := doc.AddPage(folio.A4)
	p.TextAt(20, 25, "Password Protected PDF")

	doc.SetFont("helvetica", "", 12)
	p.SetXY(20, 40)
	p.MultiCell(170, 6, "This PDF is password-protected.\n\n"+
		"User password: \"user123\" (required to open)\n"+
		"Owner password: \"owner456\" (required to change permissions)\n\n"+
		"Permissions: Print + Copy allowed, Modify restricted.", "", "L", false)

	doc.SetProtection("user123", "owner456", folio.PermPrint|folio.PermCopy)

	return save(doc, "09_encryption")
}

// --- 10: AcroForms ---

func demoAcroForms() error {
	doc := folio.New()
	doc.SetFont("helvetica", "B", 18)
	p := doc.AddPage(folio.A4)
	p.TextAt(20, 20, "Interactive Form Fields")

	doc.SetFont("helvetica", "", 12)

	// Text fields
	p.TextAt(20, 40, "Full Name:")
	p.FormTextField("name", 70, 36, 100, 8, folio.WithDefaultValue("John Doe"))

	p.TextAt(20, 55, "Email:")
	p.FormTextField("email", 70, 51, 100, 8, folio.WithMaxLen(50))

	p.TextAt(20, 70, "Phone:")
	p.FormTextField("phone", 70, 66, 100, 8)

	// Checkboxes
	p.TextAt(20, 90, "Preferences:")
	p.FormCheckbox("newsletter", 20, 98, 5, true)
	p.TextAt(28, 98, "Subscribe to newsletter")

	p.FormCheckbox("terms", 20, 108, 5, false)
	p.TextAt(28, 108, "I agree to terms and conditions")

	// Dropdown
	p.TextAt(20, 125, "Country:")
	p.FormDropdown("country", 70, 121, 80, 8, []string{"Thailand", "Japan", "USA", "UK", "Germany"})

	return save(doc, "10_acroforms")
}

// --- 11: Digital Signature ---

func demoSignature() error {
	doc := folio.New()
	doc.SetFont("helvetica", "B", 18)
	p := doc.AddPage(folio.A4)
	p.TextAt(20, 20, "Digitally Signed Document")

	doc.SetFont("helvetica", "", 12)
	p.SetXY(20, 35)
	p.MultiCell(170, 6, "This document includes a digital signature field "+
		"with a PKCS#7 placeholder. In production, the placeholder would be "+
		"filled with an actual cryptographic signature.\n\n"+
		"Signer: Test Authority\n"+
		"Reason: Document approval\n"+
		"Location: Bangkok, Thailand", "", "L", false)

	// Draw visible signature box
	doc.SetDrawColor(100, 100, 200)
	doc.SetLineWidth(0.5)
	p.Rect(20, 90, 80, 25, "D")
	doc.SetFont("helvetica", "I", 10)
	doc.SetTextColor(100, 100, 200)
	p.TextAt(25, 95, "Digitally signed by")
	doc.SetFont("helvetica", "B", 11)
	p.TextAt(25, 101, "Test Authority")
	doc.SetFont("helvetica", "", 9)
	p.TextAt(25, 107, time.Now().Format("2006-01-02 15:04"))

	// Generate test certificate
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "Test Authority"},
		NotBefore:    time.Now(),
		NotAfter:     time.Now().Add(365 * 24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
	}
	certDER, _ := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	cert, _ := x509.ParseCertificate(certDER)

	doc.Sign(cert, key, p, 20, 90, 80, 25, folio.SignOptions{
		Name:     "Test Authority",
		Reason:   "Document approval",
		Location: "Bangkok, Thailand",
	})

	return save(doc, "11_signature")
}

// --- 12: PDF/A Compliance ---

func demoPDFA() error {
	doc := folio.New(folio.WithPDFA("1b"))
	doc.SetTitle("PDF/A-1b Example")
	doc.SetAuthor("Folio Library")

	// PDF/A requires embedded fonts — use Sarabun TTF
	if err := sarabun.Register(doc); err != nil {
		return err
	}
	doc.SetFont("sarabun", "", 14)

	p := doc.AddPage(folio.A4)
	p.TextAt(20, 20, "PDF/A-1b Compliant Document")

	doc.SetFont("sarabun", "", 11)
	p.SetXY(20, 35)
	p.MultiCell(170, 6,
		"This document conforms to PDF/A-1b (ISO 19005-1 Level B).\n\n"+
			"It includes:\n"+
			"- XMP metadata stream with PDF/A identification\n"+
			"- sRGB ICC output intent profile\n"+
			"- MarkInfo dictionary\n"+
			"- All fonts are embedded (no core fonts)\n\n"+
			"PDF/A ensures long-term archival preservation.",
		"", "L", false)

	// Thai text to show embedded font works
	doc.SetFont("sarabun", "B", 14)
	p.SetXY(20, 100)
	p.MultiCell(170, 7,
		"เอกสารนี้เป็นไปตามมาตรฐาน PDF/A-1b สำหรับการเก็บรักษาเอกสารระยะยาว",
		"", "L", false)

	return save(doc, "12_pdfa")
}

// --- 13: Fluent Builder API ---

func demoFluentAPI() error {
	doc := folio.New()
	doc.SetFont("helvetica", "", 12)
	p := doc.AddPage(folio.A4)

	// Title via fluent text builder
	p.Text("Fluent Builder API").Bold().Size(22).Color(30, 60, 120).At(20, 20).Draw()

	p.Text("Text and shapes drawn with chainable builder methods:").At(20, 35).Draw()

	// Shapes via fluent shape builder
	p.Shape().Rect(20, 50, 50, 30).FillColor(66, 133, 244).Fill().Draw()
	p.Text("Filled Rect").Color(255, 255, 255).At(27, 62).Draw()

	p.Shape().Rect(80, 50, 50, 30).StrokeColor(219, 68, 55).FillColor(255, 230, 230).FillStroke().Draw()
	p.Text("Fill+Stroke").Color(219, 68, 55).At(85, 62).Draw()

	p.Shape().Circle(175, 65, 15).FillColor(244, 180, 0).Fill().Draw()
	p.Text("Circle").Color(100, 70, 0).At(164, 62).Draw()

	p.Shape().Ellipse(30, 110, 25, 12).FillColor(15, 157, 88).Fill().Draw()
	p.Shape().Line(70, 100, 190, 120).StrokeColor(100, 100, 100).LineWidth(1).Draw()

	// Styled text
	p.Text("Bold Red").Bold().Color(200, 0, 0).At(20, 140).Draw()
	p.Text("Italic Blue").Italic().Color(0, 0, 200).At(80, 140).Draw()
	p.Text("Large Green").Size(18).Color(0, 150, 0).At(140, 140).Draw()

	return save(doc, "13_fluent_api")
}

// --- 14: Markdown Renderer ---

func demoMarkdown() error {
	doc := folio.New()
	doc.SetFont("helvetica", "", 12)
	p := doc.AddPage(folio.A4)

	md := `# Folio Markdown Demo

This paragraph shows **bold text**, *italic text*, and ` + "`inline code`" + `.

## Features

- Headings (h1 through h6)
- **Bold** and *italic* inline formatting
- Unordered and ordered lists
- Horizontal rules
- [Clickable links](https://github.com)

## Ordered List

1. First item
2. Second item
3. Third item

---

### Code Example

Use ` + "`folio.New()`" + ` to create a document, then ` + "`doc.AddPage()`" + ` for pages.

## Nested Content

- Item with **bold** inside
- Item with *italic* and ` + "`code`" + `
- Item with a [link](https://example.com)
`

	p.Markdown(md, folio.WithBookmarks(), folio.WithLineHeight(5.5))

	return save(doc, "14_markdown")
}

// --- 15: HTML-to-PDF ---

func demoHTML() error {
	doc := folio.New()
	doc.SetFont("helvetica", "", 12)
	p := doc.AddPage(folio.A4)

	html := `<h1>HTML to PDF</h1>

<p>This page is rendered from <b>HTML markup</b> using Folio's built-in
<i>HTML subset renderer</i>. No external dependencies required.</p>

<h2>Formatting</h2>
<p>Supports <b>bold</b>, <i>italic</i>, and <u>underline</u> tags.
You can also use <b><i>nested</i></b> formatting.</p>

<h2>Lists</h2>
<p>Unordered list:</p>
<ul>
  <li>Apples</li>
  <li>Bananas</li>
  <li>Cherries</li>
</ul>

<p>Ordered list:</p>
<ol>
  <li>First step</li>
  <li>Second step</li>
  <li>Third step</li>
</ol>

<hr/>

<h2>Table</h2>
<table>
  <tr><td>Language</td><td>Year</td><td>Creator</td></tr>
  <tr><td>Go</td><td>2009</td><td>Google</td></tr>
  <tr><td>Rust</td><td>2010</td><td>Mozilla</td></tr>
  <tr><td>Swift</td><td>2014</td><td>Apple</td></tr>
</table>

<h2>Links and Colors</h2>
<p>Visit <a href="https://github.com">GitHub</a> for source code.</p>
<p style="color: red">This text is red via inline CSS.</p>
<p style="color: #0066cc">This text is blue via hex color.</p>`

	p.HTML(html)

	return save(doc, "15_html")
}
