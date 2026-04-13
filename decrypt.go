package folio

import (
	"bytes"
	"crypto/md5"
	"crypto/rc4"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/akkaraponph/folio/internal/pdfcore"
)

// DecryptPDF removes password protection from a PDF file.
// The password can be either the user password or the owner password.
// If the PDF is not encrypted, it is copied as-is.
//
// This supports PDF Standard Security Handler V=1, R=2 (40-bit RC4).
func DecryptPDF(inputPath, outputPath, password string) error {
	if dir := filepath.Dir(outputPath); dir != "." && dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("folio: create output dir: %w", err)
		}
	}

	data, err := os.ReadFile(inputPath)
	if err != nil {
		return fmt.Errorf("folio: read PDF: %w", err)
	}
	r, err := pdfcore.ReadPDF(data)
	if err != nil {
		return fmt.Errorf("folio: parse PDF: %w", err)
	}

	trailer := r.Trailer()

	// Check if encrypted.
	encRef, hasEncrypt := trailer["/Encrypt"].(pdfcore.Ref)
	if !hasEncrypt {
		// Check for inline encrypt dict.
		if _, inline := trailer["/Encrypt"].(map[string]interface{}); !inline {
			// Not encrypted — just copy.
			return os.WriteFile(outputPath, data, 0o644)
		}
	}

	// Get the encrypt dictionary.
	var encDict map[string]interface{}
	if hasEncrypt {
		obj, err := r.Object(encRef.Num)
		if err != nil {
			return fmt.Errorf("folio: read encrypt dict: %w", err)
		}
		encDict = pdfcore.ToDict(obj)
	} else {
		encDict = trailer["/Encrypt"].(map[string]interface{})
	}
	if encDict == nil {
		return fmt.Errorf("folio: encrypt dict is not a dictionary")
	}

	// Parse encryption parameters.
	v := pdfcore.ToInt(encDict["/V"])
	revision := pdfcore.ToInt(encDict["/R"])
	if v != 1 || revision != 2 {
		return fmt.Errorf("folio: unsupported encryption V=%d R=%d (only V=1 R=2 supported)", v, revision)
	}

	ownerHash := parseHashBytes(encDict["/O"])
	userHash := parseHashBytes(encDict["/U"])
	permissions := int32(pdfcore.ToInt(encDict["/P"]))

	// Get file ID.
	var fileID []byte
	if idArr, ok := trailer["/ID"].([]interface{}); ok && len(idArr) > 0 {
		if s, ok := idArr[0].(string); ok {
			fileID = []byte(s)
		}
	}

	// Try password as user password first, then as owner password.
	encKey, err := authenticatePassword(password, ownerHash, userHash, permissions, fileID)
	if err != nil {
		return fmt.Errorf("folio: %w", err)
	}

	// Now rewrite the PDF with decrypted content.
	return writeDecryptedPDF(r, encKey, outputPath, hasEncrypt, encRef.Num)
}

// authenticatePassword tries the password as user pw, then derives from owner pw.
// Returns the encryption key on success.
func authenticatePassword(password string, ownerHash, userHash [32]byte, permissions int32, fileID []byte) ([]byte, error) {
	// Try as user password.
	key := computeEncryptionKey(password, ownerHash, permissions, fileID)
	if verifyUserPassword(key, userHash) {
		return key, nil
	}

	// Try as owner password: derive user password from owner hash, then verify.
	userPw := recoverUserPwFromOwner(password, ownerHash)
	key = computeEncryptionKey(userPw, ownerHash, permissions, fileID)
	if verifyUserPassword(key, userHash) {
		return key, nil
	}

	return nil, fmt.Errorf("incorrect password")
}

// computeEncryptionKey computes the file encryption key (Algorithm 2 from PDF spec).
func computeEncryptionKey(userPw string, ownerHash [32]byte, permissions int32, fileID []byte) []byte {
	padded := padPw(userPw)
	h := md5.New()
	h.Write(padded[:])
	h.Write(ownerHash[:])
	p := uint32(permissions)
	h.Write([]byte{byte(p), byte(p >> 8), byte(p >> 16), byte(p >> 24)})
	h.Write(fileID)
	digest := h.Sum(nil)
	return digest[:5] // 40-bit key = 5 bytes
}

// verifyUserPassword checks if the key produces the correct /U hash.
func verifyUserPassword(key []byte, userHash [32]byte) bool {
	c, err := rc4.NewCipher(key)
	if err != nil {
		return false
	}
	var computed [32]byte
	c.XORKeyStream(computed[:], pdfPadding[:])
	return computed == userHash
}

// recoverUserPwFromOwner decrypts the owner hash to recover the padded user password.
// Algorithm 3 from the PDF spec (R=2): RC4-decrypt /O with MD5(ownerPw padding)[:5].
func recoverUserPwFromOwner(ownerPw string, ownerHash [32]byte) string {
	padded := padPw(ownerPw)
	h := md5.Sum(padded[:])
	key := h[:5]
	c, _ := rc4.NewCipher(key)
	var userPadded [32]byte
	c.XORKeyStream(userPadded[:], ownerHash[:])
	// Trim padding to get actual user password.
	pw := bytes.TrimRight(userPadded[:], string(pdfPadding[:]))
	// If fully padded, the user password is empty.
	if len(pw) == 0 {
		return ""
	}
	return string(pw)
}

// pdfPadding is the standard 32-byte padding from the PDF spec.
var pdfPadding = [32]byte{
	0x28, 0xBF, 0x4E, 0x5E, 0x4E, 0x75, 0x8A, 0x41,
	0x64, 0x00, 0x4E, 0x56, 0xFF, 0xFA, 0x01, 0x08,
	0x2E, 0x2E, 0x00, 0xB6, 0xD0, 0x68, 0x3E, 0x80,
	0x2F, 0x0C, 0xA9, 0xFE, 0x64, 0x53, 0x69, 0x7A,
}

func padPw(pw string) [32]byte {
	var padded [32]byte
	n := copy(padded[:], []byte(pw))
	copy(padded[n:], pdfPadding[:32-n])
	return padded
}

func parseHashBytes(v interface{}) [32]byte {
	var result [32]byte
	switch h := v.(type) {
	case string:
		copy(result[:], []byte(h))
	}
	return result
}

// rc4Decrypt decrypts data for a specific object using the per-object RC4 key.
func rc4Decrypt(encKey []byte, objNum, genNum int, data []byte) []byte {
	// Derive per-object key: MD5(encKey + objNum(3 LE) + genNum(2 LE))
	h := md5.New()
	h.Write(encKey)
	h.Write([]byte{
		byte(objNum), byte(objNum >> 8), byte(objNum >> 16),
		byte(genNum), byte(genNum >> 8),
	})
	digest := h.Sum(nil)
	kl := len(encKey) + 5
	if kl > 16 {
		kl = 16
	}
	c, _ := rc4.NewCipher(digest[:kl])
	out := make([]byte, len(data))
	c.XORKeyStream(out, data)
	return out
}

// decryptObject recursively decrypts strings and streams in a PDF object.
func decryptObject(obj interface{}, encKey []byte, objNum, genNum int) interface{} {
	switch v := obj.(type) {
	case string:
		return string(rc4Decrypt(encKey, objNum, genNum, []byte(v)))
	case map[string]interface{}:
		decrypted := make(map[string]interface{}, len(v))
		for k, val := range v {
			decrypted[k] = decryptObject(val, encKey, objNum, genNum)
		}
		return decrypted
	case []interface{}:
		decrypted := make([]interface{}, len(v))
		for i, val := range v {
			decrypted[i] = decryptObject(val, encKey, objNum, genNum)
		}
		return decrypted
	case *pdfcore.Stream:
		// Decrypt stream data.
		decryptedData := rc4Decrypt(encKey, objNum, genNum, v.Raw)
		// Decrypt strings in the header dict too.
		decryptedHdr := decryptObject(v.Hdr, encKey, objNum, genNum).(map[string]interface{})
		return &pdfcore.Stream{Hdr: decryptedHdr, Raw: decryptedData}
	default:
		return obj
	}
}

// writeDecryptedPDF writes a clean PDF without encryption.
func writeDecryptedPDF(r *pdfcore.Reader, encKey []byte, outputPath string, encIsRef bool, encObjNum int) error {
	pageRefs, err := r.PageRefs()
	if err != nil {
		return fmt.Errorf("folio: read pages: %w", err)
	}

	// Collect enhanced page attributes and dependencies.
	enhanced := make([]map[string]interface{}, len(pageRefs))
	var rootNums []int
	for i, ref := range pageRefs {
		attrs, err := r.InheritedPageAttrs(ref.Num)
		if err != nil {
			return err
		}
		enhanced[i] = attrs
		rootNums = append(rootNums, ref.Num)
		for _, vref := range pdfcore.CollectValueRefs(attrs) {
			rootNums = append(rootNums, vref.Num)
		}
	}
	deps, err := r.CollectDeps(rootNums...)
	if err != nil {
		return err
	}

	// Remove the encrypt dict object from deps.
	if encIsRef {
		delete(deps, encObjNum)
	}

	var sorted []int
	for num := range deps {
		sorted = append(sorted, num)
	}
	sort.Ints(sorted)

	// Decrypt all objects.
	decrypted := make(map[int]interface{}, len(sorted))
	for _, num := range sorted {
		obj, err := r.Object(num)
		if err != nil {
			continue
		}
		entry := r.XrefEntry(num)
		gen := 0
		if entry != nil {
			gen = entry.Gen
		}
		decrypted[num] = decryptObject(obj, encKey, num, gen)
	}

	// Build final remap.
	finalRemap := make(map[int]int)
	nextNum := 3 // 1=Catalog, 2=Pages
	for _, num := range sorted {
		finalRemap[num] = nextNum
		nextNum++
	}

	// Page set for /Parent override.
	pageSet := make(map[int]bool, len(pageRefs))
	enhMap := make(map[int]map[string]interface{}, len(pageRefs))
	for i, ref := range pageRefs {
		pageSet[ref.Num] = true
		enhanced[i]["/Parent"] = pdfcore.Ref{Num: 2}
		// Decrypt the enhanced page attrs too.
		entry := r.XrefEntry(ref.Num)
		gen := 0
		if entry != nil {
			gen = entry.Gen
		}
		enhMap[ref.Num] = decryptObject(enhanced[i], encKey, ref.Num, gen).(map[string]interface{})
	}

	// Write output.
	var buf bytes.Buffer
	offsets := make(map[int]int)

	fmt.Fprintf(&buf, "%%PDF-%s\n", r.Version())

	for _, oldNum := range sorted {
		newNum := finalRemap[oldNum]
		offsets[newNum] = buf.Len()
		fmt.Fprintf(&buf, "%d 0 obj\n", newNum)

		if pageSet[oldNum] {
			pdfcore.WriteValue(&buf, enhMap[oldNum], finalRemap)
		} else {
			pdfcore.WriteValue(&buf, decrypted[oldNum], finalRemap)
		}
		buf.WriteByte('\n')
		buf.WriteString("endobj\n")
	}

	// Pages tree.
	offsets[2] = buf.Len()
	buf.WriteString("2 0 obj\n<< /Type /Pages /Kids [")
	for i, ref := range pageRefs {
		if i > 0 {
			buf.WriteByte(' ')
		}
		fmt.Fprintf(&buf, "%d 0 R", finalRemap[ref.Num])
	}
	fmt.Fprintf(&buf, "] /Count %d >>\nendobj\n", len(pageRefs))

	// Catalog (no /Encrypt reference).
	offsets[1] = buf.Len()
	buf.WriteString("1 0 obj\n<< /Type /Catalog /Pages 2 0 R >>\nendobj\n")

	totalObj := nextNum - 1

	// Xref.
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

	// Trailer — no /Encrypt, no /ID needed.
	buf.WriteString("trailer\n")
	fmt.Fprintf(&buf, "<< /Size %d /Root 1 0 R >>\n", totalObj+1)
	buf.WriteString("startxref\n")
	fmt.Fprintf(&buf, "%d\n", xrefOffset)
	buf.WriteString("%%EOF\n")

	return os.WriteFile(outputPath, buf.Bytes(), 0o644)
}
