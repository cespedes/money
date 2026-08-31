package ui

import (
	"context"
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbles/table"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"money/tui/internal/client"
)

type accountsMode int

const (
	accountsModeList accountsMode = iota
	accountsModeCreate
	accountsModeConfirmDelete
)

type accountsModel struct {
	client *client.Client

	mode   accountsMode
	table  table.Model
	rows   []client.Account
	status string
	err    string

	inputs     []textinput.Model
	inputIndex int
}

const (
	fieldAccountName = iota
	fieldAccountCode
	fieldAccountParent
)

func newAccountsModel(c *client.Client) accountsModel {
	columns := []table.Column{
		{Title: "ID", Width: 6},
		{Title: "Code", Width: 10},
		{Title: "Name", Width: 30},
		{Title: "Parent", Width: 10},
	}
	t := table.New(
		table.WithColumns(columns),
		table.WithFocused(true),
		table.WithHeight(10),
	)

	name := textinput.New()
	name.Placeholder = "Name (required)"
	code := textinput.New()
	code.Placeholder = "Code (optional)"
	parent := textinput.New()
	parent.Placeholder = "Parent account ID (optional)"

	return accountsModel{
		client: c,
		table:  t,
		inputs: []textinput.Model{name, code, parent},
	}
}

func (m accountsModel) Init() tea.Cmd {
	return m.loadAccounts
}

func (m accountsModel) Editing() bool {
	return m.mode != accountsModeList
}

func (m *accountsModel) SetSize(width, height int) {
	m.table.SetWidth(width)
	if height > 5 {
		m.table.SetHeight(height)
	}
}

type accountsLoadedMsg struct {
	accounts []client.Account
	err      error
}

type accountMutatedMsg struct {
	err error
}

func (m accountsModel) loadAccounts() tea.Msg {
	accounts, err := m.client.ListAccounts(context.Background())
	return accountsLoadedMsg{accounts: accounts, err: err}
}

func (m accountsModel) Update(msg tea.Msg) (accountsModel, tea.Cmd) {
	switch msg := msg.(type) {
	case accountsLoadedMsg:
		if msg.err != nil {
			m.err = msg.err.Error()
			return m, nil
		}
		m.err = ""
		m.rows = msg.accounts
		m.table.SetRows(accountsToRows(msg.accounts))
		return m, nil

	case accountMutatedMsg:
		if msg.err != nil {
			m.err = msg.err.Error()
			return m, nil
		}
		m.err = ""
		m.mode = accountsModeList
		return m, m.loadAccounts

	case tea.KeyMsg:
		switch m.mode {
		case accountsModeList:
			return m.updateList(msg)
		case accountsModeCreate:
			return m.updateCreate(msg)
		case accountsModeConfirmDelete:
			return m.updateConfirmDelete(msg)
		}
	}

	if m.mode == accountsModeList {
		var cmd tea.Cmd
		m.table, cmd = m.table.Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m accountsModel) updateList(msg tea.KeyMsg) (accountsModel, tea.Cmd) {
	switch msg.String() {
	case "r":
		m.status = "Refreshing..."
		return m, m.loadAccounts
	case "n":
		m.mode = accountsModeCreate
		m.inputIndex = 0
		for i := range m.inputs {
			m.inputs[i].SetValue("")
			m.inputs[i].Blur()
		}
		m.inputs[0].Focus()
		m.err = ""
		return m, nil
	case "d":
		if len(m.rows) == 0 {
			return m, nil
		}
		m.mode = accountsModeConfirmDelete
		return m, nil
	}
	var cmd tea.Cmd
	m.table, cmd = m.table.Update(msg)
	return m, cmd
}

func (m accountsModel) updateCreate(msg tea.KeyMsg) (accountsModel, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.mode = accountsModeList
		m.err = ""
		return m, nil
	case "enter":
		if m.inputIndex < len(m.inputs)-1 {
			m.inputs[m.inputIndex].Blur()
			m.inputIndex++
			m.inputs[m.inputIndex].Focus()
			return m, nil
		}
		return m.submitCreate()
	}

	var cmd tea.Cmd
	m.inputs[m.inputIndex], cmd = m.inputs[m.inputIndex].Update(msg)
	return m, cmd
}

func (m accountsModel) submitCreate() (accountsModel, tea.Cmd) {
	name := strings.TrimSpace(m.inputs[fieldAccountName].Value())
	if name == "" {
		m.err = "name is required"
		return m, nil
	}
	account := client.Account{Name: name}

	if code := strings.TrimSpace(m.inputs[fieldAccountCode].Value()); code != "" {
		account.Code = &code
	}
	if parentStr := strings.TrimSpace(m.inputs[fieldAccountParent].Value()); parentStr != "" {
		parentID, err := strconv.ParseInt(parentStr, 10, 64)
		if err != nil {
			m.err = "parent account ID must be a number"
			return m, nil
		}
		account.ParentID = &parentID
	}

	m.err = ""
	c := m.client
	return m, func() tea.Msg {
		_, err := c.CreateAccount(context.Background(), account)
		return accountMutatedMsg{err: err}
	}
}

func (m accountsModel) updateConfirmDelete(msg tea.KeyMsg) (accountsModel, tea.Cmd) {
	switch msg.String() {
	case "y":
		row := m.table.Cursor()
		if row < 0 || row >= len(m.rows) {
			m.mode = accountsModeList
			return m, nil
		}
		id := m.rows[row].ID
		m.mode = accountsModeList
		c := m.client
		return m, func() tea.Msg {
			err := c.DeleteAccount(context.Background(), id)
			return accountMutatedMsg{err: err}
		}
	default:
		m.mode = accountsModeList
		return m, nil
	}
}

func accountsToRows(accounts []client.Account) []table.Row {
	rows := make([]table.Row, 0, len(accounts))
	for _, a := range accounts {
		code := ""
		if a.Code != nil {
			code = *a.Code
		}
		parent := ""
		if a.ParentID != nil {
			parent = strconv.FormatInt(*a.ParentID, 10)
		}
		rows = append(rows, table.Row{strconv.FormatInt(a.ID, 10), code, a.Name, parent})
	}
	return rows
}

func (m accountsModel) View() string {
	var b strings.Builder

	switch m.mode {
	case accountsModeCreate:
		b.WriteString(formLabelStyle.Render("New account"))
		b.WriteString("\n\n")
		labels := []string{"Name", "Code", "Parent ID"}
		for i, input := range m.inputs {
			b.WriteString(dimStyle.Render(labels[i] + ": "))
			b.WriteString(input.View())
			b.WriteString("\n")
		}
	case accountsModeConfirmDelete:
		b.WriteString(m.table.View())
		b.WriteString("\n\n")
		b.WriteString(errorStyle.Render("Delete selected account? (y/n)"))
	default:
		b.WriteString(m.table.View())
	}

	if m.err != "" {
		b.WriteString("\n\n")
		b.WriteString(errorStyle.Render("Error: " + m.err))
	} else if m.status != "" && m.mode == accountsModeList {
		b.WriteString("\n\n")
		b.WriteString(statusStyle.Render(m.status))
	}

	return b.String()
}
