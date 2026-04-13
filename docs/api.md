# Folio API Reference

Complete reference for the public API. All types and functions are in the `folio` package unless noted otherwise.

---

## Document

### Creating a Document

```go
func New(opts ...Option) *Document
```

Options:
- `WithUnit(state.Unit)` — measurement unit (default: mm)
- `WithCompression(bool)` — zlib compression (default: true)
- `WithPDFA(level string)` — PDF/A compliance (`"1b"` or `"2b"`)

### Metadata

```go
func (d *Document) SetTitle(title string)
func (d *Document) SetAuthor(author string)
func (d *Document) SetSubject(subject string)
func (d *Document) SetCreator(creator string)
```

### Layout

```go
func (d *Document) SetMargins(left, top, right float64)
func (d *Document) SetAutoPageBreak(auto bool, margin float64)
func (d *Document) SetHeaderFunc(fn func(*Page))
func (d *Document) SetFooterFunc(fn func(*Page))
```

The header function is called at the start of each new page. The footer function is called before each page break. Graphics state is automatically saved and restored around both callbacks.

### Fonts

```go
func (d *Document) SetFont(family, style string, size float64)
func (d *Document) SetFontSize(size float64)
func (d *Document) SetFontStyle(style string)
func (d *Document) GetFontFamily() string
func (d *Document) GetFontStyle() string
func (d *Document) GetFontSize() float64
func (d *Document) AddUTF8Font(family, style string, data []byte) error
func (d *Document) AddUTF8FontFromFile(family, style, path string) error
```

**Core fonts** (no embedding required): `helvetica`, `courier`, `times`, `zapfdingbats`.
Aliases: `arial` maps to `helvetica`, `symbol` maps to `zapfdingbats`.

**Styles**: `""` (regular), `"B"` (bold), `"I"` (italic), `"BI"` (bold italic).

### Colors & Drawing State

```go
func (d *Document) SetDrawColor(r, g, b int)       // stroke color (0-255)
func (d *Document) SetFillColor(r, g, b int)       // fill color (0-255)
func (d *Document) SetTextColor(r, g, b int)       // text color (0-255)
func (d *Document) SetAlpha(alpha float64)          // transparency (0.0-1.0)
func (d *Document) SetLineWidth(w float64)          // in current units
```

### Typography

```go
func (d *Document) SetCharSpacing(spacing float64)  // PDF Tc operator
func (d *Document) SetWordSpacing(spacing float64)  // PDF Tw operator
func (d *Document) SetTextRise(rise float64)         // PDF Ts operator (baseline shift)
func (d *Document) SetUnderline(on bool)
func (d *Document) SetStrikethrough(on bool)
func (d *Document) SetWordBreaker(fn WordBreakFunc)
```

`WordBreakFunc` signature: `func(paragraph string) []string`

### Pages

```go
func (d *Document) AddPage(size PageSize) *Page
func (d *Document) CurrentPage() *Page
func (d *Document) PageCount() int
func (d *Document) PageNo() int  // 1-based
```

### Images

```go
func (d *Document) RegisterImage(name string, r io.Reader) error
```

Registers a JPEG image. Use the name with `Page.DrawImageRect()` to place it. Images are deduplicated by SHA-1 hash.

### Bookmarks

```go
func (d *Document) AddBookmark(title string, level int)
```

Level 0 = top-level, level 1 = nested under the most recent level-0 bookmark, etc.

### Security

```go
func (d *Document) SetProtection(userPw, ownerPw string, permissions int32)
```

Permission flags: `PermPrint`, `PermModify`, `PermCopy`, `PermAnnotate`, `PermAll`.

```go
func (d *Document) Sign(cert *x509.Certificate, key crypto.Signer,
    page *Page, x, y, w, h float64, opts SignOptions)
```

### Templates

```go
func (d *Document) BeginTemplate(size PageSize) *Template
func (d *Document) EndTemplate() string  // returns template name
```

Templates are Form XObjects — defined once, stamped onto multiple pages.

### Output

```go
func (d *Document) Save(path string) error
func (d *Document) WriteTo(w io.Writer) (int64, error)
func (d *Document) Bytes() ([]byte, error)
func (d *Document) Err() error
```

---

## Page

### Cursor

```go
func (p *Page) GetX() float64
func (p *Page) GetY() float64
func (p *Page) SetX(x float64)
func (p *Page) SetY(y float64)
func (p *Page) SetXY(x, y float64)
func (p *Page) Width() float64
func (p *Page) Height() float64
func (p *Page) PageBreakTrigger() float64
```

### Font (page-level)

```go
func (p *Page) SetFont(family, style string, size float64)
func (p *Page) SetFontSize(size float64)
func (p *Page) SetFontStyle(style string)
func (p *Page) GetFontFamily() string
func (p *Page) GetFontStyle() string
func (p *Page) GetFontSize() float64
func (p *Page) GetStringWidth(text string) float64
```

### Text Drawing

```go
func (p *Page) TextAt(x, y float64, text string)
```
Draws text at an absolute position. Does not affect the cursor.

```go
func (p *Page) Cell(w, h float64, text, border, align string, fill bool, ln int)
```
Single-line cell. `border`: `"1"` (all), `"L"`, `"T"`, `"R"`, `"B"`, or combinations. `align`: `"L"`, `"C"`, `"R"`. `ln`: 0 = right, 1 = next line, 2 = below.

```go
func (p *Page) MultiCell(w, h float64, text, border, align string, fill bool)
```
Word-wrapped cell. Width 0 = extend to right margin. `align`: `"L"`, `"C"`, `"R"`, `"J"` (justified).

```go
func (p *Page) Write(h float64, text string)
```
Inline text at the current cursor position. Wraps at margins.

```go
func (p *Page) RichText(h float64, markup string)
```
Inline text with markup tags for formatting.

### Content Markup

```go
func (p *Page) HTML(html string, opts ...HTMLOption)
```
Renders an HTML subset: `<h1>`-`<h6>`, `<p>`, `<b>`, `<i>`, `<u>`, `<br>`, `<hr>`, `<ul>`, `<ol>`, `<li>`, `<table>`, `<tr>`, `<td>`, `<a href>`. Inline CSS: `color`, `font-size`, `text-align`, `background-color`.

```go
func (p *Page) Markdown(md string, opts ...MarkdownOption)
```
Renders a Markdown subset: `#` headings, `**bold**`, `*italic*`, `` `code` ``, `-` unordered lists, `1.` ordered lists, `---` horizontal rules, `[text](url)` links.

Options: `WithBookmarks()`, `WithLineHeight(h)`.

### Fluent Text Builder

```go
func (p *Page) Text(s string) *TextBuilder
```

Chainable methods:
- `.At(x, y)` — position
- `.Font(family, size)` — font family and size
- `.Size(pt)` — font size only
- `.Bold()` — bold style
- `.Italic()` — italic style
- `.Color(r, g, b)` — text color (0-255)
- `.Draw()` — render to page

### Fluent Shape Builder

```go
func (p *Page) Shape() *ShapeBuilder
```

Shape selection (pick one):
- `.Rect(x, y, w, h)`
- `.Circle(cx, cy, r)`
- `.Ellipse(cx, cy, rx, ry)`
- `.Line(x1, y1, x2, y2)`

Style:
- `.Stroke()` — outline only (default)
- `.Fill()` — fill only
- `.FillStroke()` — both
- `.StrokeColor(r, g, b)` — stroke color
- `.FillColor(r, g, b)` — fill color
- `.LineWidth(w)` — line width
- `.Draw()` — render to page

### Shapes & Lines

```go
func (p *Page) Line(x1, y1, x2, y2 float64)
func (p *Page) Rect(x, y, w, h float64, style string)         // "D", "F", "DF"
func (p *Page) Circle(x, y, r float64, style string)
func (p *Page) Ellipse(x, y, rx, ry float64, style string)
func (p *Page) Arc(x, y, rx, ry, startDeg, endDeg float64, style string)
func (p *Page) SetDashPattern(dashArray []float64, phase float64)
func (p *Page) SVGPath(x, y, scale float64, d string, style string)
```

SVG path commands: M, L, H, V, C, S, Q, T, A, Z (absolute and relative).

### Images

```go
func (p *Page) DrawImageRect(name string, x, y, w, h float64)
```

### Barcodes

```go
func (p *Page) Barcode128(x, y, w, h float64, data string)
func (p *Page) BarcodeEAN13(x, y, w, h float64, digits string)
func (p *Page) QRCode(x, y, size float64, data string, ecLevel int)
```

EAN-13 auto-calculates the check digit. QR code `ecLevel`: 0=L, 1=M, 2=Q, 3=H.

### Gradients

```go
func (p *Page) LinearGradient(x, y, w, h, x1, y1, x2, y2 float64, stops ...GradientStop)
func (p *Page) RadialGradient(x, y, w, h, cx, cy, r float64, stops ...GradientStop)
```

### Clipping

```go
func (p *Page) ClipRect(x, y, w, h float64)
func (p *Page) ClipCircle(x, y, r float64)
func (p *Page) ClipEllipse(x, y, rx, ry float64)
```

Clipping restricts drawing to the clipped region. Use with `TransformBegin()`/`TransformEnd()` to scope the clip.

### Transforms

```go
func (p *Page) TransformBegin()
func (p *Page) TransformEnd()
func (p *Page) Rotate(angleDeg, x, y float64)
func (p *Page) Scale(sx, sy, x, y float64)
func (p *Page) Skew(angleX, angleY, x, y float64)
func (p *Page) Translate(tx, ty float64)
func (p *Page) TextRotatedAt(x, y, angleDeg float64, text string)
```

Always wrap transforms in `TransformBegin()`/`TransformEnd()` to restore graphics state.

### Links

```go
func (p *Page) LinkURL(x, y, w, h float64, url string)
func (p *Page) LinkAnchor(x, y, w, h float64, anchor string)
```

### Form Fields

```go
func (p *Page) FormTextField(name string, x, y, w, h float64, opts ...FieldOption)
func (p *Page) FormCheckbox(name string, x, y, size float64, checked bool)
func (p *Page) FormDropdown(name string, x, y, w, h float64, options []string)
```

Field options: `WithDefaultValue(s)`, `WithMaxLen(n)`.

### Templates

```go
func (p *Page) UseTemplate(name string, x, y, w, h float64)
```

---

## Table

```go
func NewTable(doc *Document, page *Page) *Table
```

```go
func (t *Table) SetWidths(widths ...float64)
func (t *Table) SetAligns(aligns ...string)           // "L", "C", "R" per column
func (t *Table) SetRowHeight(h float64)
func (t *Table) SetBorder(border string)               // "1", "LR", "", etc.
func (t *Table) SetHeaderStyle(style CellStyle)
func (t *Table) SetBodyStyle(style CellStyle)
func (t *Table) SetAlternateRows(even, odd [3]int)     // RGB colors
func (t *Table) SetRepeatHeader(repeat bool)            // repeat on page break
func (t *Table) Header(values ...string)
func (t *Table) Row(values ...string)
```

### CellStyle

```go
type CellStyle struct {
    FillColor [3]int  // RGB 0-255
    TextColor [3]int  // RGB 0-255
    FontStyle string  // "", "B", "I", "BI"
}
```

---

## AutoTable

```go
func AutoTableFromStructs(doc *Document, page *Page, data any) *AutoTable
func AutoTableFromJSON(doc *Document, page *Page, jsonData []byte) (*AutoTable, error)
```

Generates a table automatically from struct field names (headers) and values (rows), or from a JSON array of objects.

```go
func (at *AutoTable) SetHeaderStyle(style CellStyle)
func (at *AutoTable) Render()
```

---

## TOC (Table of Contents)

```go
func NewTOC(doc *Document) *TOC
func (toc *TOC) Add(title string, level int, page *Page, y float64)
func (toc *TOC) Render(page *Page, lineHeight float64)
func (toc *TOC) RenderWithPageNums(page *Page, lineHeight float64, startOffset int)
```

---

## ColumnLayout

```go
func NewColumnLayout(doc *Document, page *Page, numCols int, gutter float64) *ColumnLayout
func (cl *ColumnLayout) Begin()
func (cl *ColumnLayout) NextColumn()
func (cl *ColumnLayout) End()
```

---

## PDF Tools (Pure Go — No External Dependencies)

### Split PDF

```go
func SplitPDF(pdfPath, outputDir string, opts ...SplitOption) ([]string, error)
```

Splits a PDF into multiple files. Returns paths of generated files.

Options:
- `WithRanges(ranges ...PageRange)` — split by custom page ranges (default: one file per page)

```go
// Split every page into a separate file.
paths, err := folio.SplitPDF("input.pdf", "output/")

// Split by custom ranges.
paths, err := folio.SplitPDF("input.pdf", "output/",
    folio.WithRanges(
        folio.PageRange{From: 1, To: 3},
        folio.PageRange{From: 4, To: 10},
    ),
)
```

### Merge PDF

```go
func MergePDF(outputPath string, inputPaths ...string) error
```

Combines multiple PDFs into a single file. Pages appear in input order.

```go
err := folio.MergePDF("combined.pdf", "doc1.pdf", "doc2.pdf", "doc3.pdf")
```

### Watermark PDF

```go
func WatermarkPDF(inputPath, outputPath string, opts ...WatermarkOption) error
```

Adds a watermark (text or image) to every page of an existing PDF.

**Text options:**
- `WatermarkText(text string)` — watermark text content
- `WatermarkFontSize(size float64)` — font size (default: 72)
- `WatermarkColor(r, g, b int)` — text color RGB 0-255 (default: gray)

**Image options:**
- `WatermarkImage(path string)` — path to JPEG or PNG image
- `WatermarkScale(scale float64)` — image scale factor (default: 1.0)

**General options:**
- `WatermarkOpacity(alpha float64)` — transparency 0-1 (default: 0.3)
- `WatermarkRotation(degrees float64)` — rotation angle (default: 45)
- `WatermarkCenter()` — center on page (default)
- `WatermarkPosition(x, y float64)` — absolute position in points
- `WatermarkPattern(gapX, gapY float64)` — repeat across page in a grid

**Templates** (presets that configure multiple options):
- `WatermarkTemplate("draft")` — gray "DRAFT", 45°, 30% opacity
- `WatermarkTemplate("confidential")` — red "CONFIDENTIAL", 45°, 20% opacity
- `WatermarkTemplate("copy")` — gray "COPY", 45°, 30% opacity
- `WatermarkTemplate("sample")` — gray "SAMPLE", 45°, 30% opacity
- `WatermarkTemplate("do-not-copy")` — red "DO NOT COPY", 45°, 25% opacity

Templates can be combined with other options to override individual settings.

```go
// Template watermark.
folio.WatermarkPDF("in.pdf", "out.pdf", folio.WatermarkTemplate("draft"))

// Custom text watermark with pattern.
folio.WatermarkPDF("in.pdf", "out.pdf",
    folio.WatermarkText("INTERNAL"),
    folio.WatermarkPattern(200, 200),
    folio.WatermarkFontSize(36),
    folio.WatermarkOpacity(0.1),
    folio.WatermarkColor(200, 0, 0),
)

// Image watermark.
folio.WatermarkPDF("in.pdf", "out.pdf",
    folio.WatermarkImage("logo.png"),
    folio.WatermarkOpacity(0.15),
    folio.WatermarkScale(0.5),
)

// Template + override.
folio.WatermarkPDF("in.pdf", "out.pdf",
    folio.WatermarkTemplate("confidential"),
    folio.WatermarkOpacity(0.5),
    folio.WatermarkRotation(30),
)
```

### Images to PDF

```go
func ImagesToPDF(outputPath string, imagePaths []string, opts ...ImageToPDFOption) error
```

Converts JPEG/PNG images into a single PDF — one page per image.

Options:
- `ImagePageSize(size PageSize)` — fixed page size (default: auto-fit to image)
- `ImageDPI(dpi float64)` — resolution for auto-fit sizing (default: 96)
- `ImageMargin(margin float64)` — uniform margin in points (default: 0)
- `ImageFit(mode string)` — `"fit"`, `"fill"`, or `"stretch"` (default: `"fit"`)

```go
// Auto-fit: each page matches its image size.
folio.ImagesToPDF("album.pdf", []string{"a.jpg", "b.png", "c.jpg"})

// Fixed A4 pages with margins.
folio.ImagesToPDF("album.pdf", images,
    folio.ImagePageSize(folio.A4),
    folio.ImageMargin(36),
    folio.ImageFit("fit"),
)
```

---

## PDF-to-Image Conversion

```go
func ConvertToImages(pdfPath, outputDir string, opts ...ConvertOption) ([]string, error)
```

Options:
- `WithDPI(dpi int)` — rendering resolution (default: 150)
- `WithFormat(f ImageFormat)` — `PNG` or `JPEG` (default: PNG)
- `WithPages(pages ...int)` — specific pages to convert (1-indexed; default: all)

Requires one of: `pdftoppm` (poppler-utils), `mutool` (mupdf-tools), or `gs` (ghostscript) on PATH.

---

## PageSize

```go
type PageSize struct {
    WidthPt  float64
    HeightPt float64
}

func (s PageSize) Landscape() PageSize
```

Pre-defined sizes: `A3`, `A4`, `A5`, `Letter`, `Legal` and their landscape variants (`A4Landscape`, etc.).

---

## Font Packages

### fonts/sarabun

```go
func Register(doc *folio.Document) error
```

Registers Sarabun (Thai font) in all 4 styles: Regular, Bold, Italic, BoldItalic. Font family name: `"sarabun"`.

### fonts/dejavu

```go
func Register(doc *folio.Document) error
```

Registers DejaVu Sans Condensed in all 4 styles. Font family name: `"dejavu"`.

---

## thai

```go
func Setup(doc *folio.Document)
func Segment(text string) []string
```

`Setup` installs the Thai word breaker so `MultiCell`, `Write`, and other wrapping functions break lines at Thai word boundaries.

`Segment` provides direct access to the word segmentation for use outside of folio's text layout.
