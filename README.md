# termunicator

A Terminal User Interface (TUI) for unified chat communication.

## Prerequisites

- Go 1.21 or later
- libcommunicator built and available

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
# On Linux
LD_LIBRARY_PATH=../libcommunicator/target/release ./termunicator

# On macOS
DYLD_LIBRARY_PATH=../libcommunicator/target/release ./termunicator
```

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
