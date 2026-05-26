# TUIkit

Toolkit of SSH Connection.

TUIkit is a Go SSH application built with `golang.org/x/crypto/ssh`. Users connect with an SSH client, then use an in-terminal TUI to register, log in, and access user or administrator menus.

## Features

- SSH server using `golang.org/x/crypto/ssh`
- Terminal UI over SSH
- User registration and login
- Password hashing with bcrypt
- Local JSON user database
- Custom real-time chat channels with message history
- Live channel updates when other users post messages or administrators moderate content
- Administrator chat moderation: view deleted content, delete messages, clear channels, mute and unmute users
- Automatic host key generation
- Administrator menu for user listing, role changes, activation, password reset, and deletion
- **API relay**: HTTP reverse proxy for forwarding API requests to upstream providers, with admin management and user access

## Run

```sh
go run ./cmd/tuikit -addr :2222 -api-addr :8080
```

On first startup, TUIkit creates an `admin` account. To choose the first admin password:

```sh
TUIKIT_ADMIN_PASSWORD='change-this-password' go run ./cmd/tuikit -addr :2222 -api-addr :8080
```

If `TUIKIT_ADMIN_PASSWORD` is not set, a random password is printed once in the server logs.

Then connect from another terminal:

```sh
ssh -p 2222 localhost
```

The default files are:

- `data/users.json` for users
- `data/chat.json` for channels, messages, and mutes
- `data/host_key` for the SSH host key

Useful flags:

```sh
go run ./cmd/tuikit \
  -addr :2222 \
  -api-addr :8080 \
  -data data/users.json \
  -chat-data data/chat.json \
  -proxy-data data/proxy.json \
  -host-key data/host_key
```

## API Relay

TUIkit includes an HTTP reverse proxy that forwards API requests to upstream providers (e.g. OpenAI, Anthropic). The proxy server runs alongside the SSH server on a separate port.

### Admin setup (via SSH TUI)

1. Log in as admin via SSH
2. Navigate to `Admin > API relay > Add route`
3. Fill in:
   - **Name**: route identifier (e.g. `openai`)
   - **Upstream**: target URL (e.g. `https://api.openai.com`)
   - **Path prefix**: URL prefix for matching (e.g. `/v1`)
   - **API key**: your upstream API key
   - **Key header**: header name for the key (default: `Authorization`, uses `Bearer <key>` format)

### User access (via SSH TUI)

1. Log in via SSH
2. Press `x` from the dashboard to view available API endpoints
3. The screen shows endpoint URLs and authentication method

### Calling the relay

```sh
curl http://localhost:8080/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{"model":"gpt-4","messages":[{"role":"user","content":"hello"}]}'
```

The proxy automatically injects the configured API key into the request header.

## Build

```sh
go build ./...
```
