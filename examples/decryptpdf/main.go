package main

import (
	"fmt"
	"log"
	"os"

	"github.com/akkaraponph/folio"
)

func main() {
	os.MkdirAll("output", 0o755)

	// Create an encrypted PDF.
	src := createEncryptedPDF()
	fmt.Println("Created encrypted PDF:", src)

	// Decrypt with user password.
	err := folio.DecryptPDF(src, "output/decrypted_user.pdf", "hello")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("Decrypted with user password → output/decrypted_user.pdf")

	// Decrypt with owner password.
	err = folio.DecryptPDF(src, "output/decrypted_owner.pdf", "admin")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("Decrypted with owner password → output/decrypted_owner.pdf")

	// Try wrong password.
	err = folio.DecryptPDF(src, "output/should_fail.pdf", "wrongpass")
	if err != nil {
		fmt.Println("Wrong password (expected):", err)
	}

	// Decrypt owner-only protection (empty user password).
	src2 := createOwnerOnlyPDF()
	err = folio.DecryptPDF(src2, "output/decrypted_owneronly.pdf", "")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("Decrypted owner-only PDF → output/decrypted_owneronly.pdf")
}

func createEncryptedPDF() string {
	doc := folio.New()
	doc.SetFont("helvetica", "B", 24)
	doc.SetProtection("hello", "admin", folio.PermAll)

	page := doc.AddPage(folio.A4)
	page.TextAt(50, 50, "This was encrypted!")
	doc.SetFont("helvetica", "", 14)
	page.TextAt(50, 80, "If you can read this, decryption worked.")

	path := "output/encrypted.pdf"
	if err := doc.Save(path); err != nil {
		log.Fatal(err)
	}
	return path
}

func createOwnerOnlyPDF() string {
	doc := folio.New()
	doc.SetFont("helvetica", "", 14)
	doc.SetProtection("", "secretowner", folio.PermPrint)

	page := doc.AddPage(folio.A4)
	page.TextAt(50, 50, "Owner-only protected document")

	path := "output/owner_only.pdf"
	if err := doc.Save(path); err != nil {
		log.Fatal(err)
	}
	return path
}
