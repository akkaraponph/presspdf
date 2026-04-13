<p align="center">
  <img src="assets/logo-folio.png" alt="Folio" width="120">
</p>

# Security & Compliance

## Password protection

Encrypt the PDF with user and owner passwords:

```go
doc := folio.New()
// ... add content ...

doc.SetProtection("user123", "owner456", folio.PermPrint|folio.PermCopy)
doc.Save("protected.pdf")
```

- **User password** — required to open the document
- **Owner password** — required to change permissions or remove protection

### Permission flags

Control what users can do with the document:

| Flag | Allows |
|------|--------|
| `folio.PermPrint` | Printing |
| `folio.PermModify` | Modifying contents |
| `folio.PermCopy` | Copying/extracting text |
| `folio.PermAnnotate` | Adding annotations |
| `folio.PermAll` | All of the above |

Combine flags with `|`:

```go
doc.SetProtection("", "owner", folio.PermPrint|folio.PermCopy)
// Empty user password = no password to open, but permissions are enforced
```

Encryption uses RC4 (PDF 1.4 compatible with all viewers).

## Digital signatures

Sign a PDF with an X.509 certificate:

```go
import (
    "crypto/ecdsa"
    "crypto/x509"
)

doc.Sign(cert, key, page, 20, 90, 80, 25, folio.SignOptions{
    Name:     "John Doe",
    Reason:   "Document approval",
    Location: "Bangkok, Thailand",
})
```

Parameters:
- `cert` — `*x509.Certificate`
- `key` — `crypto.Signer` (e.g., `*ecdsa.PrivateKey`)
- `page` — the page where the visible signature appears
- `x, y, w, h` — position and size of the visible signature rectangle
- `SignOptions` — metadata shown in the signature panel

### Generating a test certificate

```go
key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
template := &x509.Certificate{
    SerialNumber: big.NewInt(1),
    Subject:      pkix.Name{CommonName: "Test Signer"},
    NotBefore:    time.Now(),
    NotAfter:     time.Now().Add(365 * 24 * time.Hour),
    KeyUsage:     x509.KeyUsageDigitalSignature,
}
certDER, _ := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
cert, _ := x509.ParseCertificate(certDER)

doc.Sign(cert, key, page, 20, 90, 80, 25, folio.SignOptions{
    Name: "Test Signer",
})
```

## Interactive form fields

Add fillable form fields (AcroForms) that users can complete in their PDF viewer.

### Text input

```go
page.FormTextField("full_name", 70, 36, 100, 8)
page.FormTextField("email", 70, 50, 100, 8, folio.WithMaxLen(50))
page.FormTextField("notes", 70, 64, 100, 20,
    folio.WithDefaultValue("Enter notes here"),
)
```

### Checkbox

```go
page.FormCheckbox("agree_terms", 20, 80, 5, false)  // unchecked
page.FormCheckbox("newsletter", 20, 90, 5, true)    // checked
```

### Dropdown

```go
page.FormDropdown("country", 70, 100, 80, 8,
    []string{"Thailand", "Japan", "USA", "UK", "Germany"},
)
```

### Field options

| Option | Effect |
|--------|--------|
| `folio.WithDefaultValue(s)` | Pre-fill the field |
| `folio.WithMaxLen(n)` | Maximum character length |

### Example: registration form

```go
doc := folio.New()
doc.SetFont("helvetica", "", 12)
page := doc.AddPage(folio.A4)

page.Text("Registration Form").Bold().Size(18).At(20, 20).Draw()

page.TextAt(20, 40, "Name:")
page.FormTextField("name", 70, 36, 100, 8)

page.TextAt(20, 55, "Email:")
page.FormTextField("email", 70, 51, 100, 8, folio.WithMaxLen(50))

page.TextAt(20, 70, "Country:")
page.FormDropdown("country", 70, 66, 80, 8,
    []string{"Thailand", "Japan", "USA", "UK"},
)

page.FormCheckbox("terms", 20, 85, 5, false)
page.TextAt(28, 85, "I agree to the terms and conditions")

doc.Save("form.pdf")
```

## PDF/A archival compliance

Create documents that conform to PDF/A for long-term preservation:

```go
doc := folio.New(folio.WithPDFA("1b"))  // or "2b"
```

### What PDF/A adds

- XMP metadata stream with PDF/A identification
- sRGB ICC output intent profile
- MarkInfo dictionary
- All fonts must be embedded (core fonts are not allowed)

### Requirements

PDF/A requires embedded fonts. Use TrueType fonts instead of core fonts:

```go
doc := folio.New(folio.WithPDFA("1b"))
doc.SetTitle("Archival Document")

sarabun.Register(doc)  // or any TTF font
doc.SetFont("sarabun", "", 12)

page := doc.AddPage(folio.A4)
page.TextAt(20, 20, "This document is PDF/A-1b compliant.")
doc.Save("archival.pdf")
```

### Compliance levels

| Level | Standard | Description |
|-------|----------|-------------|
| `"1b"` | ISO 19005-1 Level B | Visual appearance preservation |
| `"2b"` | ISO 19005-2 Level B | Same as 1b with PDF 1.7 features |
