# megamake

Megamake is a structured prompt generator — a “prompt compiler” — as well as a workflow orchestration tool for LLM agents to make use of Megamake’s various tools.

At a high level, Megamake helps you:

- **Extract real source code into a stable `<context>` blob** you can paste into an LLM (or feed to an agent).
- **Generate machine- and human-consumable artifacts** (pseudo-XML + JSON + agent prompt) in a deterministic, audit-friendly format.
- **Orchestrate multi-tool workflows** for documentation, diagnostics, test planning, and chat.
- **Run with explicit network policy controls** (deny-by-default unless `--net` is enabled, optional allowlist via `--allow-domain`).

---

## Repository layout (local-first)

This repo is intentionally **local-directory-first**:

- The Go module and source live in: `./megamake/`
- The built binary is expected at: `./megamake/megamake` (in the module root)
- **Chat artifacts** (runs, `.env`, etc.) are expected under: `./megamake/artifacts/MEGACHAT/`

Example (what you should see after setup):

```text
megamake/
  LICENSE
  README.md
  megamake/
    megamake                 <-- compiled binary (this is where it goes)
    go.mod
    cmd/
    internal/
    artifacts/
      MEGACHAT/
        .env
        runs/
      MEGACHAT_latest.txt
```

No `~/.local/bin`, no global install, no `./bin/` directory required.

---

## Requirements

- **Go 1.22+**

Optional toolchains (used by some commands like `diagnose`):
- Node / npx / tsc / eslint (JS/TS)
- Python
- Rust / cargo
- Swift
- Java / mvn / gradle
- Lean / lake

Megamake will skip tools that aren’t installed and record warnings in output artifacts.

---

## Build (local directory install)

### macOS / Linux

From the repository root:

```sh
cd megamake
go test ./...
go build -o ./megamake ./cmd/megamake
./megamake --help
```

### Windows (PowerShell)

From the repository root:

```powershell
cd megamake
go test ./...
go build -o .\megamake.exe .\cmd\megamake
.\megamake.exe --help
```

---

## Where artifacts are written (important)

Megamake has **two artifact behaviors**:

### A) Local tools (prompt/doc/diagnose/test) — write to where you invoke the command

These commands write artifacts to the **directory you run them from** (“invocation directory”):

- `prompt`
- `doc create`
- `doc get`
- `diagnose`
- `test`

Example: if you run `megamake prompt .` from `/projects/MyApp`, your artifact files are written into `/projects/MyApp/`:

- `MEGAPROMPT_YYYYMMDD_HHMMSSZ.txt`
- `MEGAPROMPT_latest.txt`

Notes:
- These commands **ignore** the global `--artifact-dir` flag by design.
- They also automatically ignore local artifacts folders (if present in the project being scanned):
  - `megamake/artifacts/**`
  - `artifacts/**`

This prevents recursive inclusion of generated state.

### B) Chat — uses `--artifact-dir` (repo-local recommended)

Chat stores runs, settings, and `.env` under the configured artifact directory.

Recommended (repo-local):

- `./megamake/artifacts/MEGACHAT/...`

---

## Run (recommended workflow)

### 1) Prompt (from the project you want to scan)

From your target project directory:

```sh
megamake prompt .
```

This writes:
- `MEGAPROMPT_*.txt` and `MEGAPROMPT_latest.txt` into the **current directory**.

Flag ordering:
- Flags can appear **before or after** the path:

```sh
megamake prompt . --ignore build
megamake prompt --ignore build .
```

#### zsh glob note (important)
If you pass glob patterns to `--ignore`, **quote them**:

```sh
megamake prompt . --ignore 'docs/generated/**'
```

Unquoted globs may be expanded by zsh before Megamake runs, or cause `zsh: no matches found`.

---

### 2) Documentation

#### `doc create` (local repo analysis)

From the target project directory:

```sh
megamake doc create .
```

Flags can appear before or after the path:

```sh
megamake doc create . --uml ascii,plantuml --tree-depth 6
megamake doc create --uml ascii,plantuml --tree-depth 6 .
```

#### `doc get` (fetch mode)

From anywhere:

```sh
megamake doc get ./README.md
```

HTTP(S) requires `--net`:

```sh
megamake --net doc get https://example.com
megamake --net --allow-domain example.com doc get https://example.com/docs
megamake --net --allow-domain example.com doc get --crawl-depth 2 https://example.com/docs
```

Flag ordering:
- `doc get` flags can appear **before or after** URIs:

```sh
megamake doc get https://example.com/docs --crawl-depth 2
megamake doc get --crawl-depth 2 https://example.com/docs
```

---

### 3) Diagnose

From the target project directory:

```sh
megamake diagnose .
```

---

### 4) Test plan

From the target project directory:

```sh
megamake test .
```

---

## Convenience wrapper (recommended for working from ANY directory)

Many developers keep the Megamake source repo checked out in one place, but want to run:

```sh
megamake prompt .
```

from **any** project directory.

If you use a wrapper that `cd`s into the Megamake repo before running the tool, you must preserve the “caller directory” so `.` still means “the directory you invoked from”.

Megamake supports this via:

- `MEGAMAKE_CALLER_PWD` (set by your wrapper)

---

### macOS/Linux (zsh): add to `~/.zshrc`

1) Build Megamake (once):

```sh
cd /path/to/your/checkout/megamake/megamake
go build -o ./megamake ./cmd/megamake
```

2) Add this block to `~/.zshrc` (replace the placeholder path):

```sh
# --- megamake (repo-local wrapper; runs from any directory) ---
export MEGAMAKE_HOME="/ABSOLUTE/PATH/TO/your/checkout/megamake/megamake"

megamake() {
  local caller="$PWD"

  (
    export MEGAMAKE_CALLER_PWD="$caller"
    cd "$MEGAMAKE_HOME" || exit 1

    if [[ ! -x ./megamake ]]; then
      echo "megamake: binary not found or not executable at: $MEGAMAKE_HOME/megamake" >&2
      echo "megamake: build it with: (cd \"$MEGAMAKE_HOME\" && go build -o ./megamake ./cmd/megamake)" >&2
      exit 1
    fi

    # Chat should use repo-local artifacts. Local tools ignore --artifact-dir.
    if [[ "$1" == "chat" ]]; then
      ./megamake --artifact-dir ./artifacts "$@"
    else
      ./megamake "$@"
    fi
  )
}
# --- /megamake ---
```

3) Reload:

```sh
source ~/.zshrc
```

4) Test (from ANY project directory):

```sh
megamake --help
megamake prompt .
megamake doc create .
megamake diagnose .
megamake test .
```

And for chat (stores runs under `MEGAMAKE_HOME/artifacts/MEGACHAT/...`):

```sh
megamake --net chat serve --listen 127.0.0.1:8082
```

---

### Windows (PowerShell): add to your PowerShell profile

1) Find your PowerShell profile path:

```powershell
$PROFILE
```

2) Edit it (creates it if missing):

```powershell
if (!(Test-Path $PROFILE)) { New-Item -Force -ItemType File $PROFILE | Out-Null }
notepad $PROFILE
```

3) Add this block (replace the placeholder path):

```powershell
# --- megamake (repo-local wrapper; runs from any directory) ---
$env:MEGAMAKE_HOME = "C:\ABSOLUTE\PATH\TO\your\checkout\megamake\megamake"

function megamake {
  param(
    [Parameter(ValueFromRemainingArguments=$true)]
    [string[]] $Args
  )

  $caller = (Get-Location).Path

  # Run in a child scope so we can safely cd
  & {
    $env:MEGAMAKE_CALLER_PWD = $caller

    Set-Location $env:MEGAMAKE_HOME

    $exe = Join-Path $env:MEGAMAKE_HOME "megamake.exe"
    if (!(Test-Path $exe)) {
      Write-Error "megamake: binary not found at: $exe"
      Write-Error "megamake: build it with: (cd `"$env:MEGAMAKE_HOME`"; go build -o .\megamake.exe .\cmd\megamake)"
      exit 1
    }

    if ($Args.Length -ge 1 -and $Args[0] -eq "chat") {
      & $exe --artifact-dir ".\artifacts" @Args
    } else {
      & $exe @Args
    }
  }
}
# --- /megamake ---
```

4) Reload:

```powershell
. $PROFILE
```

5) Test (from any directory):

```powershell
megamake --help
megamake prompt .
megamake doc create .
```

---

## Network policy (important)

Megamake is **deny-by-default** for network access.

Global flags:
- `--net` enables network access
- `--allow-domain <domain>` restricts allowed network domains (repeatable)

Example:

```sh
megamake --net --allow-domain api.openai.com chat serve --listen 127.0.0.1:8082
```

---

## Chat (CLI + Server + UI)

Chat is stored under your chat artifact directory (recommended: repo-local `./megamake/artifacts/`):

- Runs: `./megamake/artifacts/MEGACHAT/runs/<run_name>/`
- Dotenv (provider keys): `./megamake/artifacts/MEGACHAT/.env`

### Create `.env` for providers

Create/edit:

- `./megamake/artifacts/MEGACHAT/.env`

Example:

```env
OPENAI_API_KEY=sk-...
# ANTHROPIC_API_KEY=...
# GEMINI_API_KEY=...
# GROK_API_KEY=...
# OLLAMA_BASE_URL=http://localhost:11434
```

If you change `.env`, **restart** the server so it reloads env vars.

### Start the chat server + UI

From the module directory (`./megamake/` in this repo):

```sh
./megamake --artifact-dir ./artifacts --net chat serve --listen 127.0.0.1:8082
```

Open:

- UI: `http://127.0.0.1:8082/ui`
- Health: `http://127.0.0.1:8082/health`

### Chat CLI basics

```sh
./megamake --artifact-dir ./artifacts chat new --title "My Conversation"
./megamake --artifact-dir ./artifacts chat list
./megamake --artifact-dir ./artifacts chat get --run <run_name>
```

### Providers: verify + list models (CLI)

These respect network policy:

```sh
# Stub provider (no network)
./megamake --artifact-dir ./artifacts chat verify --provider stub --json=false
./megamake --artifact-dir ./artifacts chat models --provider stub --json=false

# OpenAI (requires --net and OPENAI_API_KEY)
./megamake --artifact-dir ./artifacts --net chat verify --provider openai
./megamake --artifact-dir ./artifacts --net chat models --provider openai --limit 50 --json=false
```

Model listing supports cache controls:

```sh
./megamake --artifact-dir ./artifacts --net chat models --provider openai --no-cache
./megamake --artifact-dir ./artifacts --net chat models --provider openai --cache-ttl-seconds 60
```

### Chat artifacts on disk

A run folder contains:

- `args.json` — creation snapshot
- `meta.json` — updated over time
- `transcript.jsonl` — append-only transcript events
- `user_turn_001.txt`
- `assistant_turn_001.partial.txt` (during streaming)
- `assistant_turn_001.txt` (final)
- `turn_001.json` — per-turn metrics (durations/tokens) + effective settings snapshot

---

## Troubleshooting

### zsh: `no matches found: artifacts/**`
Quote ignore globs:

```sh
megamake prompt . --ignore 'artifacts/**'
```

Or avoid globs entirely:

```sh
megamake prompt . --ignore artifacts
```

### Flags after path don’t work
Megamake supports flags after the path for:
- `prompt`, `doc create`, `diagnose`, `test`

…and flags before/after URIs for:
- `doc get`

If you still see issues, you may be running an old binary. Run:

```sh
megamake --help
```

and rebuild if needed.

### Chat: provider verify/models fail with “network not enabled”
Start the server/CLI with `--net`, and if you use `--allow-domain`, ensure the provider host is allowlisted (e.g. `api.openai.com`).

### Chat: provider verify fails with “missing API key”
Ensure `./megamake/artifacts/MEGACHAT/.env` contains the key (e.g. `OPENAI_API_KEY=...`) and restart the server so it reloads environment variables.

---

## License

Megamake is licensed under the **GNU Affero General Public License v3.0 or later** (**AGPL-3.0-or-later**).

If you modify Megamake and run it as a network service (for example, using `megamake chat serve`), the AGPL requires you to offer the corresponding source code of your modified version to the users of that service.