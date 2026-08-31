package ui

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/table"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"money/tui/internal/client"
)

type transactionsMode int

const (
	transactionsModeList transactionsMode = iota
	transactionsModeCreate
	transactionsModeDetail
	transactionsModeConfirmDelete
)

type createStep int

const (
	stepDescription createStep = iota
	stepTimestamp
	stepEntryAccount
	stepEntryValue
	stepConfirmMore
)

const timestampLayout = "2006-01-02 15:04"

type transactionsModel struct {
	client *client.Client

	mode transactionsMode
	tbl  table.Model
	rows []client.Transaction

	status string
	err    string

	// create wizard state
	step           createStep
	descInput      textinput.Model
	timestampInput textinput.Model
	acctInput      textinput.Model
	valueInput     textinput.Model
	pendingEntries []client.Entry
	pendingAcctID  int64

	// detail view state
	detail client.Transaction
}

func newTransactionsModel(c *client.Client) transactionsModel {
	columns := []table.Column{
		{Title: "ID", Width: 6},
		{Title: "Timestamp", Width: 17},
		{Title: "Description", Width: 30},
		{Title: "Entries", Width: 8},
	}
	t := table.New(
		table.WithColumns(columns),
		table.WithFocused(true),
		table.WithHeight(10),
	)

	desc := textinput.New()
	desc.Placeholder = "Description"
	ts := textinput.New()
	ts.Placeholder = timestampLayout + " (blank = now)"
	acct := textinput.New()
	acct.Placeholder = "Account ID"
	val := textinput.New()
	val.Placeholder = "Value in cents (e.g. -1000 or 1000)"

	return transactionsModel{
		client:         c,
		tbl:            t,
		descInput:      desc,
		timestampInput: ts,
		acctInput:      acct,
		valueInput:     val,
	}
}

func (m transactionsModel) Init() tea.Cmd {
	return m.loadTransactions
}

// Editing reports whether the app-level quit/tab-switch keys should be
// swallowed by this model instead (any mode other than the plain list).
func (m transactionsModel) Editing() bool {
	return m.mode != transactionsModeList
}

func (m *transactionsModel) SetSize(width, height int) {
	m.tbl.SetWidth(width)
	if height > 5 {
		m.tbl.SetHeight(height)
	}
}

type transactionsLoadedMsg struct {
	transactions []client.Transaction
	err          error
}

type transactionMutatedMsg struct {
	err error
}

func (m transactionsModel) loadTransactions() tea.Msg {
	transactions, err := m.client.ListTransactions(context.Background())
	return transactionsLoadedMsg{transactions: transactions, err: err}
}

func (m transactionsModel) Update(msg tea.Msg) (transactionsModel, tea.Cmd) {
	switch msg := msg.(type) {
	case transactionsLoadedMsg:
		if msg.err != nil {
			m.err = msg.err.Error()
			return m, nil
		}
		m.err = ""
		m.rows = msg.transactions
		m.tbl.SetRows(transactionsToRows(msg.transactions))
		return m, nil

	case transactionMutatedMsg:
		if msg.err != nil {
			m.err = msg.err.Error()
			m.mode = transactionsModeList
			return m, nil
		}
		m.err = ""
		m.mode = transactionsModeList
		return m, m.loadTransactions

	case tea.KeyMsg:
		switch m.mode {
		case transactionsModeList:
			return m.updateList(msg)
		case transactionsModeCreate:
			return m.updateCreate(msg)
		case transactionsModeDetail:
			m.mode = transactionsModeList
			return m, nil
		case transactionsModeConfirmDelete:
			return m.updateConfirmDelete(msg)
		}
	}

	if m.mode == transactionsModeList {
		var cmd tea.Cmd
		m.tbl, cmd = m.tbl.Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m transactionsModel) updateList(msg tea.KeyMsg) (transactionsModel, tea.Cmd) {
	switch msg.String() {
	case "r":
		m.status = "Refreshing..."
		return m, m.loadTransactions
	case "n":
		m.mode = transactionsModeCreate
		m.step = stepDescription
		m.pendingEntries = nil
		m.descInput.SetValue("")
		m.timestampInput.SetValue("")
		m.acctInput.SetValue("")
		m.valueInput.SetValue("")
		m.descInput.Focus()
		m.err = ""
		return m, nil
	case "enter":
		if row := m.tbl.Cursor(); row >= 0 && row < len(m.rows) {
			m.detail = m.rows[row]
			m.mode = transactionsModeDetail
		}
		return m, nil
	case "d":
		if len(m.rows) == 0 {
			return m, nil
		}
		m.mode = transactionsModeConfirmDelete
		return m, nil
	}
	var cmd tea.Cmd
	m.tbl, cmd = m.tbl.Update(msg)
	return m, cmd
}

func (m transactionsModel) updateConfirmDelete(msg tea.KeyMsg) (transactionsModel, tea.Cmd) {
	switch msg.String() {
	case "y":
		row := m.tbl.Cursor()
		if row < 0 || row >= len(m.rows) {
			m.mode = transactionsModeList
			return m, nil
		}
		id := m.rows[row].ID
		m.mode = transactionsModeList
		c := m.client
		return m, func() tea.Msg {
			err := c.DeleteTransaction(context.Background(), id)
			return transactionMutatedMsg{err: err}
		}
	default:
		m.mode = transactionsModeList
		return m, nil
	}
}

func (m transactionsModel) updateCreate(msg tea.KeyMsg) (transactionsModel, tea.Cmd) {
	if msg.String() == "esc" {
		m.mode = transactionsModeList
		m.err = ""
		return m, nil
	}

	switch m.step {
	case stepDescription:
		if msg.String() == "enter" {
			if strings.TrimSpace(m.descInput.Value()) == "" {
				m.err = "description is required"
				return m, nil
			}
			m.err = ""
			m.descInput.Blur()
			m.step = stepTimestamp
			m.timestampInput.Focus()
			return m, nil
		}
		var cmd tea.Cmd
		m.descInput, cmd = m.descInput.Update(msg)
		return m, cmd

	case stepTimestamp:
		if msg.String() == "enter" {
			m.err = ""
			m.timestampInput.Blur()
			m.step = stepEntryAccount
			m.acctInput.Focus()
			return m, nil
		}
		var cmd tea.Cmd
		m.timestampInput, cmd = m.timestampInput.Update(msg)
		return m, cmd

	case stepEntryAccount:
		if msg.String() == "enter" {
			id, err := strconv.ParseInt(strings.TrimSpace(m.acctInput.Value()), 10, 64)
			if err != nil {
				m.err = "account ID must be a number"
				return m, nil
			}
			m.pendingAcctID = id
			m.err = ""
			m.acctInput.Blur()
			m.step = stepEntryValue
			m.valueInput.Focus()
			return m, nil
		}
		var cmd tea.Cmd
		m.acctInput, cmd = m.acctInput.Update(msg)
		return m, cmd

	case stepEntryValue:
		if msg.String() == "enter" {
			value, err := strconv.ParseInt(strings.TrimSpace(m.valueInput.Value()), 10, 64)
			if err != nil {
				m.err = "value must be an integer number of cents"
				return m, nil
			}
			m.pendingEntries = append(m.pendingEntries, client.Entry{AccountID: m.pendingAcctID, Value: value})
			m.err = ""
			m.valueInput.Blur()
			m.acctInput.SetValue("")
			m.valueInput.SetValue("")
			m.step = stepConfirmMore
			return m, nil
		}
		var cmd tea.Cmd
		m.valueInput, cmd = m.valueInput.Update(msg)
		return m, cmd

	case stepConfirmMore:
		switch msg.String() {
		case "y":
			m.step = stepEntryAccount
			m.acctInput.Focus()
			return m, nil
		case "n", "enter":
			return m.submitCreate()
		}
	}
	return m, nil
}

func (m transactionsModel) submitCreate() (transactionsModel, tea.Cmd) {
	if len(m.pendingEntries) < 2 {
		m.err = "a transaction needs at least two entries"
		m.step = stepEntryAccount
		m.acctInput.Focus()
		return m, nil
	}
	var sum int64
	for _, e := range m.pendingEntries {
		sum += e.Value
	}
	if sum != 0 {
		m.err = fmt.Sprintf("entries must sum to zero (currently %s)", formatCents(sum))
		m.step = stepEntryAccount
		m.acctInput.Focus()
		return m, nil
	}

	ts := time.Now()
	if raw := strings.TrimSpace(m.timestampInput.Value()); raw != "" {
		parsed, err := time.ParseInLocation(timestampLayout, raw, time.Local)
		if err != nil {
			m.err = "timestamp must look like " + timestampLayout
			m.step = stepTimestamp
			m.timestampInput.Focus()
			return m, nil
		}
		ts = parsed
	}

	transaction := client.Transaction{
		Description: strings.TrimSpace(m.descInput.Value()),
		Timestamp:   ts,
		Entries:     m.pendingEntries,
	}

	m.err = ""
	c := m.client
	return m, func() tea.Msg {
		_, err := c.CreateTransaction(context.Background(), transaction)
		return transactionMutatedMsg{err: err}
	}
}

func transactionsToRows(transactions []client.Transaction) []table.Row {
	rows := make([]table.Row, 0, len(transactions))
	for _, t := range transactions {
		rows = append(rows, table.Row{
			strconv.FormatInt(t.ID, 10),
			t.Timestamp.Local().Format(timestampLayout),
			t.Description,
			strconv.Itoa(len(t.Entries)),
		})
	}
	return rows
}

// formatCents renders a signed minor-unit integer as a decimal amount,
// e.g. -1234 -> "-12.34".
func formatCents(v int64) string {
	sign := ""
	if v < 0 {
		sign = "-"
		v = -v
	}
	return fmt.Sprintf("%s%d.%02d", sign, v/100, v%100)
}

func (m transactionsModel) View() string {
	var b strings.Builder

	switch m.mode {
	case transactionsModeCreate:
		b.WriteString(formLabelStyle.Render("New transaction"))
		b.WriteString("\n\n")
		b.WriteString(dimStyle.Render("Description: "))
		b.WriteString(m.descInput.View())
		b.WriteString("\n")
		if m.step >= stepTimestamp {
			b.WriteString(dimStyle.Render("Timestamp:   "))
			b.WriteString(m.timestampInput.View())
			b.WriteString("\n")
		}
		if len(m.pendingEntries) > 0 {
			b.WriteString("\n")
			b.WriteString(dimStyle.Render("Entries so far:"))
			b.WriteString("\n")
			var sum int64
			for _, e := range m.pendingEntries {
				sum += e.Value
				b.WriteString(fmt.Sprintf("  account %d: %s\n", e.AccountID, formatCents(e.Value)))
			}
			b.WriteString(dimStyle.Render(fmt.Sprintf("  sum: %s\n", formatCents(sum))))
		}
		if m.step == stepEntryAccount || m.step == stepEntryValue {
			b.WriteString("\n")
			b.WriteString(dimStyle.Render("Account ID:  "))
			b.WriteString(m.acctInput.View())
			b.WriteString("\n")
		}
		if m.step == stepEntryValue {
			b.WriteString(dimStyle.Render("Value:       "))
			b.WriteString(m.valueInput.View())
			b.WriteString("\n")
		}
		if m.step == stepConfirmMore {
			b.WriteString("\n")
			b.WriteString(statusStyle.Render("Add another entry? (y/n, n submits)"))
		}

	case transactionsModeDetail:
		b.WriteString(formLabelStyle.Render(fmt.Sprintf("Transaction #%d", m.detail.ID)))
		b.WriteString("\n\n")
		b.WriteString(dimStyle.Render("Timestamp:   "))
		b.WriteString(m.detail.Timestamp.Local().Format(timestampLayout))
		b.WriteString("\n")
		b.WriteString(dimStyle.Render("Description: "))
		b.WriteString(m.detail.Description)
		b.WriteString("\n\n")
		b.WriteString(dimStyle.Render("Entries:"))
		b.WriteString("\n")
		for _, e := range m.detail.Entries {
			b.WriteString(fmt.Sprintf("  account %d: %s\n", e.AccountID, formatCents(e.Value)))
		}
		b.WriteString("\n")
		b.WriteString(helpStyle.Render("esc/enter: back"))

	case transactionsModeConfirmDelete:
		b.WriteString(m.tbl.View())
		b.WriteString("\n\n")
		b.WriteString(errorStyle.Render("Delete selected transaction? (y/n)"))

	default:
		b.WriteString(m.tbl.View())
	}

	if m.err != "" {
		b.WriteString("\n\n")
		b.WriteString(errorStyle.Render("Error: " + m.err))
	} else if m.status != "" && m.mode == transactionsModeList {
		b.WriteString("\n\n")
		b.WriteString(statusStyle.Render(m.status))
	}

	return b.String()
}
