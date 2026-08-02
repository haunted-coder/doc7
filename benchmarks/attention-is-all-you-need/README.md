# Attention Is All You Need Benchmark

Every system received the same raster-only, single-page PDF. The case checks
whether a converter can recover both document text and the directional
relationships inside Figure 2.

The input is derived from page 4 of *Attention Is All You Need*. Source,
modifications, hashes, attribution, and permission details are recorded in the
[source record](../../examples/attention-is-all-you-need/source.json).

## Result

Run date: `2026-07-30` on `darwin/arm64`.

| System | Bytes | Paper identity | Figure labels | Scaled flow | Multi-head flow | Equation | Scaling rationale | Footnote | Score |
| --- | ---: | --- | --- | --- | --- | --- | --- | --- | ---: |
| **#1 doc7 + qwen3.5-9b** | **3,501** | **yes** | **yes** | **yes** | **yes** | **yes** | **yes** | **yes** | **7/7** |
| MarkItDown 0.1.6 + OCR 0.1.0 + qwen3.5-9b | 9,580 | yes | yes | no | no | no | no | yes | 3/7 |
| Docling 2.113.0 standard | 282,603 | no | yes | no | no | no | no | no | 1/7 |
| MarkItDown 0.1.6 default | 0 | N/A | N/A | N/A | N/A | N/A | N/A | N/A | N/A |

MarkItDown OCR and doc7 used the same `qwen3.5-9b` model through the same local
OpenAI-compatible endpoint. Docling used its standard pipeline. Endpoint and
credential values are not part of the benchmark artifacts.

MarkItDown's default path produced an empty file for this raster-only input. It
is therefore shown as diagnostic `N/A`; the official OCR plugin is scored as a
separate configuration. The scorer removes only complete leading `<think>`
blocks before evaluating content while still verifying the SHA-256 of the raw
output.

The seven checks cover paper identity, Figure 2 labels, the ordered scaled
attention flow, the ordered and repeated multi-head flow, the displayed
equation, the scaling rationale, and the variance footnote. Exact patterns are
stored in [`ground-truth.json`](./ground-truth.json).

## Reproduce The Score

```bash
go run ./scripts/score_benchmark.go benchmarks/attention-is-all-you-need/ground-truth.json
```

Raw artifacts:

- [Input PDF](../../examples/attention-is-all-you-need/input.pdf)
- [MarkItDown default](./markitdown-default-output.md)
- [MarkItDown OCR](./markitdown-ocr-output.md)
- [Docling standard](./docling-standard-output.md)
- [doc7](./doc7-output.md)
- [Machine-readable result](./result.json)

This is a focused visual-understanding case, not a universal product ranking.
Output size is diagnostic only and does not imply quality.
