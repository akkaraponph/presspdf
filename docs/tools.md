# PDF Tools

Folio includes pure Go tools for manipulating existing PDF files. These work with any valid PDF — not just files created by Folio. No external binaries required.

## Split PDF

Split a PDF into multiple files — one per page, or by custom page ranges.

```go
// Split every page into a separate file.
paths, err := folio.SplitPDF("report.pdf", "output/")
// output/page-001.pdf, output/page-002.pdf, ...

// Split by page ranges.
paths, err := folio.SplitPDF("report.pdf", "output/",
    folio.WithRanges(
        folio.PageRange{From: 1, To: 5},   // pages 1-5
        folio.PageRange{From: 6, To: 10},  // pages 6-10
    ),
)
// output/pages-001.pdf, output/pages-002.pdf
```

Returns the paths of all generated files in order.

## Merge PDF

Combine multiple PDFs into a single file. Pages appear in the order the files are listed.

```go
err := folio.MergePDF("combined.pdf",
    "chapter1.pdf",
    "chapter2.pdf",
    "appendix.pdf",
)
```

The output uses the highest PDF version among the inputs.

### Split + Merge round-trip

Split and merge compose naturally:

```go
// Extract pages 3-5 from a document.
parts, _ := folio.SplitPDF("big.pdf", "tmp/",
    folio.WithRanges(folio.PageRange{From: 3, To: 5}),
)

// Merge with another document.
folio.MergePDF("result.pdf", parts[0], "extra.pdf")
```

## Watermark PDF

Add text or image watermarks to every page of an existing PDF.

### Text watermark

```go
err := folio.WatermarkPDF("input.pdf", "output.pdf",
    folio.WatermarkText("DRAFT"),
    folio.WatermarkFontSize(100),
    folio.WatermarkColor(200, 200, 200),
    folio.WatermarkOpacity(0.3),
    folio.WatermarkRotation(45),
)
```

### Image watermark

Supports JPEG and PNG (including transparency).

```go
err := folio.WatermarkPDF("input.pdf", "output.pdf",
    folio.WatermarkImage("logo.png"),
    folio.WatermarkOpacity(0.15),
    folio.WatermarkScale(0.5),
)
```

### Templates

Pre-configured watermark presets for common use cases:

```go
folio.WatermarkPDF("in.pdf", "out.pdf", folio.WatermarkTemplate("draft"))
folio.WatermarkPDF("in.pdf", "out.pdf", folio.WatermarkTemplate("confidential"))
folio.WatermarkPDF("in.pdf", "out.pdf", folio.WatermarkTemplate("copy"))
folio.WatermarkPDF("in.pdf", "out.pdf", folio.WatermarkTemplate("sample"))
folio.WatermarkPDF("in.pdf", "out.pdf", folio.WatermarkTemplate("do-not-copy"))
```

| Template | Text | Color | Size | Opacity |
|----------|------|-------|------|---------|
| `draft` | DRAFT | Gray | 120pt | 30% |
| `confidential` | CONFIDENTIAL | Red | 72pt | 20% |
| `copy` | COPY | Gray | 120pt | 30% |
| `sample` | SAMPLE | Gray | 100pt | 30% |
| `do-not-copy` | DO NOT COPY | Red | 72pt | 25% |

Templates can be combined with other options to override individual settings:

```go
folio.WatermarkPDF("in.pdf", "out.pdf",
    folio.WatermarkTemplate("confidential"),
    folio.WatermarkOpacity(0.5),        // override opacity
    folio.WatermarkRotation(30),        // override angle
)
```

### Pattern mode

Repeat the watermark across the entire page in a grid:

```go
folio.WatermarkPDF("in.pdf", "out.pdf",
    folio.WatermarkText("INTERNAL"),
    folio.WatermarkPattern(180, 180),   // spacing in points
    folio.WatermarkFontSize(28),
    folio.WatermarkOpacity(0.08),
)
```

Use `0` for automatic spacing based on the watermark size.

### Position control

By default, the watermark is centered on each page. Override with:

```go
// Absolute position (in PDF points from bottom-left).
folio.WatermarkPosition(400, 30)

// Center (default).
folio.WatermarkCenter()
```

### All watermark options

| Option | Default | Description |
|--------|---------|-------------|
| `WatermarkText(s)` | — | Text content |
| `WatermarkImage(path)` | — | Image file (JPEG/PNG) |
| `WatermarkFontSize(pt)` | 72 | Text font size |
| `WatermarkColor(r,g,b)` | Gray | Text color (0-255) |
| `WatermarkOpacity(a)` | 0.3 | Transparency (0-1) |
| `WatermarkRotation(deg)` | 45 | Rotation angle |
| `WatermarkScale(s)` | 1.0 | Image scale factor |
| `WatermarkPosition(x,y)` | Center | Absolute position (points) |
| `WatermarkCenter()` | Yes | Center on page |
| `WatermarkPattern(gx,gy)` | Off | Repeat in grid |
| `WatermarkTemplate(name)` | — | Apply a preset |

## PDF-to-Image Conversion

Convert PDF pages to PNG or JPEG images. This feature requires an external renderer on PATH.

```go
// Convert all pages to PNG.
paths, err := folio.ConvertToImages("doc.pdf", "images/")

// Convert specific pages to JPEG at 300 DPI.
paths, err := folio.ConvertToImages("doc.pdf", "images/",
    folio.WithFormat(folio.JPEG),
    folio.WithDPI(300),
    folio.WithPages(1, 3, 5),
)

// Single page to in-memory image.
img, err := folio.ConvertPage("doc.pdf", 1)
```

Supported renderers (tried in order): `pdftoppm` (poppler-utils), `mutool` (mupdf-tools), `gs` (ghostscript).
