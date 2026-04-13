# Folio — Claude Code Instructions

## Project overview

Folio is a layered PDF generation library for Go with zero external dependencies. Module: `github.com/akkaraponph/folio`. Go 1.26+.

## Architecture

4 internal layers — each has one job and depends only on layers below:

| Layer | Package | Job |
|-------|---------|-----|
| 4 | `internal/pdfcore` | Raw PDF syntax: objects, xref, trailer |
| 3 | `internal/content` | PDF content stream operators (BT, Tj, m, re...) |
| 2 | `internal/resources` | Font/image registry and deduplication |
| 1 | `internal/state` | Unit conversion and color math |

The root `folio` package is the public API that orchestrates all layers.

**Key rule:** Layers must not import upward. `pdfcore` knows nothing about fonts. `content` knows nothing about object IDs. `resources` knows nothing about serialization.

## Code conventions

- Error accumulation: `Document` stores one `err` field. Every method checks `d.err != nil` at entry and returns early. No `if err != nil` on every call.
- Coordinates: users work in top-left origin with configurable units (default mm). Conversion to PDF bottom-left points happens at call time in `page.go`.
- Style parameter: `"D"` = stroke, `"F"` = fill, `"DF"` = both (for shapes).
- Font style: `""` (regular), `"B"` (bold), `"I"` (italic), `"BI"` (bold italic).
- Colors: always 0-255 RGB integers at the public API, normalized to 0.0-1.0 internally.

## Where things live

- **New drawing primitives** → add operator to `internal/content/stream.go`, then public method in `page.go`
- **New resource types** → add to `internal/resources/`, update `serialize.go`
- **New document features** → add state to `document.go`, hook into `serialize.go`
- **Fluent builders** → `builder.go` (TextBuilder, ShapeBuilder)
- **Tables** → `table.go` (manual), `autotable.go` (reflection/JSON)
- **Serialization pipeline** → `serialize.go` (putPages, putFonts, putImages, putResourceDict, putInfo, putCatalog)

## Testing

```bash
go test ./...           # all tests
go test ./... -v        # verbose
go test -run TestFoo    # specific test
go run ./cmd/demo       # generate demo PDF to /tmp/folio_demo.pdf
go run ./examples/showcase  # generate all feature PDFs
```

Tests write PDFs to `bytes.Buffer` and verify PDF structure strings. Visual verification: save to /tmp and `open` the file.

## Common tasks

- After adding a new Page method, add a test in `folio_test.go`
- After changing serialization, verify with `go run ./cmd/demo && open /tmp/folio_demo.pdf`
- After changing font handling, test with both core fonts AND TTF (sarabun)
- After changing text layout, test with Thai text (needs word segmentation)

## Do NOT

- Add external dependencies — this library is intentionally zero-dep
- Break the layer isolation (no upward imports in internal packages)
- Use `fmt.Errorf` in hot paths — PDF operators should write directly to buffer
- Change the public API signatures without good reason — users depend on them
