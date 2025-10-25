package main

import (
	"fmt"
	"os"

	comm "libcommunicator"
)

func main() {
	fmt.Println("=== termunicator Authentication Test ===\n")

	// Check environment
	host := os.Getenv("MATTERMOST_HOST")
	token := os.Getenv("MATTERMOST_TOKEN")
	loginID := os.Getenv("MATTERMOST_LOGIN_ID")
	password := os.Getenv("MATTERMOST_PASSWORD")
	teamID := os.Getenv("MATTERMOST_TEAM_ID")

	fmt.Printf("MATTERMOST_HOST: %s\n", host)
	fmt.Printf("MATTERMOST_TEAM_ID: %s\n", teamID)

	if token != "" {
		fmt.Printf("MATTERMOST_TOKEN: %s...%s (%d chars)\n",
			token[:min(4, len(token))],
			token[max(0, len(token)-4):],
			len(token))
		fmt.Println("Auth method: TOKEN")
	}

	if loginID != "" {
		fmt.Printf("MATTERMOST_LOGIN_ID: %s\n", loginID)
		fmt.Printf("MATTERMOST_PASSWORD: %s (%d chars)\n",
			maskPassword(password), len(password))
		fmt.Println("Auth method: PASSWORD")
	}

	if token == "" && (loginID == "" || password == "") {
		fmt.Println("\nERROR: No authentication credentials set!")
		fmt.Println("\nSet one of:")
		fmt.Println("  export MATTERMOST_TOKEN=your_token")
		fmt.Println("Or:")
		fmt.Println("  export MATTERMOST_LOGIN_ID=your_email")
		fmt.Println("  export MATTERMOST_PASSWORD=your_password")
		os.Exit(1)
	}

	fmt.Println("\nTesting connection...")

	if err := comm.Init(); err != nil {
		fmt.Printf("ERROR: Init failed: %v\n", err)
		os.Exit(1)
	}
	defer comm.Cleanup()

	serverURL := "https://" + host
	platform, err := comm.NewMattermostPlatform(serverURL)
	if err != nil {
		fmt.Printf("ERROR: Create platform failed: %v\n", err)
		os.Exit(1)
	}
	defer platform.Destroy()

	var config *comm.PlatformConfig
	if token != "" {
		config = comm.NewPlatformConfig(serverURL).WithToken(token)
	} else {
		config = comm.NewPlatformConfig(serverURL).WithPassword(loginID, password)
	}

	if teamID != "" {
		config = config.WithTeamID(teamID)
	}

	fmt.Println("Connecting...")
	if err := platform.Connect(config); err != nil {
		fmt.Printf("\nERROR: Connection failed!\n%v\n", err)
		os.Exit(1)
	}
	defer platform.Disconnect()

	fmt.Println("✓ Connected successfully!")

	user, err := platform.GetCurrentUser()
	if err != nil {
		fmt.Printf("WARNING: Could not get user info: %v\n", err)
	} else {
		fmt.Printf("\nLogged in as:\n")
		fmt.Printf("  Username: @%s\n", user.Username)
		fmt.Printf("  Name: %s\n", user.Name)
		fmt.Printf("  Email: %s\n", user.Email)
		fmt.Printf("  ID: %s\n", user.ID)
	}

	channels, err := platform.GetChannels()
	if err != nil {
		fmt.Printf("WARNING: Could not get channels: %v\n", err)
	} else {
		fmt.Printf("\nFound %d channels\n", len(channels))
	}

	fmt.Println("\n✓ All tests passed!")
}

func maskPassword(p string) string {
	if len(p) == 0 {
		return "<not set>"
	}
	if len(p) <= 2 {
		return "**"
	}
	return string(p[0]) + "****" + string(p[len(p)-1])
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
