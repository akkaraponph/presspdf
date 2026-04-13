package folio

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/akkaraponph/folio/internal/pdfcore"
)

// MergePDF combines multiple PDF files into a single PDF written to
// outputPath. Pages appear in the order the input files are listed.
//
// This is a pure Go implementation — no external tools required.
func MergePDF(outputPath string, inputPaths ...string) error {
	if len(inputPaths) == 0 {
		return fmt.Errorf("folio: no input files")
	}

	// Ensure output directory exists.
	if dir := filepath.Dir(outputPath); dir != "." && dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("folio: create output dir: %w", err)
		}
	}

	type sourceInfo struct {
		reader   *pdfcore.Reader
		pages    []pdfcore.Ref
		enhanced []map[string]interface{} // page dicts with inherited attrs
		deps     map[int]bool
		remap    map[int]int // old obj num → new obj num
	}

	var sources []sourceInfo
	var version string

	for _, path := range inputPaths {
		data, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("folio: read %s: %w", path, err)
		}
		r, err := pdfcore.ReadPDF(data)
		if err != nil {
			return fmt.Errorf("folio: parse %s: %w", path, err)
		}
		pageRefs, err := r.PageRefs()
		if err != nil {
			return fmt.Errorf("folio: pages in %s: %w", path, err)
		}

		// Use the highest PDF version among inputs.
		if r.Version() > version {
			version = r.Version()
		}

		// Resolve inherited attrs and collect dependency roots.
		enhanced := make([]map[string]interface{}, len(pageRefs))
		var roots []int
		for i, ref := range pageRefs {
			attrs, err := r.InheritedPageAttrs(ref.Num)
			if err != nil {
				return fmt.Errorf("folio: page attrs in %s: %w", path, err)
			}
			enhanced[i] = attrs
			roots = append(roots, ref.Num)
			for _, vref := range pdfcore.CollectValueRefs(attrs) {
				roots = append(roots, vref.Num)
			}
		}

		deps, err := r.CollectDeps(roots...)
		if err != nil {
			return fmt.Errorf("folio: deps in %s: %w", path, err)
		}

		sources = append(sources, sourceInfo{
			reader:   r,
			pages:    pageRefs,
			enhanced: enhanced,
			deps:     deps,
			remap:    make(map[int]int),
		})
	}

	// Assign new object numbers across all sources.
	// Reserve 1 = Catalog, 2 = Pages tree.
	nextNum := 3
	for i := range sources {
		var sorted []int
		for num := range sources[i].deps {
			sorted = append(sorted, num)
		}
		sort.Ints(sorted)
		for _, num := range sorted {
			sources[i].remap[num] = nextNum
			nextNum++
		}
	}

	// ---- Write the output PDF ----
	var buf bytes.Buffer
	offsets := make(map[int]int)
	var allPageNewNums []int

	fmt.Fprintf(&buf, "%%PDF-%s\n", version)

	for si := range sources {
		src := &sources[si]
		pageSet := make(map[int]bool, len(src.pages))
		enhancedMap := make(map[int]map[string]interface{}, len(src.pages))
		for i, ref := range src.pages {
			pageSet[ref.Num] = true
			enhancedMap[ref.Num] = src.enhanced[i]
		}

		var sorted []int
		for num := range src.deps {
			sorted = append(sorted, num)
		}
		sort.Ints(sorted)

		for _, oldNum := range sorted {
			newNum := src.remap[oldNum]
			offsets[newNum] = buf.Len()
			fmt.Fprintf(&buf, "%d 0 obj\n", newNum)

			if pageSet[oldNum] {
				dict := enhancedMap[oldNum]
				dict["/Parent"] = pdfcore.Ref{Num: 2, Gen: 0}
				pdfcore.WriteValue(&buf, dict, src.remap)
			} else {
				obj, err := src.reader.Object(oldNum)
				if err != nil {
					return err
				}
				pdfcore.WriteValue(&buf, obj, src.remap)
			}
			buf.WriteByte('\n')
			buf.WriteString("endobj\n")
		}

		for _, ref := range src.pages {
			allPageNewNums = append(allPageNewNums, src.remap[ref.Num])
		}
	}

	// Pages tree (obj 2).
	offsets[2] = buf.Len()
	buf.WriteString("2 0 obj\n<< /Type /Pages /Kids [")
	for i, num := range allPageNewNums {
		if i > 0 {
			buf.WriteByte(' ')
		}
		fmt.Fprintf(&buf, "%d 0 R", num)
	}
	fmt.Fprintf(&buf, "] /Count %d >>\nendobj\n", len(allPageNewNums))

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

	return os.WriteFile(outputPath, buf.Bytes(), 0o644)
}
