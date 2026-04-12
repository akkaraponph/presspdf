package barcode

import "fmt"

// QRCode encodes data as a QR code matrix. Returns a 2D boolean grid
// where true = black module. Supports byte mode, versions 1-10,
// error correction levels L and M.
func QRCode(data string, ecLevel int) ([][]bool, error) {
	if ecLevel < 0 || ecLevel > 1 {
		return nil, fmt.Errorf("QR: unsupported EC level %d (0=L, 1=M)", ecLevel)
	}

	dataBytes := []byte(data)

	// Find the smallest version that fits.
	ver := 0
	for v := 1; v <= 10; v++ {
		cap := dataCapacity[ecLevel][v-1]
		if len(dataBytes) <= cap {
			ver = v
			break
		}
	}
	if ver == 0 {
		return nil, fmt.Errorf("QR: data too long (%d bytes) for versions 1-10", len(dataBytes))
	}

	size := 17 + ver*4

	// Encode data bits.
	bits := encodeDataBits(dataBytes, ver, ecLevel)

	// Add error correction.
	codewords := bitsToCodewords(bits)
	ecCW := ecCodewords[ecLevel][ver-1]
	totalCW := totalCodewords[ver-1]
	dataCW := totalCW - ecCW

	// Pad data codewords.
	for len(codewords) < dataCW {
		codewords = append(codewords, 0)
	}
	codewords = codewords[:dataCW]

	// Generate EC codewords using Reed-Solomon.
	ecBytes := reedSolomon(codewords, ecCW)
	allCW := append(codewords, ecBytes...)

	// Convert to bit stream.
	var bitStream []bool
	for _, b := range allCW {
		for i := 7; i >= 0; i-- {
			bitStream = append(bitStream, (b>>uint(i))&1 == 1)
		}
	}

	// Build the matrix.
	matrix := make([][]bool, size)
	reserved := make([][]bool, size) // tracks reserved (non-data) modules
	for i := range matrix {
		matrix[i] = make([]bool, size)
		reserved[i] = make([]bool, size)
	}

	// Place finder patterns.
	placeFinder(matrix, reserved, 0, 0)
	placeFinder(matrix, reserved, size-7, 0)
	placeFinder(matrix, reserved, 0, size-7)

	// Timing patterns.
	for i := 8; i < size-8; i++ {
		matrix[6][i] = i%2 == 0
		reserved[6][i] = true
		matrix[i][6] = i%2 == 0
		reserved[i][6] = true
	}

	// Alignment patterns (versions 2+).
	if ver >= 2 {
		positions := alignmentPositions[ver-2]
		for _, row := range positions {
			for _, col := range positions {
				// Skip if overlapping with finder patterns.
				if (row < 9 && col < 9) || (row < 9 && col > size-9) || (row > size-9 && col < 9) {
					continue
				}
				placeAlignment(matrix, reserved, row, col)
			}
		}
	}

	// Dark module.
	matrix[size-8][8] = true
	reserved[size-8][8] = true

	// Reserve format info areas.
	for i := 0; i < 9; i++ {
		reserved[8][i] = true
		reserved[i][8] = true
	}
	for i := size - 8; i < size; i++ {
		reserved[8][i] = true
		reserved[i][8] = true
	}

	// Place data bits.
	placeData(matrix, reserved, bitStream, size)

	// Apply best mask.
	bestMask := 0
	bestPenalty := -1
	for mask := 0; mask < 8; mask++ {
		candidate := copyMatrix(matrix)
		applyMask(candidate, reserved, mask, size)
		placeFormatInfo(candidate, size, ecLevel, mask)
		p := penalty(candidate, size)
		if bestPenalty < 0 || p < bestPenalty {
			bestPenalty = p
			bestMask = mask
		}
	}

	applyMask(matrix, reserved, bestMask, size)
	placeFormatInfo(matrix, size, ecLevel, bestMask)

	return matrix, nil
}

// Data capacity in bytes for EC levels L(0) and M(1), versions 1-10.
var dataCapacity = [2][10]int{
	0: {17, 32, 53, 78, 106, 134, 154, 192, 230, 271},  // L
	1: {14, 26, 42, 62, 84, 106, 122, 152, 180, 213},    // M
}

// EC codewords per block for versions 1-10.
var ecCodewords = [2][10]int{
	0: {7, 10, 15, 20, 26, 18, 20, 24, 30, 18},   // L
	1: {10, 16, 26, 18, 24, 16, 18, 22, 22, 26},   // M
}

// Total codewords for versions 1-10.
var totalCodewords = [10]int{
	26, 44, 70, 100, 134, 172, 196, 242, 292, 346,
}

// Alignment pattern center positions for versions 2-10.
var alignmentPositions = [9][]int{
	{6, 18},         // v2
	{6, 22},         // v3
	{6, 26},         // v4
	{6, 30},         // v5
	{6, 34},         // v6
	{6, 22, 38},     // v7
	{6, 24, 42},     // v8
	{6, 26, 46},     // v9
	{6, 28, 50},     // v10
}

func encodeDataBits(data []byte, ver, ecLevel int) []bool {
	totalCW := totalCodewords[ver-1]
	ecCW := ecCodewords[ecLevel][ver-1]
	dataCW := totalCW - ecCW
	dataBits := dataCW * 8

	var bits []bool

	// Mode indicator: byte mode = 0100
	bits = append(bits, false, true, false, false)

	// Character count indicator (8 bits for v1-9, 16 bits for v10+).
	if ver <= 9 {
		for i := 7; i >= 0; i-- {
			bits = append(bits, (len(data)>>uint(i))&1 == 1)
		}
	} else {
		for i := 15; i >= 0; i-- {
			bits = append(bits, (len(data)>>uint(i))&1 == 1)
		}
	}

	// Data.
	for _, b := range data {
		for i := 7; i >= 0; i-- {
			bits = append(bits, (b>>uint(i))&1 == 1)
		}
	}

	// Terminator (up to 4 zero bits).
	for i := 0; i < 4 && len(bits) < dataBits; i++ {
		bits = append(bits, false)
	}

	// Pad to byte boundary.
	for len(bits)%8 != 0 {
		bits = append(bits, false)
	}

	// Pad bytes: alternate 0xEC and 0x11.
	padBytes := [2]byte{0xEC, 0x11}
	idx := 0
	for len(bits) < dataBits {
		b := padBytes[idx%2]
		for i := 7; i >= 0; i-- {
			bits = append(bits, (b>>uint(i))&1 == 1)
		}
		idx++
	}

	return bits[:dataBits]
}

func bitsToCodewords(bits []bool) []byte {
	var cw []byte
	for i := 0; i+7 < len(bits); i += 8 {
		var b byte
		for j := 0; j < 8; j++ {
			if bits[i+j] {
				b |= 1 << uint(7-j)
			}
		}
		cw = append(cw, b)
	}
	return cw
}

// reedSolomon generates error correction codewords using GF(256).
func reedSolomon(data []byte, numEC int) []byte {
	gen := rsGeneratorPoly(numEC)
	result := make([]byte, len(data)+numEC)
	copy(result, data)

	for i := 0; i < len(data); i++ {
		coef := result[i]
		if coef != 0 {
			for j := 0; j < len(gen); j++ {
				result[i+j+1] ^= gfMul(gen[j], coef)
			}
		}
	}

	return result[len(data):]
}

func rsGeneratorPoly(degree int) []byte {
	gen := []byte{1}
	for i := 0; i < degree; i++ {
		newGen := make([]byte, len(gen)+1)
		for j := 0; j < len(gen); j++ {
			newGen[j] ^= gen[j]
			newGen[j+1] ^= gfMul(gen[j], gfExp[i])
		}
		gen = newGen
	}
	return gen[1:] // drop leading 1
}

func gfMul(a, b byte) byte {
	if a == 0 || b == 0 {
		return 0
	}
	return gfExp[(int(gfLog[a])+int(gfLog[b]))%255]
}

// GF(256) exp and log tables (primitive polynomial 0x11D).
var gfExp [512]byte
var gfLog [256]byte

func init() {
	x := 1
	for i := 0; i < 255; i++ {
		gfExp[i] = byte(x)
		gfLog[x] = byte(i)
		x <<= 1
		if x >= 256 {
			x ^= 0x11D
		}
	}
	for i := 255; i < 512; i++ {
		gfExp[i] = gfExp[i-255]
	}
}

func placeFinder(matrix, reserved [][]bool, row, col int) {
	for r := 0; r < 7; r++ {
		for c := 0; c < 7; c++ {
			// Finder pattern: 7x7 with border
			dark := r == 0 || r == 6 || c == 0 || c == 6 ||
				(r >= 2 && r <= 4 && c >= 2 && c <= 4)
			matrix[row+r][col+c] = dark
			reserved[row+r][col+c] = true
		}
	}
	// Separator (1 module white border around finder).
	for i := -1; i <= 7; i++ {
		setReserved(matrix, reserved, row-1, col+i, len(matrix))
		setReserved(matrix, reserved, row+7, col+i, len(matrix))
		setReserved(matrix, reserved, row+i, col-1, len(matrix))
		setReserved(matrix, reserved, row+i, col+7, len(matrix))
	}
}

func setReserved(matrix, reserved [][]bool, r, c, size int) {
	if r >= 0 && r < size && c >= 0 && c < size {
		reserved[r][c] = true
		// matrix stays false (white)
	}
}

func placeAlignment(matrix, reserved [][]bool, row, col int) {
	for r := -2; r <= 2; r++ {
		for c := -2; c <= 2; c++ {
			dark := r == -2 || r == 2 || c == -2 || c == 2 || (r == 0 && c == 0)
			matrix[row+r][col+c] = dark
			reserved[row+r][col+c] = true
		}
	}
}

func placeData(matrix, reserved [][]bool, bits []bool, size int) {
	bitIdx := 0
	// Data is placed in 2-column strips, right to left, bottom to top,
	// alternating upward and downward.
	up := true
	for col := size - 1; col >= 1; col -= 2 {
		if col == 6 {
			col = 5 // skip timing column
		}
		if up {
			for row := size - 1; row >= 0; row-- {
				for c := 0; c < 2; c++ {
					cc := col - c
					if !reserved[row][cc] {
						if bitIdx < len(bits) {
							matrix[row][cc] = bits[bitIdx]
							bitIdx++
						}
					}
				}
			}
		} else {
			for row := 0; row < size; row++ {
				for c := 0; c < 2; c++ {
					cc := col - c
					if !reserved[row][cc] {
						if bitIdx < len(bits) {
							matrix[row][cc] = bits[bitIdx]
							bitIdx++
						}
					}
				}
			}
		}
		up = !up
	}
}

func applyMask(matrix, reserved [][]bool, mask, size int) {
	for r := 0; r < size; r++ {
		for c := 0; c < size; c++ {
			if reserved[r][c] {
				continue
			}
			var invert bool
			switch mask {
			case 0:
				invert = (r+c)%2 == 0
			case 1:
				invert = r%2 == 0
			case 2:
				invert = c%3 == 0
			case 3:
				invert = (r+c)%3 == 0
			case 4:
				invert = (r/2+c/3)%2 == 0
			case 5:
				invert = (r*c)%2+(r*c)%3 == 0
			case 6:
				invert = ((r*c)%2+(r*c)%3)%2 == 0
			case 7:
				invert = ((r+c)%2+(r*c)%3)%2 == 0
			}
			if invert {
				matrix[r][c] = !matrix[r][c]
			}
		}
	}
}

// Format info: 15-bit BCH encoded value placed around finder patterns.
var formatInfoBits [32]uint32

func init() {
	// Pre-compute format info for all EC level + mask combinations.
	// Format: 2-bit EC level indicator + 3-bit mask pattern, BCH(15,5) encoded.
	for ecl := 0; ecl < 4; ecl++ {
		for mask := 0; mask < 8; mask++ {
			data := uint32(ecl<<3 | mask)
			encoded := bch15_5(data)
			encoded ^= 0x5412 // XOR mask
			formatInfoBits[ecl*8+mask] = encoded
		}
	}
}

func bch15_5(data uint32) uint32 {
	d := data << 10
	// Generator polynomial for BCH(15,5): x^10 + x^8 + x^5 + x^4 + x^2 + x + 1 = 0x537
	gen := uint32(0x537)
	for i := 4; i >= 0; i-- {
		if d&(1<<uint(i+10)) != 0 {
			d ^= gen << uint(i)
		}
	}
	return (data << 10) | d
}

func placeFormatInfo(matrix [][]bool, size, ecLevel, mask int) {
	// EC level indicators: L=01, M=00, Q=11, H=10
	ecIndicator := [2]int{1, 0} // L=1, M=0
	info := formatInfoBits[ecIndicator[ecLevel]*8+mask]

	// Place around top-left finder.
	for i := 0; i < 6; i++ {
		matrix[8][i] = (info>>uint(14-i))&1 == 1
	}
	matrix[8][7] = (info>>uint(8))&1 == 1
	matrix[8][8] = (info>>uint(7))&1 == 1
	matrix[7][8] = (info>>uint(6))&1 == 1
	for i := 0; i < 6; i++ {
		matrix[5-i][8] = (info>>uint(i))&1 == 1
	}

	// Place around other finders.
	for i := 0; i < 8; i++ {
		matrix[size-1-i][8] = (info>>uint(14-i))&1 == 1
	}
	for i := 0; i < 7; i++ {
		matrix[8][size-7+i] = (info>>uint(7-i+1))&1 == 1 // bits 8-14 → remaining
	}

	// Corrected placement for right-side and bottom.
	// Bottom-left vertical: bits 0-6
	for i := 0; i < 7; i++ {
		matrix[size-7+i][8] = (info>>uint(i))&1 == 1
	}
	matrix[size-8][8] = true // always dark

	// Top-right horizontal: bits 7-14
	for i := 0; i < 8; i++ {
		matrix[8][size-8+i] = (info>>uint(7+i))&1 == 1
	}
}

func copyMatrix(m [][]bool) [][]bool {
	c := make([][]bool, len(m))
	for i := range m {
		c[i] = make([]bool, len(m[i]))
		copy(c[i], m[i])
	}
	return c
}

func penalty(matrix [][]bool, size int) int {
	p := 0

	// Rule 1: consecutive same-color modules in row/col.
	for r := 0; r < size; r++ {
		count := 1
		for c := 1; c < size; c++ {
			if matrix[r][c] == matrix[r][c-1] {
				count++
			} else {
				if count >= 5 {
					p += count - 2
				}
				count = 1
			}
		}
		if count >= 5 {
			p += count - 2
		}
	}
	for c := 0; c < size; c++ {
		count := 1
		for r := 1; r < size; r++ {
			if matrix[r][c] == matrix[r-1][c] {
				count++
			} else {
				if count >= 5 {
					p += count - 2
				}
				count = 1
			}
		}
		if count >= 5 {
			p += count - 2
		}
	}

	// Rule 2: 2x2 blocks of same color.
	for r := 0; r < size-1; r++ {
		for c := 0; c < size-1; c++ {
			v := matrix[r][c]
			if v == matrix[r][c+1] && v == matrix[r+1][c] && v == matrix[r+1][c+1] {
				p += 3
			}
		}
	}

	// Rule 3: finder-like pattern (simplified).
	// Skip for simplicity — mask selection still works reasonably.

	// Rule 4: proportion of dark modules.
	dark := 0
	for r := 0; r < size; r++ {
		for c := 0; c < size; c++ {
			if matrix[r][c] {
				dark++
			}
		}
	}
	total := size * size
	pct := dark * 100 / total
	prev5 := (pct / 5) * 5
	next5 := prev5 + 5
	d1 := prev5 - 50
	if d1 < 0 {
		d1 = -d1
	}
	d2 := next5 - 50
	if d2 < 0 {
		d2 = -d2
	}
	if d1 < d2 {
		p += d1 / 5 * 10
	} else {
		p += d2 / 5 * 10
	}

	return p
}
