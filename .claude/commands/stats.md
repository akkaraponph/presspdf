Show project statistics: line counts by package, test coverage, and file structure overview.

```bash
echo "=== Lines of Go code ==="
find . -name "*.go" -not -path "./vendor/*" -type f -exec wc -l {} + | sort -n

echo ""
echo "=== Test coverage ==="
go test ./... -cover

echo ""
echo "=== Package structure ==="
go list ./...
```
