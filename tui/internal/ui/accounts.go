package ui

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

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
	accountsModeLedgerCreate
)

// createFocus identifies which of the "new account" form's three fields —
// all shown on a single line — currently has focus.
type createFocus int

const (
	focusParent createFocus = iota
	focusName
	focusCode
)

// ledgerEntryFocus identifies which of the ledger's "new entry" form's
// seven fields currently has focus (see startLedgerEntry): a first row
// (Timestamp, Description, Amount, Currency) for the account whose
// ledger is open, and a second (Account, Amount, Currency) for the
// other side of the transaction.
type ledgerEntryFocus int

const (
	focusEntryTimestamp ledgerEntryFocus = iota
	focusEntryDescription
	focusEntryAmount
	focusEntryCurrency
	focusEntryOtherAccount
	focusEntryOtherAmount
	focusEntryOtherCurrency
	numLedgerEntryFocusFields
)

type accountsModel struct {
	client *client.Client

	mode       accountsMode
	table      table.Model
	rows       []client.Account
	currencies currencyByID // for rendering Balances/ledger amounts
	// currencyList is currencies' data as an ordered slice instead — for
	// the ledger's "new entry" form's currency picker (see
	// startLedgerEntry), which needs index-based navigation that a map
	// can't give it.
	currencyList []client.Currency
	status       string
	err          string

	createFocus createFocus
	// editingID is the account being edited's ID, or nil while creating a
	// new one — the same pop-up (see createPopup/startEdit) is reused for
	// both, and submitCreate branches on this to POST or PUT.
	editingID *int64
	// parentOptions is whichever accounts are currently offered as a
	// parent choice, as an indented tree (see orderAccountsAsTree) so the
	// dropdown can show each one's depth — m.rows when creating, or
	// m.rows with the account being edited and all its descendants
	// filtered out (see startEdit, accountsExcludingSubtree) — kept
	// alongside parentPicker so selectedParentID can map its cursor back
	// to an account ID regardless of which case built the dropdown.
	parentOptions []accountTreeNode
	parentPicker  table.Model // dropdown of "(none)" + parentOptions, for the parent field
	inputs        []textinput.Model

	ledgerAccount client.Account
	ledgerEntries []client.LedgerEntry
	ledgerTable   table.Model

	// Ledger "new entry" form state (see startLedgerEntry): a compact,
	// two-account, table-shaped transaction form reached from the ledger
	// view, distinct from the Transactions tab's own multi-entry creation
	// wizard.
	ledgerEntryFocus ledgerEntryFocus
	// ledgerEntryInputs holds [timestamp, description, amount,
	// otherAmount] (see fieldEntryTimestamp etc.) — both Currency fields
	// and Other account are pickers instead, not plain text fields.
	ledgerEntryInputs         []textinput.Model
	ledgerCurrencyPicker      table.Model // this account's currency (row 1)
	ledgerOtherCurrencyPicker table.Model // the other entry's currency (row 2)
	// ledgerOtherAccountOptions is every account except the one whose
	// ledger is open, as an indented tree (see orderAccountsAsTree),
	// mirroring how parentOptions works for the account form's Parent
	// field — kept alongside ledgerAccountPicker so selectedAccountID can
	// map its cursor back to an account ID.
	ledgerOtherAccountOptions []accountTreeNode
	ledgerAccountPicker       table.Model

	// windowHeight is the last content height passed to SetSize, kept so
	// the parent dropdown can be resized again when the account list
	// changes (see syncParentPickerHeight).
	windowHeight int
	// windowWidth is the last content width passed to SetSize, kept so
	// the ledger table's columns can be resized again when its entries
	// or currencies change (see syncLedgerTable).
	windowWidth int

	// selectAfterReload, when set, is an account ID whose row the cursor
	// should jump back to the next time accountsLoadedMsg arrives — used
	// after a move (see "K"/"J" in updateList) so the
	// moved account visibly stays selected instead of the cursor
	// pointing at whichever account now occupies its old row index.
	selectAfterReload *int64
}

const (
	fieldAccountName = iota
	fieldAccountCode
)

// Indices into ledgerEntryInputs (see accountsModel).
const (
	fieldEntryTimestamp = iota
	fieldEntryDescription
	fieldEntryAmount
	fieldEntryOtherAmount
)

// balancesColumnWidth is for the accounts list's "Balances"
// column, which can hold more than one currency's amount at once.
const balancesColumnWidth = 40

// createFieldWidth is the fixed column width of every field in the "new
// account" pop-up (Parent, Name, Code), so their values line up in a grid
// under their column headers.
const createFieldWidth = 16

// parentPickerWidth spans the same total width as the three field columns
// together, so the parent dropdown lines up under the row above it.
const parentPickerWidth = 3*createFieldWidth + 4

// columnGap is the whitespace joining adjacent field columns in a pop-up
// form (see createPopup/ledgerEntryPopup), matching the "  " literal
// those functions join columns with.
const columnGap = 2

// fieldPickerOffset is how many columns (0-indexed) precede a pop-up
// field whose dropdown should line up underneath it — see indentLines.
func fieldPickerOffset(columnIndex int) int {
	return columnIndex * (createFieldWidth + columnGap)
}

// ledgerPickerHeight is how many lines a stripPickerHeader'd dropdown
// view actually renders: the ledger entry form's three pickers (Currency
// x2, Other account) all share table.WithHeight(8), but that argument
// counts the table's own header row too (see table.Model.SetHeight), and
// stripPickerHeader already strips that row off — leaving 8-1 data rows.
const ledgerPickerHeight = 7

// ledgerCurrencyPickerWidth is the ledger entry form's Currency pickers'
// own column width — just wide enough for a currency's name (unlike
// account names, always short), matching the width of the Currency
// field column itself rather than borrowing the wider parentPickerWidth
// meant for account-name lists (Parent, Other account).
const ledgerCurrencyPickerWidth = createFieldWidth

// ledgerRow1PickerSlotWidth/ledgerRow2PickerSlotWidth are wide enough to
// hold whichever picker(s) can appear in that row of the ledger entry
// form — row 1 only ever shows its own Currency picker (the last of 4
// columns); row 2 shows either Other account (parentPickerWidth, in the
// first column, so no extra offset) or Currency (the last of 3 columns)
// — so a blank filler reserving this footprint (see pickerSlot) never
// has to shrink to make room for the picker that would go there.
// (fieldPickerOffset inlined: a const initializer can't call a func.)
const (
	ledgerRow1PickerSlotWidth = 3*(createFieldWidth+columnGap) + ledgerCurrencyPickerWidth
	ledgerRow2PickerSlotWidth = parentPickerWidth
)

// pickerSlot reserves a constant ledgerPickerHeight x width footprint
// for a dropdown that may or may not be showing right now — view when
// shown, a same-sized block of blank lines otherwise — so the ledger
// entry pop-up's total size, and thus its centered position, stays
// constant as focus moves between fields instead of visibly resizing
// whenever a dropdown appears or disappears.
func pickerSlot(shown bool, view string, width int) string {
	if shown {
		return view
	}
	line := strings.Repeat(" ", width)
	lines := make([]string, ledgerPickerHeight)
	for i := range lines {
		lines[i] = line
	}
	return strings.Join(lines, "\n")
}

func newAccountsModel(c *client.Client) accountsModel {
	columns := []table.Column{
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
		// A placeholder layout, replaced by syncLedgerTable's own
		// content- and width-aware columns as soon as the initial
		// WindowSizeMsg arrives (before anything is ever drawn).
		table.WithColumns(ledgerColumns(0, len("Value"), len("Balance"))),
		table.WithFocused(true),
		table.WithHeight(10),
	)

	parentPicker := table.New(
		table.WithColumns([]table.Column{{Title: "", Width: parentPickerWidth}}),
		table.WithFocused(true),
		table.WithWidth(parentPickerWidth),
	)

	entryTimestamp := textinput.New()
	entryTimestamp.Prompt = ""
	entryTimestamp.SetWidth(createFieldWidth)
	entryDesc := textinput.New()
	entryDesc.Placeholder = "required"
	entryDesc.Prompt = ""
	entryDesc.SetWidth(createFieldWidth)
	entryAmount := textinput.New()
	entryAmount.Placeholder = "e.g. -10.50 or 10.50"
	entryAmount.Prompt = ""
	entryAmount.SetWidth(createFieldWidth)
	entryOtherAmount := textinput.New()
	entryOtherAmount.Placeholder = "optional"
	entryOtherAmount.Prompt = ""
	entryOtherAmount.SetWidth(createFieldWidth)

	ledgerCurrencyPicker := table.New(
		table.WithColumns([]table.Column{{Title: "", Width: ledgerCurrencyPickerWidth}}),
		table.WithFocused(true),
		table.WithWidth(ledgerCurrencyPickerWidth),
		table.WithHeight(8),
	)
	ledgerOtherCurrencyPicker := table.New(
		table.WithColumns([]table.Column{{Title: "", Width: ledgerCurrencyPickerWidth}}),
		table.WithFocused(true),
		table.WithWidth(ledgerCurrencyPickerWidth),
		table.WithHeight(8),
	)
	ledgerAccountPicker := table.New(
		table.WithColumns([]table.Column{{Title: "", Width: parentPickerWidth}}),
		table.WithFocused(true),
		table.WithWidth(parentPickerWidth),
		table.WithHeight(8),
	)

	return accountsModel{
		client:       c,
		table:        t,
		inputs:       []textinput.Model{name, code},
		ledgerTable:  ledgerTable,
		parentPicker: parentPicker,
		ledgerEntryInputs: []textinput.Model{
			entryTimestamp, entryDesc, entryAmount, entryOtherAmount,
		},
		ledgerCurrencyPicker:      ledgerCurrencyPicker,
		ledgerOtherCurrencyPicker: ledgerOtherCurrencyPicker,
		ledgerAccountPicker:       ledgerAccountPicker,
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
	m.windowWidth = width
	m.syncParentPickerHeight()
	m.syncLedgerTable()
}

// ledgerAmountColumnWidths returns how wide the ledger table's "Value"
// and "Balance" columns need to be to show every currently-loaded
// entry's amount and running balance without truncation, floored at
// each column's own header width. Most currencies' formatted amounts
// are much shorter than a generous fixed width would reserve, so
// sizing to the actual content — rather than a one-size-fits-all
// constant — leaves the rest of the table's width for Description.
func ledgerAmountColumnWidths(entries []client.LedgerEntry, currencies currencyByID) (valueWidth, balanceWidth int) {
	valueWidth, balanceWidth = len("Value"), len("Balance")
	for _, e := range entries {
		if w := len(formatAmount(e.Amount, currencies, e.CurrencyID)); w > valueWidth {
			valueWidth = w
		}
		if w := len(formatAmount(e.Balance, currencies, e.CurrencyID)); w > balanceWidth {
			balanceWidth = w
		}
	}
	return valueWidth, balanceWidth
}

// ledgerTimestampWidth and ledgerMinDescriptionWidth bound the ledger
// table's Timestamp and Description columns respectively (see
// ledgerColumns) — Timestamp is fixed, matching timestampLayout's own
// rendered length, and Description never shrinks below a floor even on
// a narrow terminal, the same way it was fixed at 30 before this table
// became width-aware.
const (
	ledgerTimestampWidth      = 17
	ledgerMinDescriptionWidth = 20
	// ledgerCellPadding is each column's own left+right Cell padding
	// (Padding(0, 1), see bubbles' table.DefaultStyles) not reflected in
	// a Column's own Width — 4 columns' worth must be subtracted from
	// the table's total width to know how much Description can actually
	// grow to without pushing the table wider than the terminal.
	ledgerCellPadding = 2
)

// ledgerColumns computes the ledger table's column layout for the given
// terminal width and already-measured Value/Balance widths (see
// ledgerAmountColumnWidths): Description gets whatever width is left
// over once Timestamp/Value/Balance and their cell padding are
// accounted for — so a wide terminal gives Description far more room,
// instead of leaving it empty to the right of mostly-blank, generously
// fixed-width Value/Balance columns.
func ledgerColumns(width, valueWidth, balanceWidth int) []table.Column {
	descriptionWidth := width - 4*ledgerCellPadding - ledgerTimestampWidth - valueWidth - balanceWidth
	if descriptionWidth < ledgerMinDescriptionWidth {
		descriptionWidth = ledgerMinDescriptionWidth
	}
	return []table.Column{
		{Title: "Timestamp", Width: ledgerTimestampWidth},
		{Title: "Description", Width: descriptionWidth},
		{Title: "Value", Width: valueWidth},
		{Title: "Balance", Width: balanceWidth},
	}
}

// syncLedgerTable recomputes the ledger table's columns and rows
// together from the model's current entries/currencies/window width —
// called whenever any of those change, since Value/Balance's widths
// (and so Description's) depend on the entries actually being shown
// (see ledgerAmountColumnWidths/ledgerColumns).
func (m *accountsModel) syncLedgerTable() {
	valueWidth, balanceWidth := ledgerAmountColumnWidths(m.ledgerEntries, m.currencies)
	m.ledgerTable.SetColumns(ledgerColumns(m.windowWidth, valueWidth, balanceWidth))
	m.ledgerTable.SetRows(ledgerToRows(m.ledgerEntries, m.currencies, valueWidth, balanceWidth))
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

type ledgerEntryMutatedMsg struct {
	err error
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
		m.currencyList = msg.currencies
		// Re-render whatever's already on screen so amounts reflect the
		// freshly (re)loaded currency formatting, regardless of whether
		// accounts or the ledger loaded first.
		if m.rows != nil {
			m.table.SetRows(nodesToRows(orderAccountsAsTree(m.rows), m.currencies))
		}
		if m.ledgerEntries != nil {
			m.syncLedgerTable()
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
		m.syncLedgerTable()
		return m, nil

	case ledgerEntryMutatedMsg:
		if msg.err != nil {
			m.err = msg.err.Error()
			return m, nil
		}
		m.err = ""
		m.mode = accountsModeLedger
		return m, m.loadLedger(m.ledgerAccount.ID)

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
		case accountsModeLedgerCreate:
			return m.updateLedgerCreate(msg)
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
		m.parentOptions = orderAccountsAsTree(m.rows)
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
	case "K", "J":
		// Capital letters, not Shift+Up/Shift+Down: some terminals (e.g.
		// xfce4-terminal) don't report Shift held with an arrow key as a
		// distinguishable event, but every terminal reports a capital
		// letter as a plain, unmodified keystroke.
		row := m.table.Cursor()
		if row < 0 || row >= len(m.rows) {
			return m, nil
		}
		direction := client.MoveUp
		if msg.String() == "J" {
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
	case "n":
		if len(m.currencyList) == 0 {
			m.err = "create a currency first (see the Currencies tab)"
			return m, nil
		}
		if len(accountsOtherThan(m.rows, m.ledgerAccount.ID)) == 0 {
			m.err = "create another account first"
			return m, nil
		}
		m.startLedgerEntry()
		return m, nil
	}
	var cmd tea.Cmd
	m.ledgerTable, cmd = m.ledgerTable.Update(msg)
	return m, cmd
}

// startLedgerEntry opens the ledger's compact "new entry" form (see
// ledgerEntryPopup), pre-filled with the current date/time, an empty
// description and amount, and the currency last used in this account's
// own ledger (or the first available currency if it has none yet) for
// both rows — a quick transaction between the account whose ledger is
// open and one other, picked from the rest.
func (m *accountsModel) startLedgerEntry() {
	m.mode = accountsModeLedgerCreate
	m.ledgerEntryInputs[fieldEntryTimestamp].SetValue(time.Now().Format(timestampLayout))
	m.ledgerEntryInputs[fieldEntryDescription].SetValue("")
	m.ledgerEntryInputs[fieldEntryAmount].SetValue("")
	m.ledgerEntryInputs[fieldEntryOtherAmount].SetValue("")

	defaultCurrencyCursor := currencyCursorFor(lastUsedCurrencyID(m.ledgerEntries, m.currencyList), m.currencyList)
	m.ledgerCurrencyPicker.SetRows(currencyPickerRows(m.currencyList))
	m.ledgerCurrencyPicker.SetCursor(defaultCurrencyCursor)
	m.ledgerOtherCurrencyPicker.SetRows(currencyPickerRows(m.currencyList))
	m.ledgerOtherCurrencyPicker.SetCursor(defaultCurrencyCursor)

	m.ledgerOtherAccountOptions = orderAccountsAsTree(accountsOtherThan(m.rows, m.ledgerAccount.ID))
	m.ledgerAccountPicker.SetRows(parentDropdownRows(m.ledgerOtherAccountOptions)[1:]) // drop the "(none)" row: not valid here
	m.ledgerAccountPicker.SetCursor(0)

	m.setLedgerEntryFocus(focusEntryTimestamp)
	m.err = ""
}

func (m *accountsModel) setLedgerEntryFocus(f ledgerEntryFocus) {
	m.ledgerEntryFocus = f
	for i := range m.ledgerEntryInputs {
		m.ledgerEntryInputs[i].Blur()
	}
	switch f {
	case focusEntryTimestamp:
		m.ledgerEntryInputs[fieldEntryTimestamp].Focus()
	case focusEntryDescription:
		m.ledgerEntryInputs[fieldEntryDescription].Focus()
	case focusEntryAmount:
		m.ledgerEntryInputs[fieldEntryAmount].Focus()
	case focusEntryOtherAmount:
		m.ledgerEntryInputs[fieldEntryOtherAmount].Focus()
	}
}

// updateLedgerCreate handles the ledger's "new entry" form: tab/shift+tab
// are the only way to move between its seven fields (matching the
// account form's own convention), enter submits from any of them, and
// esc cancels back to the ledger view.
func (m accountsModel) updateLedgerCreate(msg tea.KeyMsg) (accountsModel, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.mode = accountsModeLedger
		m.err = ""
		return m, nil
	case "enter":
		return m.submitLedgerEntry()
	case "tab":
		m.setLedgerEntryFocus((m.ledgerEntryFocus + 1) % numLedgerEntryFocusFields)
		return m, nil
	case "shift+tab":
		m.setLedgerEntryFocus((m.ledgerEntryFocus + numLedgerEntryFocusFields - 1) % numLedgerEntryFocusFields)
		return m, nil
	}

	switch m.ledgerEntryFocus {
	case focusEntryTimestamp:
		var cmd tea.Cmd
		m.ledgerEntryInputs[fieldEntryTimestamp], cmd = m.ledgerEntryInputs[fieldEntryTimestamp].Update(msg)
		return m, cmd
	case focusEntryDescription:
		var cmd tea.Cmd
		m.ledgerEntryInputs[fieldEntryDescription], cmd = m.ledgerEntryInputs[fieldEntryDescription].Update(msg)
		return m, cmd
	case focusEntryAmount:
		var cmd tea.Cmd
		m.ledgerEntryInputs[fieldEntryAmount], cmd = m.ledgerEntryInputs[fieldEntryAmount].Update(msg)
		return m, cmd
	case focusEntryCurrency:
		var cmd tea.Cmd
		m.ledgerCurrencyPicker, cmd = m.ledgerCurrencyPicker.Update(msg)
		return m, cmd
	case focusEntryOtherAccount:
		var cmd tea.Cmd
		m.ledgerAccountPicker, cmd = m.ledgerAccountPicker.Update(msg)
		return m, cmd
	case focusEntryOtherAmount:
		var cmd tea.Cmd
		m.ledgerEntryInputs[fieldEntryOtherAmount], cmd = m.ledgerEntryInputs[fieldEntryOtherAmount].Update(msg)
		return m, cmd
	case focusEntryOtherCurrency:
		var cmd tea.Cmd
		m.ledgerOtherCurrencyPicker, cmd = m.ledgerOtherCurrencyPicker.Update(msg)
		return m, cmd
	}
	return m, nil
}

// submitLedgerEntry validates and posts the two-entry transaction built
// by the ledger's "new entry" form: this account (whose ledger is open)
// for the first row's amount/currency, and whichever account is picked
// for the second row, with its own amount/currency. If the second row's
// amount is left blank, it's taken to be whatever balances the
// transaction (the first amount, negated) when both rows share the same
// currency — the only case where that's a well-defined default, since
// there's no exchange rate applied here; a blank amount is otherwise
// rejected, requiring an explicit one instead. Beyond that, this doesn't
// duplicate the API's own zero-sum-per-currency check the way the
// Transactions tab's general wizard does — an explicit pair of amounts
// that doesn't balance simply comes back as the same error that wizard
// would also surface.
func (m accountsModel) submitLedgerEntry() (accountsModel, tea.Cmd) {
	desc := strings.TrimSpace(m.ledgerEntryInputs[fieldEntryDescription].Value())
	if desc == "" {
		m.err = "description is required"
		m.setLedgerEntryFocus(focusEntryDescription)
		return m, nil
	}
	ts, err := time.ParseInLocation(timestampLayout, strings.TrimSpace(m.ledgerEntryInputs[fieldEntryTimestamp].Value()), time.Local)
	if err != nil {
		m.err = "timestamp must look like " + timestampLayout
		m.setLedgerEntryFocus(focusEntryTimestamp)
		return m, nil
	}
	currency, ok := currencyAt(m.ledgerCurrencyPicker.Cursor(), m.currencyList)
	if !ok {
		m.err = "pick a currency"
		m.setLedgerEntryFocus(focusEntryCurrency)
		return m, nil
	}
	amount, err := currency.ParseAmount(m.ledgerEntryInputs[fieldEntryAmount].Value())
	if err != nil {
		m.err = err.Error()
		m.setLedgerEntryFocus(focusEntryAmount)
		return m, nil
	}
	otherAccountID, ok := selectedAccountID(m.ledgerAccountPicker.Cursor(), m.ledgerOtherAccountOptions)
	if !ok {
		m.err = "pick another account"
		m.setLedgerEntryFocus(focusEntryOtherAccount)
		return m, nil
	}
	otherCurrency, ok := currencyAt(m.ledgerOtherCurrencyPicker.Cursor(), m.currencyList)
	if !ok {
		m.err = "pick a currency"
		m.setLedgerEntryFocus(focusEntryOtherCurrency)
		return m, nil
	}

	var otherAmount int64
	if raw := strings.TrimSpace(m.ledgerEntryInputs[fieldEntryOtherAmount].Value()); raw != "" {
		parsed, err := otherCurrency.ParseAmount(raw)
		if err != nil {
			m.err = err.Error()
			m.setLedgerEntryFocus(focusEntryOtherAmount)
			return m, nil
		}
		otherAmount = parsed
	} else if otherCurrency.ID == currency.ID {
		otherAmount = -amount // the only currency pairing where this has an unambiguous meaning
	} else {
		m.err = "amount is required when the two entries use different currencies"
		m.setLedgerEntryFocus(focusEntryOtherAmount)
		return m, nil
	}

	transaction := client.Transaction{
		Description: desc,
		Timestamp:   ts,
		Entries: []client.Entry{
			{AccountID: m.ledgerAccount.ID, Amount: currency.FromMinorUnits(amount), CurrencyID: currency.ID},
			{AccountID: otherAccountID, Amount: otherCurrency.FromMinorUnits(otherAmount), CurrencyID: otherCurrency.ID},
		},
	}

	m.err = ""
	c := m.client
	return m, func() tea.Msg {
		_, err := c.CreateTransaction(context.Background(), transaction)
		return ledgerEntryMutatedMsg{err: err}
	}
}

// updateCreate handles the "new account" form: Parent, Name, and Code are
// all shown on one row. tab/shift+tab are the only way to move focus
// between them, leaving left/right free to move the cursor within
// whichever text field has focus (or do nothing while Parent's dropdown
// has focus, since that's navigated with up/down like any other table).
// While Parent has focus, that dropdown of "(none)" plus the existing
// accounts is shown (see createPopup).
func (m accountsModel) updateCreate(msg tea.KeyMsg) (accountsModel, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.mode = accountsModeList
		m.editingID = nil
		m.err = ""
		return m, nil
	case "enter":
		return m.submitForm()
	case "tab":
		m.setCreateFocus((m.createFocus + 1) % 3)
		return m, nil
	case "shift+tab":
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
func selectedParentID(choice int, options []accountTreeNode) *int64 {
	i := choice - 1
	if i < 0 || i >= len(options) {
		return nil
	}
	id := options[i].account.ID
	return &id
}

// parentSummary renders parentID as it would read among options —
// "(none)" for nil, the account's name for a match, or "(unknown)" if
// it isn't among options (e.g. filtered out by startEdit) — for display
// in the Parent field once it no longer has focus and its dropdown is
// hidden (see createPopup).
func parentSummary(parentID *int64, options []accountTreeNode) string {
	if parentID == nil {
		return "(none)"
	}
	for _, n := range options {
		if n.account.ID == *parentID {
			return n.account.Name
		}
	}
	return "(unknown)"
}

// parentCursorFor is selectedParentID's inverse, used to preselect an
// account's current parent when opening it for editing (see startEdit).
// Falls back to 0 ("(none)") if parentID isn't found in options — e.g. it
// was filtered out because it's the account being edited itself.
func parentCursorFor(parentID *int64, options []accountTreeNode) int {
	if parentID == nil {
		return 0
	}
	for i, n := range options {
		if n.account.ID == *parentID {
			return i + 1
		}
	}
	return 0
}

// accountsExcludingSubtree returns accounts with rootID and every one of
// its descendants (transitively, via ParentID) left out, preserving
// order. Used to keep an account being edited — and everything under it
// — out of its own Parent dropdown: choosing itself would form a
// single-node cycle, and choosing any descendant would form a longer one
// (the API's AccountStore rejects both, but there's no reason to offer
// either as an option in the first place).
func accountsExcludingSubtree(accounts []client.Account, rootID int64) []client.Account {
	excluded := map[int64]bool{rootID: true}
	// Repeatedly sweep for newly-discovered descendants until a pass
	// finds none, so this doesn't depend on parents appearing before
	// their children in accounts.
	for {
		found := false
		for _, a := range accounts {
			if a.ParentID != nil && excluded[*a.ParentID] && !excluded[a.ID] {
				excluded[a.ID] = true
				found = true
			}
		}
		if !found {
			break
		}
	}

	out := make([]client.Account, 0, len(accounts))
	for _, a := range accounts {
		if !excluded[a.ID] {
			out = append(out, a)
		}
	}
	return out
}

// accountsOtherThan returns accounts with the one matching excludeID left
// out, preserving order — for the ledger's "new entry" form (see
// startLedgerEntry), whose "other account" picker must offer every
// account except the one whose ledger is being viewed.
func accountsOtherThan(accounts []client.Account, excludeID int64) []client.Account {
	out := make([]client.Account, 0, len(accounts))
	for _, a := range accounts {
		if a.ID != excludeID {
			out = append(out, a)
		}
	}
	return out
}

// selectedAccountID maps a picker cursor position to an account ID
// within options — like selectedParentID, but with no "(none)" choice at
// index 0, since a transaction entry always needs a real account (see
// ledgerAccountPicker).
func selectedAccountID(cursor int, options []accountTreeNode) (id int64, ok bool) {
	if cursor < 0 || cursor >= len(options) {
		return 0, false
	}
	return options[cursor].account.ID, true
}

// lastUsedCurrencyID is entries' most recent CurrencyID (entries is
// assumed sorted oldest-first, as GetAccountLedger returns them), or
// fallback's first currency if entries is empty, or 0 if both are — used
// to default the ledger's "new entry" form's currency to whatever this
// account was last posted in (see startLedgerEntry).
func lastUsedCurrencyID(entries []client.LedgerEntry, fallback []client.Currency) int64 {
	if len(entries) > 0 {
		return entries[len(entries)-1].CurrencyID
	}
	if len(fallback) > 0 {
		return fallback[0].ID
	}
	return 0
}

// currencyPickerRows lists every currency in currencies, in the same
// order, so a picker cursor position can be mapped back to one directly
// by index (see currencyAt) — no ID shown, matching the accounts list's
// own convention.
func currencyPickerRows(currencies []client.Currency) []table.Row {
	rows := make([]table.Row, 0, len(currencies))
	for _, c := range currencies {
		rows = append(rows, table.Row{c.Name})
	}
	return rows
}

// currencyAt maps a picker cursor position back to a currency within
// currencies (see currencyPickerRows).
func currencyAt(cursor int, currencies []client.Currency) (client.Currency, bool) {
	if cursor < 0 || cursor >= len(currencies) {
		return client.Currency{}, false
	}
	return currencies[cursor], true
}

// currencyCursorFor is the inverse of currencyAt's index mapping, used to
// preselect a currency (see startLedgerEntry) by ID. Falls back to 0 if
// id isn't found in currencies.
func currencyCursorFor(id int64, currencies []client.Currency) int {
	for i, c := range currencies {
		if c.ID == id {
			return i
		}
	}
	return 0
}

// startEdit opens the same pop-up as "n" (see createPopup), pre-filled
// with account's current values, for submitForm to PUT back on enter
// instead of POSTing a new account.
func (m *accountsModel) startEdit(account client.Account) {
	m.mode = accountsModeCreate
	id := account.ID
	m.editingID = &id
	m.parentOptions = orderAccountsAsTree(accountsExcludingSubtree(m.rows, account.ID))
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

// parentDropdownRows lists the "(none)" option followed by every eligible
// account as an indented tree (see orderAccountsAsTree), in the same
// order as options, so a parentPicker cursor position can be mapped back
// to an account via selectedParentID.
func parentDropdownRows(options []accountTreeNode) []table.Row {
	rows := make([]table.Row, 0, len(options)+1)
	rows = append(rows, table.Row{"(none)"})
	for _, n := range options {
		rows = append(rows, table.Row{strings.Repeat("  ", n.depth) + n.account.Name})
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
		rows = append(rows, table.Row{code, name, formatBalances(a.Balances, currencies)})
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

	// An account whose ancestors form a cycle is unreachable from any
	// root; still show it rather than silently dropping it from the
	// list. The API rejects any update that would create one (see
	// AccountStore.wouldCreateCycle), so this should only matter for
	// data that predates that check.
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

func ledgerToRows(entries []client.LedgerEntry, currencies currencyByID, valueWidth, balanceWidth int) []table.Row {
	rows := make([]table.Row, 0, len(entries))
	for _, e := range entries {
		rows = append(rows, table.Row{
			e.Timestamp.Local().Format(timestampLayout),
			e.Description,
			rightAlign(formatAmount(e.Amount, currencies, e.CurrencyID), valueWidth),
			rightAlign(formatAmount(e.Balance, currencies, e.CurrencyID), balanceWidth),
		})
	}
	return rows
}

// createPopup renders the "new"/"edit account" form (see startEdit) as a
// bordered, table-shaped pop-up: a header row naming the three fields
// (Parent, Name, Code) above a row of their fixed-width values, with the
// focused column highlighted. While Parent has focus, its value is
// replaced by a dropdown listing "(none)" plus every eligible account
// directly below that row (its own column header stripped, so it sits
// flush under the field row rather than leaving a blank line), with the
// currently chosen one highlighted by the dropdown's own cursor — sized
// (see syncParentPickerHeight) to show them all at once, or as many as
// fit in the window. Once focus moves elsewhere, the dropdown is hidden
// again and Parent's value shows the chosen account as plain text
// instead, so the selection stays visible while editing Name/Code. It's
// meant to be composited over the accounts list via overlayCentered, not
// shown in place of it.
func (m accountsModel) createPopup() string {
	title := "New account"
	if m.editingID != nil {
		title = "Edit account"
	}

	headers := make([]string, 3)
	for i, label := range []string{"Parent", "Name", "Code"} {
		headers[i] = columnHeader(label, createFocus(i) == m.createFocus)
	}

	parentValue := padOrTruncate("", createFieldWidth)
	if m.createFocus != focusParent {
		selected := selectedParentID(m.parentPicker.Cursor(), m.parentOptions)
		parentValue = padOrTruncate(parentSummary(selected, m.parentOptions), createFieldWidth)
	}
	values := []string{
		parentValue,
		m.inputs[fieldAccountName].View(),
		m.inputs[fieldAccountCode].View(),
	}

	content := formLabelStyle.Render(title) + "\n\n" +
		strings.Join(headers, "  ") + "\n" +
		strings.Join(values, "  ")
	if m.createFocus == focusParent {
		content += "\n" + stripPickerHeader(m.parentPicker.View())
	}
	if m.err != "" {
		content += "\n\n" + errorStyle.Render("Error: "+m.err)
	}
	return popupStyle.Render(content)
}

// ledgerEntryPopup renders the ledger's "new entry" form (see
// startLedgerEntry) as a bordered, table-shaped pop-up, the same
// visual style as createPopup: a first row for this account —
// Timestamp, Description, Amount, Currency — above a second for the
// other side of the transaction — Account, Amount, Currency. Both
// Currency fields and Other account are pickers: while one has focus,
// its dropdown is shown below its row (header stripped, so it sits
// flush beneath); otherwise the currently chosen option shows as plain
// text instead, the same focused-vs-not treatment as the account form's
// own Parent field, so the selection stays visible while editing other
// fields. It's meant to be composited over the ledger table via
// overlayCentered, not shown in place of it. Unlike createPopup's single
// Parent dropdown, this form has three, each of which would otherwise
// grow or shrink the pop-up's own size (and so its centered position)
// as focus moves onto or off of it — jarring, since the whole pop-up
// visibly jumps around while filling in the form — so each dropdown's
// row (see pickerSlot) always reserves the same footprint whether or
// not it's currently showing, keeping the pop-up's size constant across
// every field.
func (m accountsModel) ledgerEntryPopup() string {
	row1Labels := []string{"Timestamp", "Description", "Amount", "Currency"}
	row1Focus := []ledgerEntryFocus{focusEntryTimestamp, focusEntryDescription, focusEntryAmount, focusEntryCurrency}
	currencyValue := padOrTruncate("", createFieldWidth)
	if m.ledgerEntryFocus != focusEntryCurrency {
		if c, ok := currencyAt(m.ledgerCurrencyPicker.Cursor(), m.currencyList); ok {
			currencyValue = padOrTruncate(c.Name, createFieldWidth)
		}
	}
	row1Values := []string{
		m.ledgerEntryInputs[fieldEntryTimestamp].View(),
		m.ledgerEntryInputs[fieldEntryDescription].View(),
		m.ledgerEntryInputs[fieldEntryAmount].View(),
		currencyValue,
	}

	row2Labels := []string{"Account", "Amount", "Currency"}
	row2Focus := []ledgerEntryFocus{focusEntryOtherAccount, focusEntryOtherAmount, focusEntryOtherCurrency}
	accountValue := padOrTruncate("", createFieldWidth)
	if m.ledgerEntryFocus != focusEntryOtherAccount {
		if cursor := m.ledgerAccountPicker.Cursor(); cursor >= 0 && cursor < len(m.ledgerOtherAccountOptions) {
			accountValue = padOrTruncate(m.ledgerOtherAccountOptions[cursor].account.Name, createFieldWidth)
		}
	}
	otherCurrencyValue := padOrTruncate("", createFieldWidth)
	if m.ledgerEntryFocus != focusEntryOtherCurrency {
		if c, ok := currencyAt(m.ledgerOtherCurrencyPicker.Cursor(), m.currencyList); ok {
			otherCurrencyValue = padOrTruncate(c.Name, createFieldWidth)
		}
	}
	row2Values := []string{
		accountValue,
		m.ledgerEntryInputs[fieldEntryOtherAmount].View(),
		otherCurrencyValue,
	}

	headers1 := make([]string, len(row1Labels))
	for i, l := range row1Labels {
		headers1[i] = columnHeader(l, row1Focus[i] == m.ledgerEntryFocus)
	}
	headers2 := make([]string, len(row2Labels))
	for i, l := range row2Labels {
		headers2[i] = columnHeader(l, row2Focus[i] == m.ledgerEntryFocus)
	}

	content := formLabelStyle.Render("New entry in "+m.ledgerAccount.Name) + "\n\n" +
		strings.Join(headers1, "  ") + "\n" +
		strings.Join(row1Values, "  ")
	content += "\n" + pickerSlot(m.ledgerEntryFocus == focusEntryCurrency,
		indentLines(stripPickerHeader(m.ledgerCurrencyPicker.View()), fieldPickerOffset(3)),
		ledgerRow1PickerSlotWidth)
	content += "\n\n" + strings.Join(headers2, "  ") + "\n" + strings.Join(row2Values, "  ")
	row2PickerShown := m.ledgerEntryFocus == focusEntryOtherAccount || m.ledgerEntryFocus == focusEntryOtherCurrency
	var row2Picker string
	switch m.ledgerEntryFocus {
	case focusEntryOtherAccount:
		row2Picker = stripPickerHeader(m.ledgerAccountPicker.View())
	case focusEntryOtherCurrency:
		row2Picker = indentLines(stripPickerHeader(m.ledgerOtherCurrencyPicker.View()), fieldPickerOffset(2))
	}
	content += "\n" + pickerSlot(row2PickerShown, row2Picker, ledgerRow2PickerSlotWidth)
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
		b.WriteString(confirmDeletePrompt("account"))
	case accountsModeLedger, accountsModeLedgerCreate:
		// accountsModeLedgerCreate shows its own pop-up (see
		// ledgerEntryPopup), composited over this same ledger view by
		// App.View — so the ledger table is still the right thing to
		// render underneath it here.
		b.WriteString(formLabelStyle.Render(fmt.Sprintf("Transactions for #%d %s", m.ledgerAccount.ID, m.ledgerAccount.Name)))
		b.WriteString("\n\n")
		b.WriteString(m.ledgerTable.View())
	default:
		// accountsModeCreate shows its own pop-up (see createPopup),
		// composited over this same list view by App.View — so the list
		// is still the right thing to render underneath it here.
		b.WriteString(m.table.View())
	}

	// The create/ledger-entry pop-ups carry their own error text, so it
	// isn't repeated here.
	if m.err != "" && m.mode != accountsModeCreate && m.mode != accountsModeLedgerCreate {
		b.WriteString("\n\n")
		b.WriteString(errorStyle.Render("Error: " + m.err))
	} else if m.status != "" && m.mode == accountsModeList {
		b.WriteString("\n\n")
		b.WriteString(statusStyle.Render(m.status))
	}

	return b.String()
}
