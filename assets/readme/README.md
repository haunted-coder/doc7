# README Visual Assets

The README artwork follows the canonical brand references and generation rules
in `assets/brand/README.md`.

## Hero

- Generated: 2026-07-29
- Model: `gpt-image-2`
- Quality: `medium`
- Size: `2560x1440`
- References:
  - `dist/showcase/brand-posters-v2-independent-low/05-open-source-pop.webp`

The current English and Chinese Hero images are approved direct-generation
assets stored in `assets/readme/hero/`.

Prompt summary: create a landscape open-source campaign poster with white
paper, cobalt blue, signal red, green terminal accents, dense documents and
formulas transforming into structured Markdown; reserve clean space for the
real wordmark; do not invent logos, scores, or readable text.

## Supporting Visuals

### Paper benchmark

- Updated: 2026-07-30

The left side uses the checked, raster-only benchmark input. The right side
typesets the exact, continuous lines 3-34 from the checked `output.md` directly
on the poster. It does not use a terminal screenshot, recreate Markdown text,
or add a benchmark score.

### Benchmark poster

- Approved assets:
  - `assets/readme/benchmark/benchmark.webp`
  - `assets/readme/benchmark/benchmark.zh-CN.webp`
- It uses the committed benchmark data: `15/15`, `9/15`, `3/15`, `N/A`, and the
  recorded raw Markdown sizes.

### Format overview

- Generated: 2026-07-29
- Model: `gpt-image-2`
- Quality: `medium`
- Size: `2560x1440`

This legacy asset remains in use. Future replacements must be generated as
complete images from the canonical brand references and approved before they
enter the repository.

Prompt summary: create a high-impact open-source editorial poster showing many
real document formats converging through one visual understanding system; use
white paper, cobalt blue, signal red, green accents, diagrams, formulas, tables,
scans, images, and structured Markdown; do not invent logos or readable text.

### CLI

- Updated: 2026-07-30

The CLI posters use the real command names and the same six-row colored
character logo printed by `doc7 --help`. The `DOC` segment is cobalt blue and
the final `7` is signal red; regular command execution keeps the compact
single-line wordmark. Each row of the poster's `7` uses an explicit horizontal
position because image text renderers may discard leading spaces.

All final assets are compressed WebP files tracked through Git LFS.
