package ui

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"charm.land/bubbles/v2/table"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"

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
	stepEntryCurrency
	stepEntryValue
	stepConfirmMore
)

const timestampLayout = "2006-01-02 15:04"

// pendingEntry is a not-yet-submitted transaction entry, tracked
// internally as an exact integer of the currency's minor units (see
// Currency.ParseAmount) — so the running per-currency balance shown
// while building the transaction, and the final zero-sum check, are both
// exact — converted to the API's decimal wire format (see
// Currency.FromMinorUnits) only once, in submitCreate.
type pendingEntry struct {
	AccountID  int64
	Amount     int64
	CurrencyID int64
}

type transactionsModel struct {
	client *client.Client

	mode transactionsMode
	tbl  table.Model
	rows []client.Transaction

	currencies     []client.Currency
	currencyIndex  currencyByID
	currencyPicker table.Model

	status string
	err    string

	// create wizard state
	step              createStep
	descInput         textinput.Model
	timestampInput    textinput.Model
	acctInput         textinput.Model
	valueInput        textinput.Model
	pendingEntries    []pendingEntry
	pendingAcctID     int64
	pendingCurrencyID int64

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
	desc.SetWidth(30)
	ts := textinput.New()
	ts.Placeholder = timestampLayout + " (blank = now)"
	ts.SetWidth(30)
	acct := textinput.New()
	acct.Placeholder = "Account ID"
	acct.SetWidth(20)
	val := textinput.New()
	val.Placeholder = "e.g. -10.50 or 10.50"
	val.SetWidth(30)

	currencyPicker := table.New(
		table.WithColumns([]table.Column{{Title: "Currency", Width: parentPickerWidth}}),
		table.WithFocused(true),
		table.WithWidth(parentPickerWidth),
		table.WithHeight(8),
	)

	return transactionsModel{
		client:         c,
		tbl:            t,
		descInput:      desc,
		timestampInput: ts,
		acctInput:      acct,
		valueInput:     val,
		currencyPicker: currencyPicker,
	}
}

func (m transactionsModel) Init() tea.Cmd {
	return tea.Batch(m.loadTransactions, m.loadCurrencies)
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

func (m transactionsModel) loadCurrencies() tea.Msg {
	currencies, err := m.client.ListCurrencies(context.Background())
	return currenciesLoadedMsg{currencies: currencies, err: err}
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

	case currenciesLoadedMsg:
		if msg.err != nil {
			m.err = msg.err.Error()
			return m, nil
		}
		m.err = ""
		m.currencies = msg.currencies
		m.currencyIndex = indexCurrencies(msg.currencies)
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
		if len(m.currencies) == 0 {
			m.err = "create a currency first (see the Currencies tab)"
			return m, nil
		}
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

// updateCreate drives the "new transaction" wizard: description,
// timestamp, then repeatedly account, currency (picked from a dropdown,
// like the account form's parent picker), and amount, until the user
// declines to add another entry.
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
			m.step = stepEntryCurrency
			m.currencyPicker.SetRows(currencyDropdownRows(m.currencies))
			m.currencyPicker.SetCursor(0)
			return m, nil
		}
		var cmd tea.Cmd
		m.acctInput, cmd = m.acctInput.Update(msg)
		return m, cmd

	case stepEntryCurrency:
		if msg.String() == "enter" {
			row := m.currencyPicker.Cursor()
			if row < 0 || row >= len(m.currencies) {
				m.err = "pick a currency"
				return m, nil
			}
			currency := m.currencies[row]
			m.pendingCurrencyID = currency.ID
			m.valueInput.Placeholder = fmt.Sprintf("e.g. -10%s50 or 10%s50", currency.DecimalSeparator, currency.DecimalSeparator)
			m.err = ""
			m.step = stepEntryValue
			m.valueInput.Focus()
			return m, nil
		}
		var cmd tea.Cmd
		m.currencyPicker, cmd = m.currencyPicker.Update(msg)
		return m, cmd

	case stepEntryValue:
		if msg.String() == "enter" {
			amount, err := m.currencyIndex[m.pendingCurrencyID].ParseAmount(m.valueInput.Value())
			if err != nil {
				m.err = err.Error()
				return m, nil
			}
			m.pendingEntries = append(m.pendingEntries, pendingEntry{
				AccountID: m.pendingAcctID, Amount: amount, CurrencyID: m.pendingCurrencyID,
			})
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

	// The entries must sum to zero within each currency independently —
	// amounts in different currencies are never summed together (see
	// api/internal/store/transactions.go's Create).
	sums := make(map[int64]int64)
	for _, e := range m.pendingEntries {
		sums[e.CurrencyID] += e.Amount
	}
	for _, currencyID := range sortedCurrencyIDs(sums) {
		if sum := sums[currencyID]; sum != 0 {
			m.err = fmt.Sprintf("entries in %s must sum to zero (currently %s)",
				currencyName(currencyID, m.currencyIndex), formatMinorAmount(sum, m.currencyIndex, currencyID))
			m.step = stepEntryAccount
			m.acctInput.Focus()
			return m, nil
		}
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

	entries := make([]client.Entry, len(m.pendingEntries))
	for i, e := range m.pendingEntries {
		entries[i] = client.Entry{
			AccountID:  e.AccountID,
			Amount:     m.currencyIndex[e.CurrencyID].FromMinorUnits(e.Amount),
			CurrencyID: e.CurrencyID,
		}
	}
	transaction := client.Transaction{
		Description: strings.TrimSpace(m.descInput.Value()),
		Timestamp:   ts,
		Entries:     entries,
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

// currencyDropdownRows lists every currency, in the same order as
// currencies, so a currencyPicker cursor position can be mapped back to
// one directly by index.
func currencyDropdownRows(currencies []client.Currency) []table.Row {
	rows := make([]table.Row, 0, len(currencies))
	for _, c := range currencies {
		rows = append(rows, table.Row{fmt.Sprintf("#%d  %s", c.ID, c.Name)})
	}
	return rows
}

func currencyName(id int64, currencies currencyByID) string {
	if c, ok := currencies[id]; ok {
		return c.Name
	}
	return fmt.Sprintf("currency %d", id)
}

// sortedCurrencyIDs returns sums' keys in ascending order, for
// deterministic display (map iteration order isn't).
func sortedCurrencyIDs(sums map[int64]int64) []int64 {
	ids := make([]int64, 0, len(sums))
	for id := range sums {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids
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
			sums := make(map[int64]int64)
			for _, e := range m.pendingEntries {
				sums[e.CurrencyID] += e.Amount
				b.WriteString(fmt.Sprintf("  account %d: %s\n", e.AccountID, formatMinorAmount(e.Amount, m.currencyIndex, e.CurrencyID)))
			}
			for _, currencyID := range sortedCurrencyIDs(sums) {
				b.WriteString(dimStyle.Render(fmt.Sprintf("  sum (%s): %s\n",
					currencyName(currencyID, m.currencyIndex), formatMinorAmount(sums[currencyID], m.currencyIndex, currencyID))))
			}
		}
		if m.step == stepEntryAccount || m.step == stepEntryCurrency || m.step == stepEntryValue {
			b.WriteString("\n")
			b.WriteString(dimStyle.Render("Account ID:  "))
			b.WriteString(m.acctInput.View())
			b.WriteString("\n")
		}
		if m.step == stepEntryCurrency {
			b.WriteString(dimStyle.Render("Currency (↑/↓ to choose, enter to confirm):"))
			b.WriteString("\n")
			b.WriteString(m.currencyPicker.View())
		}
		if m.step == stepEntryValue {
			b.WriteString(dimStyle.Render("Currency:    "))
			b.WriteString(currencyName(m.pendingCurrencyID, m.currencyIndex))
			b.WriteString("\n")
			b.WriteString(dimStyle.Render("Amount:      "))
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
			b.WriteString(fmt.Sprintf("  account %d: %s\n", e.AccountID, formatAmount(e.Amount, m.currencyIndex, e.CurrencyID)))
		}
		b.WriteString("\n")
		b.WriteString(helpStyle.Render("esc/enter: back"))

	case transactionsModeConfirmDelete:
		b.WriteString(m.tbl.View())
		b.WriteString("\n\n")
		b.WriteString(confirmDeletePrompt("transaction"))

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
