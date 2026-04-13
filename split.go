package folio

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/akkaraponph/folio/internal/pdfcore"
)

// PageRange represents a range of pages to extract (1-indexed, inclusive).
type PageRange struct {
	From int // first page (1-indexed)
	To   int // last page (1-indexed, same as From for a single page)
}

// SplitOption configures PDF splitting.
type SplitOption func(*splitConfig)

type splitConfig struct {
	ranges []PageRange // empty = one file per page
}

// WithRanges splits the PDF by the given page ranges. Each range
// produces one output file. If not set, every page becomes its own file.
func WithRanges(ranges ...PageRange) SplitOption {
	return func(c *splitConfig) { c.ranges = ranges }
}

// SplitPDF splits a PDF file into multiple PDF files written to outputDir.
// By default each page becomes a separate file. Use WithRanges to split
// into custom page ranges instead.
//
// Returns the paths of the generated PDF files in order.
//
// This is a pure Go implementation — no external tools required.
func SplitPDF(pdfPath, outputDir string, opts ...SplitOption) ([]string, error) {
	cfg := &splitConfig{}
	for _, o := range opts {
		o(cfg)
	}

	data, err := os.ReadFile(pdfPath)
	if err != nil {
		return nil, fmt.Errorf("folio: read PDF: %w", err)
	}

	r, err := pdfcore.ReadPDF(data)
	if err != nil {
		return nil, fmt.Errorf("folio: parse PDF: %w", err)
	}

	pageRefs, err := r.PageRefs()
	if err != nil {
		return nil, fmt.Errorf("folio: read page tree: %w", err)
	}

	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return nil, fmt.Errorf("folio: create output dir: %w", err)
	}

	// Build ranges.
	ranges := cfg.ranges
	if len(ranges) == 0 {
		ranges = make([]PageRange, len(pageRefs))
		for i := range pageRefs {
			ranges[i] = PageRange{From: i + 1, To: i + 1}
		}
	}

	var paths []string
	for i, pr := range ranges {
		var name string
		if len(cfg.ranges) == 0 {
			name = fmt.Sprintf("page-%03d.pdf", i+1)
		} else {
			name = fmt.Sprintf("pages-%03d.pdf", i+1)
		}
		outPath := filepath.Join(outputDir, name)

		if err := writeSplitPDF(r, pageRefs, pr, outPath); err != nil {
			return nil, fmt.Errorf("folio: write %s: %w", name, err)
		}
		paths = append(paths, outPath)
	}
	return paths, nil
}

// writeSplitPDF writes a new PDF containing only the pages in the given range.
func writeSplitPDF(r *pdfcore.Reader, allPages []pdfcore.Ref, pr PageRange, outPath string) error {
	if pr.From < 1 || pr.To > len(allPages) || pr.From > pr.To {
		return fmt.Errorf("invalid page range %d-%d (document has %d pages)",
			pr.From, pr.To, len(allPages))
	}

	target := allPages[pr.From-1 : pr.To]

	// Resolve inherited page attributes and collect all dependency roots.
	enhanced := make([]map[string]interface{}, len(target))
	var rootNums []int

	for i, ref := range target {
		attrs, err := r.InheritedPageAttrs(ref.Num)
		if err != nil {
			return err
		}
		enhanced[i] = attrs

		// The page itself and any refs in inherited attrs are roots.
		rootNums = append(rootNums, ref.Num)
		for _, vref := range pdfcore.CollectValueRefs(attrs) {
			rootNums = append(rootNums, vref.Num)
		}
	}

	// Collect all transitive dependencies (skips /Parent).
	deps, err := r.CollectDeps(rootNums...)
	if err != nil {
		return err
	}

	// Build old→new object number mapping.
	// Reserve 1 = Catalog, 2 = Pages tree.
	remap := make(map[int]int)
	var sorted []int
	for num := range deps {
		sorted = append(sorted, num)
	}
	sort.Ints(sorted)
	nextNum := 3
	for _, num := range sorted {
		remap[num] = nextNum
		nextNum++
	}

	// Build a set of page object numbers for quick lookup.
	pageSet := make(map[int]bool, len(target))
	for _, ref := range target {
		pageSet[ref.Num] = true
	}

	// Prepare enhanced dicts indexed by old obj num.
	enhancedMap := make(map[int]map[string]interface{}, len(target))
	for i, ref := range target {
		enhancedMap[ref.Num] = enhanced[i]
	}

	// ---- Write the output PDF ----
	var buf bytes.Buffer
	offsets := make(map[int]int) // new obj num → byte offset

	fmt.Fprintf(&buf, "%%PDF-%s\n", r.Version())

	for _, oldNum := range sorted {
		newNum := remap[oldNum]
		offsets[newNum] = buf.Len()
		fmt.Fprintf(&buf, "%d 0 obj\n", newNum)

		if pageSet[oldNum] {
			// Use enhanced dict with /Parent pointing to new Pages tree.
			dict := enhancedMap[oldNum]
			dict["/Parent"] = pdfcore.Ref{Num: 2, Gen: 0}
			pdfcore.WriteValue(&buf, dict, remap)
		} else {
			obj, err := r.Object(oldNum)
			if err != nil {
				return err
			}
			pdfcore.WriteValue(&buf, obj, remap)
		}
		buf.WriteByte('\n')
		buf.WriteString("endobj\n")
	}

	// Pages tree (obj 2).
	offsets[2] = buf.Len()
	buf.WriteString("2 0 obj\n<< /Type /Pages /Kids [")
	for i, ref := range target {
		if i > 0 {
			buf.WriteByte(' ')
		}
		fmt.Fprintf(&buf, "%d 0 R", remap[ref.Num])
	}
	fmt.Fprintf(&buf, "] /Count %d >>\nendobj\n", len(target))

	// Catalog (obj 1).
	offsets[1] = buf.Len()
	buf.WriteString("1 0 obj\n<< /Type /Catalog /Pages 2 0 R >>\nendobj\n")

	totalObj := nextNum - 1

	// Xref table.
	xrefOffset := buf.Len()
	buf.WriteString("xref\n")
	fmt.Fprintf(&buf, "0 %d\n", totalObj+1)
	buf.WriteString("0000000000 65535 f \n")
	for i := 1; i <= totalObj; i++ {
		if off, ok := offsets[i]; ok {
			fmt.Fprintf(&buf, "%010d 00000 n \n", off)
		} else {
			buf.WriteString("0000000000 00000 f \n")
		}
	}

	// Trailer.
	buf.WriteString("trailer\n")
	fmt.Fprintf(&buf, "<< /Size %d /Root 1 0 R >>\n", totalObj+1)
	buf.WriteString("startxref\n")
	fmt.Fprintf(&buf, "%d\n", xrefOffset)
	buf.WriteString("%%EOF\n")

	return os.WriteFile(outPath, buf.Bytes(), 0o644)
}
