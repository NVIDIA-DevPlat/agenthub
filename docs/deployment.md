# agenthub Deployment Guide

## Prerequisites

### macOS / Linux

1. **Go 1.25.8+** with CGO enabled
   ```bash
   go version  # should be >= 1.25.8
   ```

2. **ICU4C** (required by the Dolt embedded database via beads)
   ```bash
   # macOS
   brew install icu4c

   # Ubuntu/Debian
   sudo apt-get install libicu-dev
   ```

3. **Dolt SQL server** (for agenthub's bot registry, settings, and inbox)
   ```bash
   # Install Dolt
   curl -L https://github.com/dolthub/dolt/releases/latest/download/install.sh | bash
   # or: brew install dolt

   # Initialize and start
   mkdir -p ~/.agenthub/dolt && cd ~/.agenthub/dolt
   dolt init
   dolt sql-server --host=127.0.0.1 --port=3306 &
   ```

4. **Slack App** (see `docs/slack-integration.md`)

---

## Building

```bash
git clone https://github.com/NVIDIA-DevPlat/agenthub
cd agenthub
make build
# Binary: ./agenthub
```

The Makefile automatically detects the Homebrew ICU4C prefix and sets the required `CGO_CFLAGS` / `CGO_LDFLAGS`.

For production Linux builds, build on the target host (or a matching Linux machine) — CGO prevents cross-compilation from macOS:

```bash
# On the Linux host:
git clone https://github.com/NVIDIA-DevPlat/agenthub
cd agenthub
make deps   # installs Go, ICU4C if needed
make build
```

---

## First-Run Setup

Start the server:
```bash
./agenthub serve
```

On first run, agenthub detects that Dolt has no `admin_password_hash` and enters **setup mode**. Open the admin UI at `http://localhost:8080` — you will be redirected to `/admin/setup`. Fill in the form to:

1. Choose an admin password
2. Derive an AES-256-GCM encryption key (Argon2id)
3. Generate a random session signing secret
4. Store all values encrypted in Dolt settings
5. Receive the registration token for agent API calls

---

## Configuration

Copy and edit `config.yaml`:
```bash
cp config.yaml /etc/agenthub/config.yaml
export AGENTHUB_CONFIG=/etc/agenthub/config.yaml
```

Set secrets via the admin web UI after first login (**Admin → Secrets**):
- `openai_api_key`
- `slack_bot_token` (`xoxb-`)
- `slack_app_token` (`xapp-`)

Or use the settings API directly:
```bash
curl -s -X PUT http://localhost:8080/api/settings/openai_api_key \
  -H "Content-Type: application/json" \
  -b "agenthub_session=<cookie>" \
  -d '{"value":"sk-..."}'
```

---

## Running

```bash
./agenthub serve
```

The admin web UI will be available at `http://localhost:8080` (or the configured `server.http_addr`).

### Service Deployments (non-interactive)

For systemd or other process supervisors, set the admin password via environment variable so the server starts without a prompt:

```
Environment=AGENTHUB_ADMIN_PASSWORD=<password>
```

Example systemd unit:
```ini
[Unit]
Description=AgentHub Service
After=network.target

[Service]
ExecStart=/usr/local/bin/agenthub serve
Environment=AGENTHUB_CONFIG=/etc/agenthub/config.yaml
Environment=AGENTHUB_ADMIN_PASSWORD=<password>
Restart=on-failure
RestartSec=5

[Install]
WantedBy=multi-user.target
```

---

## Admin Password Recovery

If you lose the admin password, you must reinitialize the settings. Connect to Dolt directly and clear the settings table:

```bash
dolt sql -q "DELETE FROM settings WHERE key_name = 'admin_password_hash'" --user=root
```

Then restart agenthub — it will detect the missing `admin_password_hash` and enter setup mode again. Re-enter your Slack tokens and API keys via the Secrets page after setup completes.

---

## Deploying Updates

Stop before replacing the binary (Linux locks running binaries):

```bash
ssh agenthub "cd ~/Src/agenthub && git pull && make build && \
  sudo systemctl stop agenthub && \
  sudo cp agenthub /usr/local/bin/agenthub && \
  sudo systemctl start agenthub"
```

Or use the convenience target:
```bash
make deploy-system
```

---

## Migrating from File-Based Secret Store

If you were using the legacy `secrets.enc` file, set `store.path` in `config.yaml`:

```yaml
store:
  path: ~/.agenthub/secrets.enc
```

On the next start, all keys are automatically copied into Dolt settings (skipping any already-set ones). Once migration is confirmed, remove `store.path` from `config.yaml`.

---

## Environment Variables

| Variable | Description |
|----------|-------------|
| `AGENTHUB_CONFIG` | Override path to `config.yaml` |
| `AGENTHUB_ADMIN_PASSWORD` | Admin password for non-interactive startup |

---

## Docker (Future)

A `Dockerfile` will be added in a future release. The binary requires CGO and ICU4C at build time; the runtime image will need `libicu` installed.
