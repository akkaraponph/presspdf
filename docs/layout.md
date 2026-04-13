<p align="center">
  <img src="assets/logo-folio.png" alt="Folio" width="120">
</p>

# Layout

## Page sizes

Built-in sizes (portrait):

| Constant | Dimensions (mm) |
|----------|-----------------|
| `A3` | 297 x 420 |
| `A4` | 210 x 297 |
| `A5` | 148 x 210 |
| `Letter` | 216 x 279 |
| `Legal` | 216 x 356 |

Landscape:

```go
page := doc.AddPage(folio.A4.Landscape())  // 297 x 210
```

Custom size (values in PDF points, 1 pt = 1/72 inch):

```go
page := doc.AddPage(folio.PageSize{WidthPt: 400, HeightPt: 600})
```

Different pages can use different sizes in the same document.

## Margins

Set left, top, and right margins. These affect where `MultiCell` and `Write` wrap text.

```go
doc.SetMargins(15, 15, 15)  // 15mm on each side
```

The cursor starts at the top-left margin after `AddPage`.

## Automatic page breaks

When enabled, content that would overflow the page automatically creates a new page:

```go
doc.SetAutoPageBreak(true, 15)  // break 15mm from bottom
```

The second parameter is the bottom margin — when the cursor crosses this threshold, a new page is created. Text wrapping in `MultiCell` and `Write` respects this automatically.

## Headers and footers

Register callbacks that run on every page:

```go
doc.SetHeaderFunc(func(p *folio.Page) {
    p.SetFont("helvetica", "B", 10)
    p.TextAt(20, 10, "ACME Corp")
    p.Line(20, 14, 190, 14)
})

doc.SetFooterFunc(func(p *folio.Page) {
    p.SetFont("helvetica", "", 8)
    p.TextAt(20, 285, fmt.Sprintf("Page %d", p.Doc().PageNo()))
})
```

- Headers run at the start of each `AddPage`
- Footers run before each page break and during serialization for the last page
- Graphics state (font, color, line width) is saved before and restored after each callback — your drawing code won't interfere with the page content

## The cursor

Folio tracks a cursor position (X, Y) on the current page. `Cell`, `MultiCell`, and `Write` use and advance the cursor.

```go
page.SetXY(20, 50)   // move cursor
x := page.GetX()     // read position
y := page.GetY()

page.SetX(20)        // move X only
page.SetY(100)       // move Y only (also resets X to left margin)
```

`TextAt` does **not** move the cursor — it draws at an absolute position.

## Multi-column layout

Flow text across multiple columns:

```go
doc.SetFont("helvetica", "", 10)
page.SetXY(10, 30)

cols := folio.NewColumnLayout(doc, page, 3, 5)  // 3 columns, 5mm gutter
cols.Begin()

page.MultiCell(0, 5, "First column text...", "", "J", false)

cols.NextColumn()
page.MultiCell(0, 5, "Second column text...", "", "J", false)

cols.NextColumn()
page.MultiCell(0, 5, "Third column text...", "", "J", false)

cols.End()
```

`Begin()` sets the page width to a single column. `NextColumn()` moves the cursor to the next column. `End()` restores the full page width.

## Layout helpers

### Spacer

Add vertical space, triggering a page break if needed:

```go
page.Spacer(10)  // 10mm vertical gap
```

### PageBreakIfNeeded

Ensure a minimum amount of space remains on the current page:

```go
if page.PageBreakIfNeeded(50) {
    // A new page was created because < 50mm remained
}
```

### KeepTogether

Guarantee a block of content stays on a single page:

```go
page.KeepTogether(func() {
    page.Cell(0, 8, "Section Title", "", "L", false)
    page.MultiCell(0, 5, paragraphText, "", "L", false)
})
```

Folio does a measurement pass first. If the content fits, it draws on the current page. Otherwise it forces a page break before drawing.

### Paragraph

A convenience for multi-line text blocks with consistent spacing:

```go
page.Paragraph(170, 6, longText, "J")  // width, lineH, text, align
```

### Stack

Arrange blocks vertically with spacing:

```go
page.Stack(8, func() {
    page.Cell(0, 8, "Title", "", "L", false)
}, func() {
    page.MultiCell(0, 5, body, "", "L", false)
}, func() {
    page.Cell(0, 8, "Footer", "", "L", false)
})
```

## Page templates

Define a reusable layout (letterhead, watermark, border) once and stamp it on multiple pages:

```go
// Define the template
tpl := doc.BeginTemplate(folio.A4)
tpl.SetFillColorRGB(30, 60, 120)
tpl.Rect(0, 0, 210, 25, "F")
tpl.SetFont("helvetica", "B", 16)
tpl.TextAt(15, 10, "ACME Corporation")
name := doc.EndTemplate()

// Use on any page
page1 := doc.AddPage(folio.A4)
page1.UseTemplate(name, 0, 0, 210, 297)
page1.TextAt(15, 35, "Page 1 content...")

page2 := doc.AddPage(folio.A4)
page2.UseTemplate(name, 0, 0, 210, 297)
page2.TextAt(15, 35, "Page 2 content...")
```

Templates are stored as Form XObjects in the PDF — the template data appears only once regardless of how many pages use it.

## Table of contents

Build a TOC with clickable entries:

```go
// Reserve a page for the TOC
tocPage := doc.AddPage(folio.A4)
doc.SetFont("helvetica", "B", 20)
tocPage.TextAt(20, 20, "Table of Contents")

toc := folio.NewTOC(doc)

// Add content pages and register them
page1 := doc.AddPage(folio.A4)
doc.SetFont("helvetica", "B", 18)
page1.TextAt(20, 25, "Chapter 1")
toc.Add("Chapter 1", 0, page1, 25)

page2 := doc.AddPage(folio.A4)
page2.TextAt(20, 25, "Chapter 2")
toc.Add("Chapter 2", 0, page2, 25)

// Render the TOC (on the reserved page)
doc.SetFont("helvetica", "", 11)
toc.RenderWithPageNums(tocPage, 6, -1)
```

TOC entries include dot leaders and page numbers. The `level` parameter (0, 1, 2...) controls indentation for nested sections.

## Bookmarks

Add document outline entries (the sidebar navigation in PDF viewers):

```go
doc.AddBookmark("Chapter 1", 0)   // level 0 = top
doc.AddBookmark("Section 1.1", 1) // level 1 = nested
doc.AddBookmark("Section 1.2", 1)
doc.AddBookmark("Chapter 2", 0)
```

Call `AddBookmark` just before drawing the corresponding heading on the page.
