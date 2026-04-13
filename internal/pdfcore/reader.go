package pdfcore

import (
	"bytes"
	"compress/zlib"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// ---- PDF value types for the reader ----

// Name represents a PDF name object (e.g. /Type, /Page).
type Name string

// Ref represents an indirect object reference (e.g. 5 0 R).
type Ref struct {
	Num int
	Gen int
}

// Stream represents a PDF stream object with its dictionary and raw data.
type Stream struct {
	Hdr map[string]interface{} // stream dictionary
	Raw []byte                 // raw stream bytes (not decoded)
}

// ---- Reader ----

// Reader provides read access to a parsed PDF file.
type Reader struct {
	data    []byte
	version string
	xref    map[int]*xrefEntry
	trailer map[string]interface{}
	cache   map[int]interface{}
}

type xrefEntry struct {
	offset    int // byte offset (type 1) or containing ObjStm number (type 2)
	gen       int // generation (type 1) or index in ObjStm (type 2)
	entryType int // 0=free, 1=in-file, 2=in-object-stream
}

// ReadPDF parses a PDF file from raw bytes.
func ReadPDF(data []byte) (*Reader, error) {
	r := &Reader{
		data:  data,
		xref:  make(map[int]*xrefEntry),
		cache: make(map[int]interface{}),
	}
	r.parseVersion()
	if err := r.buildXref(); err != nil {
		return nil, fmt.Errorf("pdfcore: %w", err)
	}
	return r, nil
}

// Version returns the PDF version string (e.g. "1.4").
func (r *Reader) Version() string { return r.version }

// Trailer returns the parsed trailer dictionary.
func (r *Reader) Trailer() map[string]interface{} { return r.trailer }

func (r *Reader) parseVersion() {
	if len(r.data) >= 8 && string(r.data[:5]) == "%PDF-" {
		end := bytes.IndexByte(r.data[:min(20, len(r.data))], '\n')
		if end > 5 {
			r.version = strings.TrimRight(string(r.data[5:end]), "\r")
		}
	}
	if r.version == "" {
		r.version = "1.4"
	}
}

// ---- xref parsing ----

func (r *Reader) buildXref() error {
	offset, err := r.findStartXref()
	if err != nil {
		return err
	}
	return r.parseXrefAt(offset)
}

func (r *Reader) findStartXref() (int, error) {
	tail := r.data
	base := 0
	if len(tail) > 1024 {
		base = len(tail) - 1024
		tail = tail[base:]
	}
	idx := bytes.LastIndex(tail, []byte("startxref"))
	if idx < 0 {
		return 0, fmt.Errorf("startxref not found")
	}
	s := &scanner{data: r.data, pos: base + idx + len("startxref")}
	s.skipSpace()
	offset, err := s.readInt()
	if err != nil {
		return 0, fmt.Errorf("parse startxref offset: %w", err)
	}
	return offset, nil
}

func (r *Reader) parseXrefAt(offset int) error {
	if offset < 0 || offset >= len(r.data) {
		return fmt.Errorf("xref offset %d out of range", offset)
	}
	s := &scanner{data: r.data, pos: offset}
	s.skipSpace()
	if s.hasPrefix("xref") {
		return r.parseTraditionalXref(s)
	}
	return r.parseXrefStream(offset)
}

func (r *Reader) parseTraditionalXref(s *scanner) error {
	s.pos += 4 // skip "xref"
	s.skipSpace()

	for !s.atEnd() {
		if s.hasPrefix("trailer") {
			s.pos += 7
			break
		}
		startObj, err := s.readInt()
		if err != nil {
			return fmt.Errorf("xref subsection start: %w", err)
		}
		s.skipSpace()
		count, err := s.readInt()
		if err != nil {
			return fmt.Errorf("xref subsection count: %w", err)
		}
		s.skipSpace()

		for i := 0; i < count; i++ {
			off, err := s.readInt()
			if err != nil {
				return fmt.Errorf("xref entry offset: %w", err)
			}
			s.skipSpace()
			gen, err := s.readInt()
			if err != nil {
				return fmt.Errorf("xref entry gen: %w", err)
			}
			s.skipSpace()
			marker := s.advance()
			s.skipSpace()

			objNum := startObj + i
			if _, exists := r.xref[objNum]; !exists {
				et := 1
				if marker == 'f' {
					et = 0
				}
				r.xref[objNum] = &xrefEntry{offset: off, gen: gen, entryType: et}
			}
		}
	}

	// Parse trailer dictionary.
	s.skipSpace()
	tv, err := s.readValue()
	if err != nil {
		return fmt.Errorf("parse trailer dict: %w", err)
	}
	trailer, ok := tv.(map[string]interface{})
	if !ok {
		return fmt.Errorf("trailer is not a dictionary")
	}
	if r.trailer == nil {
		r.trailer = trailer
	} else {
		for k, v := range trailer {
			if _, exists := r.trailer[k]; !exists {
				r.trailer[k] = v
			}
		}
	}

	// Follow /Prev for incremental updates.
	if prev, ok := trailer["/Prev"]; ok {
		if p := toInt(prev); p > 0 {
			return r.parseXrefAt(p)
		}
	}
	return nil
}

func (r *Reader) parseXrefStream(offset int) error {
	s := &scanner{data: r.data, pos: offset}
	s.skipSpace()

	if _, err := s.readInt(); err != nil {
		return fmt.Errorf("xref stream obj num: %w", err)
	}
	s.skipSpace()
	if _, err := s.readInt(); err != nil {
		return fmt.Errorf("xref stream gen: %w", err)
	}
	s.skipSpace()
	if !s.expectWord("obj") {
		return fmt.Errorf("expected 'obj' in xref stream")
	}
	s.skipSpace()

	dv, err := s.readValue()
	if err != nil {
		return fmt.Errorf("xref stream dict: %w", err)
	}
	dict, ok := dv.(map[string]interface{})
	if !ok {
		return fmt.Errorf("xref stream dict is not a dict")
	}

	raw, err := s.readStreamData(dict, r)
	if err != nil {
		return fmt.Errorf("read xref stream data: %w", err)
	}
	decoded, err := decodeStream(dict, raw)
	if err != nil {
		return fmt.Errorf("decode xref stream: %w", err)
	}

	if r.trailer == nil {
		r.trailer = dict
	}

	w := toIntSlice(dict["/W"])
	if len(w) != 3 {
		return fmt.Errorf("xref stream /W must have 3 elements")
	}

	size := toInt(dict["/Size"])
	entrySize := w[0] + w[1] + w[2]

	var index []int
	if idx, ok := dict["/Index"]; ok {
		index = toIntSlice(idx)
	} else {
		index = []int{0, size}
	}

	pos := 0
	for i := 0; i+1 < len(index); i += 2 {
		startObj := index[i]
		count := index[i+1]
		for j := 0; j < count; j++ {
			if pos+entrySize > len(decoded) {
				break
			}
			f1 := readBE(decoded, pos, w[0])
			f2 := readBE(decoded, pos+w[0], w[1])
			f3 := readBE(decoded, pos+w[0]+w[1], w[2])
			pos += entrySize

			objNum := startObj + j
			if _, exists := r.xref[objNum]; !exists {
				et := f1
				if w[0] == 0 {
					et = 1
				}
				r.xref[objNum] = &xrefEntry{offset: f2, gen: f3, entryType: et}
			}
		}
	}

	if prev, ok := dict["/Prev"]; ok {
		if p := toInt(prev); p > 0 {
			return r.parseXrefAt(p)
		}
	}
	return nil
}

func readBE(data []byte, off, width int) int {
	v := 0
	for i := 0; i < width && off+i < len(data); i++ {
		v = v<<8 | int(data[off+i])
	}
	return v
}

// ---- Object access ----

// Object returns the parsed value for the given object number.
func (r *Reader) Object(num int) (interface{}, error) {
	if v, ok := r.cache[num]; ok {
		return v, nil
	}
	entry, ok := r.xref[num]
	if !ok || entry.entryType == 0 {
		return nil, fmt.Errorf("pdfcore: object %d not found", num)
	}
	var v interface{}
	var err error
	if entry.entryType == 2 {
		v, err = r.objectFromStream(entry.offset, entry.gen, num)
	} else {
		v, err = r.objectFromFile(num, entry.offset)
	}
	if err != nil {
		return nil, err
	}
	r.cache[num] = v
	return v, nil
}

func (r *Reader) objectFromFile(num, offset int) (interface{}, error) {
	if offset >= len(r.data) {
		return nil, fmt.Errorf("pdfcore: object %d offset %d out of range", num, offset)
	}
	s := &scanner{data: r.data, pos: offset}
	s.skipSpace()
	if _, err := s.readInt(); err != nil {
		return nil, fmt.Errorf("pdfcore: object %d: %w", num, err)
	}
	s.skipSpace()
	if _, err := s.readInt(); err != nil {
		return nil, fmt.Errorf("pdfcore: object %d gen: %w", num, err)
	}
	s.skipSpace()
	if !s.expectWord("obj") {
		return nil, fmt.Errorf("pdfcore: object %d: expected 'obj'", num)
	}
	s.skipSpace()
	v, err := s.readValue()
	if err != nil {
		return nil, fmt.Errorf("pdfcore: object %d: %w", num, err)
	}
	s.skipSpace()
	if s.hasPrefix("stream") {
		dict, ok := v.(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("pdfcore: object %d: stream without dict", num)
		}
		raw, err := s.readStreamData(dict, r)
		if err != nil {
			return nil, fmt.Errorf("pdfcore: object %d stream: %w", num, err)
		}
		return &Stream{Hdr: dict, Raw: raw}, nil
	}
	return v, nil
}

func (r *Reader) objectFromStream(stmObjNum, index, targetNum int) (interface{}, error) {
	stmObj, err := r.Object(stmObjNum)
	if err != nil {
		return nil, fmt.Errorf("pdfcore: ObjStm %d: %w", stmObjNum, err)
	}
	stm, ok := stmObj.(*Stream)
	if !ok {
		return nil, fmt.Errorf("pdfcore: ObjStm %d is not a stream", stmObjNum)
	}
	decoded, err := decodeStream(stm.Hdr, stm.Raw)
	if err != nil {
		return nil, fmt.Errorf("pdfcore: decode ObjStm %d: %w", stmObjNum, err)
	}
	n := toInt(stm.Hdr["/N"])
	first := toInt(stm.Hdr["/First"])
	if index >= n {
		return nil, fmt.Errorf("pdfcore: ObjStm %d index %d >= N=%d", stmObjNum, index, n)
	}

	hs := &scanner{data: decoded, pos: 0}
	offsets := make([]int, n)
	for i := 0; i < n; i++ {
		hs.skipSpace()
		if _, err := hs.readInt(); err != nil {
			return nil, err
		}
		hs.skipSpace()
		off, err := hs.readInt()
		if err != nil {
			return nil, err
		}
		offsets[i] = off
	}

	os := &scanner{data: decoded, pos: first + offsets[index]}
	os.skipSpace()
	return os.readValue()
}

// Resolve dereferences a Ref; non-Ref values are returned as-is.
func (r *Reader) Resolve(v interface{}) (interface{}, error) {
	ref, ok := v.(Ref)
	if !ok {
		return v, nil
	}
	return r.Object(ref.Num)
}

// ---- Page tree ----

// PageRefs returns indirect references to each page, in document order.
func (r *Reader) PageRefs() ([]Ref, error) {
	rootRef, ok := r.trailer["/Root"].(Ref)
	if !ok {
		return nil, fmt.Errorf("pdfcore: /Root is not a reference")
	}
	catalog, err := r.Object(rootRef.Num)
	if err != nil {
		return nil, fmt.Errorf("pdfcore: read catalog: %w", err)
	}
	catDict := toDict(catalog)
	if catDict == nil {
		return nil, fmt.Errorf("pdfcore: catalog is not a dict")
	}
	pagesRef, ok := catDict["/Pages"].(Ref)
	if !ok {
		return nil, fmt.Errorf("pdfcore: /Pages is not a reference")
	}
	return r.collectPages(pagesRef)
}

func (r *Reader) collectPages(ref Ref) ([]Ref, error) {
	obj, err := r.Object(ref.Num)
	if err != nil {
		return nil, err
	}
	dict := toDict(obj)
	if dict == nil {
		return nil, fmt.Errorf("pdfcore: page tree node %d not a dict", ref.Num)
	}
	if typ, _ := dict["/Type"].(Name); typ == "Page" {
		return []Ref{ref}, nil
	}
	kids, ok := dict["/Kids"].([]interface{})
	if !ok {
		return nil, fmt.Errorf("pdfcore: /Pages %d has no /Kids", ref.Num)
	}
	var pages []Ref
	for _, kid := range kids {
		kidRef, ok := kid.(Ref)
		if !ok {
			continue
		}
		sub, err := r.collectPages(kidRef)
		if err != nil {
			return nil, err
		}
		pages = append(pages, sub...)
	}
	return pages, nil
}

// ---- Dependency collection ----

// CollectDeps returns all object numbers transitively referenced from
// the given roots (including the roots themselves). References through
// /Parent keys are skipped to avoid pulling in the page tree ancestors.
func (r *Reader) CollectDeps(roots ...int) (map[int]bool, error) {
	deps := make(map[int]bool)
	var walk func(int) error
	walk = func(num int) error {
		if deps[num] {
			return nil
		}
		deps[num] = true
		obj, err := r.Object(num)
		if err != nil {
			return nil
		}
		for _, ref := range collectRefsSkipParent(obj) {
			if err := walk(ref.Num); err != nil {
				return err
			}
		}
		return nil
	}
	for _, root := range roots {
		if err := walk(root); err != nil {
			return nil, err
		}
	}
	return deps, nil
}

// collectRefsSkipParent extracts all Ref values, skipping /Parent entries.
func collectRefsSkipParent(v interface{}) []Ref {
	var refs []Ref
	switch v := v.(type) {
	case Ref:
		refs = append(refs, v)
	case map[string]interface{}:
		for k, val := range v {
			if k == "/Parent" {
				continue
			}
			refs = append(refs, collectRefsSkipParent(val)...)
		}
	case []interface{}:
		for _, val := range v {
			refs = append(refs, collectRefsSkipParent(val)...)
		}
	case *Stream:
		refs = collectRefsSkipParent(v.Hdr)
	}
	return refs
}

// CollectValueRefs extracts all Ref values from a parsed PDF value.
func CollectValueRefs(v interface{}) []Ref {
	var refs []Ref
	switch v := v.(type) {
	case Ref:
		refs = append(refs, v)
	case map[string]interface{}:
		for _, val := range v {
			refs = append(refs, CollectValueRefs(val)...)
		}
	case []interface{}:
		for _, val := range v {
			refs = append(refs, CollectValueRefs(val)...)
		}
	case *Stream:
		refs = CollectValueRefs(v.Hdr)
	}
	return refs
}

// ---- Inherited page attributes ----

// InheritedPageAttrs resolves inheritable attributes for a page by
// walking up the /Parent chain. Returns a copy of the page dict with
// all inherited attributes filled in and /Parent removed.
func (r *Reader) InheritedPageAttrs(pageNum int) (map[string]interface{}, error) {
	obj, err := r.Object(pageNum)
	if err != nil {
		return nil, err
	}
	dict := toDict(obj)
	if dict == nil {
		return nil, fmt.Errorf("pdfcore: object %d is not a dict", pageNum)
	}

	// Copy page dict.
	result := make(map[string]interface{}, len(dict))
	for k, v := range dict {
		if k == "/Parent" {
			continue
		}
		result[k] = v
	}

	// Walk /Parent chain for inheritable keys.
	inheritable := []string{"/Resources", "/MediaBox", "/CropBox", "/Rotate"}
	current := dict
	for {
		parentRef, ok := current["/Parent"].(Ref)
		if !ok {
			break
		}
		parentObj, err := r.Object(parentRef.Num)
		if err != nil {
			break
		}
		parentDict := toDict(parentObj)
		if parentDict == nil {
			break
		}
		for _, key := range inheritable {
			if _, exists := result[key]; !exists {
				if v, ok := parentDict[key]; ok {
					result[key] = v
				}
			}
		}
		current = parentDict
	}
	return result, nil
}

// ---- Stream decoding ----

func decodeStream(dict map[string]interface{}, raw []byte) ([]byte, error) {
	filter := dict["/Filter"]
	if filter == nil {
		return raw, nil
	}
	switch f := filter.(type) {
	case Name:
		return applyFilter(string(f), raw)
	case []interface{}:
		data := raw
		for _, fi := range f {
			name, ok := fi.(Name)
			if !ok {
				continue
			}
			var err error
			data, err = applyFilter(string(name), data)
			if err != nil {
				return nil, err
			}
		}
		return data, nil
	}
	return raw, nil
}

func applyFilter(name string, data []byte) ([]byte, error) {
	if name == "FlateDecode" {
		return inflate(data)
	}
	return nil, fmt.Errorf("unsupported filter: %s", name)
}

func inflate(data []byte) ([]byte, error) {
	zr, err := zlib.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	defer zr.Close()
	var buf bytes.Buffer
	if _, err := io.Copy(&buf, zr); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// ---- Value serialization ----

// WriteValue serializes a parsed PDF value, remapping Ref numbers
// through remap (if non-nil).
func WriteValue(w *bytes.Buffer, v interface{}, remap map[int]int) {
	switch v := v.(type) {
	case nil:
		w.WriteString("null")
	case bool:
		if v {
			w.WriteString("true")
		} else {
			w.WriteString("false")
		}
	case int:
		fmt.Fprintf(w, "%d", v)
	case float64:
		w.WriteString(strconv.FormatFloat(v, 'f', -1, 64))
	case Name:
		w.WriteByte('/')
		w.WriteString(string(v))
	case string:
		writeString(w, v)
	case Ref:
		num := v.Num
		if remap != nil {
			if n, ok := remap[v.Num]; ok {
				num = n
			}
		}
		fmt.Fprintf(w, "%d 0 R", num)
	case map[string]interface{}:
		w.WriteString("<<")
		for k, val := range v {
			w.WriteByte(' ')
			w.WriteString(k) // includes leading /
			w.WriteByte(' ')
			WriteValue(w, val, remap)
		}
		w.WriteString(" >>")
	case []interface{}:
		w.WriteByte('[')
		for i, val := range v {
			if i > 0 {
				w.WriteByte(' ')
			}
			WriteValue(w, val, remap)
		}
		w.WriteByte(']')
	case *Stream:
		hdr := make(map[string]interface{}, len(v.Hdr))
		for k, val := range v.Hdr {
			hdr[k] = val
		}
		hdr["/Length"] = len(v.Raw)
		w.WriteString("<<")
		for k, val := range hdr {
			w.WriteByte(' ')
			w.WriteString(k)
			w.WriteByte(' ')
			WriteValue(w, val, remap)
		}
		w.WriteString(" >>\nstream\n")
		w.Write(v.Raw)
		w.WriteString("\nendstream")
	default:
		fmt.Fprintf(w, "%v", v)
	}
}

func writeString(w *bytes.Buffer, s string) {
	// Use hex string for binary data, literal for text.
	for i := 0; i < len(s); i++ {
		if s[i] < 0x20 && s[i] != '\n' && s[i] != '\r' && s[i] != '\t' {
			// Binary content — use hex string.
			w.WriteByte('<')
			for j := 0; j < len(s); j++ {
				fmt.Fprintf(w, "%02x", s[j])
			}
			w.WriteByte('>')
			return
		}
	}
	w.WriteByte('(')
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '\\':
			w.WriteString("\\\\")
		case '(':
			w.WriteString("\\(")
		case ')':
			w.WriteString("\\)")
		default:
			w.WriteByte(s[i])
		}
	}
	w.WriteByte(')')
}

// ---- Scanner ----

type scanner struct {
	data []byte
	pos  int
}

func (s *scanner) atEnd() bool { return s.pos >= len(s.data) }
func (s *scanner) peek() byte {
	if s.atEnd() {
		return 0
	}
	return s.data[s.pos]
}
func (s *scanner) advance() byte { b := s.data[s.pos]; s.pos++; return b }

func (s *scanner) hasPrefix(prefix string) bool {
	return s.pos+len(prefix) <= len(s.data) &&
		string(s.data[s.pos:s.pos+len(prefix)]) == prefix
}

func (s *scanner) expectWord(word string) bool {
	if s.hasPrefix(word) {
		s.pos += len(word)
		return true
	}
	return false
}

func isWSByte(b byte) bool {
	return b == 0 || b == '\t' || b == '\n' || b == '\f' || b == '\r' || b == ' '
}

func isDelimByte(b byte) bool {
	return b == '(' || b == ')' || b == '<' || b == '>' ||
		b == '[' || b == ']' || b == '{' || b == '}' ||
		b == '/' || b == '%'
}

func (s *scanner) skipSpace() {
	for !s.atEnd() {
		b := s.peek()
		if isWSByte(b) {
			s.pos++
		} else if b == '%' {
			for !s.atEnd() && s.peek() != '\n' && s.peek() != '\r' {
				s.pos++
			}
		} else {
			break
		}
	}
}

func (s *scanner) readInt() (int, error) {
	s.skipSpace()
	start := s.pos
	if !s.atEnd() && (s.peek() == '+' || s.peek() == '-') {
		s.pos++
	}
	for !s.atEnd() && s.peek() >= '0' && s.peek() <= '9' {
		s.pos++
	}
	if s.pos == start {
		return 0, fmt.Errorf("expected integer at %d", start)
	}
	return strconv.Atoi(string(s.data[start:s.pos]))
}

func (s *scanner) readNumber() (interface{}, error) {
	start := s.pos
	if !s.atEnd() && (s.peek() == '+' || s.peek() == '-') {
		s.pos++
	}
	isFloat := false
	for !s.atEnd() && (s.peek() >= '0' && s.peek() <= '9' || s.peek() == '.') {
		if s.peek() == '.' {
			isFloat = true
		}
		s.pos++
	}
	if s.pos == start {
		return nil, fmt.Errorf("expected number at %d", start)
	}
	str := string(s.data[start:s.pos])
	if isFloat {
		return strconv.ParseFloat(str, 64)
	}
	return strconv.Atoi(str)
}

func (s *scanner) readName() (Name, error) {
	if s.peek() != '/' {
		return "", fmt.Errorf("expected '/' at %d", s.pos)
	}
	s.pos++
	var buf bytes.Buffer
	for !s.atEnd() && !isWSByte(s.peek()) && !isDelimByte(s.peek()) {
		if s.peek() == '#' && s.pos+2 < len(s.data) {
			s.pos++
			hi := unhex(s.advance())
			lo := unhex(s.advance())
			buf.WriteByte(hi<<4 | lo)
		} else {
			buf.WriteByte(s.advance())
		}
	}
	return Name(buf.String()), nil
}

func unhex(b byte) byte {
	switch {
	case b >= '0' && b <= '9':
		return b - '0'
	case b >= 'a' && b <= 'f':
		return b - 'a' + 10
	case b >= 'A' && b <= 'F':
		return b - 'A' + 10
	}
	return 0
}

func (s *scanner) readLiteralString() (string, error) {
	s.pos++ // skip '('
	var buf bytes.Buffer
	depth := 1
	for !s.atEnd() && depth > 0 {
		b := s.advance()
		switch b {
		case '(':
			depth++
			buf.WriteByte(b)
		case ')':
			depth--
			if depth > 0 {
				buf.WriteByte(b)
			}
		case '\\':
			if s.atEnd() {
				break
			}
			esc := s.advance()
			switch esc {
			case 'n':
				buf.WriteByte('\n')
			case 'r':
				buf.WriteByte('\r')
			case 't':
				buf.WriteByte('\t')
			case 'b':
				buf.WriteByte('\b')
			case 'f':
				buf.WriteByte('\f')
			case '(', ')', '\\':
				buf.WriteByte(esc)
			case '\r':
				if !s.atEnd() && s.peek() == '\n' {
					s.pos++
				}
			case '\n':
				// line continuation — skip
			default:
				if esc >= '0' && esc <= '7' {
					oct := int(esc - '0')
					for i := 0; i < 2 && !s.atEnd() && s.peek() >= '0' && s.peek() <= '7'; i++ {
						oct = oct*8 + int(s.advance()-'0')
					}
					buf.WriteByte(byte(oct))
				} else {
					buf.WriteByte(esc)
				}
			}
		default:
			buf.WriteByte(b)
		}
	}
	return buf.String(), nil
}

func (s *scanner) readHexString() (string, error) {
	s.pos++ // skip '<'
	var hex []byte
	for !s.atEnd() && s.peek() != '>' {
		if isWSByte(s.peek()) {
			s.pos++
			continue
		}
		hex = append(hex, s.advance())
	}
	if !s.atEnd() {
		s.pos++ // skip '>'
	}
	if len(hex)%2 != 0 {
		hex = append(hex, '0')
	}
	var buf bytes.Buffer
	for i := 0; i+1 < len(hex); i += 2 {
		buf.WriteByte(unhex(hex[i])<<4 | unhex(hex[i+1]))
	}
	return buf.String(), nil
}

func (s *scanner) readArray() ([]interface{}, error) {
	s.pos++ // skip '['
	var arr []interface{}
	for {
		s.skipSpace()
		if s.atEnd() {
			return nil, fmt.Errorf("unterminated array")
		}
		if s.peek() == ']' {
			s.pos++
			return arr, nil
		}
		v, err := s.readValue()
		if err != nil {
			return nil, err
		}
		arr = append(arr, v)
	}
}

func (s *scanner) readDict() (map[string]interface{}, error) {
	s.pos += 2 // skip '<<'
	dict := make(map[string]interface{})
	for {
		s.skipSpace()
		if s.atEnd() {
			return nil, fmt.Errorf("unterminated dict")
		}
		if s.hasPrefix(">>") {
			s.pos += 2
			return dict, nil
		}
		name, err := s.readName()
		if err != nil {
			return nil, fmt.Errorf("dict key: %w", err)
		}
		s.skipSpace()
		val, err := s.readValue()
		if err != nil {
			return nil, fmt.Errorf("dict value for /%s: %w", name, err)
		}
		dict["/"+string(name)] = val
	}
}

func (s *scanner) readValue() (interface{}, error) {
	s.skipSpace()
	if s.atEnd() {
		return nil, fmt.Errorf("unexpected end of data")
	}
	b := s.peek()
	switch {
	case b == '/':
		return s.readName()
	case b == '(':
		return s.readLiteralString()
	case b == '<':
		if s.pos+1 < len(s.data) && s.data[s.pos+1] == '<' {
			return s.readDict()
		}
		return s.readHexString()
	case b == '[':
		return s.readArray()
	case b == 't':
		if s.expectWord("true") {
			return true, nil
		}
		return nil, fmt.Errorf("unexpected token at %d", s.pos)
	case b == 'f':
		if s.expectWord("false") {
			return false, nil
		}
		return nil, fmt.Errorf("unexpected token at %d", s.pos)
	case b == 'n':
		if s.expectWord("null") {
			return nil, nil
		}
		return nil, fmt.Errorf("unexpected token at %d", s.pos)
	case b == '+' || b == '-' || b == '.' || (b >= '0' && b <= '9'):
		return s.readNumberOrRef()
	default:
		return nil, fmt.Errorf("unexpected byte 0x%02x at %d", b, s.pos)
	}
}

func (s *scanner) readNumberOrRef() (interface{}, error) {
	num, err := s.readNumber()
	if err != nil {
		return nil, err
	}
	intVal, isInt := num.(int)
	if !isInt || intVal < 0 {
		return num, nil
	}
	// Speculatively try "G R" for indirect reference.
	saved := s.pos
	s.skipSpace()
	if s.atEnd() || s.peek() < '0' || s.peek() > '9' {
		s.pos = saved
		return num, nil
	}
	gen, err := s.readNumber()
	if err != nil {
		s.pos = saved
		return num, nil
	}
	genInt, ok := gen.(int)
	if !ok {
		s.pos = saved
		return num, nil
	}
	saved2 := s.pos
	s.skipSpace()
	if !s.atEnd() && s.peek() == 'R' {
		s.pos++
		return Ref{Num: intVal, Gen: genInt}, nil
	}
	s.pos = saved2
	// Two numbers consumed but no 'R' — backtrack to after first number.
	s.pos = saved
	return num, nil
}

func (s *scanner) readStreamData(dict map[string]interface{}, r *Reader) ([]byte, error) {
	s.skipSpace()
	if !s.hasPrefix("stream") {
		return nil, fmt.Errorf("expected 'stream' at %d", s.pos)
	}
	s.pos += 6
	if !s.atEnd() && s.peek() == '\r' {
		s.pos++
	}
	if !s.atEnd() && s.peek() == '\n' {
		s.pos++
	}

	length := -1
	if lv, ok := dict["/Length"]; ok {
		switch v := lv.(type) {
		case int:
			length = v
		case float64:
			length = int(v)
		case Ref:
			if r != nil {
				if rv, err := r.Object(v.Num); err == nil {
					length = toInt(rv)
				}
			}
		}
	}

	if length >= 0 && s.pos+length <= len(s.data) {
		raw := make([]byte, length)
		copy(raw, s.data[s.pos:s.pos+length])
		s.pos += length
		return raw, nil
	}

	// Fallback: scan for endstream.
	end := bytes.Index(s.data[s.pos:], []byte("endstream"))
	if end < 0 {
		return nil, fmt.Errorf("endstream not found")
	}
	raw := bytes.TrimRight(s.data[s.pos:s.pos+end], "\r\n")
	result := make([]byte, len(raw))
	copy(result, raw)
	s.pos += end + 9
	return result, nil
}

// ---- helpers ----

func toInt(v interface{}) int {
	switch v := v.(type) {
	case int:
		return v
	case float64:
		return int(v)
	}
	return 0
}

func toIntSlice(v interface{}) []int {
	arr, ok := v.([]interface{})
	if !ok {
		return nil
	}
	result := make([]int, len(arr))
	for i, val := range arr {
		result[i] = toInt(val)
	}
	return result
}

func toDict(v interface{}) map[string]interface{} {
	if d, ok := v.(map[string]interface{}); ok {
		return d
	}
	if s, ok := v.(*Stream); ok {
		return s.Hdr
	}
	return nil
}
