# Slack Integration Setup

## Creating the Slack App

1. Go to [api.slack.com/apps](https://api.slack.com/apps)
2. Click **Create New App** → **From scratch**
3. Name: `agenthub`, pick your workspace

## Required Scopes

### Bot Token Scopes (`xoxb-`)

Navigate to **OAuth & Permissions** → **Scopes** → **Bot Token Scopes**:

| Scope | Purpose |
|-------|---------|
| `chat:write` | Send messages |
| `commands` | Respond to slash commands |
| `app_mentions:read` | Receive @mentions |
| `im:history` | Read DMs sent to the bot |
| `channels:read` | Read channel info |
| `channels:manage` | Create public channels for agents and projects |
| `groups:write` | Create private channels for agents and projects |

> **Note:** `channels:manage` and `groups:write` are required for agenthub to automatically create dedicated `#agent-<name>` channels when agents register. If these scopes are not granted, agent registration still succeeds but no dedicated channel is created.

### App-Level Token Scopes (`xapp-`)

Navigate to **Basic Information** → **App-Level Tokens** → **Generate Token**:

| Scope | Purpose |
|-------|---------|
| `connections:write` | Socket Mode connection |

## Enable Socket Mode

1. **Settings** → **Socket Mode** → Enable Socket Mode
2. Copy the App-Level Token (`xapp-...`) — you'll enter this in the agenthub admin UI

## Slash Commands

Navigate to **Slash Commands** → **Create New Command**:

| Command | Description |
|---------|-------------|
| `/agenthub` | Main agenthub command (bind, list, remove, task creation) |

For the Request URL, enter any placeholder (Socket Mode doesn't use an HTTP URL for slash commands, but the field is required in the UI). Example: `https://placeholder.example.com/slack/commands`

## Install the App

1. **OAuth & Permissions** → **Install to Workspace**
2. Copy the Bot User OAuth Token (`xoxb-...`)

## Configure agenthub

After running first-run setup, log into the admin web UI and navigate to **Secrets**:

1. Enter `slack_bot_token` — your `xoxb-` Bot Token
2. Enter `slack_app_token` — your `xapp-` App Token

Restart the service to pick up the new tokens:
```bash
sudo systemctl restart agenthub
```

Or update them live without restart:
```bash
curl -s -X PUT http://localhost:8080/api/settings/slack_bot_token \
  -H "Content-Type: application/json" \
  -b "agenthub_session=<cookie>" \
  -d '{"value":"xoxb-..."}'
```

## Testing the Integration

In your Slack workspace:
```
/agenthub list
```

You should see a response listing registered agents (empty on first use).

## Per-Agent Channels

When an agent registers via `POST /api/register`, agenthub automatically:
1. Creates a public Slack channel `#agent-<name>`
2. Stores the channel ID in the agent's database record
3. Announces the new agent in the configured default channel with a link to `#agent-<name>`

Users can then post directly in `#agent-<name>` to send messages to that agent's inbox. No `@mention` or slash command is needed — the message goes straight to the agent.

This requires `channels:manage` (for public channels) or `groups:write` (for private channels) scope on the bot token.

## DM Channel

Users can also DM the agenthub bot directly to:
- Create work items (routed to any available agent, or a named one with `@botname`)
- Ask questions about the agent ecosystem
- Get task status updates

The bot uses an OpenAI-compatible endpoint to understand natural language requests.

## Default Channel

Set `slack.default_channel` in `config.yaml` to a channel ID (e.g. `C01234567`) to receive registration announcements when new agents join.
