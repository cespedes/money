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

// createFocus identifies which of the "new account" form's three fields —
// all shown on a single line — currently has focus.
type createFocus int

const (
	focusParent createFocus = iota
	focusName
	focusCode
)

type accountsModel struct {
	client *client.Client

	mode       accountsMode
	table      table.Model
	rows       []client.Account
	currencies currencyByID // for rendering Balances/ledger amounts
	status     string
	err        string

	createFocus createFocus
	// editingID is the account being edited's ID, or nil while creating a
	// new one — the same pop-up (see createPopup/startEdit) is reused for
	// both, and submitCreate branches on this to POST or PUT.
	editingID *int64
	// parentOptions is whichever accounts are currently offered as a
	// parent choice — m.rows when creating, or m.rows with the account
	// being edited filtered out (see startEdit, accountsExcluding) — kept
	// alongside parentPicker so selectedParentID can map its cursor back
	// to an account ID regardless of which case built the dropdown.
	parentOptions []client.Account
	parentPicker  table.Model // dropdown of "(none)" + parentOptions, for the parent field
	inputs        []textinput.Model

	ledgerAccount client.Account
	ledgerEntries []client.LedgerEntry
	ledgerTable   table.Model

	// windowHeight is the last content height passed to SetSize, kept so
	// the parent dropdown can be resized again when the account list
	// changes (see syncParentPickerHeight).
	windowHeight int

	// selectAfterReload, when set, is an account ID whose row the cursor
	// should jump back to the next time accountsLoadedMsg arrives — used
	// after a move (see "shift+up"/"shift+down" in updateList) so the
	// moved account visibly stays selected instead of the cursor
	// pointing at whichever account now occupies its old row index.
	selectAfterReload *int64
}

const (
	fieldAccountName = iota
	fieldAccountCode
)

// moneyColumnWidth is the width of every single-amount "Value"/"Balance"
// table column (wide enough for a currency name plus a formatted
// amount); cell content for those columns is right-aligned to this same
// width (see rightAlign), since bubbles' table has no per-column
// alignment option.
const moneyColumnWidth = 18

// balancesColumnWidth is wider still, for the accounts list's "Balances"
// column, which can hold more than one currency's amount at once.
const balancesColumnWidth = 40

// createFieldWidth is the fixed column width of every field in the "new
// account" pop-up (Parent, Name, Code), so their values line up in a grid
// under their column headers.
const createFieldWidth = 16

// parentPickerWidth spans the same total width as the three field columns
// together, so the parent dropdown lines up under the row above it.
const parentPickerWidth = 3*createFieldWidth + 4

func newAccountsModel(c *client.Client) accountsModel {
	columns := []table.Column{
		{Title: "ID", Width: 6},
		{Title: "Code", Width: 10},
		{Title: "Name", Width: 30},
		{Title: "Balances", Width: balancesColumnWidth},
	}
	t := table.New(
		table.WithColumns(columns),
		table.WithFocused(true),
		table.WithHeight(10),
	)

	name := textinput.New()
	name.Placeholder = "required"
	name.Prompt = ""
	name.SetWidth(createFieldWidth)
	code := textinput.New()
	code.Placeholder = "optional"
	code.Prompt = ""
	code.SetWidth(createFieldWidth)

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

	parentPicker := table.New(
		table.WithColumns([]table.Column{{Title: "", Width: parentPickerWidth}}),
		table.WithFocused(true),
		table.WithWidth(parentPickerWidth),
	)

	return accountsModel{
		client:       c,
		table:        t,
		inputs:       []textinput.Model{name, code},
		ledgerTable:  ledgerTable,
		parentPicker: parentPicker,
	}
}

func (m accountsModel) Init() tea.Cmd {
	return tea.Batch(m.loadAccounts, m.loadCurrencies)
}

func (m accountsModel) Editing() bool {
	return m.mode != accountsModeList
}

func (m *accountsModel) SetSize(width, height int) {
	m.table.SetWidth(width)
	m.ledgerTable.SetWidth(width)
	if height > 5 {
		m.table.SetHeight(height)
	}
	// The ledger view has its own two-line header ("Transactions for #N
	// Name" plus a blank line) above the table, unlike the plain list, so
	// it needs two fewer rows to keep the footer on screen.
	if height > 7 {
		m.ledgerTable.SetHeight(height - 2)
	}
	m.windowHeight = height
	m.syncParentPickerHeight()
}

// syncParentPickerHeight sizes the parent dropdown to show every option at
// once, or as many as fit in the window: it's capped by both the number
// of options and the space left in the terminal once the pop-up's own
// chrome (title, header/value row, borders, padding) is accounted for.
func (m *accountsModel) syncParentPickerHeight() {
	const popupChrome = 10
	available := m.windowHeight - popupChrome
	if available < 3 {
		available = 3
	}
	numOptions := len(m.rows)
	if m.editingID != nil {
		numOptions-- // the account being edited is excluded from its own Parent dropdown
	}
	// SetHeight's argument covers the table's own header row too, on top
	// of the data rows actually wanted: "(none)" plus every option.
	want := numOptions + 1 /* "(none)" */ + 1 /* header row */
	if want > available {
		want = available
	}
	m.parentPicker.SetHeight(want)
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

func (m accountsModel) loadCurrencies() tea.Msg {
	currencies, err := m.client.ListCurrencies(context.Background())
	return currenciesLoadedMsg{currencies: currencies, err: err}
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
		m.table.SetRows(nodesToRows(nodes, m.currencies))
		m.syncParentPickerHeight()
		if m.selectAfterReload != nil {
			for i, a := range m.rows {
				if a.ID == *m.selectAfterReload {
					m.table.SetCursor(i)
					break
				}
			}
			m.selectAfterReload = nil
		}
		return m, nil

	case currenciesLoadedMsg:
		if msg.err != nil {
			m.err = msg.err.Error()
			return m, nil
		}
		m.err = ""
		m.currencies = indexCurrencies(msg.currencies)
		// Re-render whatever's already on screen so amounts reflect the
		// freshly (re)loaded currency formatting, regardless of whether
		// accounts or the ledger loaded first.
		if m.rows != nil {
			m.table.SetRows(nodesToRows(orderAccountsAsTree(m.rows), m.currencies))
		}
		if m.ledgerEntries != nil {
			m.ledgerTable.SetRows(ledgerToRows(m.ledgerEntries, m.currencies))
		}
		return m, nil

	case accountMutatedMsg:
		if msg.err != nil {
			m.err = msg.err.Error()
			return m, nil
		}
		m.err = ""
		m.mode = accountsModeList
		m.editingID = nil
		return m, m.loadAccounts

	case ledgerLoadedMsg:
		if msg.err != nil {
			m.err = msg.err.Error()
			return m, nil
		}
		m.err = ""
		m.ledgerEntries = msg.entries
		m.ledgerTable.SetRows(ledgerToRows(msg.entries, m.currencies))
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
		m.editingID = nil
		m.parentOptions = m.rows
		m.parentPicker.SetRows(parentDropdownRows(m.parentOptions))
		m.parentPicker.SetCursor(0)
		m.syncParentPickerHeight()
		for i := range m.inputs {
			m.inputs[i].SetValue("")
		}
		m.setCreateFocus(focusParent)
		m.err = ""
		return m, nil
	case "e":
		if len(m.rows) == 0 {
			return m, nil
		}
		row := m.table.Cursor()
		if row < 0 || row >= len(m.rows) {
			return m, nil
		}
		m.startEdit(m.rows[row])
		return m, nil
	case "d":
		if len(m.rows) == 0 {
			return m, nil
		}
		m.mode = accountsModeConfirmDelete
		return m, nil
	case "shift+up", "shift+down":
		row := m.table.Cursor()
		if row < 0 || row >= len(m.rows) {
			return m, nil
		}
		direction := client.MoveUp
		if msg.String() == "shift+down" {
			direction = client.MoveDown
		}
		id := m.rows[row].ID
		m.selectAfterReload = &id
		c := m.client
		return m, func() tea.Msg {
			err := c.MoveAccount(context.Background(), id, direction)
			return accountMutatedMsg{err: err}
		}
	case "enter":
		row := m.table.Cursor()
		if row < 0 || row >= len(m.rows) {
			return m, nil
		}
		m.mode = accountsModeLedger
		m.ledgerAccount = m.rows[row]
		m.ledgerEntries = nil
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

// updateCreate handles the "new account" form: Parent, Name, and Code are
// all shown on one row, and tab/shift+tab or left/right move focus
// between them (arrow keys always change focus rather than the text
// cursor, so there's one consistent way to navigate the form). While the
// Parent field has focus, a dropdown of "(none)" plus the existing
// accounts is shown (see createPopup), navigated like any other table.
func (m accountsModel) updateCreate(msg tea.KeyMsg) (accountsModel, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.mode = accountsModeList
		m.editingID = nil
		m.err = ""
		return m, nil
	case "enter":
		return m.submitForm()
	case "tab", "right":
		m.setCreateFocus((m.createFocus + 1) % 3)
		return m, nil
	case "shift+tab", "left":
		m.setCreateFocus((m.createFocus + 2) % 3) // +2 mod 3 == -1 mod 3
		return m, nil
	}

	switch m.createFocus {
	case focusParent:
		var cmd tea.Cmd
		m.parentPicker, cmd = m.parentPicker.Update(msg)
		return m, cmd
	case focusName:
		var cmd tea.Cmd
		m.inputs[fieldAccountName], cmd = m.inputs[fieldAccountName].Update(msg)
		return m, cmd
	case focusCode:
		var cmd tea.Cmd
		m.inputs[fieldAccountCode], cmd = m.inputs[fieldAccountCode].Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m *accountsModel) setCreateFocus(f createFocus) {
	m.createFocus = f
	if f == focusName {
		m.inputs[fieldAccountName].Focus()
	} else {
		m.inputs[fieldAccountName].Blur()
	}
	if f == focusCode {
		m.inputs[fieldAccountCode].Focus()
	} else {
		m.inputs[fieldAccountCode].Blur()
	}
}

// selectedParentID maps a parentPicker cursor position to the chosen
// parent account ID, within options (m.parentOptions, not necessarily
// m.rows — see startEdit). Index 0 is always the "(none)" option.
func selectedParentID(choice int, options []client.Account) *int64 {
	i := choice - 1
	if i < 0 || i >= len(options) {
		return nil
	}
	id := options[i].ID
	return &id
}

// parentCursorFor is selectedParentID's inverse, used to preselect an
// account's current parent when opening it for editing (see startEdit).
// Falls back to 0 ("(none)") if parentID isn't found in options — e.g. it
// was filtered out because it's the account being edited itself.
func parentCursorFor(parentID *int64, options []client.Account) int {
	if parentID == nil {
		return 0
	}
	for i, a := range options {
		if a.ID == *parentID {
			return i + 1
		}
	}
	return 0
}

// accountsExcluding returns accounts with the one matching excludeID left
// out, preserving order. Used to keep an account being edited out of its
// own Parent dropdown — choosing itself would form a single-node cycle.
func accountsExcluding(accounts []client.Account, excludeID int64) []client.Account {
	out := make([]client.Account, 0, len(accounts))
	for _, a := range accounts {
		if a.ID != excludeID {
			out = append(out, a)
		}
	}
	return out
}

// startEdit opens the same pop-up as "n" (see createPopup), pre-filled
// with account's current values, for submitForm to PUT back on enter
// instead of POSTing a new account.
func (m *accountsModel) startEdit(account client.Account) {
	m.mode = accountsModeCreate
	id := account.ID
	m.editingID = &id
	m.parentOptions = accountsExcluding(m.rows, account.ID)
	m.parentPicker.SetRows(parentDropdownRows(m.parentOptions))
	m.parentPicker.SetCursor(parentCursorFor(account.ParentID, m.parentOptions))
	m.syncParentPickerHeight()
	m.inputs[fieldAccountName].SetValue(account.Name)
	code := ""
	if account.Code != nil {
		code = *account.Code
	}
	m.inputs[fieldAccountCode].SetValue(code)
	m.setCreateFocus(focusParent)
	m.err = ""
}

func (m accountsModel) submitForm() (accountsModel, tea.Cmd) {
	name := strings.TrimSpace(m.inputs[fieldAccountName].Value())
	if name == "" {
		m.err = "name is required"
		m.setCreateFocus(focusName)
		return m, nil
	}
	account := client.Account{
		Name:     name,
		ParentID: selectedParentID(m.parentPicker.Cursor(), m.parentOptions),
	}
	if code := strings.TrimSpace(m.inputs[fieldAccountCode].Value()); code != "" {
		account.Code = &code
	}

	m.err = ""
	c := m.client
	if m.editingID != nil {
		id := *m.editingID
		return m, func() tea.Msg {
			_, err := c.UpdateAccount(context.Background(), id, account)
			return accountMutatedMsg{err: err}
		}
	}
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

// parentDropdownRows lists the "(none)" option followed by every existing
// account, in the same order as accounts, so a parentPicker cursor
// position can be mapped back to an account via selectedParentID.
func parentDropdownRows(accounts []client.Account) []table.Row {
	rows := make([]table.Row, 0, len(accounts)+1)
	rows = append(rows, table.Row{"(none)"})
	for _, a := range accounts {
		rows = append(rows, table.Row{fmt.Sprintf("#%d  %s", a.ID, a.Name)})
	}
	return rows
}

// accountsToRows lays accounts out as an indented tree: each account is
// immediately followed by its own children, and accounts at the same
// level (no parent, or the same parent) are ordered by ID.
func accountsToRows(accounts []client.Account, currencies currencyByID) []table.Row {
	return nodesToRows(orderAccountsAsTree(accounts), currencies)
}

func nodesToRows(nodes []accountTreeNode, currencies currencyByID) []table.Row {
	rows := make([]table.Row, 0, len(nodes))
	for _, node := range nodes {
		a := node.account
		code := ""
		if a.Code != nil {
			code = *a.Code
		}
		name := strings.Repeat("  ", node.depth) + a.Name
		rows = append(rows, table.Row{strconv.FormatInt(a.ID, 10), code, name, formatBalances(a.Balances, currencies)})
	}
	return rows
}

// formatBalances renders an account's per-currency balances (see
// client.Account.Balances) as a single comma-separated cell, since
// bubbles' table has no notion of a multi-line cell.
func formatBalances(balances []client.CurrencyAmount, currencies currencyByID) string {
	if len(balances) == 0 {
		return ""
	}
	parts := make([]string, len(balances))
	for i, b := range balances {
		parts[i] = formatAmount(b.Amount, currencies, b.CurrencyID)
	}
	return strings.Join(parts, ", ")
}

type accountTreeNode struct {
	account client.Account
	depth   int
}

// orderAccountsAsTree walks the parent/child forest depth-first: root
// accounts (no parent) first, each one immediately followed by its own
// children, and siblings at every level ordered by Position (see
// AccountStore.Move) — ID only breaks a tie, which position values
// assigned by the API should never actually produce.
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
	sortAccountsByPosition(roots)
	for id := range childrenByParent {
		sortAccountsByPosition(childrenByParent[id])
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
	sortAccountsByPosition(leftover)
	for _, a := range leftover {
		walk(a, 0)
	}

	return nodes
}

func sortAccountsByPosition(accounts []client.Account) {
	sort.Slice(accounts, func(i, j int) bool {
		if accounts[i].Position != accounts[j].Position {
			return accounts[i].Position < accounts[j].Position
		}
		return accounts[i].ID < accounts[j].ID
	})
}

// rightAlign right-justifies s within width, for "Value"/"Balance" table
// cells (bubbles' table has no per-column alignment option, and left-
// aligns cell content by default).
func rightAlign(s string, width int) string {
	return fmt.Sprintf("%*s", width, s)
}

func ledgerToRows(entries []client.LedgerEntry, currencies currencyByID) []table.Row {
	rows := make([]table.Row, 0, len(entries))
	for _, e := range entries {
		rows = append(rows, table.Row{
			e.Timestamp.Local().Format(timestampLayout),
			e.Description,
			rightAlign(formatAmount(e.Amount, currencies, e.CurrencyID), moneyColumnWidth),
			rightAlign(formatAmount(e.Balance, currencies, e.CurrencyID), moneyColumnWidth),
		})
	}
	return rows
}

// createPopup renders the "new"/"edit account" form (see startEdit) as a
// bordered, table-shaped pop-up: a header row naming the three fields
// (Parent, Name, Code) above a row of Name/Code's fixed-width values,
// with the focused column highlighted. Parent has no value of its own in
// that row — while it has focus, a dropdown listing "(none)" plus every
// eligible account is shown below instead, with the currently chosen one
// highlighted by the dropdown's own cursor — sized (see
// syncParentPickerHeight) to show them all at once, or as many as fit in
// the window. It's meant to be composited over the accounts list via
// overlayCentered, not shown in place of it.
func (m accountsModel) createPopup() string {
	title := "New account"
	if m.editingID != nil {
		title = "Edit account"
	}

	headers := make([]string, 3)
	for i, label := range []string{"Parent", "Name", "Code"} {
		headers[i] = columnHeader(label, createFocus(i) == m.createFocus)
	}

	values := []string{
		padOrTruncate("", createFieldWidth),
		m.inputs[fieldAccountName].View(),
		m.inputs[fieldAccountCode].View(),
	}

	content := formLabelStyle.Render(title) + "\n\n" +
		strings.Join(headers, "  ") + "\n" +
		strings.Join(values, "  ")
	if m.createFocus == focusParent {
		content += "\n" + m.parentPicker.View()
	}
	if m.err != "" {
		content += "\n\n" + errorStyle.Render("Error: "+m.err)
	}
	return popupStyle.Render(content)
}

func (m accountsModel) View() string {
	var b strings.Builder

	switch m.mode {
	case accountsModeConfirmDelete:
		b.WriteString(m.table.View())
		b.WriteString("\n\n")
		b.WriteString(errorStyle.Render("Delete selected account? (y/n)"))
	case accountsModeLedger:
		b.WriteString(formLabelStyle.Render(fmt.Sprintf("Transactions for #%d %s", m.ledgerAccount.ID, m.ledgerAccount.Name)))
		b.WriteString("\n\n")
		b.WriteString(m.ledgerTable.View())
	default:
		// accountsModeCreate shows its own pop-up (see createPopup),
		// composited over this same list view by App.View — so the list
		// is still the right thing to render underneath it here.
		b.WriteString(m.table.View())
	}

	// The create pop-up carries its own error text, so it isn't repeated
	// here.
	if m.err != "" && m.mode != accountsModeCreate {
		b.WriteString("\n\n")
		b.WriteString(errorStyle.Render("Error: " + m.err))
	} else if m.status != "" && m.mode == accountsModeList {
		b.WriteString("\n\n")
		b.WriteString(statusStyle.Render(m.status))
	}

	return b.String()
}
