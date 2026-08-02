# Visual Report Demo

This self-authored page is a public demo input for `doc7`. It deliberately combines a chart, table, formula, workflow, and screenshot-like UI state in one document.

Start with the [before/after showcase](./showcase.png), then compare the [raw PDF](./input.pdf) with the [Markdown output](./output-pdf.md).

Create the raster input:

```bash
rsvg-convert -o input.png source.svg
```

Process it with any configured OpenAI-compatible vision model:

```bash
doc7 read input.png -o output
```

See [`output.md`](./output.md) for one real model result. It preserves the visible text and table, writes the formula as LaTeX, and describes the chart, workflow, and UI state with `[Visual]` blocks. Results vary with the configured model.

`input.pdf` contains the same page exported as PDF. [`output-pdf.md`](./output-pdf.md) shows the result after the PDF renderer path; its key values, table rows, formula, and visual relationships remain consistent with the PNG result.
