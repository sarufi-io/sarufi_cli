package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/sarufi-io/sarufi-golang-sdk"
)

var app sarufi.Application

func word_wrap(text string, lineWidth int) string {
	words := strings.Fields(strings.TrimSpace(text))
	if len(words) == 0 {
		return text
	}
	wrapped := words[0]
	spaceLeft := lineWidth - len(wrapped)
	for _, word := range words[1:] {
		if len(word)+1 > spaceLeft {
			wrapped += "\n" + word
			spaceLeft = lineWidth - len(word)
		} else {
			wrapped += " " + word
			spaceLeft -= 1 + len(word)
		}
	}

	return wrapped
}

type item struct {
	title, desc string
	id          int
}

func (i item) Title() string       { return i.title }
func (i item) Description() string { return i.desc }
func (i item) FilterValue() string { return i.title }

type authenticateMsg struct {
	message string
	bots    []sarufi.Bot
	error   error
}

type botResponseMsg struct {
	message string
	error   error
}

type states int

const (
	authenticating = iota
	listingBots
	messagingBot
	waitingForResponse
)

type model struct {
	state                     states
	allBots                   list.Model
	selectedBot               sarufi.Bot
	quit                      bool
	error                     error
	spinner                   spinner.Model
	messageInput              textinput.Model
	messageResponse           string
	messageHistory            []string
	waitingForResponseSpinner spinner.Model
}

func (m model) Init() tea.Cmd {
	return m.spinner.Tick
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "esc":
			m.quit = true
			return m, tea.Quit
		case "ctrl+b":
			switch m.state {
			case messagingBot:
				m.state = listingBots
				m.messageInput.Blur()
				return m, nil
			}
		case "enter":
			switch m.state {
			case listingBots:
				i, ok := m.allBots.SelectedItem().(item)
				if ok {
					bot, err := app.GetBot(i.id)
					if err != nil {
						m.error = err
						return m, nil
					}
					m.selectedBot = *bot
					m.state = messagingBot
					m.messageInput.SetValue("")
					m.messageInput.Placeholder = "Write a message"
					m.messageInput.Focus()
					m.messageInput.CharLimit = 156
					m.messageInput.Width = 20
					m.messageHistory = nil
					return m, m.messageInput.Cursor.BlinkCmd()
				}
			case messagingBot:
				msg := m.messageInput.Value()
				m.messageInput.SetValue("")
				m.messageHistory = append(m.messageHistory, "You: "+msg)
				m.state = waitingForResponse

				return m, tea.Batch(
					m.sendMsgToBot(msg),
					m.waitingForResponseSpinner.Tick,
				)
			}
		case "q":
			return m, nil

		}
	case authenticateMsg:
		if msg.message == "success" {
			items := []list.Item{}
			for _, v := range msg.bots {
				items = append(items, item{
					title: v.Name,
					desc:  v.Description,
					id:    v.Id,
				})
			}
			m.allBots = list.New(items, list.NewDefaultDelegate(), 50, 30)
			m.allBots.Title = "My Sarufi Bots"
			m.state = listingBots
			return m, nil
		}
		m.error = msg.error
		return m, nil
	case botResponseMsg:
		if msg.error != nil {
			m.error = msg.error
			return m, nil
		}
		m.messageResponse = msg.message
		m.state = messagingBot

		m.messageHistory = append(m.messageHistory, "Bot: "+m.messageResponse)

	case spinner.TickMsg:
		var cmd1, cmd2 tea.Cmd
		m.spinner, cmd1 = m.spinner.Update(msg)
		m.waitingForResponseSpinner, cmd2 = m.waitingForResponseSpinner.Update(msg)
		return m, tea.Batch(cmd1, cmd2)
	}

	var cmd1, cmd2 tea.Cmd

	m.allBots, cmd1 = m.allBots.Update(msg)
	m.messageInput, cmd2 = m.messageInput.Update(msg)
	return m, tea.Batch(cmd1, cmd2)
}

func (m model) View() string {
	if m.quit {
		return ""
	}

	if m.error != nil {
		return m.error.Error()
	}

	switch m.state {
	case authenticating:
		return m.spinner.View() + " Authenticating"
	case listingBots:
		return m.allBots.View()
	case messagingBot:
		s := "Bot: " + m.selectedBot.Name + "\n"
		for _, v := range m.messageHistory {
			m := word_wrap(v, 50)
			s += m + "\n"
		}
		s += "\n\n" + m.messageInput.View()
		return s
	case waitingForResponse:
		s := "Bot: " + m.selectedBot.Name + "\n"
		for _, v := range m.messageHistory {
			m := word_wrap(v, 50)
			s += m + "\n"
		}
		s += "\n\n" + m.waitingForResponseSpinner.View()
		return s

	}

	return "Hello"
}

func (m model) sendMsgToBot(msg string) tea.Cmd {
	return func() tea.Msg {
		var resp string
		err := m.selectedBot.Respond(msg, "general")
		if err != nil {
			return botResponseMsg{message: "", error: err}
		}
		if m.selectedBot.ModelName == "" {
			switch resp := m.selectedBot.ConversationWithKnowledge.Message[0].ResponseMessage[0].(type) {
			case string:
				return botResponseMsg{message: resp, error: err}
			}
		} else {
			resp = m.selectedBot.Conversation.Message[0]
		}
		return botResponseMsg{message: resp, error: err}
	}
}

func main() {

	key := os.Getenv("SARUFI_API_KEY")
	if key == "" {
		fmt.Println("SARUFI_API_KEY not found, kindly add the key to your environment")
		os.Exit(1)
	}

	app.SetToken(key)
	m := model{}

	items := []list.Item{}
	m.allBots = list.New(items, list.NewDefaultDelegate(), 50, 30)
	m.state = authenticating
	m.spinner = spinner.New()
	m.spinner.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("69"))
	m.spinner.Spinner = spinner.Dot
	m.waitingForResponseSpinner = spinner.New()
	m.waitingForResponseSpinner.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("69"))
	m.waitingForResponseSpinner.Spinner = spinner.Jump
	m.messageInput = textinput.New()

	p := tea.NewProgram(m)

	go func() {
		var msg string
		msg = "success"
		bots, err := app.GetAllBots()
		if err != nil {
			msg = "fail"
		}

		p.Send(authenticateMsg{message: msg, bots: bots, error: err})

	}()

	if _, err := p.Run(); err != nil {
		fmt.Println("OOOOPs, something went wrong")
		os.Exit(1)
	}
}
