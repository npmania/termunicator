# termunicator

## Overview

termunicator is a Terminal User Interface (TUI) application for the Communicator project. It provides a rich, interactive command-line interface for accessing multiple chat platforms.

## Architecture

### Core Design
- Written in Go for excellent terminal handling and cross-platform support
- Uses libcommunicator (via cgo) for all platform communication
- Keyboard-driven interface with vim-style keybindings
- Multi-pane layout for channels, messages, and user lists

### Technology Stack
- Go programming language
- TUI library (bubbletea/tview/termui - to be determined)
- cgo for interfacing with libcommunicator

## Project Structure

```
termunicator/
├── claude.md              # This file
├── go.mod                 # Go module definition
├── go.sum                 # Go dependencies
├── main.go                # Application entry point
├── cmd/                   # Command-line interface
├── internal/
│   ├── ui/               # TUI components
│   │   ├── layout.go     # Main layout management
│   │   ├── channels.go   # Channel list pane
│   │   ├── messages.go   # Message display pane
│   │   ├── input.go      # Message input pane
│   │   └── users.go      # User list pane
│   ├── lib/              # libcommunicator bindings
│   │   └── communicator.go # cgo wrapper
│   ├── state/            # Application state management
│   └── config/           # Configuration handling
└── assets/               # Static resources
```

## Key Features

### UI Components
- **Channel List**: Browse and switch between channels/conversations
- **Message View**: Display messages with timestamps, authors, reactions
- **Input Field**: Compose and send messages
- **User List**: View online/offline status of users
- **Status Bar**: Connection status, notifications, keybinding hints

### Functionality
- Multi-platform support through libcommunicator
- Real-time message updates
- Message composition with multi-line support
- Keyboard shortcuts for navigation
- Search functionality for messages and channels
- Thread support for conversations
- Notification handling

## Keybindings (Planned)

```
General:
  Ctrl+C / q    - Quit
  Ctrl+P        - Switch platform/workspace
  /             - Search
  ?             - Help

Navigation:
  Tab / Shift+Tab  - Cycle through panes
  j / k            - Move up/down in lists
  Enter            - Select/open item
  Esc              - Cancel/back

Messaging:
  i                - Enter insert mode (compose message)
  Esc              - Exit insert mode
  Enter            - Send message (in insert mode)
  Shift+Enter      - New line (in insert mode)
```

## Configuration

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
- TUI framework (to be selected)
- cgo for libcommunicator integration
- Configuration library (viper/koanf)
- Logging library (zerolog/zap)

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
