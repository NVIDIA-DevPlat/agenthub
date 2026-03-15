# agenthub

**agenthub** is a Go service that acts as a hub between a Slack workspace and a fleet of [openclaw](https://github.com/NVIDIA-DevPlat/openclaw) AI agent instances. It handles agent registration, liveness monitoring, work item routing, per-agent Slack channels, and provides an admin web UI for managing the full agent ecosystem.

## Features

### Agent Registry
- Register openclaw AI agent instances via `POST /api/register` (registration-token authenticated)
- Agents report liveness via `POST /api/heartbeat`; the kanban board shows live status
- Name conflict detection with auto-suggested alternatives on collision
- Remove agents via the admin UI

### Per-Agent Slack Channels
- On registration, agenthub automatically creates a dedicated `#agent-<name>` Slack channel
- Users can post directly in that channel — messages are routed straight to the agent's inbox
- No `@botname` prefix needed; no task creation overhead
- Channel ID is announced in the default Slack channel on registration

### Agent Inbox
- Every agent has a persistent inbox backed by Dolt
- Agents poll `GET /api/inbox?bot_name=<name>` to receive messages
- Agents acknowledge messages via `POST /api/inbox/{id}/ack`
- Slack DMs to agenthub and per-agent channel messages both enqueue to the inbox
- Agent replies posted back to the originating Slack thread via `POST /api/inbox/{id}/reply`

### Work Item Routing
- Create Beads issues from Slack: `/agenthub <task description> [@botname]`
- Optionally route to a specific agent by name, or let agenthub assign to any alive agent
- All work items tracked as [Beads](https://github.com/steveyegge/beads) issues in an embedded Dolt database
- Assign tasks to agents by dragging them on the kanban board

### Kanban Board
- Admin web UI kanban board backed by Beads issues
- Columns configurable in `config.yaml` (default: backlog → ready → in_progress → review → done)
- Falls back to an empty board if Beads is unavailable

### Projects
- Create and manage projects in the admin UI
- Each project auto-creates a dedicated Slack channel (`#project-<name>`)
- Assign agents and resources to projects

### Slack Integration
- Uses Socket Mode — no public HTTP endpoint required
- Handles slash commands, `app_mention` events, and DMs
- DM the bot directly to create work items or ask questions in natural language
- Per-agent channels route messages directly to agent inboxes

### OpenAI-Powered Intelligence
- Uses a configurable OpenAI-compatible endpoint (default: `gpt-4o-mini`)
- Powers responses to direct mentions and DMs
- Model, max tokens, and system prompt all configurable at runtime via the admin UI
- System prompt and other settings can be updated live without restart

### Admin Settings API
- `PUT /api/settings/{key}` — update any runtime setting immediately (no restart)
- `GET /api/settings` — list all stored setting keys
- Settings stored encrypted in Dolt; changes propagate to all watchers in-memory

### Encrypted Settings Store
- All secrets (API keys, tokens, password hash, session key) stored in the Dolt `settings` table
- Per-row AES-256-GCM encryption; key derived from admin password via Argon2id
- Auto-migrates from legacy `secrets.enc` file on first boot
- Live updates: `PUT /api/settings/{key}` takes effect immediately

### Security
- Admin password hashed with bcrypt
- Random 32-byte session signing secret generated at first-run setup
- Admin password prompted on startup (echo-suppressed on TTY); `AGENTHUB_ADMIN_PASSWORD` env var for non-interactive/service deployments
- Session cookie with configurable name

---

## Prerequisites

| Requirement | Version | Notes |
|-------------|---------|-------|
| Go | 1.25.8+ | CGO must be enabled |
| ICU4C | any recent | Required by Dolt's embedded regex engine |
| Dolt SQL server | latest | For agenthub's bot registry, settings, and inbox |
| Slack App | — | Socket Mode + required scopes |

### Install ICU4C

```bash
# macOS
brew install icu4c

# Ubuntu / Debian
sudo apt-get install libicu-dev
```

### Install and start Dolt

```bash
# macOS
brew install dolt

# Linux
curl -L https://github.com/dolthub/dolt/releases/latest/download/install.sh | bash

# Initialize and start the SQL server
mkdir -p ~/.agenthub/dolt && cd ~/.agenthub/dolt
dolt init
dolt sql-server --host=127.0.0.1 --port=3306
```

---

## Building

```bash
git clone https://github.com/NVIDIA-DevPlat/agenthub
cd agenthub
make build
# Binary: ./agenthub
```

The Makefile automatically detects the Homebrew ICU4C prefix and sets the required `CGO_CFLAGS` / `CGO_LDFLAGS`.

---

## Setup (first run)

agenthub detects first-run automatically. Start the server:

```bash
./agenthub serve
```

On first run (Dolt `settings` table is empty), the server starts in **setup mode** and redirects all admin requests to `/admin/setup`. Fill in the setup form to choose an admin password. This will:

1. Derive an AES-256-GCM encryption key from the password (Argon2id)
2. Generate a random session signing secret
3. Hash the password with bcrypt
4. Write all values to the Dolt `settings` table (encrypted)
5. Return a registration token for agent API calls

On subsequent starts the password is required to unlock the settings:
- **Interactive:** prompted on the terminal (echo-suppressed)
- **Service/non-interactive:** set `AGENTHUB_ADMIN_PASSWORD=<password>` in the environment

---

## Configuration

All tunable behavior lives in `config.yaml`. No secrets belong here.

```bash
cp config.yaml /etc/agenthub/config.yaml
export AGENTHUB_CONFIG=/etc/agenthub/config.yaml
```

### Key settings

| Section | Key | Default | Description |
|---------|-----|---------|-------------|
| `server` | `http_addr` | `:8080` | Admin UI listen address |
| `server` | `public_url` | `""` | Public base URL (used in Slack links) |
| `server` | `read_timeout` | `30s` | HTTP read timeout |
| `server` | `write_timeout` | `30s` | HTTP write timeout |
| `slack` | `command_prefix` | `/agenthub` | Slash command prefix |
| `slack` | `default_channel` | `""` | Channel ID for registration announcements |
| `openclaw` | `liveness_interval` | `60s` | How often to poll all agents |
| `openclaw` | `liveness_timeout` | `10s` | Per-agent health check timeout |
| `openai` | `model` | `gpt-4o-mini` | OpenAI model for Slack intelligence |
| `openai` | `max_tokens` | `1024` | Max tokens per OpenAI response |
| `beads` | `db_path` | `.beads/dolt` | Beads/Dolt embedded database path |
| `dolt` | `dsn` | `root:@tcp(127.0.0.1:3306)/agenthub` | Dolt SQL server DSN |
| `kanban` | `columns` | `[backlog, ready, in_progress, review, done]` | Kanban column names and order |
| `log` | `level` | `info` | Log level: `debug`, `info`, `warn`, `error` |
| `log` | `format` | `json` | Log format: `json` or `text` |

### Environment Variables

| Variable | Description |
|----------|-------------|
| `AGENTHUB_CONFIG` | Override path to `config.yaml` |
| `AGENTHUB_ADMIN_PASSWORD` | Admin password for non-interactive startup (service deployments) |

---

## Slack App Setup

1. Go to [api.slack.com/apps](https://api.slack.com/apps) → **Create New App** → **From scratch**
2. Under **OAuth & Permissions** → **Bot Token Scopes**, add:

   | Scope | Purpose |
   |-------|---------|
   | `chat:write` | Send messages |
   | `commands` | Respond to slash commands |
   | `app_mentions:read` | Receive @mentions |
   | `im:history` | Read DMs |
   | `channels:read` | Channel info |
   | `channels:manage` | Create public channels for agents/projects |
   | `groups:write` | Create private channels for agents/projects |

3. Under **Basic Information** → **App-Level Tokens**, generate a token with `connections:write`. This is your `xapp-` token.
4. Under **Settings** → **Socket Mode**, enable Socket Mode.
5. Under **Slash Commands**, create `/agenthub` (any placeholder URL is fine for Socket Mode).
6. Install the app to your workspace and copy the `xoxb-` Bot Token.

After the first-run setup, log into the admin UI and go to **Secrets** to enter:
- `slack_bot_token` — your `xoxb-` token
- `slack_app_token` — your `xapp-` token
- `openai_api_key` — your OpenAI API key (or compatible endpoint key)

---

## Running

```bash
./agenthub serve
```

On a real terminal, the admin password is prompted with echo suppressed. For service deployments set `AGENTHUB_ADMIN_PASSWORD`. The admin UI will be available at `http://localhost:8080`.

### Subcommands

| Command | Description |
|---------|-------------|
| `agenthub serve` | Start the server (default) |
| `agenthub version` | Print version and build info |

---

## Slack Commands

All commands use the prefix `/agenthub` (configurable).

| Command | Description |
|---------|-------------|
| `/agenthub bind host:port name` | Register an openclaw bot in this channel (legacy) |
| `/agenthub remove name` | Unregister a bot |
| `/agenthub list` | List bots in this channel with alive/dead status |
| `/agenthub chatty name` | Toggle chatty mode (bot responds to all messages) |
| `/agenthub <task> [@botname]` | Create a work item, optionally routed to a specific agent |

You can also:
- **DM** the agenthub bot for natural language interaction and task creation
- **Post in `#agent-<name>`** to message a specific agent directly (routed to inbox, no task created)

---

## Admin Web UI Routes

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/admin/` | Dashboard |
| `GET` | `/admin/login` | Login form |
| `POST` | `/admin/login` | Authenticate |
| `POST` | `/admin/logout` | Clear session |
| `GET` | `/admin/setup` | First-run setup form |
| `POST` | `/admin/setup` | Submit first-run setup |
| `GET` | `/admin/bots` | List all registered agents |
| `POST` | `/admin/bots/{name}/remove` | Remove an agent |
| `POST` | `/admin/bots/{name}/check` | Trigger immediate liveness check |
| `GET` | `/admin/kanban` | Kanban board |
| `GET` | `/admin/projects` | Project list |
| `GET` | `/admin/projects/{id}` | Project detail |
| `GET` | `/admin/secrets` | Secrets manager |
| `POST` | `/admin/secrets` | Save secrets |
| `GET` | `/health` | Service health check (unauthenticated) |

## Agent REST API

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| `POST` | `/api/register` | Registration token | Register a new agent |
| `POST` | `/api/heartbeat` | Registration token | Report liveness and status |
| `GET` | `/api/inbox` | Registration token | Poll pending inbox messages |
| `POST` | `/api/inbox/{id}/ack` | Registration token | Acknowledge a message |
| `POST` | `/api/inbox/{id}/reply` | Registration token | Post reply to originating Slack thread |
| `POST` | `/api/tasks/{id}/status` | Registration token | Update task status |
| `POST` | `/api/tasks/{id}/log` | Registration token | Append task log entry |
| `PUT` | `/api/settings/{key}` | Admin session | Update a runtime setting |
| `GET` | `/api/settings` | Admin session | List setting keys |
| `GET` | `/api/events` | Admin session | SSE stream of live events |

---

## Development

```bash
# Run all tests
make test

# Run tests with coverage (minimum 90% enforced)
make test-cover

# Format source
make fmt

# Run static analysis
make lint

# Remove build artifacts
make clean
```

Integration tests require a running Dolt SQL server:

```bash
make test-integration
```

See [AGENTS.md](AGENTS.md) for contribution guidelines, the development workflow, and the pre-merge checklist.

---

## Project Structure

```
agenthub/
├── AGENTS.md                   # Contribution guidelines and commandments
├── VERSION                     # SEMVER version
├── config.yaml                 # All tunable settings
├── Makefile
├── docs/
│   ├── architecture.md         # Component diagram and data flows
│   ├── api.md                  # Agent REST API + admin HTTP routes
│   ├── configuration.md        # Full configuration reference
│   ├── deployment.md           # Deployment prerequisites and instructions
│   └── slack-integration.md    # Slack app setup guide
├── plans/                      # Phase implementation plans
├── src/
│   ├── cmd/agenthub/           # Main entry point
│   └── internal/
│       ├── api/                # HTTP handlers and admin UI server
│       ├── auth/               # Session auth, bcrypt, cookie management
│       ├── beads/              # Beads task tracker wrapper (CGO)
│       ├── config/             # config.yaml loader
│       ├── dolt/               # Dolt SQL client, schema migrations, settings store
│       ├── kanban/             # Kanban board grouping logic
│       ├── openclaw/           # Openclaw HTTP client and liveness checker
│       ├── openai/             # OpenAI chat wrapper
│       ├── settings/           # Reactive in-memory settings cache
│       ├── slack/              # Slack Socket Mode handler and slash commands
│       └── store/              # Legacy AES-256-GCM encrypted file store
├── web/
│   ├── templates/              # Go HTML templates (embedded in binary)
│   └── static/                 # CSS and static assets (embedded in binary)
└── tests/
    └── integration/            # Integration test suite
```

---

## License

Copyright © 2025 NVIDIA Corporation. All rights reserved.
