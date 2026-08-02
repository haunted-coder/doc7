# olmOCR-Bench adapter

doc7 includes a native runner for the public [olmOCR-Bench](https://github.com/allenai/olmocr/tree/main/olmocr/bench). This is the large-scale benchmark path for document quality: 1,403 single-page PDFs and 7,010 machine-checkable facts across formulas, tables, scans, headers and footers, multi-column reading order, and dense small text.

The benchmark is pinned in [`benchmark.json`](./benchmark.json). The PDFs and annotations are third-party data under the dataset's ODC-BY-1.0 terms; they are intentionally not vendored in doc7 or included in releases.

## Prepare the dataset

Use the exact dataset revision recorded in `benchmark.json`:

```bash
huggingface-cli download \
  --repo-type dataset \
  --revision 54a96a6fb6a2bd3b297e59869491db4d3625b711 \
  allenai/olmOCR-bench \
  --local-dir ./olmOCR-bench
```

The directory passed to doc7 must contain `pdfs/` and the category JSONL files.

## Convert with doc7

The runner uses the public Go API, preserves the upstream candidate filename contract, resumes non-empty outputs, and writes `run.json` plus `errors.jsonl` beside the candidate outputs.

```bash
go run ./scripts/olmocr-bench \
  --bench-dir ./olmOCR-bench/bench_data \
  --base-url http://127.0.0.1:1234/v1 \
  --model qwen3.5-4b \
  --parallel 1 \
  --timeout 10m
```

For a deterministic smoke conversion without model requests:

```bash
go run ./scripts/olmocr-bench \
  --bench-dir ./olmOCR-bench/bench_data \
  --categories headers_footers \
  --limit 2 \
  --dry-run
```

The default output directory is `./olmOCR-bench/bench_data/doc7`, which is the candidate layout expected by the upstream checker. `--text-grounding` is off by default so the initial run measures doc7's visual path; enable it explicitly when publishing a separate configuration.

## Score with the upstream checker

Install the benchmark dependencies from the pinned olmOCR commit, then run its own checker from the dataset directory:

```bash
git clone https://github.com/allenai/olmocr.git
cd olmocr
git checkout f7cfe4c22098b154c76b6ec950d1c0a464eecf8d
pip install -e '.[bench]'
python -m olmocr.bench.benchmark \
  --dir /path/to/olmOCR-bench/bench_data \
  --candidate doc7
```

Do not publish a full score from a limited `--limit` run. Report the upstream checker version, dataset revision, model ID, endpoint type, DPI, fallback settings, repeat count, and whether text grounding was enabled alongside any result.
