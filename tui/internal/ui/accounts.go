package ui

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"charm.land/bubbles/v2/table"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"

	"money/tui/internal/client"
)

type accountsMode int

const (
	accountsModeList accountsMode = iota
	accountsModeCreate
	accountsModeConfirmDelete
	accountsModeLedger
)

// accountCreateStep is one step of the "new account" wizard: pick an
// optional parent from the existing accounts first, then a name, then an
// optional code.
type accountCreateStep int

const (
	stepPickParent accountCreateStep = iota
	stepAccountName
	stepAccountCode
)

type accountsModel struct {
	client *client.Client

	mode   accountsMode
	table  table.Model
	rows   []client.Account
	status string
	err    string

	createStep      accountCreateStep
	parentPicker    table.Model
	pendingParentID *int64
	inputs          []textinput.Model

	ledgerAccount client.Account
	ledgerTable   table.Model
}

const (
	fieldAccountName = iota
	fieldAccountCode
)

// moneyColumnWidth is the width of every "Value"/"Balance" table column;
// cell content for those columns is right-aligned to this same width (see
// rightAlign), since bubbles' table has no per-column alignment option.
const moneyColumnWidth = 12

func newAccountsModel(c *client.Client) accountsModel {
	columns := []table.Column{
		{Title: "ID", Width: 6},
		{Title: "Code", Width: 10},
		{Title: "Name", Width: 30},
		{Title: "Balance", Width: moneyColumnWidth},
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

	parentPicker := table.New(
		table.WithColumns([]table.Column{{Title: "Account", Width: 40}}),
		table.WithFocused(true),
		table.WithHeight(8),
	)

	ledgerTable := table.New(
		table.WithColumns([]table.Column{
			{Title: "Timestamp", Width: 17},
			{Title: "Description", Width: 30},
			{Title: "Value", Width: moneyColumnWidth},
			{Title: "Balance", Width: moneyColumnWidth},
		}),
		table.WithFocused(true),
		table.WithHeight(10),
	)

	return accountsModel{
		client:       c,
		table:        t,
		parentPicker: parentPicker,
		inputs:       []textinput.Model{name, code},
		ledgerTable:  ledgerTable,
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
	m.parentPicker.SetWidth(width)
	m.ledgerTable.SetWidth(width)
	if height > 5 {
		m.table.SetHeight(height)
		m.parentPicker.SetHeight(height)
	}
	// The ledger view has its own two-line header ("Transactions for #N
	// Name" plus a blank line) above the table, unlike the plain list, so
	// it needs two fewer rows to keep the footer on screen.
	if height > 7 {
		m.ledgerTable.SetHeight(height - 2)
	}
}

type accountsLoadedMsg struct {
	accounts []client.Account
	err      error
}

type accountMutatedMsg struct {
	err error
}

type ledgerLoadedMsg struct {
	entries []client.LedgerEntry
	err     error
}

func (m accountsModel) loadAccounts() tea.Msg {
	accounts, err := m.client.ListAccounts(context.Background())
	return accountsLoadedMsg{accounts: accounts, err: err}
}

func (m accountsModel) loadLedger(accountID int64) tea.Cmd {
	c := m.client
	return func() tea.Msg {
		entries, err := c.GetAccountLedger(context.Background(), accountID)
		return ledgerLoadedMsg{entries: entries, err: err}
	}
}

func (m accountsModel) Update(msg tea.Msg) (accountsModel, tea.Cmd) {
	switch msg := msg.(type) {
	case accountsLoadedMsg:
		if msg.err != nil {
			m.err = msg.err.Error()
			return m, nil
		}
		m.err = ""
		// m.rows must be kept in the exact same order as the table's rows,
		// since "enter"/"d" look up the account under the cursor by
		// indexing into m.rows — and the table displays accounts in tree
		// order (children under their parent), not the API's ID order.
		nodes := orderAccountsAsTree(msg.accounts)
		m.rows = make([]client.Account, len(nodes))
		for i, n := range nodes {
			m.rows[i] = n.account
		}
		m.table.SetRows(nodesToRows(nodes))
		return m, nil

	case accountMutatedMsg:
		if msg.err != nil {
			m.err = msg.err.Error()
			return m, nil
		}
		m.err = ""
		m.mode = accountsModeList
		return m, m.loadAccounts

	case ledgerLoadedMsg:
		if msg.err != nil {
			m.err = msg.err.Error()
			return m, nil
		}
		m.err = ""
		m.ledgerTable.SetRows(ledgerToRows(msg.entries))
		return m, nil

	case tea.KeyMsg:
		switch m.mode {
		case accountsModeList:
			return m.updateList(msg)
		case accountsModeCreate:
			return m.updateCreate(msg)
		case accountsModeConfirmDelete:
			return m.updateConfirmDelete(msg)
		case accountsModeLedger:
			return m.updateLedger(msg)
		}
	}

	switch m.mode {
	case accountsModeList:
		var cmd tea.Cmd
		m.table, cmd = m.table.Update(msg)
		return m, cmd
	case accountsModeLedger:
		var cmd tea.Cmd
		m.ledgerTable, cmd = m.ledgerTable.Update(msg)
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
		m.createStep = stepPickParent
		m.pendingParentID = nil
		m.parentPicker.SetRows(parentPickerRows(m.rows))
		m.parentPicker.SetCursor(0)
		for i := range m.inputs {
			m.inputs[i].SetValue("")
			m.inputs[i].Blur()
		}
		m.err = ""
		return m, nil
	case "d":
		if len(m.rows) == 0 {
			return m, nil
		}
		m.mode = accountsModeConfirmDelete
		return m, nil
	case "enter":
		row := m.table.Cursor()
		if row < 0 || row >= len(m.rows) {
			return m, nil
		}
		m.mode = accountsModeLedger
		m.ledgerAccount = m.rows[row]
		m.ledgerTable.SetRows(nil)
		m.err = ""
		return m, m.loadLedger(m.ledgerAccount.ID)
	}
	var cmd tea.Cmd
	m.table, cmd = m.table.Update(msg)
	return m, cmd
}

func (m accountsModel) updateLedger(msg tea.KeyMsg) (accountsModel, tea.Cmd) {
	switch msg.String() {
	case "esc", "q":
		m.mode = accountsModeList
		m.err = ""
		return m, nil
	}
	var cmd tea.Cmd
	m.ledgerTable, cmd = m.ledgerTable.Update(msg)
	return m, cmd
}

func (m accountsModel) updateCreate(msg tea.KeyMsg) (accountsModel, tea.Cmd) {
	if msg.String() == "esc" {
		m.mode = accountsModeList
		m.err = ""
		return m, nil
	}

	switch m.createStep {
	case stepPickParent:
		if msg.String() == "enter" {
			m.pendingParentID = selectedParentID(m.parentPicker.Cursor(), m.rows)
			m.createStep = stepAccountName
			m.inputs[fieldAccountName].Focus()
			return m, nil
		}
		var cmd tea.Cmd
		m.parentPicker, cmd = m.parentPicker.Update(msg)
		return m, cmd

	case stepAccountName:
		if msg.String() == "enter" {
			if strings.TrimSpace(m.inputs[fieldAccountName].Value()) == "" {
				m.err = "name is required"
				return m, nil
			}
			m.err = ""
			m.inputs[fieldAccountName].Blur()
			m.createStep = stepAccountCode
			m.inputs[fieldAccountCode].Focus()
			return m, nil
		}
		var cmd tea.Cmd
		m.inputs[fieldAccountName], cmd = m.inputs[fieldAccountName].Update(msg)
		return m, cmd

	case stepAccountCode:
		if msg.String() == "enter" {
			return m.submitCreate()
		}
		var cmd tea.Cmd
		m.inputs[fieldAccountCode], cmd = m.inputs[fieldAccountCode].Update(msg)
		return m, cmd
	}
	return m, nil
}

// selectedParentID maps a parentPicker cursor position to the chosen
// parent account ID. Row 0 is always the "(none)" option.
func selectedParentID(cursor int, accounts []client.Account) *int64 {
	i := cursor - 1
	if i < 0 || i >= len(accounts) {
		return nil
	}
	id := accounts[i].ID
	return &id
}

func (m accountsModel) submitCreate() (accountsModel, tea.Cmd) {
	account := client.Account{
		Name:     strings.TrimSpace(m.inputs[fieldAccountName].Value()),
		ParentID: m.pendingParentID,
	}
	if code := strings.TrimSpace(m.inputs[fieldAccountCode].Value()); code != "" {
		account.Code = &code
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

// parentPickerRows lists the "(none)" option followed by every existing
// account, in the same order as accounts, so a picker cursor position can
// be mapped back to an account via selectedParentID.
func parentPickerRows(accounts []client.Account) []table.Row {
	rows := make([]table.Row, 0, len(accounts)+1)
	rows = append(rows, table.Row{"(none)"})
	for _, a := range accounts {
		rows = append(rows, table.Row{fmt.Sprintf("#%d  %s", a.ID, a.Name)})
	}
	return rows
}

func parentSummary(parentID *int64, accounts []client.Account) string {
	if parentID == nil {
		return "(none)"
	}
	for _, a := range accounts {
		if a.ID == *parentID {
			return fmt.Sprintf("#%d  %s", a.ID, a.Name)
		}
	}
	return fmt.Sprintf("#%d", *parentID)
}

// accountsToRows lays accounts out as an indented tree: each account is
// immediately followed by its own children, and accounts at the same
// level (no parent, or the same parent) are ordered by ID.
func accountsToRows(accounts []client.Account) []table.Row {
	return nodesToRows(orderAccountsAsTree(accounts))
}

func nodesToRows(nodes []accountTreeNode) []table.Row {
	rows := make([]table.Row, 0, len(nodes))
	for _, node := range nodes {
		a := node.account
		code := ""
		if a.Code != nil {
			code = *a.Code
		}
		name := strings.Repeat("  ", node.depth) + a.Name
		rows = append(rows, table.Row{strconv.FormatInt(a.ID, 10), code, name, rightAlign(formatCents(a.Balance), moneyColumnWidth)})
	}
	return rows
}

type accountTreeNode struct {
	account client.Account
	depth   int
}

// orderAccountsAsTree walks the parent/child forest depth-first: root
// accounts (no parent) first, each one immediately followed by its own
// children, and siblings at every level ordered by ID.
func orderAccountsAsTree(accounts []client.Account) []accountTreeNode {
	childrenByParent := make(map[int64][]client.Account)
	var roots []client.Account
	for _, a := range accounts {
		if a.ParentID == nil {
			roots = append(roots, a)
		} else {
			childrenByParent[*a.ParentID] = append(childrenByParent[*a.ParentID], a)
		}
	}
	sortAccountsByID(roots)
	for id := range childrenByParent {
		sortAccountsByID(childrenByParent[id])
	}

	var nodes []accountTreeNode
	visited := make(map[int64]bool, len(accounts))
	var walk func(a client.Account, depth int)
	walk = func(a client.Account, depth int) {
		if visited[a.ID] {
			return // guards against a parent_id cycle
		}
		visited[a.ID] = true
		nodes = append(nodes, accountTreeNode{account: a, depth: depth})
		for _, child := range childrenByParent[a.ID] {
			walk(child, depth+1)
		}
	}
	for _, r := range roots {
		walk(r, 0)
	}

	// An account whose ancestors form a cycle (possible since nothing
	// prevents setting an account's parent to one of its own
	// descendants) is unreachable from any root; still show it rather
	// than silently dropping it from the list.
	var leftover []client.Account
	for _, a := range accounts {
		if !visited[a.ID] {
			leftover = append(leftover, a)
		}
	}
	sortAccountsByID(leftover)
	for _, a := range leftover {
		walk(a, 0)
	}

	return nodes
}

func sortAccountsByID(accounts []client.Account) {
	sort.Slice(accounts, func(i, j int) bool { return accounts[i].ID < accounts[j].ID })
}

// rightAlign right-justifies s within width, for "Value"/"Balance" table
// cells (bubbles' table has no per-column alignment option, and left-
// aligns cell content by default).
func rightAlign(s string, width int) string {
	return fmt.Sprintf("%*s", width, s)
}

func ledgerToRows(entries []client.LedgerEntry) []table.Row {
	rows := make([]table.Row, 0, len(entries))
	for _, e := range entries {
		rows = append(rows, table.Row{
			e.Timestamp.Local().Format(timestampLayout),
			e.Description,
			rightAlign(formatCents(e.Value), moneyColumnWidth),
			rightAlign(formatCents(e.Balance), moneyColumnWidth),
		})
	}
	return rows
}

func (m accountsModel) View() string {
	var b strings.Builder

	switch m.mode {
	case accountsModeCreate:
		b.WriteString(formLabelStyle.Render("New account"))
		b.WriteString("\n\n")

		if m.createStep == stepPickParent {
			b.WriteString(dimStyle.Render("Parent account (↑/↓ to choose, enter to confirm):"))
			b.WriteString("\n")
			b.WriteString(m.parentPicker.View())
		} else {
			b.WriteString(dimStyle.Render("Parent: "))
			b.WriteString(parentSummary(m.pendingParentID, m.rows))
			b.WriteString("\n")
			b.WriteString(dimStyle.Render("Name: "))
			b.WriteString(m.inputs[fieldAccountName].View())
			b.WriteString("\n")
		}
		if m.createStep == stepAccountCode {
			b.WriteString(dimStyle.Render("Code: "))
			b.WriteString(m.inputs[fieldAccountCode].View())
			b.WriteString("\n")
		}
	case accountsModeConfirmDelete:
		b.WriteString(m.table.View())
		b.WriteString("\n\n")
		b.WriteString(errorStyle.Render("Delete selected account? (y/n)"))
	case accountsModeLedger:
		b.WriteString(formLabelStyle.Render(fmt.Sprintf("Transactions for #%d %s", m.ledgerAccount.ID, m.ledgerAccount.Name)))
		b.WriteString("\n\n")
		b.WriteString(m.ledgerTable.View())
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
