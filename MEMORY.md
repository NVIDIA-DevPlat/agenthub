# MEMORY.md — Agent Session Log

A running log of every substantive agent (AI/Claude) session working on this repository.
Each entry records what was done, key decisions made, and the state left behind.

---

## 2026-03-14 — Claude Sonnet 4.6 (sessions 1–3)

### Session 1: EnsureInitialized bug fix + beads status alignment

**Problem diagnosed:** `EnsureInitialized` never wrote `issue_prefix` to the beads DB because
`GetConfig` returns `("", nil)` for missing keys (not an error). The `if err != nil` guard
was never triggered, so the prefix was silently skipped on every startup.

**Root cause found by:** Extracting and reading the beads library source from the module cache
zip at `/root/go/pkg/mod/cache/download/github.com/steveyegge/beads/@v/v0.60.0.zip`.

**Fix:** Changed `EnsureInitialized` condition from `if err != nil` to `if value == ""`.

**Also fixed:**
- Valid status values in `tasks_handler.go` — added `"deferred"`, `"closed"`; removed `"done"`
  (which beads does not accept)
- `GetConfig` mock in `beads/mock_storage_test.go` now returns `("", nil)` for absent keys
  (matching real beads behavior)

**Tested live:** `POST /api/tasks` returned `{"id":"AH-tjm","title":"Test task","status":"open"}`.
`POST /api/tasks/{id}/status` returned HTTP 204.

**Commits:** `b84e398`, `92449d7`, `202e620`, `395948f`, `445f89d`

---

### Session 2: Full API test coverage

**Added:** `src/internal/api/api_task_form_test.go` (26 tests)
- `spyTaskManager` pattern captures last `TaskCreateRequest` for field-level assertions
- Tests for `GET /admin/kanban/tasks/new`: auth required, renders OK, shows columns
- Tests for `POST /admin/kanban/tasks`: all 11 fields forwarded, default priority=2
- Tests for `POST /api/tasks`: actor resolution (body > header > "bot"), response JSON
- Tests for `POST /api/tasks/{id}/status`: all 5 valid statuses → 204, 6 invalid → 400

**Also added:** `task-create.html` test template to `testTemplates()` in `api_test.go`.

**Commit:** `(pre-session-3 commit)`

---

### Session 3: Five agent coordination features

**Context:** Agents are outbound-only sandboxed VMs — they can reach the internet but cannot
receive inbound connections. agenthub runs in Azure with full bidirectional internet access.
Slack Socket Mode is impossible for agents; they must poll agenthub instead.

**Feature 1 — Agent Inbox API** (`src/internal/api/inbox.go`)
- `GET /api/inbox` — returns pending messages without consuming them (persist until Ack'd)
- `POST /api/inbox/{id}/ack` — remove a single message (idempotent)
- `POST /api/inbox/{id}/reply` — post reply text to original Slack channel, auto-ack
- Slack handler now enqueues DMs in the assigned bot's inbox (`slack.InboxEnqueuer` interface)
- `InboxReplier` interface wired to Slack bot token client in main.go (`slackReplier`)

**Feature 2 — Heartbeat + Live Agent Grid** (`src/internal/api/heartbeat.go`)
- `POST /api/heartbeat` — agents POST status/current_task/message; response includes `inbox_count`
- `GET /admin/heartbeats` — cookie-auth JSON endpoint for admin dashboard
- Dashboard replaced 30s HTMX poll with SSE-triggered JS; renders agent cards with staleness

**Feature 3 — Task Activity Log** (`src/internal/api/activity_log.go`)
- `POST /api/tasks/{id}/log` — agents append `{"message":"…","level":"warn"}` as beads comments
- `TaskLogger` optional interface; `beadsTaskManager.AddLog` bridges to `client.AddComment`
- Broadcasts `task-log` SSE event on each log entry

**Feature 4 — SSE Real-time Kanban** (`src/internal/api/events.go`)
- `GET /admin/events` — `text/event-stream` with 25s keepalives, nginx buffering disabled
- `EventBroadcaster` fans out to all connected browsers; non-blocking (skips slow clients)
- Kanban template: removed `hx-trigger="every 30s"`, added vanilla JS SSE listener
- All task create/status/log handlers broadcast `kanban-update` or `task-log` events

**Feature 5 — Webhook Relay** (`src/internal/api/webhooks.go`)
- `POST /api/webhooks/{channel}` — unauthenticated external receive (channel name = shared secret)
- `POST /api/webhooks/subscribe` / `unsubscribe` — agent channel management (token-auth)
- `GET /api/webhooks/subscriptions` — list subscribed channels for calling agent
- Payloads routed to all subscribed bots' inboxes; broadcasts `inbox-update` SSE event

**Wire-up in main.go:**
- `beadsTaskManager` additionally implements `api.TaskLogger` via `AddLog`
- `slackReplier` bridges `api.InboxReplier` to `github.com/slack-go/slack` client
- `srv.Inbox()` and `srv.SetReplier()` public methods for post-creation wiring
- `slack.Deps.Inbox` field wired to `srv.Inbox()` so DMs route to bot inboxes

**Test file:** `src/internal/api/api_features_test.go` (42 tests covering all 5 features)

**Commits:** `82439ab` (features), `a0add06` (AGENTS.md deployment docs)

**Deployed to production:** `agenthub 0.1.0 (build a0add06)` running on Azure VM `20.124.109.29`.

---

## Deployment state as of 2026-03-14

| Item | Value |
|---|---|
| Production version | `0.1.0 (build a0add06)` |
| VM | `agenthub` / `20.124.109.29` |
| Service | `systemd agenthub.service` — active |
| Branch | `main` — clean |
| Test coverage | all packages passing |

---

*Append a new dated section for each agent session. Include: what problem was solved,
key decisions, files changed, and the commit hash(es). Keep entries factual and terse.*
