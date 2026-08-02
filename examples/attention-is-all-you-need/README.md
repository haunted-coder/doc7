# Attention Is All You Need Example

This example uses a raster-only, single-page PDF derived from page 4 of
*Attention Is All You Need* (Vaswani et al., NeurIPS 2017). The page combines
Figure 2, dense two-column text, mathematical notation, a display equation, and
a technical footnote.

[View the input](./input.webp) and compare it with the [doc7 Markdown](./output.md).

Run the same input with any configured OpenAI-compatible vision model:

```bash
doc7 read input.pdf -o output --workers 1 --retry 0 --max-tokens 8192
```

The public example records the provider type, model identifier, parameters,
hashes, and outputs. It never records the private model endpoint or credentials.

## Source And Use

- Paper: *Attention Is All You Need*
- Authors: Ashish Vaswani et al.
- Venue: NeurIPS 2017
- arXiv: `1706.03762`, version 7 dated August 2, 2023
- Source: `https://arxiv.org/abs/1706.03762`

The paper states: "Provided proper attribution is provided, Google hereby grants
permission to reproduce the tables and figures in this paper solely for use in
journalistic or scholarly works."

This page composition is included for attributed scholarly evaluation and is
not covered by doc7's MIT license. The full paper is not redistributed. See
[`source.json`](./source.json) for the exact source and derived-asset record.
