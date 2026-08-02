# One Page, Three Document Containers

The same self-authored visual report is packaged as a raster-only PDF, a DOCX,
and a PPTX. Each input contains the same KPI cards, chart, table, formula,
workflow, and application error state.

All three conversions use the same `qwen3.5-4b` model and the same faithful
visual prompt. The result is scored with the eight content-presence rules from
the [visual report benchmark](../../benchmarks/visual-report/README.md):

| Container | Input | Output | Score |
| --- | --- | --- | ---: |
| PDF | [`../visual-report/input.pdf`](../visual-report/input.pdf) | [`output-pdf.md`](./output-pdf.md) | 8/8 |
| DOCX | [`input.docx`](./input.docx) | [`output-docx.md`](./output-docx.md) | 8/8 |
| PPTX | [`input.pptx`](./input.pptx) | [`output-pptx.md`](./output-pptx.md) | 8/8 |

This demonstrates format coverage, not a universal quality guarantee. Results
depend on the selected model and prompt. The exact prompt and artifact hashes
are recorded in [`manifest.json`](./manifest.json).

## Reproduce

Start an OpenAI-compatible multimodal endpoint, then run:

```bash
MODEL_ARGS=(
  --base-url http://127.0.0.1:1234/v1
  --model qwen3.5-4b
  --credential-store env
)

doc7 read ../visual-report/input.pdf -o output-pdf \
  "${MODEL_ARGS[@]}" \
  --prompt-file faithful-visual-prompt.txt

doc7 read input.docx -o output-docx \
  "${MODEL_ARGS[@]}" \
  --prompt-file faithful-visual-prompt.txt

doc7 read input.pptx -o output-pptx \
  "${MODEL_ARGS[@]}" \
  --prompt-file faithful-visual-prompt.txt
```

The PDF source is also used by the public competitor benchmark. The DOCX and
PPTX fixtures are generated from the same self-authored image, with no private
or third-party document content.
