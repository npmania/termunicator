package ui

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"termunicator/internal/lib"
)

type Message struct {
	Author    string
	Content   string
	Timestamp time.Time
	IsOwn     bool
}

type ServerMessageMsg struct {
	Message Message
}

type ChatModel struct {
	username string
	messages []Message
	input    string
	height   int
	width    int
	context  *lib.Context
}

func NewChatUI(username string) *ChatModel {
	// Initialize libcommunicator
	if err := lib.Initialize(); err != nil {
		fmt.Printf("Warning: Failed to initialize libcommunicator: %v\n", err)
	}

	// Create context
	ctx, err := lib.CreateContext(fmt.Sprintf("chat-%s", username))
	if err != nil {
		fmt.Printf("Warning: Failed to create context: %v\n", err)
		ctx = nil
	}

	model := &ChatModel{
		username: username,
		messages: []Message{
			{
				Author:    "system",
				Content:   fmt.Sprintf("Connecting to @%s via libcommunicator %s", username, lib.GetVersion()),
				Timestamp: time.Now(),
				IsOwn:     false,
			},
		},
		input:   "",
		context: ctx,
	}

	// Set up message callback if context was created successfully
	if ctx != nil {
		ctx.SetMessageCallback(func(author, content string) {
			// This would be called when messages are received from libcommunicator
			// For now, we'll handle this in the bubbletea event loop
		})

		// Initialize context
		if err := ctx.Initialize(); err != nil {
			model.messages = append(model.messages, Message{
				Author:    "system",
				Content:   fmt.Sprintf("Failed to initialize context: %v", err),
				Timestamp: time.Now(),
				IsOwn:     false,
			})
		} else {
			model.messages = append(model.messages, Message{
				Author:    "system",
				Content:   "libcommunicator context initialized successfully",
				Timestamp: time.Now(),
				IsOwn:     false,
			})
		}
	}

	return model
}

func (m ChatModel) Init() tea.Cmd {
	return listenForServerMessages(m.username)
}

func (m ChatModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		return m.handleKeyboardInput(msg)
	case ServerMessageMsg:
		return m.handleServerMessage(msg)
	case tea.WindowSizeMsg:
		m.height = msg.Height
		m.width = msg.Width
		return m, nil
	}
	return m, nil
}

func (m ChatModel) handleKeyboardInput(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c", "esc":
		return m, tea.Quit
	case "enter":
		if strings.TrimSpace(m.input) != "" {
			newMsg := Message{
				Author:    "you",
				Content:   m.input,
				Timestamp: time.Now(),
				IsOwn:     true,
			}
			m.messages = append(m.messages, newMsg)
			
			// Send message to server (via libcommunicator)
			cmd := m.sendMessageToServer(m.username, m.input)
			m.input = ""
			return m, cmd
		}
	case "backspace":
		if len(m.input) > 0 {
			m.input = m.input[:len(m.input)-1]
		}
	default:
		// Handle printable characters
		if len(msg.String()) == 1 && msg.String() >= " " && msg.String() <= "~" {
			m.input += msg.String()
		}
	}
	return m, nil
}

func (m ChatModel) handleServerMessage(msg ServerMessageMsg) (tea.Model, tea.Cmd) {
	m.messages = append(m.messages, msg.Message)
	return m, listenForServerMessages(m.username)
}

func (m ChatModel) View() string {
	var content strings.Builder
	
	// Header
	headerStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("#FAFAFA")).
		Background(lipgloss.Color("#7D56F4")).
		Padding(0, 1).
		Width(m.width)
	
	header := headerStyle.Render(fmt.Sprintf("DM: @%s", m.username))
	content.WriteString(header + "\n")
	
	// Messages area
	messageHeight := m.height - 4 // Account for header, input, and help
	visibleMessages := m.messages
	if len(m.messages) > messageHeight {
		visibleMessages = m.messages[len(m.messages)-messageHeight:]
	}
	
	for _, msg := range visibleMessages {
		timestamp := msg.Timestamp.Format("15:04")
		
		if msg.IsOwn {
			// Own messages (right-aligned, blue)
			msgStyle := lipgloss.NewStyle().
				Foreground(lipgloss.Color("#7D56F4")).
				Align(lipgloss.Right).
				Width(m.width - 2)
			content.WriteString(msgStyle.Render(fmt.Sprintf("[%s] you: %s", timestamp, msg.Content)) + "\n")
		} else {
			// Other messages (left-aligned, green)
			msgStyle := lipgloss.NewStyle().
				Foreground(lipgloss.Color("#04B575")).
				Width(m.width - 2)
			content.WriteString(msgStyle.Render(fmt.Sprintf("[%s] %s: %s", timestamp, msg.Author, msg.Content)) + "\n")
		}
	}
	
	// Fill remaining space
	for i := len(visibleMessages); i < messageHeight; i++ {
		content.WriteString("\n")
	}
	
	// Input area
	inputStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#874BFD")).
		Padding(0, 1).
		Width(m.width - 2)
	
	inputPrompt := inputStyle.Render(fmt.Sprintf("Message: %s▋", m.input))
	content.WriteString(inputPrompt + "\n")
	
	// Help
	helpStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#626262")).
		Width(m.width)
	help := helpStyle.Render("Esc/Ctrl+C: quit • Enter: send")
	content.WriteString(help)
	
	return content.String()
}

// Mock server message listener (replace with actual libcommunicator integration)
func listenForServerMessages(username string) tea.Cmd {
	return func() tea.Msg {
		// Simulate waiting for server message
		time.Sleep(5 * time.Second)
		return ServerMessageMsg{
			Message: Message{
				Author:    username,
				Content:   "Hellow!",
				Timestamp: time.Now(),
				IsOwn:     false,
			},
		}
	}
}

// Send message to server via libcommunicator
func (m *ChatModel) sendMessageToServer(username, content string) tea.Cmd {
	return func() tea.Msg {
		if m.context != nil {
			if err := m.context.SendMessage(username, content); err != nil {
				return ServerMessageMsg{
					Message: Message{
						Author:    "system",
						Content:   fmt.Sprintf("Failed to send message: %v", err),
						Timestamp: time.Now(),
						IsOwn:     false,
					},
				}
			}
		}
		return nil
	}
}

func (m *ChatModel) Run() error {
	// Always use simple mode for now to avoid TTY issues
	// In a real terminal environment, you could use bubbletea
	return m.runSimpleMode()
}

func isatty() bool {
	_, err := os.OpenFile("/dev/tty", os.O_RDWR, 0)
	return err == nil
}

func (m *ChatModel) runSimpleMode() error {
	fmt.Printf("Chat with @%s (simple mode)\n", m.username)
	fmt.Println("Type messages and press Enter (Ctrl+C to quit)")
	fmt.Println("----------------------------------------")

	// Show context status
	if m.context != nil {
		fmt.Printf("libcommunicator context: active (%s)\n", lib.GetVersion())
	} else {
		fmt.Println("libcommunicator context: not available")
	}

	// Create context for goroutine cancellation
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Wait group to track active goroutines
	var wg sync.WaitGroup

	// Mutex for synchronized stdout access
	var outputMutex sync.Mutex

	// Helper function for synchronized printing
	printWithLock := func(format string, args ...interface{}) {
		outputMutex.Lock()
		defer outputMutex.Unlock()
		fmt.Printf(format, args...)
	}

	// Simulate server message
	wg.Add(1)
	go func() {
		defer wg.Done()
		select {
		case <-time.After(3 * time.Second):
			printWithLock("[%s] %s: Hellow!\n", time.Now().Format("15:04"), m.username)
		case <-ctx.Done():
			return
		}
	}()

	scanner := bufio.NewScanner(os.Stdin)
	for {
		outputMutex.Lock()
		fmt.Print("> ")
		outputMutex.Unlock()

		if !scanner.Scan() {
			break
		}

		input := strings.TrimSpace(scanner.Text())
		if input == "" {
			continue
		}

		printWithLock("[%s] you: %s\n", time.Now().Format("15:04"), input)

		// Try to send via libcommunicator
		if m.context != nil {
			if err := m.context.SendMessage(m.username, input); err != nil {
				printWithLock("[%s] system: Failed to send via libcommunicator: %v\n", time.Now().Format("15:04"), err)
			}
		}

		// Simulate response
		wg.Add(1)
		go func(msg string) {
			defer wg.Done()
			select {
			case <-time.After(1 * time.Second):
				printWithLock("[%s] %s: Echo: %s\n", time.Now().Format("15:04"), m.username, msg)
			case <-ctx.Done():
				return
			}
		}(input)
	}

	// Cancel all goroutines and wait for them to finish
	cancel()
	wg.Wait()

	// Cleanup
	if m.context != nil {
		m.context.Destroy()
	}
	lib.Cleanup()

	return scanner.Err()
}