<p align="center">
  <img src="assets/logo-folio.png" alt="Folio" width="120">
</p>

# Architecture

This guide is for contributors and anyone curious about how Folio works internally.

## The four layers

Folio is built as four independent layers. Each has a single job and depends only on layers below it.

```
┌──────────────────────────────────────────────┐
│  folio (public API)                          │
│  Document, Page, options, serialization      │
├──────────────────────────────────────────────┤
│  internal/state      │  internal/resources   │
│  units, colors,      │  fonts, images,       │
│  coordinate xform    │  deduplication        │
├──────────────────────┴───────────────────────┤
│  internal/content                            │
│  PDF content stream operators (BT, m, re...) │
├──────────────────────────────────────────────┤
│  internal/pdfcore                            │
│  raw PDF objects, xref, trailer, streams     │
└──────────────────────────────────────────────┘
```

### What each layer knows (and doesn't)

| Layer | Knows about | Knows nothing about |
|-------|------------|-------------------|
| **pdfcore** | Object IDs, byte offsets, xref, trailer | Fonts, images, pages, coordinates |
| **content** | PDF operators (BT, m, re, Tj...) | Object IDs, document structure |
| **resources** | Font metrics, glyph data, image bytes, dedup | Serialization order, page layout |
| **state** | Unit conversion, color math | PDF syntax, fonts, pages |
| **folio** (root) | Everything — it orchestrates all layers | — |

This separation means you can add a new drawing primitive without touching serialization, or add a new resource type without touching the content stream.

## File map

```
folio/
├── doc.go              # package documentation
├── options.go          # PageSize constants, Option funcs
├── document.go         # Document struct and methods
├── page.go             # Page: text, shapes, transforms, clipping, gradients
├── builder.go          # TextBuilder and ShapeBuilder fluent APIs
├── serialize.go        # PDF serialization pipeline
├── escape.go           # PDF string escaping
├── table.go            # Table helper
├── autotable.go        # AutoTable from structs/JSON
├── barcode.go          # Code128, EAN13, QR
├── form.go             # AcroForms
├── template.go         # Page templates (Form XObjects)
├── toc.go              # Table of contents
├── columns.go          # Multi-column layout
├── html.go             # HTML subset renderer
├── markdown.go         # Markdown subset renderer
├── svg.go              # SVG path drawing
├── pdfa.go             # PDF/A compliance
├── signature.go        # Digital signatures
├── convert.go          # PDF-to-image conversion
│
├── internal/
│   ├── pdfcore/writer.go     # PDF object writer
│   ├── content/stream.go     # Content stream operators
│   ├── resources/
│   │   ├── fonts.go          # Font registry
│   │   ├── fonts_core.go     # 13 core font width tables
│   │   ├── ttf.go            # TrueType parser + CIDFont
│   │   └── images.go         # Image registry (SHA-1 dedup)
│   ├── state/
│   │   ├── state.go          # Color struct
│   │   └── units.go          # Unit conversion
│   ├── barcode/              # Barcode algorithms
│   ├── wordcut/              # Thai word segmentation
│   └── crypto/               # RC4 encryption
│
├── fonts/sarabun/            # Embedded Sarabun (Thai)
├── fonts/dejavu/             # Embedded DejaVu Sans Condensed
├── thai/                     # Thai language setup
├── cmd/demo/                 # Getting-started demo
└── examples/                 # Feature examples
```

## How a PDF is built

### Step 1: User draws content

```go
doc := folio.New()
doc.SetFont("helvetica", "", 16)
page := doc.AddPage(folio.A4)
page.TextAt(40, 60, "Hello")
```

Each call appends PDF operators to the page's `content.Stream` buffer. Coordinates are converted from user units (mm, top-left) to PDF points (bottom-left) at call time:

```
x_pdf = x_user * k
y_pdf = (pageHeight - y_user) * k
```

where `k = 72 / 25.4 ≈ 2.8346` for millimeters.

### Step 2: Serialization

When `Save()` or `WriteTo()` is called, `serialize()` runs:

```
WriteHeader("1.4")
  ├─ putPages()         page dicts + content streams, then Pages root (obj 1)
  ├─ putFonts()         /Type /Font for each font
  ├─ putImages()        /Type /XObject for each image
  ├─ putResourceDict()  shared resource dict (obj 2)
  ├─ putInfo()          metadata (title, author, dates)
  └─ putCatalog()       /Type /Catalog → Pages root
      ├─ WriteXref()     cross-reference table
      ├─ WriteTrailer()  /Root + /Info refs
      └─ WriteStartXref() + %%EOF
```

### Step 3: Object numbering

Objects 1 and 2 are reserved (written last via deferred offsets):

| Obj | Content | Written by |
|-----|---------|------------|
| 1 | Pages root | putPages (deferred) |
| 2 | Resource dict | putResourceDict (deferred) |
| 3 | Page dict | putPages |
| 4 | Content stream | putPages |
| 5 | Font: Helvetica | putFonts |
| 6+ | ... | ... |

## The PDF file structure

```
%PDF-1.4                          ← header
3 0 obj ... endobj                ← objects (sequential)
4 0 obj ... stream ... endstream endobj
1 0 obj ... endobj                ← Pages root (written last)
2 0 obj ... endobj                ← Resource dict (written last)
xref                              ← byte offset of each object
0 N
0000000000 65535 f
0000000XXX 00000 n
...
trailer
<< /Size N /Root R /Info I >>
startxref
XXXX
%%EOF
```

## Content stream operators

PDF operators emitted by `internal/content/stream.go`:

| Operator | Method | What it does |
|----------|--------|-------------|
| `q` / `Q` | SaveState / RestoreState | Push/pop graphics state |
| `w` | SetLineWidth | Line width |
| `RG` / `rg` | SetStrokeColorRGB / SetFillColorRGB | Colors |
| `BT` / `ET` | BeginText / EndText | Text block |
| `Tf` | SetFont | Font name + size |
| `Td` | MoveText | Move text position |
| `Tj` | ShowText | Draw text |
| `m` / `l` / `c` | MoveTo / LineTo / CurveTo | Path building |
| `re` | Rect | Rectangle |
| `S` / `f` / `B` | Stroke / Fill / FillStroke | Path painting |
| `cm` + `Do` | DrawImage | Place image XObject |

## Error handling

Folio uses error accumulation. The `Document` stores a single `err` field. Every method checks it first and returns early if set. This lets users write linear code without `if err != nil` on every line:

```go
doc := folio.New()
doc.SetFont("bad", "", 12)  // sets d.err
page := doc.AddPage(folio.A4) // no-op, returns dummy page
page.TextAt(10, 10, "test")   // no-op
err := doc.Save("out.pdf")    // returns the stored error
```

## How to add a new feature

### New drawing primitive

1. Add the PDF operator to `internal/content/stream.go`
2. Add the public method to `page.go` (convert coordinates, call stream method)
3. Add a test

No changes needed in pdfcore, resources, or serialize.

### New resource type (e.g., PNG images)

1. Add parsing in `internal/resources/` (decode format, compute dedup hash)
2. Update `putImages()` in `serialize.go` to write the new format
3. Update `RegisterImage()` in `document.go` to detect and route the format

### New document-level feature

1. Add state to `Document` struct in `document.go`
2. Add public setter/config methods
3. Hook into serialization in `serialize.go` if the feature produces PDF objects

## Core fonts reference

13 standard fonts with pre-built width tables (WinAnsiEncoding):

| Key | PDF Name |
|-----|----------|
| `helvetica` | Helvetica |
| `helveticaB` | Helvetica-Bold |
| `helveticaI` | Helvetica-Oblique |
| `helveticaBI` | Helvetica-BoldOblique |
| `courier` | Courier |
| `courierB` | Courier-Bold |
| `courierI` | Courier-Oblique |
| `courierBI` | Courier-BoldOblique |
| `times` | Times-Roman |
| `timesB` | Times-Bold |
| `timesI` | Times-Italic |
| `timesBI` | Times-BoldItalic |
| `zapfdingbats` | ZapfDingbats |

## Running tests

```bash
go test ./...                    # all tests
go test ./... -v                 # verbose
go test ./internal/pdfcore/ -v   # specific package
go run ./cmd/demo/               # generate demo PDF
```
