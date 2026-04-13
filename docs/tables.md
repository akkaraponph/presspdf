<p align="center">
  <img src="assets/logo-folio.png" alt="Folio" width="120">
</p>

# Tables

## Basic table

Create a table, define column widths, then add header and rows:

```go
doc := folio.New()
doc.SetFont("helvetica", "", 10)
page := doc.AddPage(folio.A4)
page.SetXY(20, 20)

tbl := folio.NewTable(doc, page)
tbl.SetWidths(60, 60, 50)
tbl.Header("Name", "Department", "Salary")
tbl.Row("Alice Johnson", "Engineering", "$95,000")
tbl.Row("Bob Smith", "Marketing", "$72,000")
tbl.Row("Charlie Lee", "Design", "$78,000")
```

The table draws at the current cursor position and advances the cursor downward as rows are added.

## Styling

### Header style

```go
tbl.SetHeaderStyle(folio.CellStyle{
    FillColor: [3]int{40, 60, 120},    // dark blue background
    TextColor: [3]int{255, 255, 255},  // white text
    FontStyle: "B",                     // bold
})
```

### Body style

```go
tbl.SetBodyStyle(folio.CellStyle{
    FillColor: [3]int{245, 245, 245},
    TextColor: [3]int{0, 0, 0},
    FontStyle: "",
})
```

### Zebra striping

Alternate row colors for readability:

```go
tbl.SetAlternateRows(
    [3]int{240, 245, 255},  // even rows (light blue)
    [3]int{255, 255, 255},  // odd rows (white)
)
```

### Column alignment

```go
tbl.SetAligns("L", "L", "R")  // left, left, right-aligned
```

### Row height and borders

```go
tbl.SetRowHeight(8)       // 8mm per row
tbl.SetBorder("1")        // all borders (use "" for none, "LR" for sides only)
```

### Repeat header on page break

When a table spans multiple pages:

```go
tbl.SetRepeatHeader(true)
```

## Complete example

```go
doc := folio.New()
doc.SetFont("helvetica", "", 10)
page := doc.AddPage(folio.A4)
page.SetXY(20, 20)

tbl := folio.NewTable(doc, page)
tbl.SetWidths(50, 50, 40, 30)
tbl.SetAligns("L", "L", "R", "C")
tbl.SetRowHeight(7)
tbl.SetHeaderStyle(folio.CellStyle{
    FillColor: [3]int{40, 60, 120},
    TextColor: [3]int{255, 255, 255},
    FontStyle: "B",
})
tbl.SetAlternateRows([3]int{240, 245, 255}, [3]int{255, 255, 255})
tbl.SetRepeatHeader(true)

tbl.Header("Product", "Category", "Price", "Stock")
tbl.Row("Widget A", "Hardware", "$12.99", "142")
tbl.Row("Widget B", "Hardware", "$24.50", "87")
tbl.Row("Gadget X", "Electronics", "$49.99", "23")
tbl.Row("Service Y", "Support", "$99.00", "-")
```

## Complex tables (colspan, rowspan, multi-line)

For advanced layouts, use the buffered API with `AddHeader`/`AddRow` and `Render`:

```go
tbl := folio.NewTable(doc, page)
tbl.SetWidths(30, 60, 50, 50)
tbl.SetCellPadding(2)

// Spanning header
tbl.AddHeader(folio.TableCell{Text: "Quarterly Report", ColSpan: 4, Align: "C"})
tbl.AddHeader(
    folio.TableCell{Text: "Region"},
    folio.TableCell{Text: "Q1"},
    folio.TableCell{Text: "Q2"},
    folio.TableCell{Text: "Total"},
)

// Rowspan: "North" spans 2 rows
tbl.AddRow(
    folio.TableCell{Text: "North", RowSpan: 2},
    folio.TableCell{Text: "100"},
    folio.TableCell{Text: "150"},
    folio.TableCell{Text: "250"},
)
tbl.AddRow(
    folio.TableCell{Text: "200"},
    folio.TableCell{Text: "180"},
    folio.TableCell{Text: "380"},
)

tbl.Render()
```

### Multi-line cells

Cell text supports `\n` for explicit line breaks. Use `SetLineHeight` to control spacing:

```go
tbl.SetLineHeight(5)
tbl.AddRow(
    folio.TableCell{Text: "Line 1\nLine 2\nLine 3"},
    folio.TableCell{Text: "Single line"},
)
tbl.Render()
```

### Per-cell styling

Override style on individual cells:

```go
tbl.AddRow(
    folio.TableCell{Text: "Normal"},
    folio.TableCell{
        Text: "Highlighted",
        Style: &folio.CellStyle{
            FillColor: [3]int{255, 255, 0},
            FontStyle: "B",
            Fill:      true,
        },
    },
)
```

## Auto-table from structs

Generate a table automatically from a slice of structs. Field names become headers:

```go
type Employee struct {
    Name       string
    Department string
    Salary     string
}

data := []Employee{
    {"Alice Johnson", "Engineering", "$95,000"},
    {"Bob Smith", "Marketing", "$72,000"},
    {"Charlie Lee", "Design", "$78,000"},
}

page.SetXY(20, 20)
at := folio.AutoTableFromStructs(doc, page, data)
at.SetHeaderStyle(folio.CellStyle{
    FillColor: [3]int{40, 60, 120},
    TextColor: [3]int{255, 255, 255},
    FontStyle: "B",
})
at.Render()
```

## Auto-table from JSON

Generate a table from a JSON array of objects. Object keys become headers:

```go
jsonData := []byte(`[
    {"Product": "Widget A", "Price": "$12.99", "Stock": "142"},
    {"Product": "Widget B", "Price": "$24.50", "Stock": "87"},
    {"Product": "Gadget X", "Price": "$49.99", "Stock": "23"}
]`)

page.SetXY(20, 20)
jt, err := folio.AutoTableFromJSON(doc, page, jsonData)
if err != nil {
    log.Fatal(err)
}
jt.Render()
```
