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
	tabCurrencies
	numTabs
)

var tabLabels = [numTabs]string{"Accounts", "Transactions", "Currencies"}

type App struct {
	client *client.Client

	active tab
	width  int
	height int

	accounts     accountsModel
	transactions transactionsModel
	currencies   currenciesModel
}

func New(c *client.Client) App {
	return App{
		client:       c,
		accounts:     newAccountsModel(c),
		transactions: newTransactionsModel(c),
		currencies:   newCurrenciesModel(c),
	}
}

func (m App) Init() tea.Cmd {
	return tea.Batch(m.accounts.Init(), m.transactions.Init(), m.currencies.Init())
}

func (m App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.accounts.SetSize(msg.Width, msg.Height-6)
		m.transactions.SetSize(msg.Width, msg.Height-6)
		m.currencies.SetSize(msg.Width, msg.Height-6)
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c":
			return m, tea.Quit
		case "q":
			if !m.currentEditing() {
				return m, tea.Quit
			}
		case "tab":
			if !m.currentEditing() {
				m.active = (m.active + 1) % numTabs
				return m, nil
			}
		case "shift+tab":
			if !m.currentEditing() {
				m.active = (m.active + numTabs - 1) % numTabs
				return m, nil
			}
		}

		// Keyboard input only goes to the active tab; the other tabs are
		// not visible and must not react to keystrokes meant for this one.
		var cmd tea.Cmd
		switch m.active {
		case tabAccounts:
			m.accounts, cmd = m.accounts.Update(msg)
		case tabTransactions:
			m.transactions, cmd = m.transactions.Update(msg)
		case tabCurrencies:
			m.currencies, cmd = m.currencies.Update(msg)
		}
		return m, cmd
	}

	// Any other message (async load results, cursor blinks, ...) must
	// reach every tab regardless of which is active, or an inactive tab
	// would never see the results of its own commands (e.g. its initial
	// data load).
	var cmd1, cmd2, cmd3 tea.Cmd
	m.accounts, cmd1 = m.accounts.Update(msg)
	m.transactions, cmd2 = m.transactions.Update(msg)
	m.currencies, cmd3 = m.currencies.Update(msg)
	return m, tea.Batch(cmd1, cmd2, cmd3)
}

func (m App) currentEditing() bool {
	switch m.active {
	case tabAccounts:
		return m.accounts.Editing()
	case tabTransactions:
		return m.transactions.Editing()
	default:
		return m.currencies.Editing()
	}
}

func (m App) View() tea.View {
	var b strings.Builder

	b.WriteString(titleStyle.Render("Money — Accounting TUI"))
	b.WriteString("\n\n")

	for i, t := range tabLabels {
		if tab(i) == m.active {
			b.WriteString(tabActiveStyle.Render(t))
		} else {
			b.WriteString(tabInactiveStyle.Render(t))
		}
	}
	b.WriteString("\n\n")

	switch m.active {
	case tabAccounts:
		b.WriteString(m.accounts.View())
	case tabTransactions:
		b.WriteString(m.transactions.View())
	case tabCurrencies:
		b.WriteString(m.currencies.View())
	}

	b.WriteString("\n")
	b.WriteString(helpStyle.Render(m.footer()))

	content := b.String()
	switch {
	case m.active == tabAccounts && m.accounts.mode == accountsModeCreate:
		content = overlayCentered(content, m.accounts.createPopup(), m.width, m.height)
	case m.active == tabAccounts && m.accounts.mode == accountsModeLedgerCreate:
		content = overlayCentered(content, m.accounts.ledgerEntryPopup(), m.width, m.height)
	case m.active == tabCurrencies && m.currencies.mode == currenciesModeCreate:
		content = overlayCentered(content, m.currencies.createPopup(), m.width, m.height)
	}

	v := tea.NewView(content)
	v.AltScreen = true
	return v
}

func (m App) footer() string {
	switch m.active {
	case tabAccounts:
		switch m.accounts.mode {
		case accountsModeLedger:
			return "↑/↓: navigate  •  n: new entry  •  esc/q: back"
		case accountsModeCreate:
			return fmt.Sprintf("tab: switch field  •  ↑/↓: choose parent  •  enter: %s  •  esc: cancel", submitVerb(m.accounts.editingID))
		case accountsModeLedgerCreate:
			return "tab: switch field  •  ↑/↓: choose currency/account  •  enter: create  •  esc: cancel"
		}
	case tabCurrencies:
		if m.currencies.mode == currenciesModeCreate {
			return fmt.Sprintf("tab/←/→: switch field  •  enter: %s  •  esc: cancel", submitVerb(m.currencies.editingID))
		}
	}
	if m.currentEditing() {
		return "enter: confirm field  •  esc: cancel"
	}
	base := "tab: switch view  •  ↑/↓: navigate  •  r: refresh  •  n: new"
	switch m.active {
	case tabAccounts, tabCurrencies:
		base = fmt.Sprintf("%s  •  e: edit", base)
	}
	base = fmt.Sprintf("%s  •  d: delete  •  q: quit", base)
	switch m.active {
	case tabAccounts:
		base = fmt.Sprintf("%s  •  K/J: move  •  enter: view transactions", base)
	case tabTransactions:
		base = fmt.Sprintf("%s  •  enter: view details", base)
	}
	return base
}

// submitVerb picks the footer hint's verb for the "new"/"edit" pop-up
// shared by Accounts and Currencies (see accountsModel.startEdit,
// currenciesModel.startEdit): "create" while editingID is nil, "save"
// once it's been set to the record being edited.
func submitVerb(editingID *int64) string {
	if editingID != nil {
		return "save"
	}
	return "create"
}
