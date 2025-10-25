package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"

	comm "libcommunicator"
)

func main() {
	if err := comm.Init(); err != nil {
		log.Fatal(err)
	}
	defer comm.Cleanup()

	host := os.Getenv("MATTERMOST_HOST")
	token := os.Getenv("MATTERMOST_TOKEN")

	if host == "" || token == "" {
		log.Fatal("MATTERMOST_HOST and MATTERMOST_TOKEN required")
	}

	serverURL := "https://" + host
	platform, err := comm.NewMattermostPlatform(serverURL)
	if err != nil {
		log.Fatal(err)
	}
	defer platform.Destroy()

	// Connect without team ID first to get user info
	config := comm.NewPlatformConfig(serverURL).WithToken(token)
	if err := platform.Connect(config); err != nil {
		log.Fatal(err)
	}
	defer platform.Disconnect()

	// Get connection info which might have team
	if connInfo, err := platform.GetConnectionInfo(); err == nil {
		fmt.Println("Connection Info:")
		j, _ := json.MarshalIndent(connInfo, "", "  ")
		fmt.Println(string(j))

		if connInfo.TeamID != "" {
			fmt.Printf("\nSet this: export MATTERMOST_TEAM_ID=%s\n", connInfo.TeamID)
		}
	}

	// Get current user
	if user, err := platform.GetCurrentUser(); err == nil {
		fmt.Println("\nCurrent User:")
		j, _ := json.MarshalIndent(user, "", "  ")
		fmt.Println(string(j))
	}
}
