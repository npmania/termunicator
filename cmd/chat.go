package cmd

import (
	"fmt"
	"os"
	"strings"

	"termunicator/internal/ui"
)

func HandleChatCommand(args []string) error {
	if len(args) < 2 {
		return fmt.Errorf("usage: termunicator chat @username")
	}

	username := args[1]
	if !strings.HasPrefix(username, "@") {
		return fmt.Errorf("username must start with @")
	}

	username = strings.TrimPrefix(username, "@")
	
	// Load config only when needed for chat
	chatUI := ui.NewChatUI(username)
	if err := chatUI.Run(); err != nil {
		return fmt.Errorf("chat error: %v", err)
	}
	return nil
}

func ParseArgs() error {
	args := os.Args[1:]
	
	if len(args) == 0 {
		return fmt.Errorf("usage: termunicator chat @username")
	}

	switch args[0] {
	case "chat":
		return HandleChatCommand(args)
	default:
		return fmt.Errorf("unknown command: %s", args[0])
	}
}