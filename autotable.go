package folio

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
)

// AutoTable generates a table automatically from structured data.
// It uses reflection (for structs/slices) or JSON unmarshalling to
// extract headers and row data, then delegates to the Table helper.
type AutoTable struct {
	doc  *Document
	page *Page

	headers []string
	rows    [][]string

	headerStyle    CellStyle
	hasHeaderStyle bool
	bodyStyle      CellStyle
	hasBodyStyle   bool
	rowHeight      float64
	border         string
	computedWidths []float64
}

// AutoTableFromStructs creates an AutoTable from a slice of structs.
// data must be a slice (or pointer to slice) of struct values.
// Exported struct field names become column headers; field values are
// converted to strings via fmt.Sprint.
func AutoTableFromStructs(doc *Document, page *Page, data any) *AutoTable {
	at := &AutoTable{
		doc:       doc,
		page:      page,
		rowHeight: 8,
		border:    "1",
	}

	rv := reflect.ValueOf(data)
	if rv.Kind() == reflect.Ptr {
		rv = rv.Elem()
	}
	if rv.Kind() != reflect.Slice {
		doc.err = fmt.Errorf("AutoTableFromStructs: expected slice, got %s", rv.Kind())
		return at
	}
	if rv.Len() == 0 {
		return at
	}

	// Extract type info from first element.
	elemType := rv.Type().Elem()
	if elemType.Kind() == reflect.Ptr {
		elemType = elemType.Elem()
	}
	if elemType.Kind() != reflect.Struct {
		doc.err = fmt.Errorf("AutoTableFromStructs: expected slice of structs, got slice of %s", elemType.Kind())
		return at
	}

	// Headers from exported fields.
	for i := 0; i < elemType.NumField(); i++ {
		f := elemType.Field(i)
		if f.IsExported() {
			at.headers = append(at.headers, f.Name)
		}
	}

	// Rows.
	for i := 0; i < rv.Len(); i++ {
		elem := rv.Index(i)
		if elem.Kind() == reflect.Ptr {
			elem = elem.Elem()
		}
		var row []string
		for j := 0; j < elemType.NumField(); j++ {
			f := elemType.Field(j)
			if f.IsExported() {
				row = append(row, fmt.Sprint(elem.Field(j).Interface()))
			}
		}
		at.rows = append(at.rows, row)
	}

	return at
}

// AutoTableFromJSON creates an AutoTable from a JSON byte slice.
// The JSON must be an array of objects. Keys from the first object
// become column headers.
func AutoTableFromJSON(doc *Document, page *Page, jsonData []byte) (*AutoTable, error) {
	at := &AutoTable{
		doc:       doc,
		page:      page,
		rowHeight: 8,
		border:    "1",
	}

	var records []map[string]any
	if err := json.Unmarshal(jsonData, &records); err != nil {
		return nil, fmt.Errorf("AutoTableFromJSON: %w", err)
	}
	if len(records) == 0 {
		return at, nil
	}

	// Extract headers from first record (sorted by key order from JSON).
	// Since map iteration order is non-deterministic, we re-parse to get
	// key order.
	var orderedKeys []string
	orderedKeys = extractJSONKeys(jsonData)
	if len(orderedKeys) == 0 {
		// Fallback: use map keys.
		for k := range records[0] {
			orderedKeys = append(orderedKeys, k)
		}
	}
	at.headers = orderedKeys

	// Rows.
	for _, rec := range records {
		var row []string
		for _, k := range orderedKeys {
			row = append(row, fmt.Sprint(rec[k]))
		}
		at.rows = append(at.rows, row)
	}

	return at, nil
}

// extractJSONKeys parses JSON array to get keys from the first object
// in their original order.
func extractJSONKeys(data []byte) []string {
	dec := json.NewDecoder(bytes.NewReader(data))
	// Read opening '['
	t, err := dec.Token()
	if err != nil || t != json.Delim('[') {
		return nil
	}
	// Read first object token by token.
	t, err = dec.Token()
	if err != nil || t != json.Delim('{') {
		return nil
	}
	var keys []string
	depth := 1
	for dec.More() && depth > 0 {
		t, err = dec.Token()
		if err != nil {
			break
		}
		switch v := t.(type) {
		case json.Delim:
			switch v {
			case '{', '[':
				depth++
			case '}', ']':
				depth--
			}
		case string:
			if depth == 1 {
				keys = append(keys, v)
				// Skip the value.
				var raw json.RawMessage
				if err := dec.Decode(&raw); err != nil {
					return keys
				}
			}
		}
	}
	return keys
}

// SetHeaderStyle sets the header cell style.
func (at *AutoTable) SetHeaderStyle(s CellStyle) {
	at.headerStyle = s
	at.hasHeaderStyle = true
}

// SetBodyStyle sets the body cell style.
func (at *AutoTable) SetBodyStyle(s CellStyle) {
	at.bodyStyle = s
	at.hasBodyStyle = true
}

// SetRowHeight sets the row height.
func (at *AutoTable) SetRowHeight(h float64) { at.rowHeight = h }

// SetBorder sets the cell border style.
func (at *AutoTable) SetBorder(b string) { at.border = b }

// AutoFitWidths computes column widths proportional to the maximum
// content width in each column, fitting within availWidth user units.
func (at *AutoTable) AutoFitWidths(availWidth float64) {
	if len(at.headers) == 0 {
		return
	}

	p := at.page.active()
	maxWidths := make([]float64, len(at.headers))

	// Measure headers.
	for i, h := range at.headers {
		w := p.GetStringWidth(h)
		if w > maxWidths[i] {
			maxWidths[i] = w
		}
	}

	// Measure rows.
	for _, row := range at.rows {
		for i, cell := range row {
			if i >= len(maxWidths) {
				break
			}
			w := p.GetStringWidth(cell)
			if w > maxWidths[i] {
				maxWidths[i] = w
			}
		}
	}

	// Distribute proportionally.
	total := 0.0
	for _, w := range maxWidths {
		total += w
	}
	if total == 0 {
		// Equal widths.
		eq := availWidth / float64(len(maxWidths))
		for i := range maxWidths {
			maxWidths[i] = eq
		}
	} else {
		for i := range maxWidths {
			maxWidths[i] = maxWidths[i] / total * availWidth
		}
	}

	at.computedWidths = maxWidths
}

// Render draws the table on the page. If AutoFitWidths was not called,
// columns are distributed equally across the available width.
func (at *AutoTable) Render() {
	if at.doc.err != nil {
		return
	}
	if len(at.headers) == 0 {
		return
	}

	p := at.page.active()
	d := at.doc

	tbl := NewTable(d, p)
	tbl.SetRowHeight(at.rowHeight)
	tbl.SetBorder(at.border)

	// Set widths.
	var widths []float64
	if at.computedWidths != nil {
		widths = at.computedWidths
	} else {
		avail := p.w - d.lMargin - d.rMargin
		eq := avail / float64(len(at.headers))
		widths = make([]float64, len(at.headers))
		for i := range widths {
			widths[i] = eq
		}
	}
	tbl.SetWidths(widths...)

	if at.hasHeaderStyle {
		tbl.SetHeaderStyle(at.headerStyle)
	}
	if at.hasBodyStyle {
		tbl.SetBodyStyle(at.bodyStyle)
	}

	tbl.Header(at.headers...)
	for _, row := range at.rows {
		tbl.Row(row...)
	}
}
