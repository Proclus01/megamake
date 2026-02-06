# megamake

Megamake is a structured prompt generator — a “prompt compiler” — as well as a workflow orchestration tool for LLM agents to make use of Megamake’s various tools.

At a high level, Megamake helps you:

- **Extract real source code into a stable `<context>` blob** you can paste into an LLM (or feed to an agent).
- **Generate machine- and human-consumable artifacts** (pseudo-XML + JSON + agent prompt) in a deterministic, audit-friendly format.
- **Orchestrate multi-tool workflows** for documentation, diagnostics, test planning, and chat.
- **Run with explicit network policy controls** (deny-by-default unless `--net` is enabled, optional allowlist via `--allow-domain`).

---

## Features / Commands

Megamake currently provides these commands:

- `prompt` — scan a repo and emit a `<context>` blob (pseudo-XML) containing source files.
- `doc create` — generate a documentation report from a local repo: directory tree, import graph, external dependency summary, optional UML.
- `doc get` — fetch documentation from local paths or HTTP(S) with explicit network policy gating, optional crawl depth.
- `diagnose` — run best-effort diagnostics using local toolchains (Go, Rust, JS/TS, Python, Swift, Java, Lean) and generate a fix-oriented prompt.
- `test` — build a structured test plan (subjects + scenario suggestions + rough coverage signals).
- `chat` — a local-first chat orchestration tool (CLI + server + UI) that:
  - manages chat “runs” stored in the artifact directory
  - streams assistant output to disk (`assistant_turn_###.partial.txt`)
  - persists full transcripts (`transcript.jsonl`) and per-turn metrics (`turn_###.json`)
  - supports provider verification, model listing (with server-side caching), and per-run settings overrides (Playground-like)

Run `--help` to see full usage.

---

## Requirements

- **Go 1.22+** (the module uses `go 1.22` in `go.mod`)

Optional (only needed for certain commands / languages):

- Go toolchain (`go`) for Go diagnostics
- Node + `npx` / `tsc` / `eslint` for JS/TS diagnostics
- Python (`python3` or `python`) for Python diagnostics
- Rust (`cargo`) for Rust diagnostics
- Swift (`swift`) for Swift diagnostics
- Java (`mvn` or `gradle`) for Java diagnostics
- Lean (`lake`) for Lean diagnostics

Megamake will skip tools that are not installed and record warnings in output artifacts.

---

## Install / Build

Megamake’s CLI entrypoint is:

- `./cmd/megamake`

### Build (macOS / Linux)

From the repository root (the directory containing `go.mod`):

```sh
go build -o megamake ./cmd/megamake
```

Run it:

```sh
./megamake --help
```

### Build (Windows PowerShell)

```powershell
go build -o megamake.exe .\cmd\megamake
.\megamake.exe --help
```

### Quick run without producing a binary

This is useful while iterating:

```sh
go run ./cmd/megamake --help
```

---

## Compile / Verify Everything Builds

The simplest “compile the whole module” command is:

```sh
go test ./...
```

Even if you have no tests, `go test ./...` compiles all packages and fails fast if anything is broken.

---

## Formatting / Linting (Optional)

Format the repository:

```sh
gofmt -w .
```

If you use `golangci-lint`:

```sh
golangci-lint run ./...
```

(If you don’t have it installed, you can skip this.)

---

## Usage

### Global flags

- `--artifact-dir <dir>`  
  Directory where Megamake artifacts and pointer files are written.
- `--net`  
  Enable network access (deny-by-default otherwise).
- `--allow-domain <domain>` (repeatable)  
  Allowed domain when `--net` is set. If none provided, all domains are allowed when `--net` is set.

Examples:

```sh
./megamake --artifact-dir ./artifacts prompt .
./megamake --net --allow-domain example.com doc get https://example.com
```

---

## Chat (CLI + Server + UI)

Megamake Chat is stored under your artifact directory:

- Runs live at: `<artifactDir>/MEGACHAT/runs/<run_name>/`
- Default dotenv path (for provider keys): `<artifactDir>/MEGACHAT/.env`

### Create/list/get runs (CLI)

```sh
./megamake --artifact-dir ./artifacts chat new --title "My Conversation"
./megamake --artifact-dir ./artifacts chat list
./megamake --artifact-dir ./artifacts chat get --run <run_name>
```

### Providers: verify + list models (CLI)

These commands respect global network policy:

- Network providers require `--net`
- If you set `--allow-domain`, provider API hosts must be allowlisted

Examples:

```sh
# Stub provider (no network)
./megamake --artifact-dir ./artifacts chat verify --provider stub --json=false
./megamake --artifact-dir ./artifacts chat models --provider stub --json=false

# OpenAI (requires --net and OPENAI_API_KEY)
./megamake --artifact-dir ./artifacts --net chat verify --provider openai
./megamake --artifact-dir ./artifacts --net chat models --provider openai --limit 50 --json=false
```

Model listing supports server-side caching controls:

```sh
./megamake --artifact-dir ./artifacts --net chat models --provider openai --no-cache
./megamake --artifact-dir ./artifacts --net chat models --provider openai --cache-ttl-seconds 60
```

### Start the chat server + UI

```sh
./megamake --artifact-dir ./artifacts --net chat serve --listen 127.0.0.1:8082
```

Open:

- UI: `http://127.0.0.1:8082/ui`
- Health: `http://127.0.0.1:8082/health`

The UI provides:
- run list + transcript view
- streaming preview (jobs + tail polling)
- Stop (server-side cancel)
- provider model fetch (cached, refresh bypasses cache)
- per-run settings overrides (Playground-like):
  - system/developer
  - model/provider override (optional)
  - textFormat, verbosity, effort, summaryAuto, maxOutputTokens
  - tool toggles

### `.env` for provider keys

Create:

- `<artifactDir>/MEGACHAT/.env`

Example:

```env
OPENAI_API_KEY=sk-...
# ANTHROPIC_API_KEY=...
# GEMINI_API_KEY=...
# GROK_API_KEY=...
# OLLAMA_BASE_URL=http://localhost:11434
```

You can override the dotenv path when serving:

```sh
./megamake --artifact-dir ./artifacts --net chat serve --env-file /path/to/.env
```

### Chat artifacts on disk

A run folder contains:

- `args.json` — creation snapshot
- `meta.json` — updated over time
- `transcript.jsonl` — append-only transcript events
- `user_turn_001.txt`
- `assistant_turn_001.partial.txt` (during streaming)
- `assistant_turn_001.txt` (final)
- `turn_001.json` — metrics, tokens (best-effort), durations, and effective settings snapshot

---

## `prompt`

Generate a `<context>` blob from a project folder:

```sh
./megamake prompt .
```

Common options:

- `--ignore <name-or-glob>` (repeatable), `-I` alias
- `--max-file-bytes N`
- `--force` (bypass “safety stop” if the directory doesn’t look like a code project)
- `--copy` (best-effort copy of `<context>` to clipboard)
- `--json-out PATH`, `--prompt-out PATH`

Example:

```sh
./megamake prompt . \
  --ignore node_modules \
  --ignore build \
  --max-file-bytes 1500000 \
  --copy
```

---

## `doc create`

Generate documentation from a local repo:

```sh
./megamake doc create .
```

Notable flags:

- `--uml ascii,plantuml|ascii|plantuml|none`
- `--uml-granularity file|module|package`
- `--tree-depth N`
- `--ignore ...`
- `--force`

---

## `doc get`

Fetch docs from local paths and/or HTTP(S). HTTP(S) requires `--net`.

```sh
./megamake doc get ./README.md
./megamake --net doc get https://example.com
./megamake --net --allow-domain example.com doc get https://example.com/docs
./megamake --net --allow-domain example.com doc get --crawl-depth 2 https://example.com/docs
```

---

## `diagnose`

Run diagnostics for detected languages (best-effort, based on what toolchains exist locally):

```sh
./megamake diagnose .
```

Useful flags:

- `--include-tests` (compile/analyze tests too; does not run them)
- `--timeout-seconds N`
- `--ignore ...`
- `--force`

---

## `test`

Generate a structured test plan:

```sh
./megamake test .
```

Useful flags:

- `--levels smoke,unit,integration,e2e,regression` (default: all)
- `--limit-subjects N`
- `--max-analyze-bytes N`
- `--regression-since REF`
- `--regression-range A..B`
- `--no-regression`

---

## Artifacts (non-chat)

Megamake writes tool artifacts as text files with a unified “artifact envelope”:

- `MEGAPROMPT_YYYYMMDD_HHMMSSZ.txt`
- `MEGADOC_YYYYMMDD_HHMMSSZ.txt`
- `MEGADIAG_YYYYMMDD_HHMMSSZ.txt`
- `MEGATEST_YYYYMMDD_HHMMSSZ.txt`

And a “latest pointer” file per tool:

- `MEGAPROMPT_latest.txt`
- `MEGADOC_latest.txt`
- `MEGADIAG_latest.txt`
- `MEGATEST_latest.txt`

---

## Cross-platform notes

- Megamake avoids symlinks for “latest” pointers and uses plain files instead for portability.
- Clipboard support is best-effort and platform-dependent:
  - macOS: `pbcopy`
  - Linux Wayland: `wl-copy`
  - Linux X11: `xclip` or `xsel`
  - Windows: `clip`

---

## Troubleshooting

### `--help` prints an error / exits non-zero
Megamake should treat `--help` as a successful exit. If you see “flag: help requested” as an error again, ensure you special-case `flag.ErrHelp` and return exit code 0.

### Toolchain missing warnings
`diagnose` is best-effort; it will skip missing tools and record warnings in output artifacts.

### Chat: provider verify/models fail with “network not enabled”
Start server/CLI with `--net`, and if you use `--allow-domain`, ensure the provider host is allowlisted (e.g. `api.openai.com`).

### Chat: provider verify fails with “missing API key”
Ensure you have a dotenv file at `<artifactDir>/MEGACHAT/.env` (or use `--env-file`) and restart the server so it loads the updated environment.

---

## License

(TODO: add license information)