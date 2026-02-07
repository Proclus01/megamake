# megamake

Megamake is a structured prompt generator — a “prompt compiler” — as well as a workflow orchestration tool for LLM agents to make use of Megamake’s various tools.

At a high level, Megamake helps you:

- **Extract real source code into a stable `<context>` blob** you can paste into an LLM (or feed to an agent).
- **Generate machine- and human-consumable artifacts** (pseudo-XML + JSON + agent prompt) in a deterministic, audit-friendly format.
- **Orchestrate multi-tool workflows** for documentation, diagnostics, test planning, and chat.
- **Run with explicit network policy controls** (deny-by-default unless `--net` is enabled, optional allowlist via `--allow-domain`).

---

## Quickstart (recommended install)

### Canonical artifact directory (recommended)
Megamake writes artifacts to an artifact directory you choose. A good default is:

- macOS/Linux: `~/.megamake/artifacts`
- Windows: `%USERPROFILE%\.megamake\artifacts`

You can still override per command with `--artifact-dir ./artifacts` (project-local), but having a canonical default is convenient.

---

## Install / Build

Megamake’s CLI entrypoint is:

- `./cmd/megamake`

### Option A (recommended): build a local binary and add a wrapper that always sets `--artifact-dir`

This gives you the ergonomic command:

```sh
megamake prompt .
```

without re-typing `--artifact-dir` every time.

#### macOS / Linux (zsh/bash)

From the repo root (where `go.mod` is):

```sh
# 1) Build to ~/.local/bin (no sudo)
mkdir -p "$HOME/.local/bin" "$HOME/.megamake/artifacts" \
  && go test ./... \
  && go build -o "$HOME/.local/bin/megamake" ./cmd/megamake

# 2) Ensure ~/.local/bin is in PATH (zsh example)
# (If you use bash, put this in ~/.bashrc instead)
printf '\nexport PATH="$HOME/.local/bin:$PATH"\n' >> "$HOME/.zshrc"

# 3) Add a wrapper function so `megamake ...` always uses the canonical artifact dir
printf '\nexport MEGAMAKE_ARTIFACT_DIR="$HOME/.megamake/artifacts"\n' >> "$HOME/.zshrc"
printf '\nmegamake(){ command megamake --artifact-dir "$MEGAMAKE_ARTIFACT_DIR" "$@"; }\n' >> "$HOME/.zshrc"

# 4) Reload shell config
source "$HOME/.zshrc"

# 5) Confirm
megamake --help
megamake prompt . --help
```

Notes:
- Using `command megamake` inside the function bypasses the function itself and executes the real binary.
- If you don’t want a wrapper function, just call `megamake --artifact-dir "$HOME/.megamake/artifacts" ...`.

#### Windows (PowerShell)

From the repo root:

```powershell
# 1) Build to your Go bin dir
go test ./...
$bin = Join-Path $HOME "go\bin"
New-Item -Force -ItemType Directory $bin | Out-Null
go build -o (Join-Path $bin "megamake.exe") .\cmd\megamake

# 2) Create canonical artifact dir
$art = Join-Path $HOME ".megamake\artifacts"
New-Item -Force -ItemType Directory $art | Out-Null

# 3) Add a wrapper function to your PowerShell profile so `megamake ...` always uses the artifact dir
# (creates the profile file if missing)
if (!(Test-Path $PROFILE)) { New-Item -Force -ItemType File $PROFILE | Out-Null }

Add-Content -Path $PROFILE -Value ""
Add-Content -Path $PROFILE -Value ('$MEGAMAKE_EXE = "' + (Join-Path $bin "megamake.exe") + '"')
Add-Content -Path $PROFILE -Value ('$MEGAMAKE_ARTIFACT_DIR = "' + $art + '"')
Add-Content -Path $PROFILE -Value 'function megamake { param([Parameter(ValueFromRemainingArguments=$true)][string[]]$Args) & $MEGAMAKE_EXE --artifact-dir $MEGAMAKE_ARTIFACT_DIR @Args }'

# 4) Reload profile
. $PROFILE

# 5) Confirm
megamake --help
```

If `go\bin` is not on your PATH, add it via Windows Environment Variables (recommended) or call the executable directly.

---

### Option B: `go install` (simple, but depends on PATH)
From repo root:

```sh
go install ./cmd/megamake
```

This installs `megamake` to `$(go env GOBIN)` if set, otherwise `$(go env GOPATH)/bin`.

You’ll still want:
- to ensure that directory is on PATH, and
- optionally add the wrapper function for `--artifact-dir`.

---

## Compile / Verify Everything Builds

```sh
go test ./...
```

Even if you have no tests, `go test ./...` compiles all packages.

---

## Formatting / Linting (Optional)

```sh
gofmt -w .
golangci-lint run ./...   # if you use golangci-lint
```

---

## Usage

### Global flags

- `--artifact-dir <dir>`  
  Directory where Megamake artifacts and pointer files are written.
- `--net`  
  Enable network access (deny-by-default otherwise).
- `--allow-domain <domain>` (repeatable)  
  Allowed domain when `--net` is set. If none provided, all domains are allowed when `--net` is set.

---

## Commands (overview)

- `prompt` — scan a repo and emit a `<context>` blob (pseudo-XML) containing source files.
- `doc create` — generate a documentation report from a local repo.
- `doc get` — fetch documentation from local paths or HTTP(S), with network gating.
- `diagnose` — run best-effort diagnostics using local toolchains and generate a fix-oriented prompt.
- `test` — build a structured test plan.
- `chat` — local-first chat orchestration tool (CLI + server + UI).

---

## Chat (CLI + Server + UI)

Megamake Chat is stored under your artifact directory:

- Runs: `<artifactDir>/MEGACHAT/runs/<run_name>/`
- Default dotenv path (provider keys): `<artifactDir>/MEGACHAT/.env`

### Start the chat server + UI

```sh
megamake --net chat serve --listen 127.0.0.1:8082
```

Open (in a browser):

```text
http://127.0.0.1:8082/ui
```

Health:

```text
http://127.0.0.1:8082/health
```

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

After editing `.env`, **restart the server** so it loads the environment.

You can override the dotenv path when serving:

```sh
megamake --net chat serve --env-file /path/to/.env --listen 127.0.0.1:8082
```

### Create/list/get runs (CLI)

```sh
megamake chat new --title "My Conversation"
megamake chat list
megamake chat get --run <run_name>
```

### Providers: verify + list models (CLI)

These commands respect network policy:

- network providers require `--net`
- if you set `--allow-domain`, provider API hosts must be allowlisted

Examples:

```sh
# Stub provider (no network)
megamake chat verify --provider stub --json=false
megamake chat models --provider stub --json=false

# OpenAI (requires --net and OPENAI_API_KEY)
megamake --net chat verify --provider openai
megamake --net chat models --provider openai --limit 50 --json=false
```

Model listing supports server-side caching controls:

```sh
megamake --net chat models --provider openai --no-cache
megamake --net chat models --provider openai --cache-ttl-seconds 60
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

## Troubleshooting

### `--help` prints an error / exits non-zero
Megamake should treat `--help` as a successful exit. If you see “flag: help requested” as an error again, ensure you special-case `flag.ErrHelp` and return exit code 0.

### Chat: provider verify/models fail with “network not enabled”
Start server/CLI with `--net`, and if you use `--allow-domain`, ensure the provider host is allowlisted (e.g. `api.openai.com`).

### Chat: provider verify fails with “missing API key”
Ensure you have a dotenv file at `<artifactDir>/MEGACHAT/.env` (or use `--env-file`) and restart the server so it loads the updated environment.

---

## License

Megamake is licensed under the **GNU Affero General Public License v3.0 or later** (**AGPL-3.0-or-later**).

If you modify Megamake and run it as a network service (for example, using `megamake chat serve`), the AGPL requires you to offer the corresponding source code of your modified version to the users of that service.