# Getting Started

This guide walks you through creating your first PDF with Folio.

## Installation

```bash
go get github.com/akkaraponph/folio
```

## Your first PDF

Every Folio program follows the same pattern: create a document, add pages, draw content, save.

```go
package main

import "github.com/akkaraponph/folio"

func main() {
    // 1. Create a document
    doc := folio.New()
    doc.SetTitle("My First PDF")
    doc.SetFont("helvetica", "", 12)

    // 2. Add a page
    page := doc.AddPage(folio.A4)

    // 3. Draw content
    page.TextAt(20, 20, "Hello, World!")

    // 4. Save
    if err := doc.Save("output.pdf"); err != nil {
        panic(err)
    }
}
```

That's it. Run `go run main.go` and open `output.pdf`.

## Coordinate system

Folio uses a **top-left origin** with millimeters as the default unit. The point (0, 0) is the top-left corner of the page.

```
(0,0) ──────────────────── x
  │
  │     • (20, 30)
  │       Your text goes here
  │
  y
```

A4 in millimeters is 210 wide x 297 tall. You can change units:

```go
import "github.com/akkaraponph/folio/internal/state"

doc := folio.New(folio.WithUnit(state.UnitInch))  // inches
doc := folio.New(folio.WithUnit(state.UnitPt))    // PDF points (1/72 inch)
doc := folio.New(folio.WithUnit(state.UnitCM))    // centimeters
```

## Adding text

There are several ways to put text on a page:

### TextAt — absolute positioning

Place text at exact coordinates. The cursor doesn't move.

```go
page.TextAt(20, 30, "Positioned exactly here")
```

### Cell — single-line box

A cell is a rectangular box with text inside. Useful for labels, table cells, and aligned content.

```go
// Cell(width, height, text, border, align, fill, lineBreak)
page.SetXY(20, 40)
page.Cell(80, 8, "Left aligned", "1", "L", false, 0)
page.Cell(80, 8, "Centered", "1", "C", false, 1)  // ln=1 moves to next line
```

Parameters:
- `border`: `""` (none), `"1"` (all sides), `"LR"` (left+right), etc.
- `align`: `"L"` (left), `"C"` (center), `"R"` (right)
- `fill`: `true` to fill background with current fill color
- `ln`: `0` = cursor moves right, `1` = next line, `2` = below

### MultiCell — word-wrapped text

For paragraphs and flowing text. Wraps at the specified width.

```go
page.SetXY(20, 60)
page.MultiCell(170, 6,
    "This text wraps automatically when it reaches the edge. "+
        "Each wrapped line becomes its own cell. "+
        "Great for paragraphs.",
    "", "L", false)
```

Set width to `0` to extend to the right margin.

Alignment options: `"L"`, `"C"`, `"R"`, `"J"` (justified).

### Write — inline text

Text that flows inline from the current cursor position. Good for mixed formatting.

```go
doc.SetFont("helvetica", "", 12)
page.Write(6, "This is normal text. ")
doc.SetFont("helvetica", "B", 12)
page.Write(6, "This is bold. ")
doc.SetFont("helvetica", "", 12)
page.Write(6, "Back to normal.")
```

## Drawing shapes

```go
// Line
page.Line(20, 100, 190, 100)

// Rectangle: "D" = stroke, "F" = fill, "DF" = both
page.Rect(20, 110, 80, 30, "D")

// Filled rectangle
doc.SetFillColor(200, 220, 255)
page.Rect(20, 110, 80, 30, "DF")

// Circle
page.Circle(60, 170, 15, "DF")
```

## Colors

Three color channels: stroke (outlines), fill (backgrounds), text.

```go
doc.SetDrawColor(255, 0, 0)    // red outlines
doc.SetFillColor(200, 230, 255) // light blue fills
doc.SetTextColor(0, 0, 128)     // dark blue text
```

All values are 0-255 RGB.

## Multiple pages

```go
page1 := doc.AddPage(folio.A4)
page1.TextAt(20, 20, "Page 1")

page2 := doc.AddPage(folio.Letter)  // different size is fine
page2.TextAt(20, 20, "Page 2")
```

Available sizes: `A3`, `A4`, `A5`, `Letter`, `Legal`. For landscape:

```go
page := doc.AddPage(folio.A4.Landscape())
```

## Images

Register a JPEG image, then draw it on any page:

```go
f, _ := os.Open("photo.jpg")
doc.RegisterImage("photo", f)
f.Close()

page.DrawImageRect("photo", 20, 20, 60, 40)  // x, y, width, height
```

Images are deduplicated by content hash — register the same image twice and it only appears once in the PDF.

## Error handling

Folio accumulates errors instead of returning them on every call. This lets you write clean, linear code:

```go
doc := folio.New()
doc.SetFont("nonexistent", "", 12)  // error stored internally
page := doc.AddPage(folio.A4)       // returns a no-op page
page.TextAt(10, 10, "test")         // silently skipped

// Check at the end
if err := doc.Save("out.pdf"); err != nil {
    log.Fatal(err)
}

// Or check at any point
if doc.Err() != nil {
    log.Fatal(doc.Err())
}
```

## What's next?

- [Text & Fonts](text-and-fonts.md) — TrueType fonts, Thai language, typography settings
- [Layout](layout.md) — margins, automatic page breaks, headers/footers
- [Tables](tables.md) — styled tables, auto-tables from structs
- [Drawing](drawing.md) — circles, SVG paths, transforms, gradients
- [Rich Content](rich-content.md) — render HTML and Markdown directly
- [Security](security.md) — encryption, signatures, forms, PDF/A
- [PDF Tools](tools.md) — split, merge, watermark, convert (pure Go)
