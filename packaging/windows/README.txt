doc7 for Windows
================

1. Start LM Studio or Ollama with a local vision model. Then run the universal
   entrypoint directly:

   doc7.exe report.pdf
   doc7.exe screenshot.png

   doc7 automatically detects the system language, discovers local model
   servers, selects a real model ID, and saves the choice on this machine.

   If several models are running, doc7 asks which one to use. No API key is
   required for an unauthenticated local endpoint.

   Chat directly with the configured local model, or start an interactive session:

   doc7.exe chat "Hello, introduce yourself"
   doc7.exe chat

   Chat can also discover local LM Studio/Ollama models, verify image understanding,
   and guide configuration in natural language. Configuration changes are previewed
   first and require an interactive ask_user confirmation. When an endpoint needs an
   API key, Chat opens a hidden local prompt; the model can see the entered length but
   never the key content. For non-interactive setup, use doc7.exe setup config
   --api-key-stdin. For an explicit one-shot authorization of a non-secret change,
   use doc7.exe --yes chat "...".

   When the user explicitly provides a document, models with OpenAI-compatible
   Tool Calling can invoke doc7's restricted conversion tool:

   doc7.exe chat "Turn report.pdf into knowledge-base Markdown"

   The agent can search authorized directories with structured read-only commands
   such as ls, find, file, stat, wc, and realpath when the user gives a vague filename.
   It never executes arbitrary shell strings or write commands. head and tail require
   confirmation because their limited text preview is visible to the model. Use
   doc7.exe <file> for direct conversion when the selected model does not support
   Tool Calling.

   To inspect the setup and find the file to edit, run:

   doc7.exe config
   doc7.exe config path
   doc7.exe config set

   To update an installed CLI, run:

   doc7.exe update --check
   doc7.exe update

   The updater downloads the matching Windows package, verifies checksums, and
   replaces the executable after it exits. The install directory must be writable.

2. To configure a model manually, first list the model IDs exposed by your
   OpenAI-compatible server:

   doc7.exe models --base-url "http://127.0.0.1:1234/v1"

   For LM Studio, start the server in the Developer page and enable Serve on
   Local Network when doc7 runs on another machine. Replace the URL above with
   the reachable address and port.

3. Configure one returned model ID:

   doc7.exe setup config --base-url "http://127.0.0.1:1234/v1" --model "MODEL_ID"

   Authenticated endpoints can add --api-key "API_KEY". To use an existing
   environment variable, pass --api-key-env explicitly, for example
   --api-key-env OPENAI_API_KEY. doc7 never scans provider-specific variables
   automatically. Endpoints without authentication need no placeholder key;
   doc7 omits the Authorization header.

4. Check the current machine:

   doc7.exe doctor

5. Drag a document onto run-file.bat, drag a directory onto run-folder.bat,
   or run doc7.exe read from Command Prompt or PowerShell.

   Reprocess selected source pages when a page needs another model or settings:

   doc7.exe read "C:\path\report.pdf" -o "C:\doc7\report-pages-5-7" --pages 5,7

   Page numbers are 1-based and inclusive. The output manifest records the
   source page count and selected range, and keeps files such as page_005.md.
   Artifact paths in the manifest are relative to the output directory, so the
   result can be moved after conversion.

   Retry only failed pages in an existing output directory:

   doc7.exe read "C:\path\report.pdf" -o "C:\doc7\report-doc7" --resume
   doc7.exe read "C:\path\report.pdf" -o "C:\doc7\report-doc7" --resume --pages 5,7

   Without --pages, doc7 selects every failed page from manifest.json. Explicit
   pages must be failed pages only, and the input SHA-256 must match. Successful
   page Markdown is preserved. Previous manifests are kept under history\.

   Command Prompt can also pipe a document through stdin when its filename and
   extension are supplied explicitly:

   type report.pdf | doc7.exe read - --stdin-name report.pdf --stdout > report.md

6. Run the local HTTP job service when another application needs an API:

   doc7.exe serve --addr 127.0.0.1:8787 --data-dir "C:\doc7-server"

   You can also double-click run-server.bat to use the default local address
   and keep job data in the doc7-server directory beside doc7.exe.

   Submit a document from PowerShell:

   curl.exe -F "file=@report.pdf" http://127.0.0.1:8787/v1/jobs

   Submit only selected pages with an additional multipart field:

   curl.exe -F "file=@report.pdf" -F "pages=5,7" http://127.0.0.1:8787/v1/jobs

   Poll the returned job URL, then download /markdown or /artifacts. To bind
   beyond localhost, set DOC7_SERVER_TOKEN and pass the bearer token in the
   Authorization header. The service accepts documents and ZIP archives.

   Retry failed pages in the same job:

   curl.exe -X POST -H "Content-Type: application/json" -d "{}" http://127.0.0.1:8787/v1/jobs/JOB_ID/resume
   curl.exe -X POST -H "Content-Type: application/json" -d "{\"pages\":\"5,7\"}" http://127.0.0.1:8787/v1/jobs/JOB_ID/resume

   Resume uses the server's current model. Stop the service, change DOC7_MODEL,
   and restart it with the same --data-dir to retry with another model. Invalid
   page selections return HTTP 409 without changing the existing job.

7. Configure doc7 as an MCP server in an AI client by using:

   doc7.exe mcp

   The MCP client must launch doc7.exe over stdio. Configure DOC7_BASE_URL and
   DOC7_MODEL in the client environment, or pass --config before mcp.

The regular Windows release contains doc7.exe and the launcher files, but not
third-party renderers. Office documents require LibreOffice, and PDF rendering
requires MuPDF or another renderer reported by doc7.exe doctor. A portable kit
places these tools beside doc7.exe under the tools directory, so it can run on
a machine without winget or system package installation.

Run ensure-tools.bat, optionally passing the document path, to check the exact
dependencies required by that input:

  ensure-tools.bat C:\path\report.docx

Portable kits use a short root such as C:\doc7 to reduce Windows path-length
problems. The model endpoint and model ID are always configured by the user;
this package does not contain a private endpoint.

Read THIRD-PARTY-NOTICES.txt before redistributing a portable kit. LibreOffice
and MuPDF keep their own licenses, which are separate from doc7.

EPUB, EML, MHTML/MHT, Outlook MSG, and Jupyter Notebook files also require
Chrome or Edge together with the PDF renderer.

EML, MHTML/MHT, and Outlook MSG are parsed natively; Microsoft Outlook is not
required.

Markdown, TXT, CSV, TSV, JSON, XML, and YAML files are converted locally and do
not require a model endpoint. BMP and TIFF images are normalized to PNG pages
before they are sent to the configured vision endpoint; multi-page TIFF files
become multiple document pages.

Model output is limited to 8192 tokens per page by default. Use --max-tokens
to raise or lower the limit. If the provider rejects a request because the
prompt and image exceed its context window, or stops early for the same reason,
doc7 automatically retries with a lower-resolution request image. The default
allows two fallbacks down to a 720-pixel longest side. Configure this with
--context-fallbacks and --min-image-dimension, or the matching DOC7_* variables.
The original rendered page remains unchanged. Pages only fail after the output
limit is reached or all fallbacks are exhausted; truncated Markdown is never
silently accepted. Page metadata records the request dimension and fallback
count. Increasing --max-tokens alone cannot exceed the provider context window.

Use --keep-images=false when only Markdown and metadata are needed. The
completed output will not contain the rendered images, while later runs can
still reuse the page cache.

Use --text-grounding for PDF or Office pages with an embedded text layer when
exact numbers and identifiers need an additional check. It does not run OCR.
Unresolved exact tokens remain visible as grounding_warnings in the summary
and page metadata instead of being silently marked as corrected.
