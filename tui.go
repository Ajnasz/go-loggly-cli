package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/Ajnasz/go-loggly-cli/search"
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Custom styles for result items
var (
	resultItemStyle = lipgloss.NewStyle().
			PaddingLeft(2).
			PaddingRight(2)

	selectedResultStyle = lipgloss.NewStyle().
				PaddingLeft(2).
				PaddingRight(2).
				Background(lipgloss.Color("62")).
				Foreground(lipgloss.Color("230"))

	detailViewStyle = lipgloss.NewStyle().
			PaddingLeft(2).
			PaddingRight(2).
			BorderStyle(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("170"))
)

// resultItemDelegate is a custom delegate for rendering result items in list view
type resultItemDelegate struct{}

func (d resultItemDelegate) Height() int                               { return 2 }
func (d resultItemDelegate) Spacing() int                              { return 1 }
func (d resultItemDelegate) Update(msg tea.Msg, m *list.Model) tea.Cmd { return nil }
func (d resultItemDelegate) Render(w io.Writer, m list.Model, index int, item list.Item) {
	result, ok := item.(resultItem)
	if !ok {
		return
	}

	// Show a compact preview across 2 lines
	data, _ := json.Marshal(result.data)
	preview := string(data)

	// Calculate available width per line
	maxLen := m.Width() - 4 // Reduce the padding subtraction

	isSelected := index == m.Index()

	// Split into two lines
	line1 := fmt.Sprintf("%d. %s", index+1, preview)
	line2 := ""

	if len(preview) > maxLen {
		line1 = preview[:maxLen]
		if len(preview) > maxLen*2 {
			line2 = preview[maxLen : maxLen*2]
		} else {
			line2 = preview[maxLen:]
		}
	}

	firstLine := line1
	secondLine := fmt.Sprintf("%s", line2)

	output := firstLine
	if line2 != "" {
		output += "\n" + secondLine
	}

	if isSelected {
		fmt.Fprint(w, selectedResultStyle.Render(output))
	} else {
		fmt.Fprint(w, resultItemStyle.Render(output))
	}
}

// resultItemDelegate is a custom delegate for rendering result items in list view

type pane int

const (
	queryPane pane = iota
	fieldsPane
	valuesPane
	resultsPane
	detailPane
)

type fieldItem struct {
	name  string
	count int
}

func (i fieldItem) FilterValue() string { return i.name }
func (i fieldItem) Title() string       { return i.name }
func (i fieldItem) Description() string { return fmt.Sprintf("%d occurrences", i.count) }

type resultItem struct {
	index int
	data  map[string]any
}

func (i resultItem) FilterValue() string {
	data, _ := json.Marshal(i.data)
	return string(data)
}
func (i resultItem) Title() string {
	data, _ := json.Marshal(i.data)
	preview := string(data)
	maxLen := 80
	if len(preview) > maxLen {
		preview = preview[:maxLen] + "..."
	}
	return preview
}
func (i resultItem) Description() string { return "" }

type valueItem struct {
	value string
	count int
}

func (i valueItem) FilterValue() string { return i.value }
func (i valueItem) Title() string       { return i.value }
func (i valueItem) Description() string { return fmt.Sprintf("%d occurrences", i.count) }

type model struct {
	ctx         context.Context
	account     string
	token       string
	from        string
	concurrency int
	to          string
	size        int
	maxPages    int64

	queryInput  textinput.Model
	fieldsList  list.Model
	valuesList  list.Model
	resultsList list.Model
	detailView  viewport.Model
	spinner     spinner.Model
	debugView   string

	selectedField fieldItem

	currentPane pane
	width       int
	height      int

	fieldsWidth  int
	valuesWidth  int
	resultsWidth int
	paneHeight   int

	results       []map[string]any
	fieldPath     []string // Current nested path like ["nested", "field1"]
	allFields     map[string]int
	fieldValues   map[string]map[string]int
	showingDetail bool

	err     error
	loading bool
}

type resultsMsg struct {
	results []map[string]any
	err     error
}

type fieldSelectedMsg struct{}

var (
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("170")).
			MarginBottom(1)

	activeStyle = lipgloss.NewStyle().
			BorderStyle(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("62"))

	inactiveStyle = lipgloss.NewStyle().
			BorderStyle(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("240"))

	helpStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("241"))
)

func initialModel(ctx context.Context, config Config, query string) model {
	ti := textinput.New()
	ti.Placeholder = "Enter your Loggly query..."
	ti.Focus()
	ti.CharLimit = 500
	ti.SetValue(query)

	fieldsList := list.New([]list.Item{}, list.NewDefaultDelegate(), 20, 20)
	fieldsList.Title = "Fields"
	fieldsList.SetShowStatusBar(false)
	fieldsList.SetShowHelp(false)
	fieldsList.SetFilteringEnabled(true)

	valuesList := list.New([]list.Item{}, list.NewDefaultDelegate(), 20, 20)
	valuesList.Title = "Values"
	valuesList.SetShowStatusBar(false)
	valuesList.SetShowHelp(false)
	valuesList.SetFilteringEnabled(true)

	// Results list showing compact previews
	resultsList := list.New([]list.Item{}, resultItemDelegate{}, 80, 20)
	resultsList.Title = "Results"
	resultsList.SetShowStatusBar(false)
	resultsList.SetFilteringEnabled(false)
	resultsList.SetShowHelp(false)
	resultsList.SetShowPagination(true)
	resultsList.SetShowTitle(true)
	resultsList.DisableQuitKeybindings()
	resultsList.SetFilteringEnabled(true)

	// Detail viewport for full JSON view
	detailView := viewport.New(0, 0)

	return model{
		ctx:           ctx,
		account:       config.Account,
		token:         config.Token,
		size:          config.Size,
		maxPages:      config.MaxPages,
		from:          config.From,
		to:            config.To,
		concurrency:   config.Concurrency,
		queryInput:    ti,
		fieldsList:    fieldsList,
		valuesList:    valuesList,
		resultsList:   resultsList,
		detailView:    detailView,
		spinner:       spinner.New(),
		debugView:     "",
		currentPane:   queryPane,
		allFields:     make(map[string]int),
		fieldValues:   make(map[string]map[string]int),
		fieldPath:     []string{},
		showingDetail: false,
	}
}

func (m model) Init() tea.Cmd {
	return tea.Batch(textinput.Blink, m.spinner.Tick)
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.updateSizes()
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			return m, tea.Quit

		case "esc":
			if m.showingDetail {
				m.showingDetail = false
				m.currentPane = resultsPane
				return m, nil
			}

		case "tab":
			if !m.showingDetail {
				m.currentPane = (m.currentPane + 1) % 4
				m.updateFocus()
			}
			return m, nil

		case "shift+tab":
			if !m.showingDetail {
				m.currentPane = (m.currentPane - 1 + 4) % 4
				m.updateFocus()
			}
			return m, nil

		case "enter":
			if m.currentPane == queryPane && !m.loading {
				m.loading = true
				return m, m.executeQuery()
			}

			if m.currentPane == fieldsPane {
				cmd := m.selectField()
				return m, cmd
			}

			if m.currentPane == valuesPane {
				m.addValueToQuery()
				if m.loading {
					return m, nil
				}

				m.loading = true
				return m, m.executeQuery()
			}

			if m.currentPane == resultsPane {
				// Show detail view for selected result
				if item, ok := m.resultsList.SelectedItem().(resultItem); ok {
					m.showDetailView(item)
					m.showingDetail = true
					m.currentPane = detailPane
				}
				return m, nil
			}

		case "backspace":
			if m.currentPane == fieldsPane && len(m.fieldPath) > 0 {
				m.fieldPath = m.fieldPath[:len(m.fieldPath)-1]
				m.updateFieldsList()
				return m, nil
			}
		}

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd

	case resultsMsg:
		m.loading = false
		if msg.err != nil {
			m.err = msg.err
			m.debugView = fmt.Sprintf("Error: %v", msg.err)
			return m, nil
		}
		m.results = msg.results
		m.analyzeResults()
		m.updateFieldsList()
		m.updateResultsView()
		// Ensure sizes are updated after adding items
		if m.width > 0 && m.height > 0 {
			m.updateSizes()
		}
		m.debugView = fmt.Sprintf("Loaded %d results", len(msg.results))
		return m, nil

	case fieldSelectedMsg:
		return m, nil
	}

	// Update active pane
	if m.showingDetail {
		var cmd tea.Cmd
		m.detailView, cmd = m.detailView.Update(msg)
		cmds = append(cmds, cmd)
	} else {
		switch m.currentPane {
		case queryPane:
			var cmd tea.Cmd
			m.queryInput, cmd = m.queryInput.Update(msg)
			cmds = append(cmds, cmd)
		case fieldsPane:
			var cmd tea.Cmd
			m.fieldsList, cmd = m.fieldsList.Update(msg)
			cmds = append(cmds, cmd)
		case valuesPane:
			var cmd tea.Cmd
			m.valuesList, cmd = m.valuesList.Update(msg)
			cmds = append(cmds, cmd)
		case resultsPane:
			var cmd tea.Cmd
			m.resultsList, cmd = m.resultsList.Update(msg)
			cmds = append(cmds, cmd)
		}
	}

	return m, tea.Batch(cmds...)
}

func (m *model) updateSizes() {
	// Each pane with border takes content_width + 2 (for left/right border)
	// We have 3 panes, so 6 chars total for borders
	cellWidth := m.width / 12
	leftPaneWidth := cellWidth * 2 // ~16% for fields
	midPaneWidth := cellWidth * 2  // ~16% for values

	// Calculate results width: total - fields - values - all borders
	borderWidth := 6 // 2 chars per pane * 3 panes
	rightPaneWidth := m.width - leftPaneWidth - midPaneWidth - borderWidth

	paneHeight := m.height - 8

	m.queryInput.Width = m.width - 4

	// Set sizes to content area (borders will be added by lipgloss)
	m.fieldsList.SetSize(leftPaneWidth, paneHeight-2)
	m.valuesList.SetSize(midPaneWidth, paneHeight-2)
	m.resultsList.SetSize(rightPaneWidth, paneHeight-2)

	// Store widths and height for rendering
	m.fieldsWidth = leftPaneWidth
	m.valuesWidth = midPaneWidth
	m.resultsWidth = rightPaneWidth
	m.paneHeight = paneHeight

	// Detail view uses most of the screen
	m.detailView.Width = m.width - 10
	m.detailView.Height = m.height - 6

	m.debugView = fmt.Sprintf("Sizes: total=%d, left=%d, mid=%d, right=%d, hight=%d", m.width, leftPaneWidth, midPaneWidth, rightPaneWidth, paneHeight)
}

func (m *model) updateFocus() {
	m.queryInput.Blur()

	switch m.currentPane {
	case queryPane:
		m.queryInput.Focus()
	}
}

func (m model) View() string {
	if m.width == 0 {
		return m.spinner.View()
	}

	// If showing detail view, render it full screen
	if m.showingDetail {
		helpText := helpStyle.Render("↑/↓: Scroll • Esc: Back to list • q: Quit")
		content := detailViewStyle.Width(m.width - 4).Render(m.detailView.View())
		return lipgloss.JoinVertical(lipgloss.Left,
			titleStyle.Render("Result Detail"),
			content,
			"",
			helpText,
		)
	}

	// Query input at top
	queryStyle := inactiveStyle
	if m.currentPane == queryPane {
		queryStyle = activeStyle
	}
	querySection := queryStyle.Width(m.width - 2).Render(
		lipgloss.JoinVertical(lipgloss.Left,
			titleStyle.Render("Query"),
			m.queryInput.View(),
		),
	)

	// Three panes below
	fieldsStyle := inactiveStyle
	if m.currentPane == fieldsPane {
		fieldsStyle = activeStyle
	}

	valuesStyle := inactiveStyle
	if m.currentPane == valuesPane {
		valuesStyle = activeStyle
	}

	resultsStyle := inactiveStyle
	if m.currentPane == resultsPane {
		resultsStyle = activeStyle
	}

	// Show field path if nested
	fieldTitle := "Fields"
	if len(m.fieldPath) > 0 {
		fieldTitle = strings.Join(m.fieldPath, " > ")
	}

	fieldsSection := fieldsStyle.Width(m.fieldsWidth).MaxHeight(m.paneHeight).Render(m.fieldsList.View())
	valuesSection := valuesStyle.Width(m.valuesWidth).MaxHeight(m.paneHeight).Render(m.valuesList.View())
	resultsSection := resultsStyle.Width(m.resultsWidth).MaxHeight(m.paneHeight).Render(m.resultsList.View())

	panesRow := lipgloss.JoinHorizontal(lipgloss.Top,
		fieldsSection,
		valuesSection,
		resultsSection,
	)

	help := helpStyle.Render("Tab/Shift+Tab: Switch panes • Enter: Execute/Select/View • Backspace: Go up • q: Quit")

	status := ""
	if m.loading {
		status = m.spinner.View() + " Loading..."
	} else if m.err != nil {
		status = fmt.Sprintf("Error: %s", m.err)
	} else if len(m.results) > 0 {
		status = fmt.Sprintf("%d results", len(m.results))
	}

	status = status + "    " + m.debugView

	return lipgloss.JoinVertical(
		lipgloss.Left,
		querySection,
		lipgloss.NewStyle().Foreground(lipgloss.Color("170")).Render(fieldTitle),
		panesRow,
		status,
		// m.debugView,
		help,
	)
}

func (m *model) executeQuery() tea.Cmd {
	return func() tea.Msg {
		query := m.queryInput.Value()
		if query == "" {
			return resultsMsg{results: []map[string]any{}}
		}

		c := search.New(m.account, m.token).SetConcurrency(m.concurrency)
		q := search.NewQuery(query).Size(m.size).From(m.from).To(m.to).MaxPage(m.maxPages)
		resChan, errChan := c.Fetch(m.ctx, *q)

		var results []map[string]any

		for {
			select {
			case <-m.ctx.Done():
				return resultsMsg{err: m.ctx.Err()}
			case res, ok := <-resChan:
				if !ok {
					return resultsMsg{results: results}
				}
				for _, event := range res.Events {
					eventMap := event.(map[string]any)
					if logmsg, ok := eventMap["logmsg"].(string); ok {
						var parsed map[string]any
						if err := json.Unmarshal([]byte(logmsg), &parsed); err == nil {
							results = append(results, parsed)
						}
					}
				}
			case err := <-errChan:
				if err != nil {
					return resultsMsg{err: err}
				}
			}
		}
	}
}

func (m *model) analyzeResults() {
	m.allFields = make(map[string]int)
	m.fieldValues = make(map[string]map[string]int)

	for _, result := range m.results {
		m.analyzeObject(result, []string{})
	}
}

func (m *model) analyzeObject(obj map[string]any, path []string) {
	for key, value := range obj {
		fullPath := append(path, key)
		pathStr := strings.Join(fullPath, ".")
		m.allFields[pathStr]++

		switch v := value.(type) {
		case map[string]any:
			m.analyzeObject(v, fullPath)
		default:
			valueStr := fmt.Sprintf("%v", v)
			if m.fieldValues[pathStr] == nil {
				m.fieldValues[pathStr] = make(map[string]int)
			}
			m.fieldValues[pathStr][valueStr]++
		}
	}
}

func (m *model) updateFieldsList() {
	// Get fields at current path level
	prefix := ""
	if len(m.fieldPath) > 0 {
		prefix = strings.Join(m.fieldPath, ".") + "."
	}

	// Map to track unique values count for each field at current level
	fieldValueCounts := make(map[string]int)

	for fieldPath, values := range m.fieldValues {
		if after, ok := strings.CutPrefix(fieldPath, prefix); ok {
			remainder := after
			parts := strings.SplitN(remainder, ".", 2)
			// Count unique values for this field
			fieldValueCounts[parts[0]] = len(values)
		}
	}

	var fields []fieldItem
	for field, count := range fieldValueCounts {
		fields = append(fields, fieldItem{name: field, count: count})
	}

	sort.Slice(fields, func(i, j int) bool {
		return fields[i].count > fields[j].count
	})

	var items []list.Item

	for _, f := range fields {
		items = append(items, f)
	}

	m.fieldsList.SetItems(items)
}

func (m *model) selectField() tea.Cmd {
	if item, ok := m.fieldsList.SelectedItem().(fieldItem); ok {
		m.selectedField = item
		// Check if this field has nested fields
		testPath := append(m.fieldPath, item.name)
		pathStr := strings.Join(testPath, ".")

		hasNested := false
		for field := range m.allFields {
			if strings.HasPrefix(field, pathStr+".") {
				hasNested = true
				break
			}
		}

		if hasNested {
			m.fieldPath = testPath
			m.updateFieldsList()
			m.debugView = fmt.Sprintf("Selected nested field: %s", pathStr)
		} else {
			m.debugView = fmt.Sprintf("Selected leaf field: %s", pathStr)
		}

		// Update values list for this field
		m.updateValuesList(pathStr)
	}
	return func() tea.Msg { return fieldSelectedMsg{} }
}

func (m *model) updateValuesList(fieldPath string) {
	var items []list.Item

	if values, ok := m.fieldValues[fieldPath]; ok {
		var valueItems []valueItem
		for value, count := range values {
			valueItems = append(valueItems, valueItem{value: value, count: count})
		}

		sort.Slice(valueItems, func(i, j int) bool {
			return valueItems[i].count > valueItems[j].count
		})

		for _, v := range valueItems {
			items = append(items, v)
		}
	}

	m.valuesList.SetItems(items)
}

func (m *model) updateResultsView() {
	var items []list.Item

	for i, result := range m.results {
		m.resultsList.SetItems(items)
		items = append(items, resultItem{
			index: i,
			data:  result,
		})
	}

	m.resultsList.SetItems(items)
}

func replaceExisitingSearch(query, field, value string) string {
	// Simple replacement logic: look for field:value and replace it
	parts := strings.Split(query, " AND ")
	for i, part := range parts {
		if strings.HasPrefix(part, field+":") {
			parts[i] = fmt.Sprintf("%s:%s", field, value)
			return strings.Join(parts, " AND ")
		}
	}
	// If not found, append
	if query != "" {
		return query + " AND " + fmt.Sprintf("%s:%s", field, value)
	}
	return fmt.Sprintf("%s:%s", field, value)
}

func (m *model) addValueToQuery() tea.Cmd {
	if m.selectedField.name == "" {
		return nil
	}

	selectedField := m.selectedField
	if selectedValue, ok := m.valuesList.SelectedItem().(valueItem); ok {
		fieldPath := append(m.fieldPath, selectedField.name)
		fieldStr := "json." + strings.Join(fieldPath, ".")

		current := m.queryInput.Value()
		value := selectedValue.value

		m.queryInput.SetValue(replaceExisitingSearch(current, fieldStr, value))
		m.debugView = fmt.Sprintf("Added to query: %s:%s", fieldStr, value)
		return func() tea.Msg { return fieldSelectedMsg{} }
	}

	return nil
}

func (m *model) showDetailView(item resultItem) {
	data, _ := json.MarshalIndent(item.data, "", "  ")
	m.detailView.SetContent(string(data))
}

func runInteractive(ctx context.Context, config Config, query string) {
	p := tea.NewProgram(
		initialModel(ctx, config, query),
		tea.WithAltScreen(),
	)

	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error running interactive mode: %s\n", err)
		os.Exit(1)
	}
}
