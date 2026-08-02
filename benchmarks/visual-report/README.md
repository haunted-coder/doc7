# Visual Report Benchmark

Every system received the same raster-only, self-authored product report. The
page combines document text, KPI cards, a chart, a table, a displayed formula,
a workflow diagram, and an application status panel.

## Result

Run date: `2026-07-29` on `darwin/arm64`.

| System | Bytes | Document text | KPI cards | Chart | Table | Formula | LaTeX | Workflow | UI state | Score |
| --- | ---: | --- | --- | --- | --- | --- | --- | --- | --- | ---: |
| **#1 doc7 + qwen3.5-9b** | **1,792** | **yes** | **yes** | **yes** | **yes** | **yes** | **yes** | **yes** | **yes** | **8/8** |
| MarkItDown 0.1.6 + OCR plugin 0.1.0 + qwen3.5-9b | 3,562 | yes | yes | no | yes | yes | no | yes | yes | 6/8 |
| Docling 2.113.0 standard | 2,288,842 | yes | no | no | yes | no | no | no | no | 2/8 |
| MarkItDown 0.1.6 default | 0 | N/A | N/A | N/A | N/A | N/A | N/A | N/A | N/A | N/A |

MarkItDown OCR and doc7 used the same `qwen3.5-9b` model through the same local
OpenAI-compatible endpoint. Docling used its standard pipeline. Endpoint and
credential values are not part of the benchmark artifacts.

MarkItDown's default path produced an empty file for this raster-only input. It
is shown as diagnostic `N/A`, while the official OCR plugin is scored as a
separate configuration. Docling's output includes Base64-embedded page images,
which explains its raw size; byte count is not used as a quality score.

The eight checks cover document identity, all KPI values, chart values and
trend, table rows, formula semantics, display LaTeX, workflow order, and the UI
state. Exact patterns are stored in [`ground-truth.json`](./ground-truth.json).

## Reproduce The Score

```bash
go run ./scripts/score_benchmark.go benchmarks/visual-report/ground-truth.json
```

Raw artifacts:

- [Input PDF](../../examples/visual-report/input.pdf)
- [MarkItDown default](./markitdown-default-output.md)
- [MarkItDown OCR](./markitdown-ocr-output.md)
- [Docling standard](./docling-standard-output.md)
- [doc7](./doc7-output.md)
- [Machine-readable result](./result.json)

This is a focused visual-document case, not a universal product ranking. Model
dependent results can vary, so the raw outputs and scoring rules remain
available for inspection.
