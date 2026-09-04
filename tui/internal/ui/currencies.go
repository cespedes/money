package ui

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"

	"charm.land/bubbles/v2/table"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"

	"money/tui/internal/client"
)

type currenciesMode int

const (
	currenciesModeList currenciesMode = iota
	currenciesModeCreate
	currenciesModeConfirmDelete
)

// currencyFocus identifies which of the "new currency" form's three
// fields currently has focus.
type currencyFocus int

const (
	focusCurrencyName currencyFocus = iota
	focusCurrencyFormat
	focusCurrencyISIN
	numCurrencyFocusFields
)

const (
	fieldCurrencyName = iota
	fieldCurrencyFormat
	fieldCurrencyISIN
)

type currenciesModel struct {
	client *client.Client

	mode   currenciesMode
	table  table.Model
	rows   []client.Currency
	status string
	err    string

	createFocus currencyFocus
	// editingID is the currency being edited's ID, or nil while creating a
	// new one — the same pop-up (see createPopup/startEdit) is reused for
	// both, and submitForm branches on this to POST or PUT.
	editingID *int64
	inputs    []textinput.Model // [name, format, isin]
}

func newCurrenciesModel(c *client.Client) currenciesModel {
	columns := []table.Column{
		{Title: "Name", Width: 24},
		{Title: "Format", Width: 20},
		{Title: "ISIN", Width: 14},
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

	format := textinput.New()
	format.Placeholder = `e.g. "$1,234.00"`
	format.Prompt = ""
	format.SetWidth(createFieldWidth)

	isin := textinput.New()
	isin.Placeholder = "optional"
	isin.Prompt = ""
	isin.SetWidth(createFieldWidth)

	return currenciesModel{
		client: c,
		table:  t,
		inputs: []textinput.Model{name, format, isin},
	}
}

func (m currenciesModel) Init() tea.Cmd {
	return m.loadCurrencies
}

func (m currenciesModel) Editing() bool {
	return m.mode != currenciesModeList
}

func (m *currenciesModel) SetSize(width, height int) {
	m.table.SetWidth(width)
	if height > 5 {
		m.table.SetHeight(height)
	}
}

type currenciesLoadedMsg struct {
	currencies []client.Currency
	err        error
}

type currencyMutatedMsg struct {
	err error
}

func (m currenciesModel) loadCurrencies() tea.Msg {
	currencies, err := m.client.ListCurrencies(context.Background())
	return currenciesLoadedMsg{currencies: currencies, err: err}
}

func (m currenciesModel) Update(msg tea.Msg) (currenciesModel, tea.Cmd) {
	switch msg := msg.(type) {
	case currenciesLoadedMsg:
		if msg.err != nil {
			m.err = msg.err.Error()
			return m, nil
		}
		m.err = ""
		m.rows = msg.currencies
		m.table.SetRows(currenciesToRows(msg.currencies))
		return m, nil

	case currencyMutatedMsg:
		if msg.err != nil {
			m.err = msg.err.Error()
			return m, nil
		}
		m.err = ""
		m.mode = currenciesModeList
		m.editingID = nil
		return m, m.loadCurrencies

	case tea.KeyMsg:
		switch m.mode {
		case currenciesModeList:
			return m.updateList(msg)
		case currenciesModeCreate:
			return m.updateCreate(msg)
		case currenciesModeConfirmDelete:
			return m.updateConfirmDelete(msg)
		}
	}

	if m.mode == currenciesModeList {
		var cmd tea.Cmd
		m.table, cmd = m.table.Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m currenciesModel) updateList(msg tea.KeyMsg) (currenciesModel, tea.Cmd) {
	switch msg.String() {
	case "r":
		m.status = "Refreshing..."
		return m, m.loadCurrencies
	case "n":
		m.mode = currenciesModeCreate
		m.editingID = nil
		for i := range m.inputs {
			m.inputs[i].SetValue("")
		}
		m.setCreateFocus(focusCurrencyName)
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
		m.mode = currenciesModeConfirmDelete
		return m, nil
	}
	var cmd tea.Cmd
	m.table, cmd = m.table.Update(msg)
	return m, cmd
}

// updateCreate handles the "new currency" form: Name, Format, and ISIN,
// navigated the same way as the "new account" form — tab/shift+tab or
// left/right move focus. Format's value isn't validated here as the user
// types; that happens once, on submit (see submitCreate/parseCurrencyFormat).
func (m currenciesModel) updateCreate(msg tea.KeyMsg) (currenciesModel, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.mode = currenciesModeList
		m.editingID = nil
		m.err = ""
		return m, nil
	case "enter":
		return m.submitForm()
	case "tab", "right":
		m.setCreateFocus((m.createFocus + 1) % numCurrencyFocusFields)
		return m, nil
	case "shift+tab", "left":
		m.setCreateFocus((m.createFocus + numCurrencyFocusFields - 1) % numCurrencyFocusFields)
		return m, nil
	}

	i := fieldForFocus(m.createFocus)
	var cmd tea.Cmd
	m.inputs[i], cmd = m.inputs[i].Update(msg)
	return m, cmd
}

func fieldForFocus(f currencyFocus) int {
	switch f {
	case focusCurrencyFormat:
		return fieldCurrencyFormat
	case focusCurrencyISIN:
		return fieldCurrencyISIN
	default:
		return fieldCurrencyName
	}
}

func (m *currenciesModel) setCreateFocus(f currencyFocus) {
	m.createFocus = f
	for i := range m.inputs {
		m.inputs[i].Blur()
	}
	m.inputs[fieldForFocus(f)].Focus()
}

// startEdit opens the same pop-up as "n" (see createPopup), pre-filled
// with cur's current values — Format is reconstructed from its stored
// rules via Currency.Format/exampleAmount, the inverse of what submitForm
// parses back out of it — for submitForm to PUT back on enter instead of
// POSTing a new currency.
func (m *currenciesModel) startEdit(cur client.Currency) {
	m.mode = currenciesModeCreate
	id := cur.ID
	m.editingID = &id
	m.inputs[fieldCurrencyName].SetValue(cur.Name)
	m.inputs[fieldCurrencyFormat].SetValue(cur.Format(exampleAmount(cur.DecimalPlaces)))
	isin := ""
	if cur.ISIN != nil {
		isin = *cur.ISIN
	}
	m.inputs[fieldCurrencyISIN].SetValue(isin)
	m.setCreateFocus(focusCurrencyName)
	m.err = ""
}

func (m currenciesModel) submitForm() (currenciesModel, tea.Cmd) {
	name := strings.TrimSpace(m.inputs[fieldCurrencyName].Value())
	if name == "" {
		m.err = "name is required"
		m.setCreateFocus(focusCurrencyName)
		return m, nil
	}

	format := strings.TrimSpace(m.inputs[fieldCurrencyFormat].Value())
	before, space, thousandsSep, decimalSep, decimalPlaces, ok := parseCurrencyFormat(name, format)
	if !ok {
		m.err = `format must start or end with the name, e.g. "$1,234.00" or "1.234,00 EUR"`
		m.setCreateFocus(focusCurrencyFormat)
		return m, nil
	}

	cur := client.Currency{
		Name:               name,
		SymbolBefore:       before,
		SymbolSpace:        space,
		ThousandsSeparator: thousandsSep,
		DecimalSeparator:   decimalSep,
		DecimalPlaces:      decimalPlaces,
	}
	if isin := strings.TrimSpace(m.inputs[fieldCurrencyISIN].Value()); isin != "" {
		cur.ISIN = &isin
	}

	m.err = ""
	c := m.client
	if m.editingID != nil {
		id := *m.editingID
		return m, func() tea.Msg {
			_, err := c.UpdateCurrency(context.Background(), id, cur)
			return currencyMutatedMsg{err: err}
		}
	}
	return m, func() tea.Msg {
		_, err := c.CreateCurrency(context.Background(), cur)
		return currencyMutatedMsg{err: err}
	}
}

func (m currenciesModel) updateConfirmDelete(msg tea.KeyMsg) (currenciesModel, tea.Cmd) {
	switch msg.String() {
	case "y":
		row := m.table.Cursor()
		if row < 0 || row >= len(m.rows) {
			m.mode = currenciesModeList
			return m, nil
		}
		id := m.rows[row].ID
		m.mode = currenciesModeList
		c := m.client
		return m, func() tea.Msg {
			err := c.DeleteCurrency(context.Background(), id)
			return currencyMutatedMsg{err: err}
		}
	default:
		m.mode = currenciesModeList
		return m, nil
	}
}

func currenciesToRows(currencies []client.Currency) []table.Row {
	rows := make([]table.Row, 0, len(currencies))
	for _, c := range currencies {
		isin := ""
		if c.ISIN != nil {
			isin = *c.ISIN
		}
		rows = append(rows, table.Row{c.Name, c.Format(exampleAmount(c.DecimalPlaces)), isin})
	}
	return rows
}

// exampleAmount is the minor-unit amount representing a fixed test
// quantity of 1234 whole units of a currency with the given number of
// decimal places (e.g. 2 decimal places -> 123400 cents). Used both to
// render a currency's Format example in the currencies list and, in
// reverse, to parse one back out of a "new currency" form's Format field
// (see parseCurrencyFormat).
func exampleAmount(decimalPlaces int) int64 {
	scale := int64(1)
	for range decimalPlaces {
		scale *= 10
	}
	return 1234 * scale
}

// currencyByID indexes a list of currencies for O(1) lookup by ID, used
// by the Accounts and Transactions tabs to render amounts without
// needing to track currencies' own formatting rules themselves.
type currencyByID map[int64]client.Currency

func indexCurrencies(currencies []client.Currency) currencyByID {
	m := make(currencyByID, len(currencies))
	for _, c := range currencies {
		m[c.ID] = c
	}
	return m
}

// formatAmount renders wireAmount — a decimal amount as received from
// the API (see client.CurrencyAmount/Entry/LedgerEntry) — in the given
// currency, or a fallback if that currency isn't in the local cache yet
// (e.g. it was created by another client and this tab hasn't refreshed)
// or wireAmount can't be parsed against it.
func formatAmount(wireAmount json.Number, currencies currencyByID, currencyID int64) string {
	if c, ok := currencies[currencyID]; ok {
		if minor, err := c.ToMinorUnits(wireAmount); err == nil {
			return c.Format(minor)
		}
	}
	return string(wireAmount) + " (?)"
}

// formatMinorAmount is formatAmount's counterpart for an amount that's
// already an exact integer of the currency's minor units — e.g. one
// computed locally (see transactionsModel's pendingEntry), rather than a
// decimal json.Number received from the API.
func formatMinorAmount(amount int64, currencies currencyByID, currencyID int64) string {
	if c, ok := currencies[currencyID]; ok {
		return c.Format(amount)
	}
	return strconv.FormatInt(amount, 10) + " (?)"
}

// createPopup renders the "new"/"edit currency" form (see startEdit) as a
// bordered, table-shaped pop-up: Name, Format, and ISIN as fixed-width
// columns with their names above, the focused one highlighted. It's
// meant to be composited over the currencies list via overlayCentered,
// not shown in place of it.
func (m currenciesModel) createPopup() string {
	title := "New currency"
	if m.editingID != nil {
		title = "Edit currency"
	}

	labels := []string{"Name", "Format", "ISIN"}
	focuses := []currencyFocus{focusCurrencyName, focusCurrencyFormat, focusCurrencyISIN}
	values := []string{
		m.inputs[fieldCurrencyName].View(),
		m.inputs[fieldCurrencyFormat].View(),
		m.inputs[fieldCurrencyISIN].View(),
	}

	headers := make([]string, len(labels))
	for i, l := range labels {
		headers[i] = columnHeader(l, focuses[i] == m.createFocus)
	}

	content := formLabelStyle.Render(title) + "\n\n" +
		strings.Join(headers, "  ") + "\n" +
		strings.Join(values, "  ")
	if m.err != "" {
		content += "\n\n" + errorStyle.Render("Error: "+m.err)
	}
	return popupStyle.Render(content)
}

// parseCurrencyFormat derives a currency's SymbolBefore/SymbolSpace/
// ThousandsSeparator/DecimalSeparator/DecimalPlaces from a single example
// of how it renders a fixed test quantity of 1234 whole units, e.g.
// "$1,234.00" or "1.234,00 EUR" or "1234 PTS". name must appear at
// exactly the start or the end of format (optionally separated from the
// number by a single space); the rest must be exactly "1234", optionally
// split by a thousands separator into "1"+"234" and/or followed by a
// decimal separator and one or more decimal digits.
func parseCurrencyFormat(name, format string) (before, space bool, thousandsSep, decimalSep string, decimalPlaces int, ok bool) {
	if name == "" {
		return false, false, "", "", 0, false
	}
	if strings.HasPrefix(format, name) {
		if sp, num, valid := splitAfterName(format[len(name):]); valid {
			if ts, ds, dp, numOK := parseFormatNumber(num); numOK {
				return true, sp, ts, ds, dp, true
			}
		}
	}
	if strings.HasSuffix(format, name) {
		if sp, num, valid := splitBeforeName(format[:len(format)-len(name)]); valid {
			if ts, ds, dp, numOK := parseFormatNumber(num); numOK {
				return false, sp, ts, ds, dp, true
			}
		}
	}
	return false, false, "", "", 0, false
}

// splitAfterName splits what follows the name in a "before" format (e.g.
// "1,234.00" out of "$1,234.00") into whether a space separates them and
// the remaining number part.
func splitAfterName(rest string) (space bool, numPart string, ok bool) {
	if rest == "" {
		return false, "", false
	}
	if rest[0] == ' ' {
		if len(rest) > 1 && isASCIIDigit(rest[1]) {
			return true, rest[1:], true
		}
		return false, "", false
	}
	if isASCIIDigit(rest[0]) {
		return false, rest, true
	}
	return false, "", false
}

// splitBeforeName is splitAfterName's mirror for an "after" format (e.g.
// "1.234,00" out of "1.234,00 EUR").
func splitBeforeName(rest string) (space bool, numPart string, ok bool) {
	if rest == "" {
		return false, "", false
	}
	last := rest[len(rest)-1]
	if last == ' ' {
		if len(rest) > 1 && isASCIIDigit(rest[len(rest)-2]) {
			return true, rest[:len(rest)-1], true
		}
		return false, "", false
	}
	if isASCIIDigit(last) {
		return false, rest, true
	}
	return false, "", false
}

func isASCIIDigit(b byte) bool {
	return b >= '0' && b <= '9'
}

// parseFormatNumber parses numPart, the number portion of a format
// example, against the fixed test quantity "1234". Since that quantity
// always has exactly four digits, a thousands separator (if any) can only
// split it as "1"+"234", so this fully determines thousandsSep, decimalSep,
// and decimalPlaces without any ambiguity between the two separators.
func parseFormatNumber(numPart string) (thousandsSep, decimalSep string, decimalPlaces int, ok bool) {
	type run struct {
		digits bool
		s      string
	}
	var runs []run
	for i := 0; i < len(numPart); {
		if isASCIIDigit(numPart[i]) {
			j := i
			for j < len(numPart) && isASCIIDigit(numPart[j]) {
				j++
			}
			runs = append(runs, run{digits: true, s: numPart[i:j]})
			i = j
			continue
		}
		runs = append(runs, run{digits: false, s: numPart[i : i+1]})
		i++
	}

	switch len(runs) {
	case 1:
		if runs[0].digits && runs[0].s == "1234" {
			return "", "", 0, true
		}
	case 3:
		if runs[0].digits && !runs[1].digits && runs[2].digits {
			if runs[0].s == "1" && runs[2].s == "234" {
				return runs[1].s, "", 0, true
			}
			if runs[0].s == "1234" {
				return "", runs[1].s, len(runs[2].s), true
			}
		}
	case 5:
		if runs[0].digits && !runs[1].digits && runs[2].digits && !runs[3].digits && runs[4].digits {
			if runs[0].s == "1" && runs[2].s == "234" {
				return runs[1].s, runs[3].s, len(runs[4].s), true
			}
		}
	}
	return "", "", 0, false
}

func (m currenciesModel) View() string {
	var b strings.Builder

	switch m.mode {
	case currenciesModeConfirmDelete:
		b.WriteString(m.table.View())
		b.WriteString("\n\n")
		b.WriteString(confirmDeletePrompt("currency"))
	default:
		// currenciesModeCreate shows its own pop-up (see createPopup),
		// composited over this same list view by App.View.
		b.WriteString(m.table.View())
	}

	if m.err != "" && m.mode != currenciesModeCreate {
		b.WriteString("\n\n")
		b.WriteString(errorStyle.Render("Error: " + m.err))
	} else if m.status != "" && m.mode == currenciesModeList {
		b.WriteString("\n\n")
		b.WriteString(statusStyle.Render(m.status))
	}

	return b.String()
}
