<p align="center">
  <img src="assets/logo-folio.png" alt="Folio" width="120">
</p>

# Rich Content

## HTML rendering

Render an HTML subset directly onto the page:

```go
doc.SetFont("helvetica", "", 12)
page := doc.AddPage(folio.A4)

page.HTML(`<h1>Document Title</h1>
<p>This is a paragraph with <b>bold</b>, <i>italic</i>,
and <u>underline</u> text.</p>

<h2>Lists</h2>
<ul>
  <li>Unordered item</li>
  <li>Another item</li>
</ul>

<ol>
  <li>First step</li>
  <li>Second step</li>
</ol>

<h2>Table</h2>
<table>
  <tr><td>Name</td><td>Value</td></tr>
  <tr><td>Alpha</td><td>100</td></tr>
  <tr><td>Beta</td><td>200</td></tr>
</table>

<p>Visit <a href="https://example.com">our website</a>.</p>
<p style="color: red">Red text via inline CSS.</p>`)
```

### Supported HTML tags

| Tag | Effect |
|-----|--------|
| `<h1>` - `<h6>` | Headings (sized from large to small) |
| `<p>` | Paragraph |
| `<b>`, `<strong>` | Bold |
| `<i>`, `<em>` | Italic |
| `<u>` | Underline |
| `<br>` | Line break |
| `<hr>` | Horizontal rule |
| `<ul>`, `<ol>`, `<li>` | Lists (bulleted and numbered) |
| `<table>`, `<tr>`, `<td>` | Tables |
| `<a href="...">` | Clickable hyperlink |

### Supported inline CSS

Apply via `style` attribute on any element:

- `color` — text color (`red`, `#ff0000`, `rgb(255,0,0)`)
- `font-size` — text size (`14px`, `12pt`)
- `text-align` — alignment (`left`, `center`, `right`)
- `background-color` — element background

## Markdown rendering

Render a Markdown subset:

```go
doc.SetFont("helvetica", "", 12)
page := doc.AddPage(folio.A4)

page.Markdown(`# Main Heading

A paragraph with **bold**, *italic*, and ` + "`inline code`" + `.

## Sub-heading

- Bullet one
- Bullet two
- Bullet three

1. Numbered one
2. Numbered two

---

Visit [GitHub](https://github.com) for more.
`)
```

### Supported Markdown syntax

| Syntax | Effect |
|--------|--------|
| `# Heading` | h1 (up to `######` for h6) |
| `**bold**` | Bold text |
| `*italic*` | Italic text |
| `` `code` `` | Inline code (monospace) |
| `- item` | Unordered list |
| `1. item` | Ordered list |
| `---` | Horizontal rule |
| `[text](url)` | Clickable link |

### Markdown options

```go
page.Markdown(md,
    folio.WithBookmarks(),     // auto-create PDF bookmarks from headings
    folio.WithLineHeight(5.5), // custom line height
)
```

## Fluent text builder

For quick styled text without changing document state:

```go
page.Text("Title").Bold().Size(24).Color(30, 60, 120).At(20, 20).Draw()
page.Text("Subtitle").Italic().Color(100, 100, 100).At(20, 35).Draw()
page.Text("Body text").Font("times", 12).At(20, 50).Draw()
```

Each builder call is self-contained — font and color are restored after `Draw()`.

### Builder methods

| Method | Effect |
|--------|--------|
| `.At(x, y)` | Position on page |
| `.Font(family, size)` | Font family and size |
| `.Size(pt)` | Font size only |
| `.Bold()` | Bold style |
| `.Italic()` | Italic style |
| `.Color(r, g, b)` | Text color (0-255) |
| `.Draw()` | Render to page |

Without `.At()`, text is drawn at the current cursor position using `Write`.

## Rich inline text

Mix formatting in a single `Write`-like call:

```go
page.RichText(6, "Normal <b>bold</b> and <i>italic</i> text")
```

## Links

### External URL

```go
page.LinkURL(20, 100, 50, 8, "https://example.com")
```

Creates a clickable rectangle that opens the URL.

### Internal anchor

```go
// On one page
page1.LinkAnchor(20, 100, 50, 8, "chapter2")

// On another page
// (anchor is set via TOC or bookmark system)
```
