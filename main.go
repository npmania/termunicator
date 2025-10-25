package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	comm "libcommunicator"
)

// irssi-style colors - simple terminal colors
var (
	statusStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("15")).Background(lipgloss.Color("4"))                         // white on blue
	nickStyle       = lipgloss.NewStyle().Foreground(lipgloss.Color("10"))                                                         // green
	timeStyle       = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))                                                          // gray
	inputStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("15"))                                                         // white
	activityStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("11"))                                                         // yellow
	currentStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("11")).Bold(true)                                              // yellow bold for current
	selectedStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("14")).Bold(true)                                              // cyan bold for selected
)

type config struct {
	host     string
	token    string
	loginID  string
	password string
	teamID   string
}

type focusArea int

const (
	focusSidebar focusArea = iota
	focusMain
)

type navItemType int

const (
	navTeam navItemType = iota
	navChannel
	navDM
)

type navItem struct {
	itemType navItemType
	index    int // index into teams or channels array
}

type model struct {
	platform      *comm.Platform
	eventStream   *comm.EventStream
	teams         []comm.Team
	channels      []comm.Channel
	messages      []comm.Message
	users         map[string]*comm.User // cache users by ID
	currentTeam   int                   // current active team
	current       int                   // current active channel
	selected      int                   // selected channel in sidebar
	focus         focusArea             // which window has focus
	scrollOffset  int                   // scroll position in message list (0 = bottom)
	input         string
	cursorPos     int                   // cursor position in input
	teamSelected  bool                  // whether a team has been selected
	cursorVisible bool                  // for blinking cursor
	err           error
	connected     bool
	ctx           context.Context
	cancel        context.CancelFunc
	width         int
	height        int
	config        config
}

type messagesMsg []comm.Message
type olderMessagesMsg []comm.Message
type connectedMsg struct {
	platform    *comm.Platform
	eventStream *comm.EventStream
	teams       []comm.Team
	channels    []comm.Channel
}
type newMessageMsg comm.Message
type eventMsg *comm.Event
type errMsg error
type tickMsg time.Time

func initialModel(cfg config) model {
	ctx, cancel := context.WithCancel(context.Background())
	return model{
		ctx:           ctx,
		cancel:        cancel,
		users:         make(map[string]*comm.User),
		config:        cfg,
		focus:         focusSidebar, // Start with sidebar focused for team selection
		current:       -1,            // No channel selected initially
		selected:      -1,            // No channel selected initially
		cursorVisible: true,          // Start with cursor visible
	}
}

func (m model) Init() tea.Cmd {
	return tea.Batch(m.connectToMattermost, tickCmd())
}

// tickCmd returns a command that sends a tick message for cursor blinking
func tickCmd() tea.Cmd {
	return tea.Tick(time.Millisecond*500, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

// waitForEvent waits for the next event from the event stream
func waitForEvent(stream *comm.EventStream) tea.Cmd {
	return func() tea.Msg {
		select {
		case event := <-stream.Events():
			if event != nil {
				return eventMsg(event)
			}
		case err := <-stream.Errors():
			if err != nil {
				return errMsg(err)
			}
		}
		return nil
	}
}

func (m model) connectToMattermost() tea.Msg {
	// Initialize library
	if err := comm.Init(); err != nil {
		return errMsg(fmt.Errorf("init failed: %w", err))
	}

	host := m.config.host
	token := m.config.token
	loginID := m.config.loginID
	password := m.config.password
	teamID := m.config.teamID

	if host == "" {
		return errMsg(fmt.Errorf("-host is required"))
	}

	// Check authentication method
	hasToken := token != ""
	hasPassword := loginID != "" && password != ""

	if !hasToken && !hasPassword {
		return errMsg(fmt.Errorf("authentication required.\n\nOption 1 - Token:\n  -token your_token\n\nOption 2 - Password:\n  -user your_email -pass your_password"))
	}

	serverURL := "https://" + host

	// Create platform
	platform, err := comm.NewMattermostPlatform(serverURL)
	if err != nil {
		return errMsg(fmt.Errorf("create platform failed: %w", err))
	}

	// Connect with appropriate auth method
	var config *comm.PlatformConfig
	if hasToken {
		config = comm.NewPlatformConfig(serverURL).WithToken(token)
	} else {
		config = comm.NewPlatformConfig(serverURL).WithPassword(loginID, password)
	}

	if teamID != "" {
		config = config.WithTeamID(teamID)
	}

	if err := platform.Connect(config); err != nil {
		// Provide more helpful error messages
		errStr := err.Error()
		if strings.Contains(errStr, "401") {
			if hasToken {
				return errMsg(fmt.Errorf("authentication failed: Invalid token.\n\nYour token: %s...\n\nPlease check:\n1. Token is a valid Personal Access Token\n2. Token hasn't been revoked\n3. You have access to the server", token[:min(10, len(token))]))
			}
			return errMsg(fmt.Errorf("authentication failed: Invalid username/password.\n\nYour username: %s\n\nPlease check:\n1. -user should be your actual email or username (not 'YOUR_EMAIL')\n2. -pass should be your actual password (not 'YOUR_PASSWORD')\n3. Account is not locked", loginID))
		}
		return errMsg(fmt.Errorf("connect failed: %w", err))
	}

	// Get teams only - channels will be fetched when user selects a team
	teams, err := platform.GetTeams()
	if err != nil {
		return errMsg(fmt.Errorf("get teams failed: %w", err))
	}

	// Create event stream for real-time updates
	ctx := context.Background()
	eventStream, err := platform.NewEventStream(ctx, 100, 100*time.Millisecond)
	if err != nil {
		return errMsg(fmt.Errorf("create event stream failed: %w", err))
	}

	return connectedMsg{platform: platform, eventStream: eventStream, teams: teams, channels: nil}
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c":
			m.cancel()
			if m.eventStream != nil {
				m.eventStream.Close()
			}
			if m.platform != nil {
				m.platform.Disconnect()
				m.platform.Destroy()
			}
			comm.Cleanup()
			return m, tea.Quit

		case "ctrl+b":
			// Toggle focus between sidebar and main
			if m.focus == focusSidebar {
				m.focus = focusMain
			} else {
				m.focus = focusSidebar
			}
			return m, nil

		case "enter":
			if m.focus == focusMain {
				// Send message
				if m.input == "" || !m.connected || len(m.channels) == 0 || m.current < 0 {
					return m, nil
				}
				channelID := m.channels[m.current].ID
				if _, err := m.platform.SendMessage(channelID, m.input); err != nil {
					m.err = err
				}
				m.input = ""
				m.cursorPos = 0
				return m, fetchMessages(m.platform, channelID)
			}
			return m, nil

		case "up":
			if m.focus == focusSidebar {
				m.navigateSidebar(-1)
			} else if m.focus == focusMain {
				m.scrollOffset = m.clampScrollOffset(m.scrollOffset + 1)
			}
			return m, nil

		case "down":
			if m.focus == focusSidebar {
				m.navigateSidebar(1)
			} else if m.focus == focusMain {
				m.scrollOffset = m.clampScrollOffset(m.scrollOffset - 1)
			}
			return m, nil


		case "pgup":
			if m.focus == focusMain {
				// Page up - scroll by full page
				oldOffset := m.scrollOffset
				m.scrollOffset = m.clampScrollOffset(m.scrollOffset + m.msgHeight())
				// At the top, try to fetch older messages
				if m.scrollOffset == m.maxScroll() && m.scrollOffset == oldOffset && len(m.messages) > 0 && m.current >= 0 && m.current < len(m.channels) {
					oldestMsg := m.messages[0]
					return m, fetchOlderMessages(m.platform, m.channels[m.current].ID, oldestMsg.ID)
				}
			}
			return m, nil

		case "pgdown":
			if m.focus == focusMain {
				// Page down - scroll by full page
				m.scrollOffset = m.clampScrollOffset(m.scrollOffset - m.msgHeight())
			}
			return m, nil

		case " ":
			if m.focus == focusSidebar {
				if !m.teamSelected {
					// Select team with space key
					if m.selected >= 0 && m.selected < len(m.teams) {
						m.currentTeam = m.selected
						m.teamSelected = true
						// Clear messages and input
						m.messages = nil
						m.input = ""
						m.cursorPos = 0
						// Set team ID in platform and refresh channels
						if err := m.platform.SetTeamID(m.teams[m.currentTeam].ID); err != nil {
							m.err = fmt.Errorf("SetTeamID error: %w", err)
							return m, nil
						}
						channels, err := m.platform.GetChannels()
						if err != nil {
							m.err = fmt.Errorf("GetChannels error: %w", err)
							return m, nil
						}
						m.channels = channels
						m.current = -1
						m.selected = -1
						// Stay in sidebar to select channel - don't auto-focus
						// Debug: Log channel count
						if len(channels) == 0 {
							m.err = fmt.Errorf("Warning: GetChannels returned 0 channels for team %s (%s)", m.teams[m.currentTeam].DisplayName, m.teams[m.currentTeam].ID)
						}
					}
				} else {
					// Select channel/DM with space key
					if m.selected >= 0 && m.selected < len(m.channels) {
						m.current = m.selected
						m.scrollOffset = 0 // Reset scroll
						// Clear messages and input when switching channel
						m.messages = nil
						m.input = ""
						m.cursorPos = 0
						// Switch focus to main area
						m.focus = focusMain
						return m, fetchMessages(m.platform, m.channels[m.current].ID)
					}
				}
			} else {
				// In main area, space is part of input
				m.input += " "
				m.cursorPos++
			}
			return m, nil

		case "backspace", "ctrl+h":
			// Backspace removes character in typing section
			// Some terminals send "backspace", others send "ctrl+h"
			if len(m.input) > 0 && m.cursorPos > 0 {
				// Handle UTF-8 correctly by converting to runes
				runes := []rune(m.input)
				if m.cursorPos <= len(runes) {
					m.input = string(runes[:m.cursorPos-1]) + string(runes[m.cursorPos:])
					m.cursorPos--
				}
			}
			return m, nil

		case "ctrl+enter", "ctrl+m":
			// Ctrl+Enter adds newline in typing section
			if m.focus == focusMain {
				runes := []rune(m.input)
				m.input = string(runes[:m.cursorPos]) + "\n" + string(runes[m.cursorPos:])
				m.cursorPos++
			}
			return m, nil

		default:
			// Only accept printable input when main area is focused
			if m.focus == focusMain {
				str := msg.String()

				// Ignore other ctrl and alt combinations (but not the ones above)
				if strings.HasPrefix(str, "ctrl+") || strings.HasPrefix(str, "alt+") {
					return m, nil
				}

				// Only add single printable characters
				if len(str) == 1 && str[0] >= 32 && str[0] <= 126 {
					runes := []rune(m.input)
					m.input = string(runes[:m.cursorPos]) + str + string(runes[m.cursorPos:])
					m.cursorPos++
				}
			}
		}

	case connectedMsg:
		m.platform = msg.platform
		m.eventStream = msg.eventStream
		m.teams = msg.teams
		m.channels = msg.channels
		m.connected = true
		// If teamID was provided via config, position cursor on that team
		if m.config.teamID != "" {
			for i, team := range m.teams {
				if team.ID == m.config.teamID {
					m.currentTeam = i
					break
				}
			}
		}
		// Always show team selection screen - user must select with arrow keys
		// Start listening for events
		return m, waitForEvent(m.eventStream)

	case eventMsg:
		// Handle real-time events
		if msg != nil {
			switch msg.Type {
			case comm.EventMessagePosted:
				// Try MessageID first, then extract from Data if needed
				msgID := msg.MessageID
				if msgID == "" && msg.Data != nil {
					if dataMap, ok := msg.Data.(map[string]interface{}); ok {
						if id, ok := dataMap["id"].(string); ok {
							msgID = id
						}
					}
				}
				if msgID != "" {
					return m, tea.Batch(
						waitForEvent(m.eventStream),
						fetchMessage(m.platform, msgID),
					)
				}
			case comm.EventMessageUpdated:
				// Message was edited - could refresh if needed
				// For now, just ignore
			case comm.EventMessageDeleted:
				// Message was deleted - could remove from display
				// For now, just ignore
			case comm.EventUserStatusChanged:
				// User status changed - could update user cache
				// For now, just ignore
			case comm.EventUserTyping:
				// User is typing - could show indicator
				// For now, just ignore
			case comm.EventChannelCreated, comm.EventChannelUpdated, comm.EventChannelDeleted:
				// Channel changed - could refresh channel list
				// For now, just ignore
			case comm.EventUserJoinedChannel, comm.EventUserLeftChannel:
				// User joined/left channel
				// For now, just ignore
			case comm.EventConnectionStateChange:
				// Connection state changed
				// For now, just ignore
			default:
				// Unknown event type - ignore silently
			}
		}
		// Continue listening for events
		return m, waitForEvent(m.eventStream)

	case newMessageMsg:
		// Append new message to current channel
		if m.current >= 0 && m.current < len(m.channels) {
			newMsg := comm.Message(msg)
			if newMsg.ChannelID == m.channels[m.current].ID {
				// Check if message already exists (avoid duplicates)
				exists := false
				for _, existingMsg := range m.messages {
					if existingMsg.ID == newMsg.ID {
						exists = true
						break
					}
				}
				if !exists {
					m.messages = append(m.messages, newMsg)
					m.scrollOffset = m.clampScrollOffset(m.scrollOffset)
				}
			}
		}

	case messagesMsg:
		m.messages = msg
		m.scrollOffset = 0 // Reset scroll to bottom (newest messages) when loading new channel

	case olderMessagesMsg:
		// Prepend older messages to the beginning
		if len(msg) > 0 {
			m.messages = append(msg, m.messages...)
			// Adjust scroll to maintain viewing position
			m.scrollOffset += len(msg)
			// Clamp scroll position to valid range
			m.scrollOffset = m.clampScrollOffset(m.scrollOffset)
		}
		// If no messages returned, we've hit the end - stay at current position

	case errMsg:
		m.err = msg

	case tickMsg:
		// Toggle cursor visibility
		m.cursorVisible = !m.cursorVisible
		return m, tickCmd()
	}

	// Continue listening for events if connected
	if m.connected && m.eventStream != nil {
		return m, waitForEvent(m.eventStream)
	}
	return m, nil
}

func fetchMessages(platform *comm.Platform, channelID string) tea.Cmd {
	return func() tea.Msg {
		messages, err := platform.GetMessages(channelID, 50)
		if err != nil {
			return errMsg(err)
		}
		return messagesMsg(messages)
	}
}

func fetchOlderMessages(platform *comm.Platform, channelID, beforeID string) tea.Cmd {
	return func() tea.Msg {
		messages, err := platform.GetMessagesBefore(channelID, beforeID, 20)
		if err != nil {
			return errMsg(err)
		}
		return olderMessagesMsg(messages)
	}
}

func fetchMessage(platform *comm.Platform, messageID string) tea.Cmd {
	return func() tea.Msg {
		msg, err := platform.GetMessage(messageID)
		if err != nil {
			return errMsg(err)
		}
		return newMessageMsg(*msg)
	}
}

// msgHeight returns the height available for messages
func (m *model) msgHeight() int {
	h := m.height - 1
	if h < 3 {
		h = 3
	}
	return h
}

// maxScroll returns the maximum scroll offset
func (m *model) maxScroll() int {
	max := len(m.messages) - m.msgHeight()
	if max < 0 {
		return 0
	}
	return max
}

// clampScrollOffset ensures scroll offset is within valid bounds
func (m *model) clampScrollOffset(offset int) int {
	if offset < 0 {
		return 0
	}
	max := m.maxScroll()
	if offset > max {
		return max
	}
	return offset
}

// scrollbarChar returns the character to display at given line in scrollbar
func scrollbarChar(line, height, scrollOffset, totalMsgs int) string {
	if totalMsgs <= height || height <= 0 {
		return " " // No scroll needed
	}

	// Calculate thumb position as single character
	maxScroll := totalMsgs - height
	if maxScroll < 0 {
		maxScroll = 0
	}

	// Calculate which line should have the thumb
	var thumbLine int
	if maxScroll > 0 {
		// Invert: when scrollOffset=0 (bottom), thumb at bottom
		// when scrollOffset=maxScroll (top), thumb at top
		scrollRatio := float64(scrollOffset) / float64(maxScroll)
		thumbLine = int((1.0 - scrollRatio) * float64(height-1))
	} else {
		thumbLine = height - 1 // At bottom
	}

	// Return appropriate character for this line
	if line == thumbLine {
		return "█" // Thumb (single block)
	}
	return "│" // Track
}

// getNavItems returns all navigable items in sidebar order
func (m *model) getNavItems() []navItem {
	var items []navItem

	// Always add teams
	for i := range m.teams {
		items = append(items, navItem{itemType: navTeam, index: i})
	}

	// Add channels and DMs if team selected
	if m.teamSelected {
		// Add regular channels
		for i, ch := range m.channels {
			if ch.Type == comm.ChannelTypeDirectMessage || ch.Type == comm.ChannelTypeGroupMessage {
				continue
			}
			items = append(items, navItem{itemType: navChannel, index: i})
		}

		// Add DMs
		for i, ch := range m.channels {
			if ch.Type != comm.ChannelTypeDirectMessage && ch.Type != comm.ChannelTypeGroupMessage {
				continue
			}
			items = append(items, navItem{itemType: navDM, index: i})
		}
	}

	return items
}

// getCurrentNavPosition returns the current position in the nav list
func (m *model) getCurrentNavPosition() int {
	items := m.getNavItems()

	// Find current position based on teamSelected and selected index
	if !m.teamSelected {
		// In teams section
		for i, item := range items {
			if item.itemType == navTeam && item.index == m.selected {
				return i
			}
		}
	} else {
		// In channels/DMs section
		for i, item := range items {
			if (item.itemType == navChannel || item.itemType == navDM) && item.index == m.selected {
				return i
			}
		}
	}

	// Default to first item
	return 0
}

// isItemSelected checks if an item is the currently selected one
func (m *model) isItemSelected(itemType navItemType, index int) bool {
	items := m.getNavItems()
	currentPos := m.getCurrentNavPosition()
	if currentPos >= 0 && currentPos < len(items) {
		item := items[currentPos]
		return item.itemType == itemType && item.index == index
	}
	return false
}

// navigateSidebar moves cursor up/down in sidebar with wrap-around
func (m *model) navigateSidebar(delta int) {
	items := m.getNavItems()
	if len(items) == 0 {
		return
	}
	currentPos := m.getCurrentNavPosition()
	newPos := (currentPos + delta) % len(items)
	if newPos < 0 {
		newPos += len(items)
	}
	m.selected = items[newPos].index
}

// nick returns username for display
func (m *model) nick(userID string) string {
	if userID == "" {
		return "unknown"
	}
	if user, ok := m.users[userID]; ok {
		if user.Username != "" {
			return user.Username
		}
	}
	// Fetch and cache
	if m.platform != nil {
		if user, err := m.platform.GetUser(userID); err == nil && user != nil {
			m.users[userID] = user
			if user.Username != "" {
				return user.Username
			}
		}
	}
	// Fallback
	if len(userID) > 8 {
		return userID[:8]
	}
	return userID
}

func (m model) View() string {
	if !m.connected {
		if m.err != nil {
			return fmt.Sprintf("Error: %v\n\nPress Ctrl+C to quit.", m.err)
		}
		return "Connecting to Mattermost...\n"
	}

	// Default dimensions
	width := m.width
	if width == 0 {
		width = 80
	}
	height := m.height
	if height == 0 {
		height = 24
	}

	// Layout: sidebar | messages | scrollbar
	sidebarWidth := 20
	if width < 50 {
		sidebarWidth = 15
	}
	scrollbarWidth := 1
	// Main width fills the space between sidebar and scrollbar
	mainWidth := width - sidebarWidth - scrollbarWidth - 1 // -1 for separator

	var left, right strings.Builder

	// LEFT: Teams, Channels, and DMs list
	// Teams section
	teamHeader := "=Teams="
	if m.focus == focusSidebar {
		teamHeader = "[Teams]"
	}
	left.WriteString(teamHeader + "\n")
	for i, team := range m.teams {
		name := team.DisplayName
		if name == "" {
			name = team.Name
		}
		if len(name) > sidebarWidth-3 {
			name = name[:sidebarWidth-4] + "~"
		}
		// Marker: * for cursor, > for active team
		marker := " "
		baseText := fmt.Sprintf("%s%s", marker, name)

		// Show marker based on state
		if m.teamSelected && i == m.currentTeam {
			// This is the active team
			marker = ">"
			baseText = fmt.Sprintf("%s%s", marker, name)
			if len(baseText) < sidebarWidth {
				baseText += strings.Repeat(" ", sidebarWidth-len(baseText))
			}
			left.WriteString(currentStyle.Render(baseText) + "\n")
		} else if m.isItemSelected(navTeam, i) {
			// Cursor is on this team
			marker = "*"
			baseText = fmt.Sprintf("%s%s", marker, name)
			if len(baseText) < sidebarWidth {
				baseText += strings.Repeat(" ", sidebarWidth-len(baseText))
			}
			left.WriteString(selectedStyle.Render(baseText) + "\n")
		} else {
			if len(baseText) < sidebarWidth {
				baseText += strings.Repeat(" ", sidebarWidth-len(baseText))
			}
			left.WriteString(baseText + "\n")
		}
	}
	left.WriteString("\n")

	// Channels section
	header := "=Channels="
	if m.focus == focusSidebar {
		header = "[Channels]"
	}
	left.WriteString(header + "\n")

	if m.teamSelected {
		chCount := 0
		// Channels are already filtered by team from GetChannels() - no need to filter again
		for i, ch := range m.channels {
			if ch.Type == comm.ChannelTypeDirectMessage || ch.Type == comm.ChannelTypeGroupMessage {
				continue
			}
			name := ch.DisplayName
			if name == "" {
				name = ch.Name
			}
			if len(name) > sidebarWidth-3 {
				name = name[:sidebarWidth-4] + "~"
			}
			// Marker: * for cursor, > for current active channel
			marker := " "
			baseText := fmt.Sprintf("%s%d:%s", marker, chCount+1, name)
			if i == m.current {
				marker = ">"
				baseText = fmt.Sprintf("%s%d:%s", marker, chCount+1, name)
				if len(baseText) < sidebarWidth {
					baseText += strings.Repeat(" ", sidebarWidth-len(baseText))
				}
				left.WriteString(currentStyle.Render(baseText) + "\n")
			} else if m.isItemSelected(navChannel, i) {
				marker = "*"
				baseText = fmt.Sprintf("%s%d:%s", marker, chCount+1, name)
				if len(baseText) < sidebarWidth {
					baseText += strings.Repeat(" ", sidebarWidth-len(baseText))
				}
				left.WriteString(selectedStyle.Render(baseText) + "\n")
			} else {
				if len(baseText) < sidebarWidth {
					baseText += strings.Repeat(" ", sidebarWidth-len(baseText))
				}
				left.WriteString(baseText + "\n")
			}
			chCount++
			if chCount >= 9 {
				break
			}
		}
	}

	// DMs section
	dmHeader := "\n=DMs="
	if m.focus == focusSidebar {
		dmHeader = "\n[DMs]"
	}
	left.WriteString(dmHeader + "\n")

	if m.teamSelected {
		dmCount := 0
		for i, ch := range m.channels {
			if ch.Type != comm.ChannelTypeDirectMessage && ch.Type != comm.ChannelTypeGroupMessage {
				continue
			}
			name := ch.DisplayName
			if len(name) > sidebarWidth-3 {
				name = name[:sidebarWidth-4] + "~"
			}
			// Marker: * for cursor, > for current active DM
			marker := " "
			baseText := fmt.Sprintf("%s%s", marker, name)
			if i == m.current {
				marker = ">"
				baseText = fmt.Sprintf("%s%s", marker, name)
				if len(baseText) < sidebarWidth {
					baseText += strings.Repeat(" ", sidebarWidth-len(baseText))
				}
				left.WriteString(currentStyle.Render(baseText) + "\n")
			} else if m.isItemSelected(navDM, i) {
				marker = "*"
				baseText = fmt.Sprintf("%s%s", marker, name)
				if len(baseText) < sidebarWidth {
					baseText += strings.Repeat(" ", sidebarWidth-len(baseText))
				}
				left.WriteString(selectedStyle.Render(baseText) + "\n")
			} else {
				if len(baseText) < sidebarWidth {
					baseText += strings.Repeat(" ", sidebarWidth-len(baseText))
				}
				left.WriteString(baseText + "\n")
			}
			dmCount++
			if dmCount >= 5 {
				break
			}
		}
	}

	// RIGHT: Messages + Input
	// Get channel name for input line
	channel := ""
	if len(m.channels) > 0 && m.current >= 0 && m.current < len(m.channels) {
		ch := m.channels[m.current]
		name := ch.DisplayName
		if name == "" {
			name = ch.Name
		}
		channel = name
	}

	// Messages with scroll support
	// Reserve space for input line (messages + input)
	msgHeight := height - 1
	if msgHeight < 3 {
		msgHeight = 3
	}

	// scrollOffset=0 means bottom (latest), >0 means scrolled up
	totalMsgs := len(m.messages)
	end := totalMsgs - m.scrollOffset
	start := end - msgHeight
	if start < 0 {
		start = 0
	}
	if end > totalMsgs {
		end = totalMsgs
	}

	displayCount := 0
	for i := start; i < end; i++ {
		msg := m.messages[i]
		t := msg.CreatedAt.Format("15:04")
		nick := m.nick(msg.SenderID)
		text := msg.Text

		line := fmt.Sprintf("%s %s %s",
			timeStyle.Render(t),
			nickStyle.Render(fmt.Sprintf("<%s>", nick)),
			text)

		if len(line) > mainWidth {
			line = line[:mainWidth-3] + "..."
		}
		right.WriteString(line)
		right.WriteString("\n")
		displayCount++
	}

	// Fill empty lines
	for i := displayCount; i < msgHeight; i++ {
		right.WriteString("\n")
	}

	// Note: Input will be rendered directly in the combining loop
	// to ensure it's always at the bottom line

	// Combine left and right
	leftLines := strings.Split(left.String(), "\n")
	rightLines := strings.Split(right.String(), "\n")

	var b strings.Builder
	for i := 0; i < height; i++ {
		// Left side
		// Note: We already padded text before styling, so don't pad again
		// as that would break ANSI codes
		if i < len(leftLines) {
			line := leftLines[i]
			// Check if line has ANSI codes (styled)
			if strings.Contains(line, "\x1b[") {
				// Already padded before styling, use as-is
				b.WriteString(line)
			} else {
				// No styling, apply normal padding
				if len(line) < sidebarWidth {
					line += strings.Repeat(" ", sidebarWidth-len(line))
				} else if len(line) > sidebarWidth {
					line = line[:sidebarWidth]
				}
				b.WriteString(line)
			}
		} else {
			b.WriteString(strings.Repeat(" ", sidebarWidth))
		}

		b.WriteString("|")

		// Right side - messages fill to mainWidth, then scrollbar at far right
		var msgLine string
		if i == height-1 {
			// Always render input at bottom (last line)
			// Replace newlines with visible marker for display
			displayInput := strings.ReplaceAll(m.input, "\n", "↵")
			// Add cursor - blink when focused on main
			runes := []rune(displayInput)
			var inputWithCursor string
			cursorChar := " "
			if m.focus == focusMain && m.cursorVisible {
				cursorChar = "█"
			} else if m.focus == focusMain {
				cursorChar = " "
			} else {
				// Not focused, show static cursor
				cursorChar = "█"
			}
			if m.cursorPos >= len(runes) {
				inputWithCursor = displayInput + cursorChar
			} else {
				inputWithCursor = string(runes[:m.cursorPos]) + cursorChar + string(runes[m.cursorPos:])
			}
			inputLine := fmt.Sprintf("[%s] %s", channel, inputWithCursor)
			if len(inputLine) > mainWidth {
				inputLine = inputLine[:mainWidth]
			}
			msgLine = inputStyle.Render(inputLine)
		} else if i < len(rightLines) {
			// Show messages from rightLines
			msgLine = rightLines[i]
		} else {
			// Empty line
			msgLine = ""
		}

		// Pad message line to mainWidth (accounting for ANSI codes if styled)
		// For simplicity, render as-is and pad with spaces
		b.WriteString(msgLine)

		// Calculate visible width (rough - lipgloss may add ANSI codes)
		// For now, pad to ensure proper alignment
		visibleLen := len(msgLine)
		// Strip ANSI codes for length calculation (simple approach)
		if strings.Contains(msgLine, "\x1b[") {
			// Has ANSI codes, approximate visible length
			visibleLen = 0
			inEscape := false
			for _, ch := range msgLine {
				if ch == '\x1b' {
					inEscape = true
				} else if inEscape && ch == 'm' {
					inEscape = false
				} else if !inEscape {
					visibleLen++
				}
			}
		}

		if visibleLen < mainWidth {
			b.WriteString(strings.Repeat(" ", mainWidth-visibleLen))
		}

		// Add scrollbar at far right
		if i == height-1 {
			b.WriteString(" ") // No scrollbar on input line
		} else {
			b.WriteString(scrollbarChar(i, msgHeight, m.scrollOffset, len(m.messages)))
		}

		if i < height-1 {
			b.WriteString("\n")
		}
	}

	return b.String()
}

func main() {
	// Parse CLI flags - NO environment variable fallbacks
	host := flag.String("host", "", "Mattermost server host (e.g., chat.example.com)")
	token := flag.String("token", "", "Personal Access Token")
	user := flag.String("user", "", "Username or email for login")
	pass := flag.String("pass", "", "Password for login")
	teamID := flag.String("teamid", "", "Team ID (optional)")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "termunicator - irssi-style TUI for Mattermost\n\n")
		fmt.Fprintf(os.Stderr, "Usage: termunicator -host HOST [-token TOKEN | -user USER -pass PASS]\n\n")
		fmt.Fprintf(os.Stderr, "Options:\n")
		flag.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\nKeys:\n")
		fmt.Fprintf(os.Stderr, "  Ctrl+B         Switch focus (sidebar/main)\n")
		fmt.Fprintf(os.Stderr, "\n  Sidebar focus:\n")
		fmt.Fprintf(os.Stderr, "    Up/Down      Select channel (* marker)\n")
		fmt.Fprintf(os.Stderr, "    Space        Switch to selected (> marker)\n")
		fmt.Fprintf(os.Stderr, "\n  Main focus:\n")
		fmt.Fprintf(os.Stderr, "    Up/Down      Scroll by line (auto-fetch older)\n")
		fmt.Fprintf(os.Stderr, "    PgUp/PgDown  Scroll by page (auto-fetch older)\n")
		fmt.Fprintf(os.Stderr, "    Enter        Send message\n")
		fmt.Fprintf(os.Stderr, "    Ctrl+Enter   New line in message\n")
		fmt.Fprintf(os.Stderr, "    Backspace    Delete character\n")
		fmt.Fprintf(os.Stderr, "    (any key)    Type message\n")
		fmt.Fprintf(os.Stderr, "\n  Ctrl+C         Quit\n")
	}

	flag.Parse()

	// Validate required flags
	if *host == "" {
		fmt.Fprintf(os.Stderr, "Error: -host is required\n\n")
		flag.Usage()
		os.Exit(1)
	}

	cfg := config{
		host:     *host,
		token:    *token,
		loginID:  *user,
		password: *pass,
		teamID:   *teamID,
	}

	p := tea.NewProgram(initialModel(cfg))
	if _, err := p.Run(); err != nil {
		log.Fatal(err)
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
