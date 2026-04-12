package folio

import (
	"math"
	"strconv"
	"strings"
	"unicode"

	"github.com/akkaraponph/folio/internal/state"
)

// SVGPath draws an SVG path at position (x, y) in user units, scaled by
// scale. The d parameter is an SVG path data string (the value of the "d"
// attribute). style is "D" (stroke), "F" (fill), or "DF"/"FD" (both).
//
// Supported SVG commands: M, L, H, V, C, S, Q, T, A, Z (and lowercase
// relative variants). Implicit repeated commands are handled.
//
//	page.SVGPath(10, 10, 0.5, "M 0 0 L 100 0 L 100 100 Z", "D")
func (p *Page) SVGPath(x, y, scale float64, d string, style string) {
	p = p.active()
	if p.doc.err != nil {
		return
	}

	commands := parseSVGPath(d)
	if len(commands) == 0 {
		return
	}

	k := p.doc.k
	originX := state.ToPointsX(x, k)
	originY := state.ToPointsY(y, p.h, k)

	// SVG has Y-down; PDF has Y-up. We apply scale and flip Y.
	toX := func(sx float64) float64 { return originX + sx*scale*k }
	toY := func(sy float64) float64 { return originY - sy*scale*k }

	var curX, curY float64     // current point (SVG coords)
	var startX, startY float64 // subpath start
	var lastCX, lastCY float64 // last control point for S/T reflection

	for _, cmd := range commands {
		switch cmd.op {
		case 'M':
			for i := 0; i+1 < len(cmd.params); i += 2 {
				curX, curY = cmd.params[i], cmd.params[i+1]
				if i == 0 {
					p.stream.MoveTo(toX(curX), toY(curY))
					startX, startY = curX, curY
				} else {
					// Subsequent M pairs are implicit LineTo.
					p.stream.LineTo(toX(curX), toY(curY))
				}
			}
		case 'm':
			for i := 0; i+1 < len(cmd.params); i += 2 {
				curX += cmd.params[i]
				curY += cmd.params[i+1]
				if i == 0 {
					p.stream.MoveTo(toX(curX), toY(curY))
					startX, startY = curX, curY
				} else {
					p.stream.LineTo(toX(curX), toY(curY))
				}
			}

		case 'L':
			for i := 0; i+1 < len(cmd.params); i += 2 {
				curX, curY = cmd.params[i], cmd.params[i+1]
				p.stream.LineTo(toX(curX), toY(curY))
			}
		case 'l':
			for i := 0; i+1 < len(cmd.params); i += 2 {
				curX += cmd.params[i]
				curY += cmd.params[i+1]
				p.stream.LineTo(toX(curX), toY(curY))
			}

		case 'H':
			for _, px := range cmd.params {
				curX = px
				p.stream.LineTo(toX(curX), toY(curY))
			}
		case 'h':
			for _, dx := range cmd.params {
				curX += dx
				p.stream.LineTo(toX(curX), toY(curY))
			}

		case 'V':
			for _, py := range cmd.params {
				curY = py
				p.stream.LineTo(toX(curX), toY(curY))
			}
		case 'v':
			for _, dy := range cmd.params {
				curY += dy
				p.stream.LineTo(toX(curX), toY(curY))
			}

		case 'C':
			for i := 0; i+5 < len(cmd.params); i += 6 {
				x1, y1 := cmd.params[i], cmd.params[i+1]
				x2, y2 := cmd.params[i+2], cmd.params[i+3]
				ex, ey := cmd.params[i+4], cmd.params[i+5]
				p.stream.CurveTo(toX(x1), toY(y1), toX(x2), toY(y2), toX(ex), toY(ey))
				lastCX, lastCY = x2, y2
				curX, curY = ex, ey
			}
		case 'c':
			for i := 0; i+5 < len(cmd.params); i += 6 {
				x1 := curX + cmd.params[i]
				y1 := curY + cmd.params[i+1]
				x2 := curX + cmd.params[i+2]
				y2 := curY + cmd.params[i+3]
				ex := curX + cmd.params[i+4]
				ey := curY + cmd.params[i+5]
				p.stream.CurveTo(toX(x1), toY(y1), toX(x2), toY(y2), toX(ex), toY(ey))
				lastCX, lastCY = x2, y2
				curX, curY = ex, ey
			}

		case 'S':
			for i := 0; i+3 < len(cmd.params); i += 4 {
				// Reflect last control point.
				x1 := 2*curX - lastCX
				y1 := 2*curY - lastCY
				x2, y2 := cmd.params[i], cmd.params[i+1]
				ex, ey := cmd.params[i+2], cmd.params[i+3]
				p.stream.CurveTo(toX(x1), toY(y1), toX(x2), toY(y2), toX(ex), toY(ey))
				lastCX, lastCY = x2, y2
				curX, curY = ex, ey
			}
		case 's':
			for i := 0; i+3 < len(cmd.params); i += 4 {
				x1 := 2*curX - lastCX
				y1 := 2*curY - lastCY
				x2 := curX + cmd.params[i]
				y2 := curY + cmd.params[i+1]
				ex := curX + cmd.params[i+2]
				ey := curY + cmd.params[i+3]
				p.stream.CurveTo(toX(x1), toY(y1), toX(x2), toY(y2), toX(ex), toY(ey))
				lastCX, lastCY = x2, y2
				curX, curY = ex, ey
			}

		case 'Q':
			for i := 0; i+3 < len(cmd.params); i += 4 {
				qx, qy := cmd.params[i], cmd.params[i+1]
				ex, ey := cmd.params[i+2], cmd.params[i+3]
				// Promote quadratic to cubic.
				c1x := curX + 2.0/3.0*(qx-curX)
				c1y := curY + 2.0/3.0*(qy-curY)
				c2x := ex + 2.0/3.0*(qx-ex)
				c2y := ey + 2.0/3.0*(qy-ey)
				p.stream.CurveTo(toX(c1x), toY(c1y), toX(c2x), toY(c2y), toX(ex), toY(ey))
				lastCX, lastCY = qx, qy
				curX, curY = ex, ey
			}
		case 'q':
			for i := 0; i+3 < len(cmd.params); i += 4 {
				qx := curX + cmd.params[i]
				qy := curY + cmd.params[i+1]
				ex := curX + cmd.params[i+2]
				ey := curY + cmd.params[i+3]
				c1x := curX + 2.0/3.0*(qx-curX)
				c1y := curY + 2.0/3.0*(qy-curY)
				c2x := ex + 2.0/3.0*(qx-ex)
				c2y := ey + 2.0/3.0*(qy-ey)
				p.stream.CurveTo(toX(c1x), toY(c1y), toX(c2x), toY(c2y), toX(ex), toY(ey))
				lastCX, lastCY = qx, qy
				curX, curY = ex, ey
			}

		case 'T':
			for i := 0; i+1 < len(cmd.params); i += 2 {
				qx := 2*curX - lastCX
				qy := 2*curY - lastCY
				ex, ey := cmd.params[i], cmd.params[i+1]
				c1x := curX + 2.0/3.0*(qx-curX)
				c1y := curY + 2.0/3.0*(qy-curY)
				c2x := ex + 2.0/3.0*(qx-ex)
				c2y := ey + 2.0/3.0*(qy-ey)
				p.stream.CurveTo(toX(c1x), toY(c1y), toX(c2x), toY(c2y), toX(ex), toY(ey))
				lastCX, lastCY = qx, qy
				curX, curY = ex, ey
			}
		case 't':
			for i := 0; i+1 < len(cmd.params); i += 2 {
				qx := 2*curX - lastCX
				qy := 2*curY - lastCY
				ex := curX + cmd.params[i]
				ey := curY + cmd.params[i+1]
				c1x := curX + 2.0/3.0*(qx-curX)
				c1y := curY + 2.0/3.0*(qy-curY)
				c2x := ex + 2.0/3.0*(qx-ex)
				c2y := ey + 2.0/3.0*(qy-ey)
				p.stream.CurveTo(toX(c1x), toY(c1y), toX(c2x), toY(c2y), toX(ex), toY(ey))
				lastCX, lastCY = qx, qy
				curX, curY = ex, ey
			}

		case 'A', 'a':
			rel := cmd.op == 'a'
			for i := 0; i+6 < len(cmd.params); i += 7 {
				rx := math.Abs(cmd.params[i])
				ry := math.Abs(cmd.params[i+1])
				xRot := cmd.params[i+2]
				largeArc := cmd.params[i+3] != 0
				sweep := cmd.params[i+4] != 0
				ex, ey := cmd.params[i+5], cmd.params[i+6]
				if rel {
					ex += curX
					ey += curY
				}
				svgArcToBezier(p, toX, toY, curX, curY, rx, ry, xRot, largeArc, sweep, ex, ey)
				curX, curY = ex, ey
				lastCX, lastCY = curX, curY
			}

		case 'Z', 'z':
			p.stream.ClosePath()
			curX, curY = startX, startY
		}

		// Reset last control point for commands that don't set it.
		switch cmd.op {
		case 'C', 'c', 'S', 's', 'Q', 'q', 'T', 't':
			// lastCX/lastCY already set above.
		default:
			lastCX, lastCY = curX, curY
		}
	}

	// Paint the path.
	style = strings.ToUpper(style)
	switch style {
	case "F":
		p.stream.Fill()
	case "DF", "FD":
		p.stream.FillStroke()
	default:
		p.stream.Stroke()
	}
}

// svgCommand represents a parsed SVG path command.
type svgCommand struct {
	op     byte      // 'M', 'L', 'C', etc. (case-sensitive)
	params []float64 // command parameters
}

// parseSVGPath tokenizes an SVG path data string into commands.
func parseSVGPath(d string) []svgCommand {
	var commands []svgCommand
	var currentOp byte
	var params []float64

	flush := func() {
		if currentOp != 0 && len(params) > 0 {
			commands = append(commands, svgCommand{op: currentOp, params: params})
			params = nil
		} else if currentOp != 0 {
			commands = append(commands, svgCommand{op: currentOp})
		}
	}

	i := 0
	runes := []byte(d)
	n := len(runes)

	for i < n {
		ch := runes[i]

		// Skip whitespace and commas.
		if ch == ' ' || ch == '\t' || ch == '\n' || ch == '\r' || ch == ',' {
			i++
			continue
		}

		// Check for command letter.
		if isCommand(ch) {
			flush()
			currentOp = ch
			params = nil
			i++
			continue
		}

		// Parse a number.
		start := i
		if i < n && (runes[i] == '-' || runes[i] == '+') {
			i++
		}
		for i < n && (runes[i] >= '0' && runes[i] <= '9') {
			i++
		}
		if i < n && runes[i] == '.' {
			i++
			for i < n && (runes[i] >= '0' && runes[i] <= '9') {
				i++
			}
		}
		// Scientific notation.
		if i < n && (runes[i] == 'e' || runes[i] == 'E') {
			i++
			if i < n && (runes[i] == '+' || runes[i] == '-') {
				i++
			}
			for i < n && (runes[i] >= '0' && runes[i] <= '9') {
				i++
			}
		}

		if i > start {
			if val, err := strconv.ParseFloat(d[start:i], 64); err == nil {
				params = append(params, val)
			}
		} else {
			i++ // skip unknown character
		}
	}
	flush()

	// Handle implicit repeated commands: if a command has more params
	// than its arity, split into multiple commands.
	var result []svgCommand
	for _, cmd := range commands {
		arity := commandArity(cmd.op)
		if arity == 0 || len(cmd.params) <= arity {
			result = append(result, cmd)
			continue
		}
		for j := 0; j < len(cmd.params); j += arity {
			end := j + arity
			if end > len(cmd.params) {
				break
			}
			repeatOp := cmd.op
			// After M, implicit repeats are L; after m, implicit repeats are l.
			if j > 0 {
				if cmd.op == 'M' {
					repeatOp = 'L'
				} else if cmd.op == 'm' {
					repeatOp = 'l'
				}
			}
			result = append(result, svgCommand{op: repeatOp, params: cmd.params[j:end]})
		}
	}

	return result
}

func isCommand(ch byte) bool {
	return unicode.IsLetter(rune(ch)) && ch != 'e' && ch != 'E'
}

func commandArity(op byte) int {
	switch op {
	case 'M', 'm', 'L', 'l', 'T', 't':
		return 2
	case 'H', 'h', 'V', 'v':
		return 1
	case 'C', 'c':
		return 6
	case 'S', 's', 'Q', 'q':
		return 4
	case 'A', 'a':
		return 7
	case 'Z', 'z':
		return 0
	}
	return 0
}

// svgArcToBezier converts an SVG arc to cubic Bézier curves and emits them.
// Implements the endpoint-to-center parameterization from the SVG spec.
func svgArcToBezier(p *Page, toX, toY func(float64) float64,
	x1, y1, rx, ry, xRotDeg float64, largeArc, sweep bool, x2, y2 float64) {

	if rx == 0 || ry == 0 {
		p.stream.LineTo(toX(x2), toY(y2))
		return
	}

	xRot := xRotDeg * math.Pi / 180.0
	cosR := math.Cos(xRot)
	sinR := math.Sin(xRot)

	// Step 1: Compute (x1', y1')
	dx := (x1 - x2) / 2
	dy := (y1 - y2) / 2
	x1p := cosR*dx + sinR*dy
	y1p := -sinR*dx + cosR*dy

	// Step 2: Compute (cx', cy')
	x1pSq := x1p * x1p
	y1pSq := y1p * y1p
	rxSq := rx * rx
	rySq := ry * ry

	// Check if radii are large enough; scale up if not.
	lambda := x1pSq/rxSq + y1pSq/rySq
	if lambda > 1 {
		s := math.Sqrt(lambda)
		rx *= s
		ry *= s
		rxSq = rx * rx
		rySq = ry * ry
	}

	num := rxSq*rySq - rxSq*y1pSq - rySq*x1pSq
	den := rxSq*y1pSq + rySq*x1pSq
	if den == 0 {
		p.stream.LineTo(toX(x2), toY(y2))
		return
	}
	sq := math.Sqrt(math.Abs(num / den))
	if largeArc == sweep {
		sq = -sq
	}
	cxp := sq * rx * y1p / ry
	cyp := -sq * ry * x1p / rx

	// Step 3: Compute (cx, cy)
	cx := cosR*cxp - sinR*cyp + (x1+x2)/2
	cy := sinR*cxp + cosR*cyp + (y1+y2)/2

	// Step 4: Compute theta1 and dtheta
	theta1 := svgAngle(1, 0, (x1p-cxp)/rx, (y1p-cyp)/ry)
	dtheta := svgAngle((x1p-cxp)/rx, (y1p-cyp)/ry, (-x1p-cxp)/rx, (-y1p-cyp)/ry)

	if !sweep && dtheta > 0 {
		dtheta -= 2 * math.Pi
	} else if sweep && dtheta < 0 {
		dtheta += 2 * math.Pi
	}

	// Split into segments of at most π/2 each.
	nSegs := int(math.Ceil(math.Abs(dtheta) / (math.Pi / 2)))
	if nSegs < 1 {
		nSegs = 1
	}
	segAngle := dtheta / float64(nSegs)

	for seg := 0; seg < nSegs; seg++ {
		a1 := theta1 + float64(seg)*segAngle
		a2 := a1 + segAngle
		alpha := 4.0 / 3.0 * math.Tan((a2-a1)/4.0)

		cos1 := math.Cos(a1)
		sin1 := math.Sin(a1)
		cos2 := math.Cos(a2)
		sin2 := math.Sin(a2)

		// Endpoints and control points in the rotated ellipse frame.
		ep1x := rx * cos1
		ep1y := ry * sin1
		ep2x := rx * cos2
		ep2y := ry * sin2

		cp1x := ep1x - alpha*rx*sin1
		cp1y := ep1y + alpha*ry*cos1
		cp2x := ep2x + alpha*rx*sin2
		cp2y := ep2y - alpha*ry*cos2

		// Transform back to user coordinates.
		c1x := cosR*cp1x - sinR*cp1y + cx
		c1y := sinR*cp1x + cosR*cp1y + cy
		c2x := cosR*cp2x - sinR*cp2y + cx
		c2y := sinR*cp2x + cosR*cp2y + cy
		endX := cosR*ep2x - sinR*ep2y + cx
		endY := sinR*ep2x + cosR*ep2y + cy

		p.stream.CurveTo(toX(c1x), toY(c1y), toX(c2x), toY(c2y), toX(endX), toY(endY))
	}
}

// svgAngle computes the angle between two vectors.
func svgAngle(ux, uy, vx, vy float64) float64 {
	n := math.Sqrt((ux*ux+uy*uy) * (vx*vx + vy*vy))
	if n == 0 {
		return 0
	}
	c := (ux*vx + uy*vy) / n
	if c < -1 {
		c = -1
	}
	if c > 1 {
		c = 1
	}
	angle := math.Acos(c)
	if ux*vy-uy*vx < 0 {
		angle = -angle
	}
	return angle
}
