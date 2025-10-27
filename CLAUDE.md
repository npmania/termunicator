# termunicator

## Overview

termunicator is a Terminal User Interface (TUI) application for the Communicator project. It provides a simple, irssi-style command-line interface for accessing multiple chat platforms.

## Architecture

### Core Design
- Written in Go following Russ Cox and Rob Pike programming principles
- Simple, clear code with small interfaces
- Uses libcommunicator (via cgo) for all platform communication
- Keyboard-driven interface inspired by irssi IRC client
- Minimalist single-pane layout: status bar, messages, input

### Technology Stack
- Go programming language (following Pike/Cox best practices)
- bubbletea for TUI framework
- lipgloss for minimal styling (basic terminal colors)
- cgo for interfacing with libcommunicator

## Project Structure

```
termunicator/
├── CLAUDE.md             # This file - project documentation
├── go.mod                # Go module definition
├── go.sum                # Go dependencies
├── main.go               # Main application (simple, single file)
└── internal/
    ├── lib/              # libcommunicator bindings (legacy)
    │   └── communicator.go
    ├── ui/               # Legacy UI code
    └── config/           # Legacy config code
```

Note: Following Pike/Cox principles, the main application is kept simple in a single
main.go file. The internal packages are legacy code from earlier iterations.

## Key Features

### UI Layout (irssi-style)
- **Status Bar**: Time, current channel, activity indicators
- **Message Area**: Simple format `HH:MM <nick> message`
- **Input Line**: Channel prefix with input text

All in a single, focused view - no multiple panes or complex layouts.

### Functionality
- Multi-platform support through libcommunicator
- Real-time message updates
- Simple message composition
- Channel switching (irssi-style)
- Thread filtering: Shows only root posts (first post of each thread) in both channels and DMs
- Message cursor: Navigate through messages with arrow keys, selected message highlighted with cyan background (black text on cyan)
- Multi-line message support: Messages with newlines display properly with indentation on continuation lines
- Smart text fitting: Long lines are truncated to fit within the post area width (no horizontal terminal scrolling)
- Smart message fitting: Displays as many complete messages as possible in available screen space

## Keybindings

```
Messaging:
  Enter            - Send message
  Backspace        - Delete character
  (any char)       - Type message

Navigation:
  Up/Down          - Move cursor through messages
  PgUp/PgDown      - Scroll by page
  Ctrl+B           - Switch focus (sidebar/main)

  Sidebar:
    Up/Down        - Navigate channels
    Space          - Select channel

General:
  Ctrl+C           - Quit
```

## Configuration

### Environment Variables

termunicator supports two authentication methods:

#### Option 1: Token Authentication (Recommended)
```bash
export MATTERMOST_HOST=chat.mysite.io
export MATTERMOST_TOKEN=your_personal_access_token_here
export MATTERMOST_TEAM_ID=your_team_id_here  # Optional
```

**Creating a Personal Access Token:**

1. Log into your Mattermost instance in your browser
2. Go to **Account Settings** (click your profile picture → Account Settings)
3. Navigate to **Security** → **Personal Access Tokens**
4. Click **Create Token**
5. Give it a description (e.g., "termunicator CLI")
6. Copy the token immediately (you won't be able to see it again!)
7. Set it as `MATTERMOST_TOKEN` environment variable

Personal Access Tokens do NOT expire your browser session and are designed for API/CLI access.

#### Option 2: Password Authentication
```bash
export MATTERMOST_HOST=chat.mysite.io
export MATTERMOST_LOGIN_ID=your_email@example.com  # or username
export MATTERMOST_PASSWORD=your_password
export MATTERMOST_TEAM_ID=your_team_id_here  # Optional
```

**Note:** Password login creates a new session each time, which may affect browser sessions.

Configuration file locations:
- Linux: `~/.config/termunicator/config.yaml`
- macOS: `~/Library/Application Support/termunicator/config.yaml`
- Windows: `%APPDATA%\termunicator\config.yaml`

Configuration options:
- API tokens for platforms
- UI theme and colors
- Keybinding customization
- Notification preferences

## Building

```bash
# Build the application
go build -o termunicator

# Run tests
go test ./...

# Run with development mode
go run main.go
```

## Dependencies

Key Go packages:
- bubbletea - TUI framework
- lipgloss - Terminal styling
- cgo for libcommunicator integration

Following Pike/Cox principles: minimal dependencies, use standard library where possible.

## Integration with libcommunicator

### cgo Bindings
- Wrapper functions in `internal/lib/communicator.go`
- Handles conversion between Go types and C types
- Manages callback functions for async events
- Proper cleanup of C memory

### Event Handling
- Real-time message events via callbacks
- Connection state changes
- Presence updates
- Typing indicators

## Current Status

- Initial project setup
- TUI framework selection in progress
- Designing UI layout and interaction patterns
- cgo bindings for libcommunicator pending library completion

## Development Notes

- Must link against libcommunicator dynamic library
- Set `LD_LIBRARY_PATH` (Linux) or `DYLD_LIBRARY_PATH` (macOS) during development
- Handle graceful shutdown and cleanup of C resources
- Test terminal compatibility across different terminals (xterm, alacritty, kitty, etc.)
- Consider terminal size changes (SIGWINCH handling)

## Testing Strategy

- Unit tests for business logic
- Integration tests with mock libcommunicator
- Manual testing on different terminal emulators
- Cross-platform testing (Linux, macOS, Windows)
