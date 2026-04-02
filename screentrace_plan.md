# ScreenTrace Plan

## Scope

This plan defines the first shippable `ScreenTrace` feature for `baize`.

Goals:

- Add a lightweight desktop-only background feature that periodically captures the screen.
- Use a separate low-cost vision model profile for image analysis instead of the main chat model.
- Persist raw capture metadata and summarized activity in `app.db`.
- Keep the feature isolated from the conversation runtime so it does not bloat startup or agent flow.
- Expose minimal desktop controls for enable/disable, interval, retention, model selection, status, and history browsing.

Non-goals for this first version:

- No Python sidecar.
- No terminal or WeChat capture pipeline.
- No custom region picker.
- No OCR-only secondary pipeline.
- No vector store.

## Architecture Decisions

- `internal/screentrace` is the core package.
- Desktop is only an adapter for lifecycle, settings, and presentation.
- Raw screenshots are stored under `dataDir/screentrace/YYYY/MM/DD/`.
- Structured records and digests are stored in `app.db` using dedicated tables.
- ScreenTrace uses a dedicated model profile ID selected from existing model profiles.
- The main active model remains unchanged; ScreenTrace resolves its own model config explicitly.
- ScreenTrace writes compact digests to the knowledge base only when explicitly enabled.

## Delivery Checklist

### Phase 1: Core package

- [x] Add `internal/screentrace` types, store, and manager.
- [x] Add persistent settings for enable flag, interval, retention, model profile, and KB digest option.
- [x] Add screenshot storage path helpers and cleanup logic.
- [x] Add image hashing / similarity checks to skip near-duplicate captures.
- [x] Add structured record model for per-capture analysis and periodic digest rows.

### Phase 2: AI integration

- [x] Extend AI service with explicit config-based image analysis entrypoints.
- [x] Define JSON schema for screen analysis output.
- [x] Resolve ScreenTrace model config by profile ID instead of using the active chat profile.
- [x] Convert analysis output into compact record text and digest text.

### Phase 3: Desktop runtime integration

- [x] Start the ScreenTrace manager from desktop startup.
- [x] Stop the manager during desktop shutdown.
- [x] Surface runtime status to the desktop app backend.
- [x] Persist and reload ScreenTrace settings via desktop settings storage.

### Phase 4: Desktop UI

- [x] Add a dedicated `活动记录` view to browse recent ScreenTrace records.
- [x] Add settings controls for enable flag, interval seconds, retention days, and vision profile.
- [x] Show recent status: last capture time, last analysis time, last error, total records.
- [x] Allow manual refresh from the frontend.

### Phase 5: Knowledge and digest integration

- [x] Add optional digest generation on a time bucket.
- [x] Add optional write-to-KB behavior for generated digests.
- [x] Keep raw per-capture records out of the knowledge base.

### Phase 6: Tests and docs

- [x] Add tests for store migrations and settings persistence.
- [x] Add tests for similarity filtering and digest grouping.
- [x] Add tests for desktop backend APIs.
- [x] Update `README.md`.
- [x] Update `AGENTS.md` guidance if architecture assumptions change.

## Execution Notes

- The implementation target is a working MVP, not a speculative framework.
- Each completed item should be checked off in this file as code lands.
- If scope pressure appears, cut optional polish before cutting the separate-model requirement.
