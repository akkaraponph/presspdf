Help me add a new feature to the Folio PDF library.

Ask me what feature I want to add, then follow the architecture rules:

1. **Drawing primitive?** → Add PDF operator to `internal/content/stream.go`, then public method in `page.go` with coordinate conversion
2. **Resource type?** → Add parsing in `internal/resources/`, update `putImages()` or `putFonts()` in `serialize.go`
3. **Document feature?** → Add state to `document.go`, hook serialization in `serialize.go`
4. **High-level helper?** → New file in root package (like `table.go`, `barcode.go`)

Rules:
- No external dependencies
- Don't break layer isolation (internal packages must not import upward)
- Add test in `folio_test.go`
- Follow error accumulation pattern (check `d.err` at method entry)
- Convert user coordinates to PDF points at call time

After implementation, run `go test ./...` to verify.

Feature to add: $ARGUMENTS
