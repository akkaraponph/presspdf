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

## Images to PDF

Convert JPEG and PNG images into a PDF — one page per image.

```go
// Auto-fit: each page sized to its image.
err := folio.ImagesToPDF("album.pdf", []string{
    "photo1.jpg",
    "photo2.jpg",
    "scan.png",
})

// Fixed A4 pages with margins, images scaled to fit.
err := folio.ImagesToPDF("album.pdf", images,
    folio.ImagePageSize(folio.A4),
    folio.ImageMargin(36),       // 0.5 inch margin
    folio.ImageFit("fit"),       // preserve aspect ratio
)

// High DPI (smaller pages in auto-fit mode).
err := folio.ImagesToPDF("hires.pdf", images, folio.ImageDPI(300))
```

### Options

| Option | Default | Description |
|--------|---------|-------------|
| `ImagePageSize(size)` | Auto-fit | Fixed page size (e.g. `A4`, `Letter`) |
| `ImageDPI(dpi)` | 96 | Resolution for auto-fit page sizing |
| `ImageMargin(pt)` | 0 | Uniform margin in points |
| `ImageFit(mode)` | `"fit"` | Image scaling on fixed pages |

### Fit modes (for fixed page sizes)

| Mode | Behavior |
|------|----------|
| `"fit"` | Scale to fit within page, preserve aspect ratio |
| `"fill"` | Scale to cover page, preserve aspect ratio (may crop) |
| `"stretch"` | Stretch to fill page exactly (may distort) |

---

## Decrypt PDF

Remove password protection from an encrypted PDF. Pure Go — no external tools required.

```go
// Decrypt with user password.
err := folio.DecryptPDF("locked.pdf", "unlocked.pdf", "mypassword")

// Decrypt with owner password.
err := folio.DecryptPDF("locked.pdf", "unlocked.pdf", "ownerpass")

// Not encrypted — just copies the file.
err := folio.DecryptPDF("plain.pdf", "output.pdf", "")
```

### What it does

1. **Password verification** — tries the password as user password first, then as owner password.
2. **Stream decryption** — decrypts all streams and strings using per-object RC4 keys derived from the file encryption key.
3. **Clean output** — writes a new PDF without the `/Encrypt` dictionary or file ID, producing a fully unprotected file.

### Supported encryption

| Version | Revision | Algorithm | Key Length |
|---------|----------|-----------|------------|
| V=1 | R=2 | RC4 | 40-bit |

This matches the encryption produced by `SetProtection()`. Higher encryption versions (V=2/V=4, AES-128/256) are not yet supported.

---

## Compress PDF

Rewrite a PDF with compressed streams and optional image quality reduction. Pure Go — no external tools required.

```go
// Basic compression: FlateDecode + object deduplication.
err := folio.CompressPDF("input.pdf", "smaller.pdf")

// Re-encode JPEG images at lower quality (1-100).
err := folio.CompressPDF("photos.pdf", "smaller.pdf",
    folio.CompressImageQuality(60),
)

// Disable deduplication.
err := folio.CompressPDF("input.pdf", "output.pdf",
    folio.CompressDedup(false),
)
```

### What it does

1. **Stream compression** — uncompressed streams are compressed with FlateDecode (zlib). Already-compressed streams are kept as-is.
2. **JPEG re-encoding** — when `CompressImageQuality` is set, JPEG images (DCTDecode) are decoded and re-encoded at the target quality. Only applied if the result is smaller than the original.
3. **Object deduplication** — identical objects (by SHA-256 hash) are merged into a single copy, reducing redundancy in multi-page documents.

### Options

| Option | Default | Description |
|--------|---------|-------------|
| `CompressImageQuality(q)` | 0 (off) | Re-encode JPEGs at quality 1-100 |
| `CompressDedup(on)` | true | Merge identical objects |

---

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
