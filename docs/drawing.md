<p align="center">
  <img src="assets/logo-folio.png" alt="Folio" width="120">
</p>

# Drawing

## Basic shapes

All shapes take a `style` parameter: `"D"` (stroke), `"F"` (fill), `"DF"` (both).

```go
page.Line(20, 50, 190, 50)              // line segment
page.Rect(20, 60, 80, 30, "D")          // rectangle outline
page.Circle(60, 120, 15, "F")           // filled circle
page.Ellipse(60, 160, 25, 12, "DF")     // filled + stroked ellipse
page.Arc(60, 200, 20, 20, 0, 270, "D")  // arc (0 to 270 degrees)
```

## Colors

```go
doc.SetDrawColor(255, 0, 0)      // stroke color (red)
doc.SetFillColor(200, 230, 255)  // fill color (light blue)
doc.SetLineWidth(0.5)            // line thickness in current units
```

## Transparency

```go
doc.SetAlpha(0.5)  // 50% transparent (affects everything drawn after)
page.Rect(20, 20, 80, 40, "F")
doc.SetAlpha(1.0)  // back to opaque
```

## Dash patterns

```go
page.SetDashPattern([]float64{3, 2}, 0)  // 3mm dash, 2mm gap
page.Line(20, 50, 190, 50)               // dashed line
page.SetDashPattern(nil, 0)              // back to solid
```

## SVG paths

Draw shapes from SVG path data strings. Supports all standard commands: M, L, H, V, C, S, Q, T, A, Z (both absolute and relative).

```go
// Star shape
star := "M 50 0 L 61 35 L 98 35 L 68 57 L 79 91 L 50 70 L 21 91 L 32 57 L 2 35 L 39 35 Z"
page.SVGPath(20, 20, 0.6, star, "DF")

// Heart shape with arcs
heart := "M 10 30 A 20 20 0 0 1 50 30 A 20 20 0 0 1 90 30 Q 90 60 50 90 Q 10 60 10 30 Z"
page.SVGPath(100, 20, 0.5, heart, "F")
```

The `scale` parameter (third argument) multiplies all coordinates in the path.

## Transforms

Transforms modify the coordinate system for subsequent drawing operations. Always wrap them in `TransformBegin`/`TransformEnd` to restore state:

### Rotation

```go
page.TransformBegin()
page.Rotate(45, 105, 148)  // 45 degrees around point (105, 148)
page.TextAt(105, 148, "Rotated text")
page.TransformEnd()
```

### Scale

```go
page.TransformBegin()
page.Scale(2, 2, 50, 50)  // 2x scale around point (50, 50)
page.Rect(50, 50, 20, 10, "D")  // appears 40x20
page.TransformEnd()
```

### Skew

```go
page.TransformBegin()
page.Skew(15, 0, 50, 50)  // skew X by 15 degrees
page.Rect(50, 50, 40, 20, "D")
page.TransformEnd()
```

### Translate

```go
page.TransformBegin()
page.Translate(50, 30)  // shift everything 50mm right, 30mm down
page.Rect(0, 0, 20, 10, "D")  // appears at (50, 30)
page.TransformEnd()
```

### Shortcut for rotated text

```go
page.TextRotatedAt(105, 148, 45, "Rotated 45 degrees")
```

## Gradients

### Linear gradient

```go
page.LinearGradient(
    20, 20, 80, 40,      // rectangle: x, y, w, h
    0, 0, 1, 0,          // direction: left to right
    folio.GradientStop{Pos: 0, R: 255, G: 0, B: 0},     // red
    folio.GradientStop{Pos: 1, R: 0, G: 0, B: 255},     // blue
)
```

Direction is normalized: (0,0) to (1,0) = left-to-right, (0,0) to (0,1) = top-to-bottom.

### Radial gradient

```go
page.RadialGradient(
    20, 80, 60, 60,      // rectangle: x, y, w, h
    0.5, 0.5, 0.5,       // center x, center y, radius (normalized)
    folio.GradientStop{Pos: 0, R: 255, G: 255, B: 0},   // yellow center
    folio.GradientStop{Pos: 1, R: 200, G: 50, B: 0},    // dark orange edge
)
```

## Clipping

Restrict drawing to a region. Combine with `TransformBegin`/`TransformEnd` to scope it:

```go
page.TransformBegin()
page.ClipCircle(60, 60, 25)       // only draw inside this circle
page.DrawImageRect("photo", 35, 35, 50, 50)  // image cropped to circle
page.TransformEnd()  // clipping released
```

Clipping shapes:

```go
page.ClipRect(20, 20, 80, 40)        // rectangular clip
page.ClipCircle(60, 60, 25)          // circular clip
page.ClipEllipse(60, 60, 30, 20)     // elliptical clip
```

## Fluent shape builder

For one-off shapes where you want to set color/style inline:

```go
page.Shape().Rect(20, 50, 80, 40).FillColor(66, 133, 244).Fill().Draw()
page.Shape().Circle(150, 70, 20).FillColor(244, 180, 0).Fill().Draw()
page.Shape().Line(20, 100, 190, 100).StrokeColor(200, 0, 0).LineWidth(1).Draw()
page.Shape().Ellipse(50, 130, 30, 15).FillColor(15, 157, 88).FillStroke().Draw()
```

The builder saves and restores colors/line width automatically — your changes don't leak.
