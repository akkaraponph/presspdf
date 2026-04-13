Review the current uncommitted changes for code quality, correctness, and adherence to the Folio project conventions.

Check for:
- Layer isolation violations (internal packages importing upward)
- Missing error accumulation checks (`d.err != nil` at method entry)
- Coordinate conversion issues (user units → PDF points)
- Missing tests for new public methods
- Consistent style parameter handling ("D", "F", "DF")
- Color value ranges (0-255 at API, 0.0-1.0 internally)
- No external dependencies introduced
- PDF spec correctness (operators, object structure)

```bash
git diff
```

Provide a summary of issues found and suggestions.
