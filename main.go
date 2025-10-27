package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	comm "libcommunicator"
)

// Constants - Pike/Cox: named constants instead of magic numbers
const (
	// Message fetching
	messageFetchLimit     = 50
	messagePageJumpMin    = 5
	messagePageJumpDiv    = 2
	messagePrefetchBuffer = 3 // Fetch older when within this many messages of top

	// UI dimensions
	defaultWidth        = 80
	defaultHeight       = 24
	sidebarWidth        = 20
	sidebarWidthSmall   = 15
	minMainWidth        = 20
	minMessageHeight    = 3
	maxChannelsDisplay  = 9
	maxDMsDisplay       = 5
	minWidthForFullSide = 50

	// Input and formatting
	timeWidth           = 5 // "HH:MM"
	nickPrefixLen       = 1 // "<"
	nickSuffixLen       = 2 // "> "
	ellipsisLen         = 3
	minTruncateWidth    = 3
	userIDTruncateLen   = 8
	printableCharMin    = 32
	printableCharMax    = 126

	// Timing
	cursorBlinkInterval      = 500 * time.Millisecond
	eventStreamBufferSize    = 100
	eventStreamDebounceDelay = 100 * time.Millisecond
)

// Pike/Cox: group related globals into a struct for clarity
type styles struct {
	status      lipgloss.Style
	nick        lipgloss.Style
	time        lipgloss.Style
	input       lipgloss.Style
	activity    lipgloss.Style
	current     lipgloss.Style
	selected    lipgloss.Style
	highlighted lipgloss.Style
}

// irssi-style colors - simple terminal colors
var style = styles{
	status:      lipgloss.NewStyle().Foreground(lipgloss.Color("15")).Background(lipgloss.Color("4")), // white on blue
	nick:        lipgloss.NewStyle().Foreground(lipgloss.Color("10")),                                 // green
	time:        lipgloss.NewStyle().Foreground(lipgloss.Color("8")),                                  // gray
	input:       lipgloss.NewStyle().Foreground(lipgloss.Color("15")),                                 // white
	activity:    lipgloss.NewStyle().Foreground(lipgloss.Color("11")),                                 // yellow
	current:     lipgloss.NewStyle().Foreground(lipgloss.Color("11")).Bold(true),                      // yellow bold for current
	selected:    lipgloss.NewStyle().Foreground(lipgloss.Color("14")).Bold(true),                      // cyan bold for selected
	highlighted: lipgloss.NewStyle().Foreground(lipgloss.Color("0")).Background(lipgloss.Color("14")), // black on cyan for highlighted message
}

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
	selected      int                   // selected item index (in its array)
	selectedType  navItemType           // type of selected item
	focus         focusArea             // which window has focus
	scrollOffset  int                   // scroll position in message list (0 = bottom)
	messageCursor int                   // selected message index in display messages (-1 = none)
	input         string
	cursorPos     int  // cursor position in input
	teamSelected  bool // whether a team has been selected
	cursorVisible bool // for blinking cursor
	err           error
	connected     bool
	ctx           context.Context
	cancel        context.CancelFunc
	width         int
	height        int
	config        config
	// Performance caches (Pike/Cox: avoid repeated allocations)
	displayMsgsCache []comm.Message // cached filtered messages
	displayMsgsDirty bool           // true when messages changed
	navItemsCache    []navItem      // cached nav items
	navItemsDirty    bool           // true when teams/channels changed
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
		ctx:              ctx,
		cancel:           cancel,
		users:            make(map[string]*comm.User),
		config:           cfg,
		focus:            focusSidebar, // Start with sidebar focused for team selection
		current:          -1,            // No channel selected initially
		selected:         0,             // Start at first item
		selectedType:     navTeam,       // Start on teams
		messageCursor:    -1,            // No message selected initially
		cursorVisible:    true,               // Start with cursor visible
		width:            defaultWidth,       // Default width
		height:           defaultHeight,      // Default height
		displayMsgsDirty: true,          // Force initial cache build
		navItemsDirty:    true,          // Force initial cache build
	}
}

func (m model) Init() tea.Cmd {
	return tea.Batch(m.connectToMattermost, tickCmd())
}

// tickCmd returns a command that sends a tick message for cursor blinking
func tickCmd() tea.Cmd {
	return tea.Tick(cursorBlinkInterval, func(t time.Time) tea.Msg {
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
	eventStream, err := platform.NewEventStream(ctx, eventStreamBufferSize, eventStreamDebounceDelay)
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
		key := msg.String()

		// Try global keys first (ctrl+c, ctrl+b)
		if newModel, cmd, handled := m.handleGlobalKeys(key); handled {
			return newModel, cmd
		}

		// Try sidebar-specific keys
		if newModel, cmd, handled := m.handleSidebarKeys(key); handled {
			return newModel, cmd
		}

		// Try main area keys
		if newModel, cmd, handled := m.handleMainKeys(key); handled {
			return newModel, cmd
		}

		// Try regular character input
		if newModel, cmd, handled := m.handleInputChar(key); handled {
			return newModel, cmd
		}

	case connectedMsg:
		m.platform = msg.platform
		m.eventStream = msg.eventStream
		m.teams = msg.teams
		m.channels = msg.channels
		m.connected = true
		m.navItemsDirty = true // Invalidate nav cache
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
					// If at bottom, stay at bottom to show new message
					wasAtBottom := m.scrollOffset == 0
					m.messages = append(m.messages, newMsg)
					m.displayMsgsDirty = true // Invalidate cache
					if wasAtBottom {
						m.scrollOffset = 0
					} else {
						m.scrollOffset = m.clampScrollOffset(m.scrollOffset)
					}
				}
			}
		}

	case messagesMsg:
		log.Printf("messagesMsg: received %d messages for channel", len(msg))

		// Count how many are displayable (root posts only)
		displayCount := 0
		threadReplyCount := 0
		for _, newMsg := range msg {
			if isThreadReply(newMsg) {
				threadReplyCount++
			} else {
				displayCount++
			}
		}
		log.Printf("messagesMsg: %d root posts, %d thread replies", displayCount, threadReplyCount)

		m.messages = msg
		m.displayMsgsDirty = true // Invalidate cache
		m.scrollOffset = 0        // Reset scroll to bottom (newest messages) when loading new channel
		m.messageCursor = -1      // Reset cursor when messages are replaced

		// If no root posts in initial load, fetch older messages
		if displayCount == 0 && len(msg) > 0 && m.current >= 0 && m.current < len(m.channels) {
			log.Printf("messagesMsg: no root posts in initial load, fetching older...")
			oldestMsg := msg[0]
			return m, fetchOlderMessages(m.platform, m.channels[m.current].ID, oldestMsg.ID)
		} else if displayCount > 0 {
			log.Printf("messagesMsg: showing %d root posts", displayCount)
		} else {
			log.Printf("messagesMsg: channel is empty")
		}

	case olderMessagesMsg:
		// Prepend older messages to the beginning (with deduplication)
		log.Printf("olderMessagesMsg: received %d messages from server", len(msg))
		if len(msg) > 0 {
			// Log first and last message IDs for pagination tracking
			if len(msg) > 0 {
				log.Printf("olderMessagesMsg: first message ID=%s, last message ID=%s", msg[0].ID, msg[len(msg)-1].ID)
			}

			// Server returned messages - deduplicate them
			newMessages := make([]comm.Message, 0, len(msg))
			duplicateCount := 0
			for _, fetchedMsg := range msg {
				exists := false
				for _, existingMsg := range m.messages {
					if existingMsg.ID == fetchedMsg.ID {
						exists = true
						duplicateCount++
						break
					}
				}
				if !exists {
					newMessages = append(newMessages, fetchedMsg)
				}
			}

			log.Printf("olderMessagesMsg: %d new messages after dedup (%d duplicates)", len(newMessages), duplicateCount)

			// Count how many of the new messages will be displayed (only root posts)
			displayCount := 0
			threadReplyCount := 0
			for _, newMsg := range newMessages {
				if isThreadReply(newMsg) {
					threadReplyCount++
					// Log details about thread replies
					if newMsg.Metadata != nil {
						if meta, ok := newMsg.Metadata.(map[string]interface{}); ok {
							rootID, _ := meta["root_id"].(string)
							log.Printf("  Thread reply: ID=%s, root_id=%s", newMsg.ID, rootID)
						}
					}
				} else {
					displayCount++
					log.Printf("  Root post: ID=%s, text=%s", newMsg.ID, truncate(newMsg.Text, 50))
				}
			}

			log.Printf("olderMessagesMsg: %d root posts, %d thread replies", displayCount, threadReplyCount)

			// Add messages to storage (even if all duplicates, still track for pagination)
			if len(newMessages) > 0 {
				m.messages = append(newMessages, m.messages...)
				m.displayMsgsDirty = true // Invalidate cache
			}

			// Decide what to do based on whether we got displayable root posts
			if displayCount > 0 {
				// Got root posts - show them
				log.Printf("olderMessagesMsg: SUCCESS - showing %d root posts", displayCount)

				if m.messageCursor >= 0 {
					m.messageCursor += displayCount
				}

				// Show new messages at top, keep cursor visible
				showCount := displayCount / 2
				if showCount > m.msgHeight()/2 {
					showCount = m.msgHeight() / 2
				}
				if showCount < 3 && displayCount >= 3 {
					showCount = 3
				}
				m.scrollOffset += displayCount - showCount

				// Ensure cursor stays visible after all adjustments
				m.ensureCursorVisible()
			} else {
				// Server returned messages but no displayable root posts
				// Only continue if we got NEW messages (not all duplicates)
				if len(newMessages) > 0 && m.current >= 0 && m.current < len(m.channels) && len(m.messages) > 0 {
					oldestMsg := m.messages[0]
					log.Printf("olderMessagesMsg: no root posts found, continuing to fetch older (using oldest message ID=%s)", oldestMsg.ID)
					return m, fetchOlderMessages(m.platform, m.channels[m.current].ID, oldestMsg.ID)
				} else {
					if len(newMessages) == 0 {
						log.Printf("olderMessagesMsg: STOP - all messages were duplicates (pagination stuck)")
					} else {
						log.Printf("olderMessagesMsg: no root posts and cannot fetch more (no channel or no messages)")
					}
				}
			}
		} else {
			// Server returned empty - stop trying
			log.Printf("olderMessagesMsg: server returned EMPTY - no more messages available")
		}

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

// Pike/Cox: extract keyboard handlers from Update to reduce function size
// handleGlobalKeys handles keys that work regardless of focus
func (m model) handleGlobalKeys(key string) (tea.Model, tea.Cmd, bool) {
	switch key {
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
		return m, tea.Quit, true

	case "ctrl+b":
		// Toggle focus between sidebar and main
		if m.focus == focusSidebar {
			m.focus = focusMain
		} else {
			m.focus = focusSidebar
		}
		return m, nil, true
	}
	return m, nil, false
}

// handleSidebarKeys handles keyboard input when sidebar is focused
func (m model) handleSidebarKeys(key string) (tea.Model, tea.Cmd, bool) {
	if m.focus != focusSidebar {
		return m, nil, false
	}

	switch key {
	case "up":
		m.navigateSidebar(-1)
		return m, nil, true

	case "down":
		m.navigateSidebar(1)
		return m, nil, true

	case " ":
		if m.selectedType == navTeam {
			// Select team with space key
			if m.selected >= 0 && m.selected < len(m.teams) {
				m.currentTeam = m.selected
				m.teamSelected = true
				// Clear messages and input
				m.messages = nil
				m.input = ""
				m.cursorPos = 0
				m.displayMsgsDirty = true // Invalidate message cache
				m.navItemsDirty = true    // Invalidate nav cache (channels will change)
				// Set team ID in platform and refresh channels
				if err := m.platform.SetTeamID(m.teams[m.currentTeam].ID); err != nil {
					m.err = fmt.Errorf("SetTeamID error: %w", err)
					return m, nil, true
				}
				channels, err := m.platform.GetChannels()
				if err != nil {
					m.err = fmt.Errorf("GetChannels error: %w", err)
					return m, nil, true
				}
				m.channels = channels
				m.current = -1
				// Move cursor to first channel if available
				items := m.getNavItems()
				for _, item := range items {
					if item.itemType == navChannel || item.itemType == navDM {
						m.selected = item.index
						m.selectedType = item.itemType
						break
					}
				}
				if len(channels) == 0 {
					m.err = fmt.Errorf("Warning: GetChannels returned 0 channels for team %s (%s)", m.teams[m.currentTeam].DisplayName, m.teams[m.currentTeam].ID)
				}
			}
		} else if m.selectedType == navChannel || m.selectedType == navDM {
			// Select channel/DM with space key
			if m.selected >= 0 && m.selected < len(m.channels) {
				m.current = m.selected
				log.Printf("User selected channel: %s (ID=%s)", m.channels[m.current].DisplayName, m.channels[m.current].ID)
				m.scrollOffset = 0       // Reset scroll
				m.messageCursor = -1     // Reset message cursor
				m.displayMsgsDirty = true // Invalidate message cache
				// Clear messages and input when switching channel
				m.messages = nil
				m.input = ""
				m.cursorPos = 0
				// Switch focus to main area
				m.focus = focusMain
				return m, fetchMessages(m.platform, m.channels[m.current].ID), true
			}
		}
		return m, nil, true
	}
	return m, nil, false
}

// handleMainKeys handles keyboard input when main area is focused
func (m model) handleMainKeys(key string) (tea.Model, tea.Cmd, bool) {
	if m.focus != focusMain {
		return m, nil, false
	}

	switch key {
	case "enter":
		// Send message
		if m.input == "" || !m.connected || len(m.channels) == 0 || m.current < 0 {
			return m, nil, true
		}
		channelID := m.channels[m.current].ID
		if _, err := m.platform.SendMessage(channelID, m.input); err != nil {
			m.err = err
		}
		m.input = ""
		m.cursorPos = 0
		return m, fetchMessages(m.platform, channelID), true

	case "up":
		displayMsgs := m.getDisplayMessages()
		if len(displayMsgs) == 0 {
			return m, nil, true
		}
		if m.messageCursor == -1 {
			// Start from the last visible message
			totalMsgs := len(displayMsgs)
			end := totalMsgs - m.scrollOffset
			if end > 0 {
				m.messageCursor = end - 1
			}
			// Ensure cursor is in valid range
			if m.messageCursor < 0 {
				m.messageCursor = 0
			}
			if m.messageCursor >= totalMsgs {
				m.messageCursor = totalMsgs - 1
			}
		} else if m.messageCursor > 0 {
			// Move to previous message
			m.messageCursor--
			// Auto-scroll to keep cursor visible
			m.ensureCursorVisible()
		} else if m.messageCursor == 0 {
			// At first displayed message
			// Only try to scroll up if we have loaded messages above
			if m.scrollOffset < m.maxScroll() {
				// Can scroll up to show older messages that are already loaded
				m.scrollOffset = m.clampScrollOffset(m.scrollOffset + 1)
			} else if m.scrollOffset >= m.maxScroll() && len(m.messages) > 0 && m.current >= 0 && m.current < len(m.channels) {
				// At max scroll - try to fetch older messages from server
				// Cursor stays at 0, will only move if server returns root posts
				log.Printf("up arrow: fetching older messages (at top)")
				oldestMsg := m.messages[0]
				return m, fetchOlderMessages(m.platform, m.channels[m.current].ID, oldestMsg.ID), true
			}
			// If already at absolute top, do nothing (keep cursor at 0, visible)
		}
		return m, nil, true

	case "down":
		displayMsgs := m.getDisplayMessages()
		if len(displayMsgs) == 0 {
			return m, nil, true
		}

		if m.messageCursor == -1 {
			// In input mode, down scrolls down if scrolled up
			if m.scrollOffset > 0 {
				m.scrollOffset = m.clampScrollOffset(m.scrollOffset - 1)
			}
		} else if m.messageCursor < len(displayMsgs)-1 {
			// Move to next message
			m.messageCursor++
			// Auto-scroll to keep cursor visible
			m.ensureCursorVisible()
		} else if m.messageCursor == len(displayMsgs)-1 {
			// At last message
			if m.scrollOffset > 0 {
				// If scrolled up, scroll down to show newer messages
				m.scrollOffset = m.clampScrollOffset(m.scrollOffset - 1)
			}
			// If at newest message (scrollOffset == 0), stay on current message
			// New messages are handled by real-time events
		}
		return m, nil, true

	case "pgup":
		displayMsgs := m.getDisplayMessages()
		if len(displayMsgs) == 0 {
			return m, nil, true
		}

		// Move by half page for smoother navigation
		jumpSize := m.msgHeight() / messagePageJumpDiv
		if jumpSize < messagePageJumpMin {
			jumpSize = messagePageJumpMin
		}

		// If no cursor, start at last visible message
		if m.messageCursor == -1 {
			totalMsgs := len(displayMsgs)
			end := totalMsgs - m.scrollOffset
			if end > 0 {
				m.messageCursor = end - 1
			} else {
				m.messageCursor = 0
			}
		}

		// Move cursor up by jump size
		m.messageCursor -= jumpSize
		if m.messageCursor < 0 {
			m.messageCursor = 0
		}

		// Ensure cursor visible
		m.ensureCursorVisible()

		// If near top, proactively fetch older messages
		if m.messageCursor < messagePrefetchBuffer && len(m.messages) > 0 && m.current >= 0 && m.current < len(m.channels) {
			log.Printf("pgup: fetching older messages (near top)")
			oldestMsg := m.messages[0]
			return m, fetchOlderMessages(m.platform, m.channels[m.current].ID, oldestMsg.ID), true
		}
		return m, nil, true

	case "pgdown":
		displayMsgs := m.getDisplayMessages()
		if len(displayMsgs) == 0 {
			return m, nil, true
		}

		// Move by half page for smoother navigation
		jumpSize := m.msgHeight() / messagePageJumpDiv
		if jumpSize < messagePageJumpMin {
			jumpSize = messagePageJumpMin
		}

		// If no cursor, start at last visible message
		if m.messageCursor == -1 {
			totalMsgs := len(displayMsgs)
			end := totalMsgs - m.scrollOffset
			if end > 0 {
				m.messageCursor = end - 1
			} else {
				m.messageCursor = 0
			}
		}

		// Move cursor down by jump size
		m.messageCursor += jumpSize
		if m.messageCursor >= len(displayMsgs) {
			m.messageCursor = len(displayMsgs) - 1
		}

		// Ensure cursor visible
		m.ensureCursorVisible()
		return m, nil, true

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
		return m, nil, true

	case "ctrl+enter", "ctrl+m":
		// Ctrl+Enter adds newline in typing section
		runes := []rune(m.input)
		m.input = string(runes[:m.cursorPos]) + "\n" + string(runes[m.cursorPos:])
		m.cursorPos++
		return m, nil, true

	case " ":
		// In main area, space is part of input
		m.input += " "
		m.cursorPos++
		return m, nil, true
	}
	return m, nil, false
}

// handleInputChar handles regular character input in main area
func (m model) handleInputChar(str string) (tea.Model, tea.Cmd, bool) {
	if m.focus != focusMain {
		return m, nil, false
	}

	// Ignore ctrl and alt combinations
	if strings.HasPrefix(str, "ctrl+") || strings.HasPrefix(str, "alt+") {
		return m, nil, false
	}

	// Only add single printable characters
	if len(str) == 1 && str[0] >= printableCharMin && str[0] <= printableCharMax {
		runes := []rune(m.input)
		m.input = string(runes[:m.cursorPos]) + str + string(runes[m.cursorPos:])
		m.cursorPos++
		return m, nil, true
	}
	return m, nil, false
}

func fetchMessages(platform *comm.Platform, channelID string) tea.Cmd {
	return func() tea.Msg {
		log.Printf("fetchMessages: requesting initial messages for channel %s", channelID)
		messages, err := platform.GetMessages(channelID, messageFetchLimit)
		if err != nil {
			log.Printf("fetchMessages: error: %v", err)
			return errMsg(err)
		}
		log.Printf("fetchMessages: received %d messages", len(messages))
		return messagesMsg(messages)
	}
}

func fetchOlderMessages(platform *comm.Platform, channelID, beforeID string) tea.Cmd {
	return func() tea.Msg {
		log.Printf("fetchOlderMessages: requesting messages before ID=%s", beforeID)
		messages, err := platform.GetMessagesBefore(channelID, beforeID, messageFetchLimit)
		if err != nil {
			log.Printf("fetchOlderMessages: error: %v", err)
			return errMsg(err)
		}
		log.Printf("fetchOlderMessages: received %d messages", len(messages))
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

// getDisplayMessages returns messages to display (filters thread replies)
// Pike/Cox: cache filtered results to avoid repeated allocations
func (m *model) getDisplayMessages() []comm.Message {
	if !m.displayMsgsDirty {
		return m.displayMsgsCache
	}
	// Filter thread replies in both channels and DMs
	filtered := make([]comm.Message, 0, len(m.messages))
	for _, msg := range m.messages {
		if !isThreadReply(msg) {
			filtered = append(filtered, msg)
		}
	}
	m.displayMsgsCache = filtered
	m.displayMsgsDirty = false
	return filtered
}

// ensureCursorVisible adjusts scroll offset to keep message cursor visible
func (m *model) ensureCursorVisible() {
	if m.messageCursor == -1 {
		// No cursor, reset to bottom
		m.scrollOffset = 0
		return
	}

	displayMsgs := m.getDisplayMessages()
	if len(displayMsgs) == 0 {
		return
	}

	msgHeight := m.msgHeight()
	totalMsgs := len(displayMsgs)

	// Calculate visible range using same logic as View()
	// Work backward from end, counting screen lines
	end := totalMsgs - m.scrollOffset
	if end > totalMsgs {
		end = totalMsgs
	}
	if end < 0 {
		end = 0
	}

	linesUsed := 0
	start := end
	for start > 0 && linesUsed < msgHeight {
		msgIdx := start - 1
		msg := displayMsgs[msgIdx]
		msgLines := len(strings.Split(msg.Text, "\n"))
		if linesUsed+msgLines > msgHeight && linesUsed > 0 {
			break
		}
		linesUsed += msgLines
		start--
	}

	// If cursor is above visible area, scroll up to show it
	if m.messageCursor < start {
		m.scrollOffset = totalMsgs - m.messageCursor - 1
	}

	// If cursor is below visible area, scroll down to show it
	if m.messageCursor >= end {
		m.scrollOffset = totalMsgs - m.messageCursor - 1
	}

	// Clamp scroll offset
	m.scrollOffset = m.clampScrollOffset(m.scrollOffset)
}

// msgHeight returns the height available for messages
func (m model) msgHeight() int {
	// Use actual terminal height, reserve 1 line for input
	h := m.height - 1
	if h < minMessageHeight {
		h = minMessageHeight
	}
	return h
}

// maxScroll returns the maximum scroll offset (in messages)
func (m model) maxScroll() int {
	displayMsgs := m.getDisplayMessages()
	totalMsgs := len(displayMsgs)
	if totalMsgs == 0 {
		return 0
	}

	msgHeight := m.msgHeight()

	// Work forward from start, counting lines to see how many messages fit
	linesUsed := 0
	msgsFit := 0
	for i := 0; i < totalMsgs; i++ {
		msg := displayMsgs[i]
		msgLines := len(strings.Split(msg.Text, "\n"))
		if linesUsed+msgLines > msgHeight && msgsFit > 0 {
			// This message won't fit
			break
		}
		linesUsed += msgLines
		msgsFit++
		if linesUsed >= msgHeight {
			break
		}
	}

	// maxScroll is how many messages we can skip from the end
	max := totalMsgs - msgsFit
	if max < 0 {
		return 0
	}
	return max
}

// clampScrollOffset ensures scroll offset is within valid bounds
func (m model) clampScrollOffset(offset int) int {
	if offset < 0 {
		return 0
	}
	max := m.maxScroll()
	if offset > max {
		return max
	}
	return offset
}

// getNavItems returns all navigable items in sidebar order
// Pike/Cox: cache to avoid repeated allocations
func (m *model) getNavItems() []navItem {
	if !m.navItemsDirty {
		return m.navItemsCache
	}
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

	m.navItemsCache = items
	m.navItemsDirty = false
	return items
}

// getCurrentNavPosition returns the current position in the nav list
func (m *model) getCurrentNavPosition() int {
	items := m.getNavItems()
	// Find item matching both type and index
	for i, item := range items {
		if item.itemType == m.selectedType && item.index == m.selected {
			return i
		}
	}
	// Default to first item
	return 0
}

// isItemSelected checks if an item is the currently selected one
func (m *model) isItemSelected(itemType navItemType, index int) bool {
	return m.selectedType == itemType && m.selected == index
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
	newItem := items[newPos]
	m.selected = newItem.index
	m.selectedType = newItem.itemType
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
	if len(userID) > userIDTruncateLen {
		return userID[:userIDTruncateLen]
	}
	return userID
}

func isThreadReply(msg comm.Message) bool {
	// Thread replies have non-empty root_id in metadata
	if msg.Metadata == nil {
		return false
	}
	meta, ok := msg.Metadata.(map[string]interface{})
	if !ok {
		return false
	}
	rootID, ok := meta["root_id"].(string)
	return ok && rootID != ""
}

func (m model) isDMChannel() bool {
	if len(m.channels) == 0 || m.current < 0 || m.current >= len(m.channels) {
		return false
	}
	ch := m.channels[m.current]
	return ch.Type == comm.ChannelTypeDirectMessage || ch.Type == comm.ChannelTypeGroupMessage
}

// Pike/Cox: extract rendering functions from View to reduce function size
// renderSidebar renders the teams, channels, and DMs sidebar
func (m model) renderSidebar(sidebar int) string {
	var b strings.Builder

	// Teams section
	teamHeader := "=Teams="
	if m.focus == focusSidebar {
		teamHeader = "[Teams]"
	}
	b.WriteString(teamHeader + "\n")
	for i, team := range m.teams {
		name := team.DisplayName
		if name == "" {
			name = team.Name
		}
		if len(name) > sidebar-3 {
			name = name[:sidebar-4] + "~"
		}
		// Marker: * for cursor, > for active team
		marker := " "
		baseText := fmt.Sprintf("%s%s", marker, name)

		// Show marker based on state
		if m.teamSelected && i == m.currentTeam {
			// This is the active team
			marker = ">"
			baseText = fmt.Sprintf("%s%s", marker, name)
			if len(baseText) < sidebar {
				baseText += strings.Repeat(" ", sidebar-len(baseText))
			}
			b.WriteString(style.current.Render(baseText) + "\n")
		} else if m.isItemSelected(navTeam, i) {
			// Cursor is on this team
			marker = "*"
			baseText = fmt.Sprintf("%s%s", marker, name)
			if len(baseText) < sidebar {
				baseText += strings.Repeat(" ", sidebar-len(baseText))
			}
			b.WriteString(style.selected.Render(baseText) + "\n")
		} else {
			if len(baseText) < sidebar {
				baseText += strings.Repeat(" ", sidebar-len(baseText))
			}
			b.WriteString(baseText + "\n")
		}
	}
	b.WriteString("\n")

	// Channels section
	header := "=Channels="
	if m.focus == focusSidebar {
		header = "[Channels]"
	}
	b.WriteString(header + "\n")

	if m.teamSelected {
		chCount := 0
		for i, ch := range m.channels {
			if ch.Type == comm.ChannelTypeDirectMessage || ch.Type == comm.ChannelTypeGroupMessage {
				continue
			}
			name := ch.DisplayName
			if name == "" {
				name = ch.Name
			}
			if len(name) > sidebar-3 {
				name = name[:sidebar-4] + "~"
			}
			// Marker: * for cursor, > for current active channel
			marker := " "
			baseText := fmt.Sprintf("%s%d:%s", marker, chCount+1, name)
			if i == m.current {
				marker = ">"
				baseText = fmt.Sprintf("%s%d:%s", marker, chCount+1, name)
				if len(baseText) < sidebar {
					baseText += strings.Repeat(" ", sidebar-len(baseText))
				}
				b.WriteString(style.current.Render(baseText) + "\n")
			} else if m.isItemSelected(navChannel, i) {
				marker = "*"
				baseText = fmt.Sprintf("%s%d:%s", marker, chCount+1, name)
				if len(baseText) < sidebar {
					baseText += strings.Repeat(" ", sidebar-len(baseText))
				}
				b.WriteString(style.selected.Render(baseText) + "\n")
			} else {
				if len(baseText) < sidebar {
					baseText += strings.Repeat(" ", sidebar-len(baseText))
				}
				b.WriteString(baseText + "\n")
			}
			chCount++
			if chCount >= maxChannelsDisplay {
				break
			}
		}
	}

	// DMs section
	dmHeader := "\n=DMs="
	if m.focus == focusSidebar {
		dmHeader = "\n[DMs]"
	}
	b.WriteString(dmHeader + "\n")

	if m.teamSelected {
		dmCount := 0
		for i, ch := range m.channels {
			if ch.Type != comm.ChannelTypeDirectMessage && ch.Type != comm.ChannelTypeGroupMessage {
				continue
			}
			name := ch.DisplayName
			if len(name) > sidebar-3 {
				name = name[:sidebar-4] + "~"
			}
			// Marker: * for cursor, > for current active DM
			marker := " "
			baseText := fmt.Sprintf("%s%s", marker, name)
			if i == m.current {
				marker = ">"
				baseText = fmt.Sprintf("%s%s", marker, name)
				if len(baseText) < sidebar {
					baseText += strings.Repeat(" ", sidebar-len(baseText))
				}
				b.WriteString(style.current.Render(baseText) + "\n")
			} else if m.isItemSelected(navDM, i) {
				marker = "*"
				baseText = fmt.Sprintf("%s%s", marker, name)
				if len(baseText) < sidebar {
					baseText += strings.Repeat(" ", sidebar-len(baseText))
				}
				b.WriteString(style.selected.Render(baseText) + "\n")
			} else {
				if len(baseText) < sidebar {
					baseText += strings.Repeat(" ", sidebar-len(baseText))
				}
				b.WriteString(baseText + "\n")
			}
			dmCount++
			if dmCount >= maxDMsDisplay {
				break
			}
		}
	}

	return b.String()
}

// renderMessages renders the message area with proper scrolling
func (m model) renderMessages(mainWidth, msgHeight int) string {
	var b strings.Builder

	displayMsgs := m.getDisplayMessages()
	totalMsgs := len(displayMsgs)
	end := totalMsgs - m.scrollOffset
	if end > totalMsgs {
		end = totalMsgs
	}
	if end < 0 {
		end = 0
	}

	// Work backward from 'end', counting screen lines used
	linesUsed := 0
	start := end
	for start > 0 && linesUsed < msgHeight {
		msgIdx := start - 1
		msg := displayMsgs[msgIdx]
		msgLines := len(strings.Split(msg.Text, "\n"))
		if linesUsed+msgLines > msgHeight && linesUsed > 0 {
			// This message won't fit, stop here
			break
		}
		linesUsed += msgLines
		start--
	}

	// Fill empty lines at top (for bottom alignment)
	for i := 0; i < msgHeight-linesUsed; i++ {
		b.WriteString("\n")
	}

	// Render messages at bottom with multi-line support
	for i := start; i < end; i++ {
		msg := displayMsgs[i]
		t := msg.CreatedAt.Format("15:04")
		nick := m.nick(msg.SenderID)
		text := msg.Text

		// Handle multi-line messages
		lines := strings.Split(text, "\n")
		isHighlighted := i == m.messageCursor

		for lineIdx, textLine := range lines {
			var line string
			if lineIdx == 0 {
				// First line: show time and nick
				timeStr := t
				nickStr := fmt.Sprintf("<%s>", nick)
				prefixWidth := len(timeStr) + 1 + len(nickStr) + 1 // "HH:MM <nick> "
				availableWidth := mainWidth - prefixWidth
				if availableWidth < 0 {
					availableWidth = 0
				}

				// Truncate text if needed, add ellipsis
				if len(textLine) > availableWidth {
					if availableWidth > minTruncateWidth {
						textLine = textLine[:availableWidth-ellipsisLen] + "..."
					} else if availableWidth > 0 {
						textLine = textLine[:availableWidth]
					} else {
						textLine = ""
					}
				}

				if isHighlighted {
					// Use highlighted style for all parts
					line = fmt.Sprintf("%s %s %s",
						style.highlighted.Render(timeStr),
						style.highlighted.Render(nickStr),
						style.highlighted.Render(textLine))
				} else {
					// Use normal styles
					line = fmt.Sprintf("%s %s %s",
						style.time.Render(timeStr),
						style.nick.Render(nickStr),
						textLine)
				}
			} else {
				// Continuation lines: indent
				nickWidth := len(nick) + nickPrefixLen + nickSuffixLen
				indent := strings.Repeat(" ", timeWidth+1+nickWidth)
				availableWidth := mainWidth - len(indent)
				if availableWidth < 0 {
					availableWidth = 0
				}

				// Truncate text if needed, add ellipsis
				if len(textLine) > availableWidth {
					if availableWidth > minTruncateWidth {
						textLine = textLine[:availableWidth-ellipsisLen] + "..."
					} else if availableWidth > 0 {
						textLine = textLine[:availableWidth]
					} else {
						textLine = ""
					}
				}

				if isHighlighted {
					line = style.highlighted.Render(indent + textLine)
				} else {
					line = indent + textLine
				}
			}

			b.WriteString(line)
			b.WriteString("\n")
		}
	}

	return b.String()
}

// renderInput renders the input line with cursor
func (m model) renderInput(mainWidth int, channel string) string {
	displayInput := strings.ReplaceAll(m.input, "\n", "↵")
	runes := []rune(displayInput)
	var inputWithCursor string
	cursorChar := " "
	if m.focus == focusMain && m.cursorVisible {
		cursorChar = "█"
	} else if m.focus == focusMain {
		cursorChar = " "
	} else {
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
	return style.input.Render(inputLine)
}

// combinePanes combines left sidebar and right message area
func (m model) combinePanes(leftStr, rightStr string, sidebar, mainWidth, height int) string {
	leftLines := strings.Split(leftStr, "\n")
	rightLines := strings.Split(rightStr, "\n")

	var b strings.Builder
	for i := 0; i < height; i++ {
		// Left side - always pad to exact sidebar width
		if i < len(leftLines) {
			line := leftLines[i]
			visibleLen := lipgloss.Width(line)
			if visibleLen < sidebar {
				b.WriteString(line)
				b.WriteString(strings.Repeat(" ", sidebar-visibleLen))
			} else if visibleLen > sidebar {
				// Truncate if too long
				b.WriteString(line[:sidebar])
			} else {
				b.WriteString(line)
			}
		} else {
			b.WriteString(strings.Repeat(" ", sidebar))
		}

		b.WriteString("|")

		// Right side - messages fill to mainWidth
		var msgLine string
		if i == height-1 {
			// Input line is passed separately
			msgLine = rightLines[len(rightLines)-1] // Last line is input
		} else if i < len(rightLines)-1 {
			msgLine = rightLines[i]
		} else {
			msgLine = ""
		}

		// Pad message line to exact mainWidth using lipgloss.Width
		b.WriteString(msgLine)
		visibleLen := lipgloss.Width(msgLine)
		if visibleLen < mainWidth {
			b.WriteString(strings.Repeat(" ", mainWidth-visibleLen))
		}

		if i < height-1 {
			b.WriteString("\n")
		}
	}

	return b.String()
}

func (m model) View() string {
	// Pike/Cox: simplified View function using extracted rendering methods
	if !m.connected {
		if m.err != nil {
			return fmt.Sprintf("Error: %v\n\nPress Ctrl+C to quit.", m.err)
		}
		return "Connecting to Mattermost...\n"
	}

	// Calculate dimensions
	width := m.width
	if width == 0 {
		width = defaultWidth
	}
	height := m.height
	if height == 0 {
		height = defaultHeight
	}

	// Layout: sidebar | messages
	sidebar := sidebarWidth
	if width < minWidthForFullSide {
		sidebar = sidebarWidthSmall
	}
	mainWidth := width - sidebar - 1 // -1 for separator
	if mainWidth < minMainWidth {
		mainWidth = minMainWidth
	}

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

	// Render components
	leftPane := m.renderSidebar(sidebar)
	messagesPane := m.renderMessages(mainWidth, m.msgHeight())
	inputLine := m.renderInput(mainWidth, channel)

	// Combine messages and input into right pane
	rightPane := messagesPane + inputLine

	// Combine left and right panes
	return m.combinePanes(leftPane, rightPane, sidebar, mainWidth, height)
}

func main() {
	// Parse CLI flags - NO environment variable fallbacks
	host := flag.String("host", "", "Mattermost server host (e.g., chat.example.com)")
	token := flag.String("token", "", "Personal Access Token")
	user := flag.String("user", "", "Username or email for login")
	pass := flag.String("pass", "", "Password for login")
	teamID := flag.String("teamid", "", "Team ID (optional)")
	debug := flag.Bool("debug", false, "Enable debug logging to termunicator_debug.log")

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

	// Setup debug logging if requested
	if *debug {
		logFile, err := os.OpenFile("termunicator_debug.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err == nil {
			log.SetOutput(logFile)
			defer logFile.Close()
			log.Printf("=== termunicator started (debug mode) ===")
		} else {
			fmt.Fprintf(os.Stderr, "Warning: Could not open debug log file: %v\n", err)
			log.SetOutput(io.Discard)
		}
	} else {
		// Disable logging by default
		log.SetOutput(io.Discard)
	}

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

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
