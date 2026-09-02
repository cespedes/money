// Package ui implements the terminal user interface for the accounting
// application, on top of Bubble Tea.
package ui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"

	"money/tui/internal/client"
)

type tab int

const (
	tabAccounts tab = iota
	tabTransactions
)

type App struct {
	client *client.Client

	active tab
	width  int
	height int

	accounts     accountsModel
	transactions transactionsModel
}

func New(c *client.Client) App {
	return App{
		client:       c,
		accounts:     newAccountsModel(c),
		transactions: newTransactionsModel(c),
	}
}

func (m App) Init() tea.Cmd {
	return tea.Batch(m.accounts.Init(), m.transactions.Init())
}

func (m App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.accounts.SetSize(msg.Width, msg.Height-6)
		m.transactions.SetSize(msg.Width, msg.Height-6)
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c":
			return m, tea.Quit
		case "q":
			if !m.currentEditing() {
				return m, tea.Quit
			}
		case "tab", "shift+tab":
			if !m.currentEditing() {
				if m.active == tabAccounts {
					m.active = tabTransactions
				} else {
					m.active = tabAccounts
				}
				return m, nil
			}
		}

		// Keyboard input only goes to the active tab; the other tab is
		// not visible and must not react to keystrokes meant for this one.
		var cmd tea.Cmd
		if m.active == tabAccounts {
			m.accounts, cmd = m.accounts.Update(msg)
		} else {
			m.transactions, cmd = m.transactions.Update(msg)
		}
		return m, cmd
	}

	// Any other message (async load results, cursor blinks, ...) must
	// reach both models regardless of which tab is active, or the
	// inactive tab would never see the results of its own commands
	// (e.g. its initial data load).
	var cmd1, cmd2 tea.Cmd
	m.accounts, cmd1 = m.accounts.Update(msg)
	m.transactions, cmd2 = m.transactions.Update(msg)
	cmd := tea.Batch(cmd1, cmd2)
	return m, cmd
}

func (m App) currentEditing() bool {
	if m.active == tabAccounts {
		return m.accounts.Editing()
	}
	return m.transactions.Editing()
}

func (m App) View() tea.View {
	var b strings.Builder

	b.WriteString(titleStyle.Render("Money — Accounting TUI"))
	b.WriteString("\n\n")

	tabs := []string{"Accounts", "Transactions"}
	for i, t := range tabs {
		if tab(i) == m.active {
			b.WriteString(tabActiveStyle.Render(t))
		} else {
			b.WriteString(tabInactiveStyle.Render(t))
		}
	}
	b.WriteString("\n\n")

	if m.active == tabAccounts {
		b.WriteString(m.accounts.View())
	} else {
		b.WriteString(m.transactions.View())
	}

	b.WriteString("\n")
	b.WriteString(helpStyle.Render(m.footer()))

	v := tea.NewView(b.String())
	v.AltScreen = true
	return v
}

func (m App) footer() string {
	if m.active == tabAccounts && m.accounts.mode == accountsModeLedger {
		return "↑/↓: navigate  •  esc/q: back"
	}
	if m.currentEditing() {
		return "enter: confirm field  •  esc: cancel"
	}
	base := "tab: switch view  •  ↑/↓: navigate  •  r: refresh  •  n: new  •  d: delete  •  q: quit"
	if m.active == tabAccounts {
		base = fmt.Sprintf("%s  •  enter: view transactions", base)
	} else {
		base = fmt.Sprintf("%s  •  enter: view details", base)
	}
	return base
}
