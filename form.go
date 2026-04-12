package folio

import (
	"fmt"
	"strings"

	"github.com/akkaraponph/folio/internal/state"
)

// formField represents an AcroForm field stored on a page.
type formField struct {
	fieldType string  // "Tx" (text), "Btn" (checkbox), "Ch" (dropdown)
	name      string  // unique field name
	x, y, w, h float64 // position and size in user units
	page      *Page

	// Text field defaults
	defaultValue string
	maxLen       int

	// Checkbox
	checked bool

	// Dropdown
	options []string
}

// FormTextField adds an interactive text input field to the page.
func (p *Page) FormTextField(name string, x, y, w, h float64, opts ...FieldOption) {
	p = p.active()
	f := &formField{
		fieldType: "Tx",
		name:      name,
		x: x, y: y, w: w, h: h,
		page: p,
	}
	for _, opt := range opts {
		opt(f)
	}
	p.doc.formFields = append(p.doc.formFields, f)
}

// FormCheckbox adds an interactive checkbox field to the page.
func (p *Page) FormCheckbox(name string, x, y, size float64, checked bool) {
	p = p.active()
	f := &formField{
		fieldType: "Btn",
		name:      name,
		x: x, y: y, w: size, h: size,
		page:    p,
		checked: checked,
	}
	p.doc.formFields = append(p.doc.formFields, f)
}

// FormDropdown adds an interactive dropdown (choice) field to the page.
func (p *Page) FormDropdown(name string, x, y, w, h float64, options []string) {
	p = p.active()
	f := &formField{
		fieldType: "Ch",
		name:      name,
		x: x, y: y, w: w, h: h,
		page:    p,
		options: options,
	}
	p.doc.formFields = append(p.doc.formFields, f)
}

// FieldOption configures optional properties of a form field.
type FieldOption func(*formField)

// WithDefaultValue sets the default value for a text field.
func WithDefaultValue(v string) FieldOption {
	return func(f *formField) { f.defaultValue = v }
}

// WithMaxLen sets the maximum character length for a text field.
func WithMaxLen(n int) FieldOption {
	return func(f *formField) { f.maxLen = n }
}

// drawFieldAppearance generates a simple appearance stream for a field.
// This is needed so the field is visible even without an interactive viewer.
func drawFieldAppearance(f *formField, k float64) []byte {
	var sb strings.Builder

	switch f.fieldType {
	case "Tx":
		// Draw border rectangle and default text.
		wPt := f.w * k
		hPt := f.h * k
		sb.WriteString(fmt.Sprintf("%.2f %.2f %.2f %.2f re\nS\n", 0.0, 0.0, wPt, hPt))
		if f.defaultValue != "" {
			sb.WriteString("BT\n")
			sb.WriteString("/Helv 12 Tf\n")
			sb.WriteString(fmt.Sprintf("2 %.2f Td\n", hPt-12))
			sb.WriteString(fmt.Sprintf("(%s) Tj\n", pdfEscape(f.defaultValue)))
			sb.WriteString("ET\n")
		}

	case "Btn":
		// Draw checkbox border and checkmark if checked.
		wPt := f.w * k
		hPt := f.h * k
		sb.WriteString(fmt.Sprintf("%.2f %.2f %.2f %.2f re\nS\n", 0.0, 0.0, wPt, hPt))
		if f.checked {
			// Draw X checkmark.
			sb.WriteString(fmt.Sprintf("%.2f %.2f m\n", 1.0, 1.0))
			sb.WriteString(fmt.Sprintf("%.2f %.2f l\n", wPt-1, hPt-1))
			sb.WriteString("S\n")
			sb.WriteString(fmt.Sprintf("%.2f %.2f m\n", wPt-1, 1.0))
			sb.WriteString(fmt.Sprintf("%.2f %.2f l\n", 1.0, hPt-1))
			sb.WriteString("S\n")
		}

	case "Ch":
		// Draw dropdown border.
		wPt := f.w * k
		hPt := f.h * k
		sb.WriteString(fmt.Sprintf("%.2f %.2f %.2f %.2f re\nS\n", 0.0, 0.0, wPt, hPt))
		// Draw dropdown arrow.
		arrowX := wPt - hPt
		sb.WriteString(fmt.Sprintf("%.2f 0 m\n%.2f %.2f l\nS\n", arrowX, arrowX, hPt))
	}

	return []byte(sb.String())
}

// pageIndex returns the 0-based index of the page in the document, or -1.
func (d *Document) pageIndex(p *Page) int {
	for i, pg := range d.pages {
		if pg == p {
			return i
		}
	}
	return -1
}

// putAcroForm writes AcroForm field objects and appearance streams.
// Returns field object numbers grouped by page for /Annots references.
// Also writes the /AcroForm entry in the catalog.
func (d *Document) putAcroForm(w interface {
	NewObj() int
	EndObj()
	Put(s string)
	Putf(format string, args ...any)
	PutStream(data []byte)
}, pageObjNums []int) (fieldObjNums []int) {
	if len(d.formFields) == 0 {
		return nil
	}

	k := d.k

	for _, f := range d.formFields {
		pageIdx := d.pageIndex(f.page)
		if pageIdx < 0 {
			continue
		}

		// Write appearance stream (Form XObject).
		apData := drawFieldAppearance(f, k)
		apObj := w.NewObj()
		w.Put("<<")
		w.Put("/Type /XObject")
		w.Put("/Subtype /Form")
		wPt := f.w * k
		hPt := f.h * k
		w.Putf("/BBox [0 0 %.2f %.2f]", wPt, hPt)
		w.Putf("/Length %d", len(apData))
		w.Put(">>")
		w.PutStream(apData)
		w.EndObj()

		// Write field/widget annotation object.
		fieldObj := w.NewObj()
		fieldObjNums = append(fieldObjNums, fieldObj)

		x1 := state.ToPointsX(f.x, k)
		y1 := state.ToPointsY(f.y+f.h, f.page.h, k)
		x2 := x1 + wPt
		y2 := y1 + hPt

		w.Put("<<")
		w.Put("/Type /Annot")
		w.Put("/Subtype /Widget")
		w.Putf("/Rect [%.2f %.2f %.2f %.2f]", x1, y1, x2, y2)
		w.Putf("/P %d 0 R", pageObjNums[pageIdx])
		w.Putf("/T %s", pdfString(f.name))
		w.Putf("/FT /%s", f.fieldType)

		// Default appearance for text-based fields.
		if f.fieldType == "Tx" || f.fieldType == "Ch" {
			w.Put("/DA (/Helv 12 Tf 0 g)")
		}

		// Field-specific properties.
		switch f.fieldType {
		case "Tx":
			if f.defaultValue != "" {
				w.Putf("/V %s", pdfString(f.defaultValue))
			}
			if f.maxLen > 0 {
				w.Putf("/MaxLen %d", f.maxLen)
			}
		case "Btn":
			w.Put("/Ff 0") // non-pushbutton, non-radio
			if f.checked {
				w.Put("/V /Yes")
				w.Put("/AS /Yes")
			} else {
				w.Put("/V /Off")
				w.Put("/AS /Off")
			}
		case "Ch":
			// Combo box flag.
			w.Putf("/Ff %d", 1<<17) // combo
			if len(f.options) > 0 {
				optStr := "["
				for _, o := range f.options {
					optStr += pdfString(o) + " "
				}
				optStr += "]"
				w.Putf("/Opt %s", optStr)
			}
		}

		// Appearance dictionary.
		w.Putf("/AP <</N %d 0 R>>", apObj)
		w.Put(">>")
		w.EndObj()
	}

	return fieldObjNums
}

