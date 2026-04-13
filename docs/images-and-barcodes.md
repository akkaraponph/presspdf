<p align="center">
  <img src="assets/logo-folio.png" alt="Folio" width="120">
</p>

# Images & Barcodes

## JPEG images

Register an image by name, then draw it on any page:

```go
// Register from file
f, _ := os.Open("logo.jpg")
doc.RegisterImage("logo", f)
f.Close()

// Register from bytes
doc.RegisterImage("photo", bytes.NewReader(jpegBytes))

// Draw on a page (x, y, width, height in current units)
page.DrawImageRect("logo", 20, 20, 40, 20)
page.DrawImageRect("photo", 20, 50, 80, 60)
```

### Image deduplication

Images are deduplicated by SHA-1 content hash. If you register the same image data twice (even under different names), it appears only once in the PDF file. This keeps file sizes small when the same logo or graphic appears on multiple pages.

### Supported formats

Currently only JPEG is supported. PNG support is planned.

## Barcodes

### Code 128

General-purpose barcode for alphanumeric data:

```go
page.Barcode128(20, 20, 80, 20, "FOLIO-2026")
//              x   y   w   h   data
```

Supports the full ASCII range. Automatically switches between Code 128 character sets for optimal encoding.

### EAN-13

Standard retail barcode (13 digits). The check digit is calculated automatically:

```go
page.BarcodeEAN13(20, 50, 60, 20, "4901234567894")
//                x   y   w   h   digits
```

### QR codes

```go
page.QRCode(20, 80, 40, "https://example.com", 1)
//          x   y   size  data               ecLevel
```

Error correction levels:
- `0` — Level L (7% recovery)
- `1` — Level M (15% recovery)
- `2` — Level Q (25% recovery)
- `3` — Level H (30% recovery)

Higher levels make the QR code larger but more resilient to damage.

### Barcode example: invoice

```go
doc := folio.New()
doc.SetFont("helvetica", "", 10)
page := doc.AddPage(folio.A4)

// Product barcode
page.TextAt(20, 20, "Product:")
page.BarcodeEAN13(20, 24, 50, 18, "4901234567894")

// Invoice reference
page.TextAt(20, 50, "Invoice:")
page.Barcode128(20, 54, 70, 16, "INV-2026-04-001")

// Payment QR code
page.TextAt(20, 80, "Pay here:")
page.QRCode(20, 84, 35, "https://pay.example.com/inv/001", 1)

doc.Save("invoice.pdf")
```

## PDF-to-image conversion

Convert PDF pages to PNG or JPEG images. This requires an external renderer on your system PATH.

```go
files, err := folio.ConvertToImages("input.pdf", "output/",
    folio.WithDPI(300),
    folio.WithFormat(folio.JPEG),
    folio.WithPages(1, 2, 3),  // specific pages (1-indexed)
)
// files = ["output/page-1.jpg", "output/page-2.jpg", "output/page-3.jpg"]
```

### Options

| Option | Default | Description |
|--------|---------|-------------|
| `WithDPI(n)` | 150 | Rendering resolution |
| `WithFormat(f)` | PNG | Output format: `folio.PNG` or `folio.JPEG` |
| `WithPages(p...)` | all | Specific pages to convert (1-indexed) |

### Required external tools

One of these must be installed:

| Tool | Package | Install |
|------|---------|---------|
| `pdftoppm` | poppler-utils | `brew install poppler` / `apt install poppler-utils` |
| `mutool` | mupdf-tools | `brew install mupdf-tools` / `apt install mupdf-tools` |
| `gs` | ghostscript | `brew install ghostscript` / `apt install ghostscript` |

Folio auto-detects whichever is available on PATH.
