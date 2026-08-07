# AGENTS.md — AiBird Project Overview

## What Is This Project

AiBird is a Go-based IRC bot that provides AI-powered text generation, image/video/sound generation via ComfyUI, ASCII art, leaderboards, headline aggregation, and multi-network IRC state management. It connects to multiple IRC networks simultaneously, each in its own goroutine, and routes user commands through a permission-based dispatch system.

## Tech Stack

- **Language**: Go 1.25 (module: `aibird`)
- **Database**: SQLite3 via `mattn/go-sqlite3` (requires `CGO_ENABLED=1`)
- **IRC Library**: `lrstanley/girc` (forked to `birdneststream/girc`)
- **Image Generation**: ComfyUI via a direct in-tree HTTP/WebSocket client (`image/comfyui/apiclient`)
- **Text Generation**: LlamaCpp, GLM, Gemini providers
- **Configuration**: TOML via `BurntSushi/toml` + `go-playground/validator/v10`
- **Testing**: Standard Go `testing` + `stretchr/testify`
- **Linting**: golangci-lint (revive, govet, gosec, staticcheck, goimports, misspell, unused, typecheck)
- **~17,500 lines of Go across ~60 files**

## Project Structure

```
main.go                    — Entry point: config loading, IRC connection loop, event handlers
birdbase/                  — SQLite database layer (schema, queries, in-memory flood/rate limiting)
irc/commands/              — Command handlers: dispatch, owner, admin, standard, image, text, sound, video, leaderboard, headlines, mood, vibe, story, recall, summary, top, help
irc/state/                 — IRC state management (State struct, user/channel/network tracking)
irc/networks/              — Network types, connection config, server management
irc/channels/              — Channel types, mode tracking, settings
irc/users/                 — User types, admin/owner/ignored status, access levels
irc/users/modes/           — Mode types
irc/servers/               — Server types, SSL, port configuration
irc/participant/           — AI participant system for non-command chat messages
queue/                     — Single-GPU (RTX 4090) priority queue, UserAccess interface
image/comfyui/             — ComfyUI workflow integration, API-format graph loader, sidecar .meta.toml parsing
image/comfyui/apiclient/   — Direct ComfyUI HTTP/WebSocket client (replaces comfy2go)
image/ircart/              — ASCII art conversion from images
image/exifparser/          — PNG EXIF data extraction
text/                      — Text generation: LlamaCpp, GLM, Gemini providers + response caching
http/uploaders/birdhole/   — Birdhole gallery file upload service
http/request/              — HTTP request utilities
settings/                  — TOML configuration loading and types
logger/                    — Centralized structured logging (slog-based)
helpers/                   — Shared utilities (mode mapping, string helpers)
shared/meta/               — Shared metadata types (AibirdMeta, etc.)
asciistore/                — ASCII art storage and retrieval
status/                    — Status page integration
comfyuijson/               — ComfyUI API-format workflow JSON files + sibling .meta.toml sidecars (gitignored)
```

## Core Architecture

### State Struct (`irc/state/`)
Central context object passed to all command handlers. Holds Client, Event, Network, User, Channel, Config. All IRC state modifications go through this centralized manager with mutex protection.

### GPU Queue (`queue/`)
`ProcessingQueue` manages a single GPU queue for the RTX 4090. Priority routing via the `UserAccess` interface (`GetAccessLevel()`, `CanSkipQueue()`). Privileged users skip to the front of the queue.

### Command Dispatch (`irc/commands/`)
Permission-based routing: owner > admin > standard. Commands categorized as queueable (GPU-bound: image/sound/video) vs immediate (text, help, leaderboard). Dynamic command loading from ComfyUI workflows in `comfyuijson/`.

### Database (`birdbase/`)
Normalized SQLite schema: `networks`, `channels`, `irc_users`, `user_channels`, `user_modes`, `servers`, `admin_hosts`, `ignored_nicks`, `denied_commands`, `command_leaderboard`. Uses `ON CONFLICT` (not `INSERT OR REPLACE`). Thread-safe with transactions for multi-table ops.

### Security Model
Three-tier access: Owner > Admin > User. Per-command permission checks. Flood protection with in-memory rate limiting. Config files gitignored (API keys, server passwords). `#nosec` annotations for intentional exceptions.

## Key Conventions

- **Imports**: `aibird/...` module paths
- **Exported**: `CamelCase`; **unexported**: `camelCase`
- **Errors**: `fmt.Errorf("context: %w", err)`
- **Logging**: `logger.Info/Debug/Warn/Error` (never `fmt.Print` or `log`)
- **Database**: `ON CONFLICT` not `INSERT OR REPLACE`; `SaveSingleUser()` for individual updates; transactions for multi-table ops
- **TOML tags**: `toml:"snake_case"` on config structs
- **Tests**: Table-driven, `:memory:` SQLite, `_test.go` in same package
- **Concurrency**: Goroutine per IRC network; mutex-protected state; context cancellation
- **Commits**: Conventional format — `feat:`, `fix:`, `refactor:`, `chore:`, `test:`

## MCP Tools

The following MCP servers are available for research and context gathering during autonomous sessions:

| MCP Server | Purpose | Key Tools |
|---|---|---|
| **searxng** | Web search | `searxng_web_search`, `searxng_web_url_read` |
| **firecrawl** | Web page scraping | `firecrawl_scrape`, `firecrawl_search`, `firecrawl_extract` |
| **context7** | Library documentation lookup | Remote MCP at `mcp.context7.com` |
| **memory** | Persistent knowledge graph across sessions | `create_entities`, `search_nodes`, `open_nodes` |

### Web Search Workflow

When you need to research something on the internet, use this two-step pattern:

**Step 1: Search** — Use `searxng_web_search` to find relevant URLs.
**Step 2: Scrape** — Use `firecrawl_scrape` on the best result URLs to get full content.

```
DO:  searxng_web_search("go sqlite3 WAL mode best practices")
     → returns list of URLs with titles/descriptions
     firecrawl_scrape(url="https://...", formats=["markdown"])
     → returns full page content in markdown

DO:  searxng_web_url_read(url="https://...")
     → returns page text (lighter alternative for simple pages)

DO:  firecrawl_search(query="...", scrapeOptions={formats: ["markdown"]})
     → search + scrape in one call when you want results immediately

DO NOT: Use webfetch for general web searches — use searxng instead.
DO NOT: Try to scrape without searching first — always find the right URL first.
```

### Library Docs

Use `context7` when you need documentation for specific libraries used in this project:
- Go standard library, `mattn/go-sqlite3`, `lrstanley/girc`, `gorilla/websocket`, `stretchr/testify`, `BurntSushi/toml`, `go-playground/validator`

### Persistent Memory

Use the `memory` MCP to store important project context that should persist across sessions:
- Architecture decisions and their rationale
- Discovered bugs, gotchas, or patterns in the codebase
- Important relationships between components

## Quality Gates

```bash
go fmt ./...
go vet ./...
golangci-lint run --enable=revive,govet,ineffassign,misspell,gofmt,goimports,gosec,staticcheck,unused,typecheck --timeout=5m ./...
go test ./... -timeout=30s
CGO_ENABLED=1 go build -o aibird .
```

---

## Self-Iterating Improvement Loop

This project uses an **automated, multi-agent improvement pipeline** that runs in fully autonomous iterations with zero human interaction. The goal is continuous, self-directed codebase improvement — bug fixes, refactoring, test coverage, and quality enhancements.

### How It Works

The pipeline is driven by `auto-improve.sh`, which loops through iterations:

1. **Plan** — `glm-plan` agent analyzes the current codebase state (git history, uncommitted changes, `.llm/` progress files), identifies the most impactful improvements, and creates a detailed implementation plan
2. **Review Plan** — Two independent plan reviewers (GLM self-review, Tencent HY3) evaluate the plan in parallel for security, architecture, testing, and completeness
3. **Build** — `glm-build` agent implements the plan task-by-task, running quality gates after each task (format, vet, lint, test, build), getting code reviewed by two independent reviewers, and auto-committing
4. **Iterate** — After completing all tasks, the loop cools down and starts the next iteration

### Iteration Tracking

Each iteration creates a dedicated git branch based off the current branch, with a descriptive name ending in `-iteration-N`. The LLM invents the descriptive prefix based on what it's working on. This produces a clear history:

```
main
 └── feat/add-queue-tests-iteration-1
      └── feat/fix-memory-leaks-iteration-2
           └── feat/refactor-dispatch-iteration-3
```

### Agent Architecture

| Agent | Model | Role |
|-------|-------|------|
| `glm-plan` | GLM-5.2 | Plans improvements, invokes 2 plan reviewers, auto-approves |
| `glm-build` | GLM-5.2 | Implements plan tasks, runs quality gates, invokes 2 code reviewers, auto-commits |
| `deepseek-plan-review` | DeepSeek v4 Flash | Plan review subagent |
| `deepseek-code-review` | DeepSeek v4 Flash | Code review subagent |
| `hy3-plan-review` | Tencent HY3 | Plan review subagent |
| `hy3-code-review` | Tencent HY3 | Code review subagent |

### Per-Iteration Flow

```
auto-improve.sh
  ├── glm-plan (read-only)
  │     ├── Analyze codebase state
  │     ├── Create improvement plan
  │     ├── Invoke 2 plan reviewers in parallel (deepseek-plan-review + hy3-plan-review)
  │     ├── Auto-approve
  │     └── Output finalized plan
  │
  ├── glm-build (full access)
  │     ├── Create branch feat/DESCRIPTION-iteration-N (LLM picks descriptive name)
  │     ├── Save PLAN.md + PROGRESS.md to .llm/
  │     └── For each task:
  │           ├── Implement code + tests
  │           ├── Run quality gates (fmt, vet, lint, test, build)
  │           ├── Invoke 2 code reviewers in parallel (deepseek-code-review + hy3-code-review, or fast-path for trivial changes)
  │           ├── Auto-commit (no human approval needed)
  │           └── Update PROGRESS.md
  │
  └── cooldown → next iteration
```

### Progress Tracking

All iteration progress is stored in `.llm/` directories:
- `.llm/feat-DESCRIPTION-iteration-N/PLAN.md` — The approved plan (immutable)
- `.llm/feat-DESCRIPTION-iteration-N/PROGRESS.md` — Task status, commit hashes, review outcomes

### Running the Loop

```bash
# Run 5 iterations
./auto-improve.sh 5

# Run infinite loop (Ctrl+C to stop)
./auto-improve.sh 0

# Custom prompt
./auto-improve.sh 3 "Focus on adding tests for queue/ and birdbase/ packages"
```
