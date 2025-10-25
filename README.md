# termunicator

A Plan 9 acme-inspired Terminal User Interface (TUI) for Mattermost.

## Features

- 🎨 Plan 9 acme-style interface with clean separators
- 💬 Channel and Direct Message support
- ⚡ Real-time username resolution with caching
- 📏 Automatic window resizing
- ⌨️ Vim-inspired keyboard navigation

## Prerequisites

- Go 1.21 or later
- libcommunicator built and available

## Quick Start

### Option 1: Personal Access Token (Recommended)

```bash
./termunicator -host chat.example.com -token YOUR_TOKEN
```

Create a Personal Access Token in Mattermost:
1. Account Settings → Security → Personal Access Tokens
2. Click "Create Token"
3. Copy it immediately!

### Option 2: Username/Password

```bash
./termunicator -host chat.example.com -user you@example.com -pass YOUR_PASSWORD
```

### With Team ID

```bash
./termunicator -host chat.example.com -token YOUR_TOKEN -teamid TEAM_ID
```

### All Options

```bash
./termunicator -h
```

Flags:
- `-host` - Mattermost server (required)
- `-token` - Personal Access Token
- `-user` - Email or username (for password auth)
- `-pass` - Password (for password auth)
- `-teamid` - Team ID (optional)

**Note:** All configuration is via CLI flags only. Environment variables are NOT used.

## Building

First, ensure libcommunicator is built:

```bash
cd ../libcommunicator
cargo build --release
cd ../termunicator
```

Then build termunicator:

```bash
# On Linux
export LD_LIBRARY_PATH=../libcommunicator/target/release:$LD_LIBRARY_PATH
go build

# On macOS
export DYLD_LIBRARY_PATH=../libcommunicator/target/release:$DYLD_LIBRARY_PATH
go build
```

## Running

```bash
# Token auth
./termunicator -host chat.example.com -token YOUR_TOKEN

# Password auth
./termunicator -host chat.example.com -user you@example.com -pass PASSWORD

# With team ID
./termunicator -host chat.example.com -token YOUR_TOKEN -teamid TEAM_ID

# If library not in system path:
LD_LIBRARY_PATH=../libcommunicator/target/release ./termunicator -host chat.example.com -token YOUR_TOKEN
```

## Keyboard Controls

- `↑` / `↓` - Navigate between channels/DMs
- `Enter` - Send message
- Type - Compose message
- `Backspace` - Delete character
- `q` / `Ctrl+C` - Quit

## UI Layout

```
┌─────────────────────────────────────────┐
│ termunicator | Del Snarf | Look         │  ← acme-style title bar
├─────────────────────────────────────────┤
│ Channels                                 │  ← Channel list
│  ▸ general                              │
│    random                                │
├─────────────────────────────────────────┤
│ Direct Messages                          │  ← DM list
│    alice                                 │
├─────────────────────────────────────────┤
│ #general                                 │  ← Active channel
├─────────────────────────────────────────┤
│ bob: hello everyone                      │  ← Messages
│ alice: hi there!                         │
├─────────────────────────────────────────┤
│ type your message here...                │  ← Input
├─────────────────────────────────────────┤
│ Enter: send | ↑/↓: switch | q: quit     │  ← Status/help
└─────────────────────────────────────────┘
```

## Troubleshooting

### "authentication required"

Provide authentication with:
- **Token:** `-token YOUR_TOKEN`
- **Password:** `-user YOUR_EMAIL -pass YOUR_PASSWORD`

### "401 Unauthorized"

- Check your credentials are correct
- For token auth: Create a new Personal Access Token
- For password auth: Verify email/username and password

### "Browser session expired"

Using password auth creates a new session. Use token auth to avoid this.

## Development

```bash
# Run directly
# On Linux
LD_LIBRARY_PATH=../libcommunicator/target/release go run main.go

# On macOS
DYLD_LIBRARY_PATH=../libcommunicator/target/release go run main.go
```

## Testing

```bash
go test ./...
```
