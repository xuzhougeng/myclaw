# Repository Guidelines

## Project Structure & Module Organization
`cmd/myclaw` is the CLI entry point; `cmd/myclaw-desktop` hosts the Wails desktop app and checked-in web assets under `frontend/dist/`. Shared business logic lives in `internal/` packages such as `ai`, `app`, `knowledge`, `reminder`, `runtimepolicy`, `terminal`, and `weixin`. Keep new code in `internal/<domain>` unless it is an executable entry point. Repo docs and images live in `docs/`; release packaging scripts live in `scripts/` and `packaging/windows/`.

## Architecture Guardrails
This repository is being refactored toward a conversation-first, interface-thin architecture. New code and refactors should follow these rules:

- Treat terminal, desktop, HTTP dev, and WeChat as interface adapters. They can translate input/output and trigger interface-local effects, but they should not own core business rules.
- Keep conversation lifecycle decisions centralized. Reusing the current conversation, creating a new one, rebinding on `/new`, and recovering from missing bindings should be driven by shared runtime logic, not transport-specific branching.
- Model `/new` as a conversation binding operation for the current interface slot, not as a generic business command with ad hoc side effects.
- Commands and tool-like actions should have explicit policy metadata in the core runtime: whether they require conversation context, whether they persist history, whether they may create a conversation, whether they may activate desktop UI, and which interfaces can invoke them.
- A thinner interface may expose fewer capabilities, but it must not redefine the fundamental conversation semantics.
- When fixing bugs, prefer removing transport-specific special cases and moving logic into `internal/app` or a dedicated `internal/<domain>` package instead of adding more branchy behavior to `internal/weixin`, `internal/terminal`, or desktop UI glue.
- If a change alters architecture assumptions, update `README.md` and this file in the same change.

## Build, Test, and Development Commands
Use `go run ./cmd/myclaw` for the terminal app and `go run ./cmd/myclaw-desktop` for the desktop shell. `make dev` starts the desktop app in HTTP dev mode on `127.0.0.1:3415`. `make test` runs `go test ./...` across the repository. `make build-current` builds the CLI into `dist/`; `make package-linux`, `make package-windows`, and `make package-macos` create release archives.

## Coding Style & Naming Conventions
Target Go 1.24 and let `gofmt` own formatting; do not hand-align whitespace. Follow Go naming: exported identifiers use PascalCase, internal helpers use camelCase, package directories stay lowercase, and platform files use suffixes like `_windows.go` or `_stub.go`. Keep functions small and package boundaries clear; prefer extending existing `internal/*` packages over adding cross-package shortcuts.

## Tool Units
Reusable tool-style modules must be designed as self-descriptive units, so they can be reused by other AI projects without reverse-engineering app-specific code paths.

When adding a new tool unit:
- Put the executable logic in its own `internal/<tool>` package instead of burying it inside a transport layer such as WeChat, terminal, or desktop UI.
- Expose a stable tool name, a short purpose statement, a machine-oriented input contract, a machine-oriented output contract, and a human-readable help/usage text.
- Keep the phases separated: intent recognition decides whether the tool should run and prepares tool input; the tool package normalizes input and executes; the transport layer only renders results or delivers side effects.
- Tool units should not decide conversation lifecycle. If a tool depends on special persistence or activation behavior, express that through shared runtime policy instead of transport-local logic.
- If the tool also has a shortcut command such as `/find`, treat that command as a thin registration layer over the tool unit. Register the shortcut in the runtime that actually owns it, not globally by default, and route `help` back to the tool's own usage text from that runtime.
- Update the registry below whenever a reusable tool is added, renamed, or removed.

### Registered Tool Units
- `everything_file_search`
  Package: `internal/filesearch`
  Purpose: Search local Windows files via Everything (`es.exe`) using either native queries or structured semantic filters.
  Input contract: `query`, `keywords`, `drives`, `known_folders`, `paths`, `extensions`, `date_field`, `date_value`, `limit`.
  Output contract: executed query, effective limit, result count, ordered file items with `index`, `name`, and `path`.
  Shortcut registration: `/find` and `/find help`, handled by the shared app runtime; WeChat additionally supports `/send <序号>` through its interface adapter.
  Current pipeline split: intent recognition in `internal/ai` and `internal/app`; search execution and selection state in `internal/filesearch`; WeChat file delivery in `internal/weixin/filesender.go`.

## Testing Guidelines
Place tests beside the code they cover as `*_test.go`; this repo already follows that pattern in `internal/*` and `cmd/myclaw-desktop`. Prefer table-driven tests for routing, storage, and parser behavior. There is no stated coverage gate, but new logic should include focused tests and `make test` should pass before a PR is opened.

## Commit & Pull Request Guidelines
Install hooks with `make install-hooks`; `.githooks/commit-msg` enforces conventional subjects in the form `feat(scope): summary`, `docs(scope): summary`, or `chore(scope): summary`. Use lowercase scopes such as `model`, `desktop-chat`, or `ci`, matching recent history. PRs should explain user-visible behavior, list validation steps, link related issues, and include screenshots when desktop UI or docs output changes.
