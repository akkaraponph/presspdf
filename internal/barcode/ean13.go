package barcode

import "fmt"

// EAN13 encodes a 12 or 13-digit string as an EAN-13 barcode, returning
// a bitmap where true = black module. The result is 95 modules wide.
// If 12 digits are provided, the check digit is computed automatically.
// If 13 digits are provided, the check digit is validated.
func EAN13(digits string) ([]bool, error) {
	if len(digits) == 12 {
		digits += string(rune('0' + ean13CheckDigit(digits)))
	}
	if len(digits) != 13 {
		return nil, fmt.Errorf("EAN-13 requires 12 or 13 digits, got %d", len(digits))
	}
	for _, ch := range digits {
		if ch < '0' || ch > '9' {
			return nil, fmt.Errorf("EAN-13 digits must be 0-9, got %q", ch)
		}
	}

	// Validate check digit.
	cd := ean13CheckDigit(digits[:12])
	if int(digits[12]-'0') != cd {
		return nil, fmt.Errorf("EAN-13 check digit mismatch: expected %d, got %c", cd, digits[12])
	}

	var modules []bool

	// Start guard: 101
	modules = append(modules, true, false, true)

	// First digit determines the parity pattern for left-side digits.
	firstDigit := int(digits[0] - '0')
	parityPat := parityPatterns[firstDigit]

	// Left side: digits 2-7 (indices 1-6).
	for i := 1; i <= 6; i++ {
		d := int(digits[i] - '0')
		var pat [7]bool
		if parityPat[i-1] == 'O' {
			pat = lOdd[d]
		} else {
			pat = lEven[d]
		}
		for _, b := range pat {
			modules = append(modules, b)
		}
	}

	// Center guard: 01010
	modules = append(modules, false, true, false, true, false)

	// Right side: digits 8-13 (indices 7-12).
	for i := 7; i <= 12; i++ {
		d := int(digits[i] - '0')
		pat := rPat[d]
		for _, b := range pat {
			modules = append(modules, b)
		}
	}

	// End guard: 101
	modules = append(modules, true, false, true)

	return modules, nil
}

func ean13CheckDigit(digits string) int {
	sum := 0
	for i := 0; i < 12; i++ {
		d := int(digits[i] - '0')
		if i%2 == 0 {
			sum += d
		} else {
			sum += d * 3
		}
	}
	return (10 - sum%10) % 10
}

// parityPatterns for the left half based on the first digit (0-9).
// O = odd parity (L encoding), E = even parity.
var parityPatterns = [10]string{
	0: "OOOOOO",
	1: "OOEOEE",
	2: "OOEEOE",
	3: "OOEEEO",
	4: "OEOOEE",
	5: "OEEOOE",
	6: "OEEEOO",
	7: "OEOEOE",
	8: "OEOEEO",
	9: "OEEOEO",
}

// L encoding (odd parity): 7-module patterns for digits 0-9.
var lOdd = [10][7]bool{
	0: {false, false, false, true, true, false, true},
	1: {false, false, true, true, false, false, true},
	2: {false, false, true, false, false, true, true},
	3: {false, true, true, true, true, false, true},
	4: {false, true, false, false, false, true, true},
	5: {false, true, true, false, false, false, true},
	6: {false, true, false, true, true, true, true},
	7: {false, true, true, true, false, true, true},
	8: {false, true, true, false, true, true, true},
	9: {false, false, false, true, false, true, true},
}

// L encoding (even parity): 7-module patterns for digits 0-9.
var lEven = [10][7]bool{
	0: {false, true, false, false, true, true, true},
	1: {false, true, true, false, false, true, true},
	2: {false, false, true, true, false, true, true},
	3: {false, true, false, false, false, false, true},
	4: {false, false, true, true, true, false, true},
	5: {false, true, true, true, false, false, true},
	6: {false, false, false, false, true, false, true},
	7: {false, false, true, false, false, false, true},
	8: {false, false, false, true, false, false, true},
	9: {false, false, true, false, true, true, true},
}

// R encoding: 7-module patterns for digits 0-9.
var rPat = [10][7]bool{
	0: {true, true, true, false, false, true, false},
	1: {true, true, false, false, true, true, false},
	2: {true, true, false, true, true, false, false},
	3: {true, false, false, false, false, true, false},
	4: {true, false, true, true, true, false, false},
	5: {true, false, false, true, true, true, false},
	6: {true, false, true, false, false, false, false},
	7: {true, false, false, false, true, false, false},
	8: {true, false, false, true, false, false, false},
	9: {true, true, true, false, true, false, false},
}
