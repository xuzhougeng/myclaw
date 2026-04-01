# Repository Guidelines

## Project Structure & Module Organization
`cmd/myclaw` is the CLI entry point; `cmd/myclaw-desktop` hosts the Wails desktop app and checked-in web assets under `frontend/dist/`; `cmd/myclaw-eval` is the model evaluation CLI. Shared business logic lives in `internal/` packages such as `ai`, `app`, `dirlist`, `fileingest`, `knowledge`, `modelconfig`, `projectstate`, `promptlib`, `reminder`, `runtimepolicy`, `sessionstate`, `skilllib`, `sqliteutil`, `terminal`, and `weixin`. Keep new code in `internal/<domain>` unless it is an executable entry point. Repo docs and images live in `docs/`; release packaging scripts live in `scripts/` and `packaging/windows/`.

Persistent app state is now centered on a SQLite database (`app.db`) under the data directory. Keep core runtime state in shared storage packages, and treat extra files such as WeChat credentials or secret keys as interface-specific/supporting artifacts rather than the primary source of truth.

## Architecture Guardrails
This repository is being refactored toward a conversation-first, interface-thin architecture. New code and refactors should follow these rules:

- Treat terminal, desktop, HTTP dev, and WeChat as interface adapters. They can translate input/output and trigger interface-local effects, but they should not own core business rules.
- Keep conversation lifecycle decisions centralized. Reusing the current conversation, creating a new one, rebinding on `/new`, and recovering from missing bindings should be driven by shared runtime logic, not transport-specific branching.
- Model `/new` as a conversation binding operation for the current interface slot, not as a generic business command with ad hoc side effects.
- Commands and tool-like actions should have explicit policy metadata in the core runtime: whether they require conversation context, whether they persist history, whether they may create a conversation, whether they may activate desktop UI, and which interfaces can invoke them.
- User-facing slash commands should prefer stable namespaced families such as `/kb ...`, `/skill ...`, and `/prompt ...` instead of growing many flat top-level verbs.
- Persisted conversation modes are `agent` and `ask`. Treat `@kb` as a per-message override that temporarily adds knowledge-base retrieval, not as a third persisted mode.
- Do not reintroduce `/mode` as a runtime switch. New desktop conversations choose `ask` or `agent` at creation time; other interfaces default to `agent` unless the message itself uses `@ai`, `@kb`, or `@agent`.
- Prefer generic AI decision stages over tool-specific intent extractors. The default pattern is: identify need, match candidate tools, read tool contract, plan tool input, execute, and optionally iterate on prior results.
- Keep context in three layers: execution scratchpad for raw per-task artifacts, task summary for compact carry-over state, and conversation memory for persisted user/assistant turns. Do not feed raw tool output back into future turns when a summary is sufficient.
- Treat intermediate material as disposable by default. File listings, fetched page bodies, raw command output, and other execution artifacts should stay in scratchpad unless there is an explicit reason to persist or surface them.
- A thinner interface may expose fewer capabilities, but it must not redefine the fundamental conversation semantics.
- When fixing bugs, prefer removing transport-specific special cases and moving logic into `internal/app` or a dedicated `internal/<domain>` package instead of adding more branchy behavior to `internal/weixin`, `internal/terminal`, or desktop UI glue.
- If a change alters architecture assumptions, update `README.md` and this file in the same change.

## Build, Test, and Development Commands
Use `go run ./cmd/myclaw` for the terminal app, `go run ./cmd/myclaw-desktop` for the desktop shell, and `go run ./cmd/myclaw-eval -dataset <path>` to run a JSONL evaluation dataset against the AI stages; see `docs/ai-stage-eval.md` for format details. `make dev` starts the desktop app in HTTP dev mode on `127.0.0.1:3415`. `make test` runs `go test ./...` across the repository. `make build-current` builds the CLI into `dist/`; `make package-linux`, `make package-windows`, and `make package-macos` create release archives.

## Coding Style & Naming Conventions
Target Go 1.24 and let `gofmt` own formatting; do not hand-align whitespace. Follow Go naming: exported identifiers use PascalCase, internal helpers use camelCase, package directories stay lowercase, and platform files use suffixes like `_windows.go` or `_stub.go`. Keep functions small and package boundaries clear; prefer extending existing `internal/*` packages over adding cross-package shortcuts.

## Tool Units
Reusable tool-style modules must be designed as self-descriptive units, so they can be reused by other AI projects without reverse-engineering app-specific code paths.

The authoritative requirements, checklist, template, and registered tool registry now live in [docs/tool-units.md](./docs/tool-units.md).

When adding, renaming, or removing a reusable tool unit:
- Follow `docs/tool-units.md`
- Keep the tool logic in `internal/<tool>`
- Keep runtime registration and transport/UI glue outside the tool package
- Update the registry in `docs/tool-units.md` in the same change

## Testing Guidelines
Place tests beside the code they cover as `*_test.go`; this repo already follows that pattern in `internal/*` and `cmd/myclaw-desktop`. Prefer table-driven tests for routing, storage, and parser behavior. There is no stated coverage gate, but new logic should include focused tests and `make test` should pass before a PR is opened.

## Commit & Pull Request Guidelines
Install hooks with `make install-hooks`; `.githooks/commit-msg` enforces conventional subjects in the form `feat(scope): summary`, `docs(scope): summary`, or `chore(scope): summary`. Use lowercase scopes such as `model`, `desktop-chat`, or `ci`, matching recent history. PRs should explain user-visible behavior, list validation steps, link related issues, and include screenshots when desktop UI or docs output changes.
