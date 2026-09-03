package ui

import (
	"context"
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

// currencyFocus identifies which of the "new currency" form's seven
// fields currently has focus.
type currencyFocus int

const (
	focusCurrencyName currencyFocus = iota
	focusCurrencySymbolBefore
	focusCurrencySymbolSpace
	focusCurrencyDecimalPlaces
	focusCurrencyThousandsSep
	focusCurrencyDecimalSep
	focusCurrencyISIN
	numCurrencyFocusFields
)

const (
	fieldCurrencyName = iota
	fieldCurrencyThousandsSep
	fieldCurrencyDecimalSep
	fieldCurrencyISIN
)

const maxCurrencyDecimalPlaces = 10

type currenciesModel struct {
	client *client.Client

	mode   currenciesMode
	table  table.Model
	rows   []client.Currency
	status string
	err    string

	createFocus   currencyFocus
	symbolBefore  bool
	symbolSpace   bool
	decimalPlaces int
	inputs        []textinput.Model // [name, thousandsSep, decimalSep, isin]
}

func newCurrenciesModel(c *client.Client) currenciesModel {
	columns := []table.Column{
		{Title: "ID", Width: 6},
		{Title: "Name", Width: 20},
		{Title: "Position", Width: 10},
		{Title: "Decimals", Width: 10},
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

	thousandsSep := textinput.New()
	thousandsSep.Placeholder = "optional, e.g. ,"
	thousandsSep.Prompt = ""
	thousandsSep.SetWidth(createFieldWidth)

	decimalSep := textinput.New()
	decimalSep.Placeholder = "e.g. ."
	decimalSep.Prompt = ""
	decimalSep.SetWidth(createFieldWidth)

	isin := textinput.New()
	isin.Placeholder = "optional"
	isin.Prompt = ""
	isin.SetWidth(createFieldWidth)

	return currenciesModel{
		client: c,
		table:  t,
		inputs: []textinput.Model{name, thousandsSep, decimalSep, isin},
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
		m.symbolBefore = true
		m.symbolSpace = false
		m.decimalPlaces = 2
		for i := range m.inputs {
			m.inputs[i].SetValue("")
		}
		m.setCreateFocus(focusCurrencyName)
		m.err = ""
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

// updateCreate handles the "new currency" form: seven fields laid out as
// a two-row grid (see createPopup), navigated the same way as the "new
// account" form — tab/shift+tab or left/right move focus, since arrow
// keys always change focus rather than a text cursor. While a toggle
// field (Position, Space) or the numeric Decimals field has focus,
// up/down change its value instead.
func (m currenciesModel) updateCreate(msg tea.KeyMsg) (currenciesModel, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.mode = currenciesModeList
		m.err = ""
		return m, nil
	case "enter":
		return m.submitCreate()
	case "tab", "right":
		m.setCreateFocus((m.createFocus + 1) % numCurrencyFocusFields)
		return m, nil
	case "shift+tab", "left":
		m.setCreateFocus((m.createFocus + numCurrencyFocusFields - 1) % numCurrencyFocusFields)
		return m, nil
	}

	switch m.createFocus {
	case focusCurrencySymbolBefore:
		if msg.String() == "up" || msg.String() == "down" {
			m.symbolBefore = !m.symbolBefore
		}
		return m, nil
	case focusCurrencySymbolSpace:
		if msg.String() == "up" || msg.String() == "down" {
			m.symbolSpace = !m.symbolSpace
		}
		return m, nil
	case focusCurrencyDecimalPlaces:
		switch msg.String() {
		case "up":
			if m.decimalPlaces < maxCurrencyDecimalPlaces {
				m.decimalPlaces++
			}
		case "down":
			if m.decimalPlaces > 0 {
				m.decimalPlaces--
			}
		}
		return m, nil
	case focusCurrencyName:
		var cmd tea.Cmd
		m.inputs[fieldCurrencyName], cmd = m.inputs[fieldCurrencyName].Update(msg)
		return m, cmd
	case focusCurrencyThousandsSep:
		var cmd tea.Cmd
		m.inputs[fieldCurrencyThousandsSep], cmd = m.inputs[fieldCurrencyThousandsSep].Update(msg)
		return m, cmd
	case focusCurrencyDecimalSep:
		var cmd tea.Cmd
		m.inputs[fieldCurrencyDecimalSep], cmd = m.inputs[fieldCurrencyDecimalSep].Update(msg)
		return m, cmd
	case focusCurrencyISIN:
		var cmd tea.Cmd
		m.inputs[fieldCurrencyISIN], cmd = m.inputs[fieldCurrencyISIN].Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m *currenciesModel) setCreateFocus(f currencyFocus) {
	m.createFocus = f
	for i := range m.inputs {
		m.inputs[i].Blur()
	}
	switch f {
	case focusCurrencyName:
		m.inputs[fieldCurrencyName].Focus()
	case focusCurrencyThousandsSep:
		m.inputs[fieldCurrencyThousandsSep].Focus()
	case focusCurrencyDecimalSep:
		m.inputs[fieldCurrencyDecimalSep].Focus()
	case focusCurrencyISIN:
		m.inputs[fieldCurrencyISIN].Focus()
	}
}

func (m currenciesModel) submitCreate() (currenciesModel, tea.Cmd) {
	name := strings.TrimSpace(m.inputs[fieldCurrencyName].Value())
	if name == "" {
		m.err = "name is required"
		m.setCreateFocus(focusCurrencyName)
		return m, nil
	}
	cur := client.Currency{
		Name:               name,
		SymbolBefore:       m.symbolBefore,
		SymbolSpace:        m.symbolSpace,
		ThousandsSeparator: m.inputs[fieldCurrencyThousandsSep].Value(),
		DecimalSeparator:   m.inputs[fieldCurrencyDecimalSep].Value(),
		DecimalPlaces:      m.decimalPlaces,
	}
	if isin := strings.TrimSpace(m.inputs[fieldCurrencyISIN].Value()); isin != "" {
		cur.ISIN = &isin
	}

	m.err = ""
	c := m.client
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
		rows = append(rows, table.Row{
			strconv.FormatInt(c.ID, 10),
			c.Name,
			symbolPositionText(c.SymbolBefore),
			strconv.Itoa(c.DecimalPlaces),
			isin,
		})
	}
	return rows
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

// formatAmount renders amount in the given currency, or a fallback if
// that currency isn't in the local cache yet (e.g. it was created by
// another client and this tab hasn't refreshed).
func formatAmount(amount int64, currencies currencyByID, currencyID int64) string {
	if c, ok := currencies[currencyID]; ok {
		return c.Format(amount)
	}
	return strconv.FormatInt(amount, 10) + " (?)"
}

func yesNo(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}

func symbolPositionText(before bool) string {
	if before {
		return "before"
	}
	return "after"
}

// createPopup renders the "new currency" form as a bordered, table-shaped
// pop-up: two rows of fixed-width fields (Name/Position/Space/Decimals,
// then Thousands/Decimal separators/ISIN), each with its column header
// above it and the focused one highlighted. It's meant to be composited
// over the currencies list via overlayCentered, not shown in place of it.
func (m currenciesModel) createPopup() string {
	row1Labels := []string{"Name", "Position", "Space", "Decimals"}
	row1Focus := []currencyFocus{focusCurrencyName, focusCurrencySymbolBefore, focusCurrencySymbolSpace, focusCurrencyDecimalPlaces}
	row1Values := []string{
		m.inputs[fieldCurrencyName].View(),
		padOrTruncate(symbolPositionText(m.symbolBefore), createFieldWidth),
		padOrTruncate(yesNo(m.symbolSpace), createFieldWidth),
		padOrTruncate(strconv.Itoa(m.decimalPlaces), createFieldWidth),
	}
	// Position/Space/Decimals are plain text (not a textinput with its
	// own focus rendering), so highlight the focused one explicitly. Name
	// (index 0) needs no such handling: it's a textinput.
	for i := 1; i < len(row1Focus); i++ {
		if row1Focus[i] == m.createFocus {
			row1Values[i] = focusedFieldStyle.Render(row1Values[i])
		}
	}

	row2Labels := []string{"Thousands", "Decimal Sep", "ISIN"}
	row2Focus := []currencyFocus{focusCurrencyThousandsSep, focusCurrencyDecimalSep, focusCurrencyISIN}
	row2Values := []string{
		m.inputs[fieldCurrencyThousandsSep].View(),
		m.inputs[fieldCurrencyDecimalSep].View(),
		m.inputs[fieldCurrencyISIN].View(),
	}

	headers1 := make([]string, len(row1Labels))
	for i, l := range row1Labels {
		headers1[i] = columnHeader(l, row1Focus[i] == m.createFocus)
	}
	headers2 := make([]string, len(row2Labels))
	for i, l := range row2Labels {
		headers2[i] = columnHeader(l, row2Focus[i] == m.createFocus)
	}

	content := formLabelStyle.Render("New currency") + "\n\n" +
		strings.Join(headers1, "  ") + "\n" +
		strings.Join(row1Values, "  ") + "\n\n" +
		strings.Join(headers2, "  ") + "\n" +
		strings.Join(row2Values, "  ")
	if m.err != "" {
		content += "\n\n" + errorStyle.Render("Error: "+m.err)
	}
	return popupStyle.Render(content)
}

func (m currenciesModel) View() string {
	var b strings.Builder

	switch m.mode {
	case currenciesModeConfirmDelete:
		b.WriteString(m.table.View())
		b.WriteString("\n\n")
		b.WriteString(errorStyle.Render("Delete selected currency? (y/n)"))
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
