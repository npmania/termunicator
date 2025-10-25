# termunicator

Terminal User Interface (TUI) for Mattermost.

## Features

- 💬 Channel and Direct Message support
- ⚡ Real-time message updates via WebSocket
- 🎯 Simple, focused single-pane layout
- ⌨️ Keyboard-driven navigation

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

### Sidebar Navigation
- `↑` / `↓` - Navigate teams/channels/DMs (wrap-around)
- `Space` - Select team or channel/DM
- `Ctrl+B` - Toggle between sidebar and message area

### Message Area
- `↑` / `↓` - Scroll messages one line
- `PgUp` / `PgDown` - Scroll messages by page
- `Enter` - Send message
- Type - Compose message
- `Backspace` - Delete character

### General
- `Ctrl+C` - Quit

## UI Layout

```
┌────────────┬─────────────────────────────┬─┐
│ [Teams]    │ 10:23 @alice: Hello!        │█│
│ *MyTeam    │ 10:24 @bob: Hi there        │ │
│  OtherTeam │ 10:25 @carol: How are you?  │ │
│            │                              │ │
│ [Channels] │                              │ │
│ >1:general │                              │ │
│  2:random  │                              │ │
│            │                              │ │
│ [DMs]      │                              │ │
│  alice     │                              │ │
│            │                              │ │
│            │                              │ │
│            │ #general> type here_         │ │
└────────────┴─────────────────────────────┴─┘
```

Legend:
- `*` - Cursor position (before selection)
- `>` - Active team/channel/DM

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
