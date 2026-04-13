# Copilot Instructions — Folio

## What is this project?

Folio is a layered PDF generation library for Go (`github.com/akkaraponph/folio`). Zero external dependencies. Go 1.26+.

## Architecture — 4 layers

```
folio (public API) → orchestrates everything
  ├── internal/state      → unit conversion (mm→pt), color math
  ├── internal/resources  → font/image registry, deduplication
  ├── internal/content    → PDF content stream operators
  └── internal/pdfcore    → raw PDF objects, xref, trailer
```

**Critical rule:** layers must not import upward. `pdfcore` knows nothing about fonts. `content` knows nothing about object IDs.

## Code patterns to follow

### Error accumulation
Every method on `Document` and `Page` must check the error field first:
```go
func (d *Document) SomeMethod(...) {
    if d.err != nil {
        return
    }
    // actual work
}
```

### Coordinate conversion
User coordinates (top-left, user units) must be converted to PDF points (bottom-left) at call time:
```go
func (p *Page) SomeDrawing(x, y float64) {
    k := p.doc.k
    xPt := state.ToPointsX(x, k)
    yPt := state.ToPointsY(y, p.h, k)
    p.stream.SomeOperator(xPt, yPt)
}
```

### Style parameter
Shapes use `"D"` (stroke), `"F"` (fill), `"DF"` (both).

### Font style
`""` = regular, `"B"` = bold, `"I"` = italic, `"BI"` = bold italic.

### Colors
Public API: 0-255 RGB integers. Internal: 0.0-1.0 float64.

## Where to add things

| Feature type | Where |
|-------------|-------|
| Drawing primitive | `internal/content/stream.go` + `page.go` |
| Resource type | `internal/resources/` + `serialize.go` |
| Document feature | `document.go` + `serialize.go` |
| High-level helper | New file in root (like `table.go`) |
| Fluent builder | `builder.go` |

## Do NOT

- Add external dependencies (zero-dep is a design constraint)
- Break layer isolation
- Skip error accumulation checks
- Forget coordinate conversion in Page methods
- Use `fmt.Sprintf` in hot paths (write directly to buffer)

## Testing

Tests are in `folio_test.go`. They write PDFs to `bytes.Buffer` and verify structure.

```bash
go test ./...
go run ./cmd/demo        # visual verification
go run ./examples/showcase  # all features
```

## Key types

- `Document` — top-level, holds state, fonts, pages
- `Page` — drawing surface with content stream
- `PageSize` — `{WidthPt, HeightPt}` (A4, Letter, etc.)
- `Table` / `AutoTable` — tabular layout helpers
- `TextBuilder` / `ShapeBuilder` — fluent drawing APIs
- `CellStyle` — `{FillColor, TextColor, FontStyle}`

## Serialization pipeline (serialize.go)

```
putPages → putFonts → putImages → putResourceDict → putInfo → putCatalog → xref → trailer
```

Objects 1 (Pages root) and 2 (Resource dict) are reserved and written last via deferred offsets.
