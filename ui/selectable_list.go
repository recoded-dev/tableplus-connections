package ui

import (
    "fmt"
    "io"
    "os"
    "reflect"

    tea "github.com/charmbracelet/bubbletea"
    "github.com/charmbracelet/bubbles/list"
)

var ErrAborted = fmt.Errorf("Selection aborted by user")

// Group represents metadata for a group of items.
type Group struct {
    ID          string
    Name        string
    Description string
}

// Public item type your Cobra command can use.
type Item struct {
    ID              string
    TitleText       string
    DescriptionText string
    GroupID         string // empty => orphan (no group)
    Selected        bool
}

// Implement list.Item interface
func (i Item) Title() string       { return i.TitleText }
func (i Item) Description() string { return i.DescriptionText }
func (i Item) FilterValue() string { return i.TitleText }

// groupRow is a synthetic row representing a group header.
type groupRow struct {
    GroupID string
    Name    string
}

// list.Item interface for group rows.
func (g groupRow) Title() string       { return g.Name }
func (g groupRow) Description() string { return "" }
func (g groupRow) FilterValue() string { return g.Name }

// ---- list delegate (checkbox rendering, with optional grouping) ----

type itemDelegate struct{
    grouped bool
}

func (d itemDelegate) Height() int                               { return 1 }
func (d itemDelegate) Spacing() int                              { return 0 }
func (d itemDelegate) Update(msg tea.Msg, m *list.Model) tea.Cmd { return nil }

func (d itemDelegate) Render(w io.Writer, m list.Model, index int, listItem list.Item) {
    cursor := " "
    if index == m.Index() {
        cursor = ">"
    }

    switch v := listItem.(type) {
    case Item:
        // Normal item row
        check := "[ ]"
        if v.Selected {
            check = "[x]"
        }

        indent := ""

        if d.grouped && v.GroupID != "" {
            indent = " |"
        }

        fmt.Fprintf(w, "%s%s %s %s", cursor, indent, check, v.Title())

    case groupRow:
        // Group row: compute aggregate selection state
        groupID := v.GroupID
        any := false
        all := true

        for _, li := range m.Items() {
            it, ok := li.(Item)
            if !ok || it.GroupID != groupID {
                continue
            }
            if it.Selected {
                any = true
            } else {
                all = false
            }
        }

        check := "[ ]"
        if all && any {
            check = "[x]"
        } else if any {
            check = "[-]"
        }

        fmt.Fprintf(w, "%s %s %s:", cursor, check, v.Name)

    default:
        panic(fmt.Errorf("Unknown type: %s", reflect.TypeOf(listItem)));
    }
}

// ---- Bubble Tea model ----

type model struct {
    list     list.Model
    delegate itemDelegate
    ready    bool
    grouped  bool
    groups   map[string]Group
    items    []Item
    done     bool
    aborted  bool
}

func newModel(items []Item, groups []*Group) model {
    groupMap := make(map[string]Group, len(groups))
    for _, g := range groups {
        if g == nil {
            continue
        }
        groupMap[g.ID] = *g
    }

    delegate := itemDelegate{
        grouped: true,
    }
    l := list.New(nil, delegate, 0, 0)
    l.Title = "Which connections would you like to export?"
    l.DisableQuitKeybindings() // we handle quitting ourselves
    l.SetShowStatusBar(false)

    m := model{
        list:     l,
        delegate: delegate,
        grouped:  true, // default: grouped view
        groups:   groupMap,
        items:    append([]Item(nil), items...), // copy
    }

    m.rebuildListItems()
    return m
}

// rebuildListItems rebuilds the list.Model items from the canonical m.items.
func (m *model) rebuildListItems() {
    m.list.SetItems(buildListItems(m.items, m.groups, m.grouped))
}

// buildListItems constructs list items, optionally inserting group rows.
func buildListItems(items []Item, groups map[string]Group, grouped bool) []list.Item {
    if !grouped {
        out := make([]list.Item, 0, len(items))
        for _, it := range items {
            out = append(out, it)
        }
        return out
    }

    out := make([]list.Item, 0, len(items)*2)
    for i, it := range items {
        // Insert group header before first item of a group block
        if it.GroupID != "" {
            if i == 0 || items[i-1].GroupID != it.GroupID {
                name := it.GroupID
                if g, ok := groups[it.GroupID]; ok && g.Name != "" {
                    name = g.Name
                }
                out = append(out, groupRow{
                    GroupID: it.GroupID,
                    Name:    name,
                })
            }
        }
        out = append(out, it)
    }
    return out
}

func (m model) Init() tea.Cmd {
    return nil
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
    switch msg := msg.(type) {

    case tea.WindowSizeMsg:
        // Make the list fill (most of) the terminal.
        // Leave a couple of rows for the help text at the bottom.
        m.ready = true
        m.list.SetSize(msg.Width, msg.Height-3)
        return m, nil

    case tea.KeyMsg:
        switch msg.String() {
        case "ctrl+c", "q":
            m.done = true
            m.aborted = true
            return m, tea.Quit

        case "enter":
            m.done = true
            return m, tea.Quit

        case " ":
            // Toggle selection on the current row:
            // - Item: toggle just that item.
            // - Group row: toggle entire group.
            idx := m.list.Index()
            if idx < 0 || idx >= len(m.list.Items()) {
                return m, nil
            }

            switch v := m.list.Items()[idx].(type) {
            case Item:
                // Toggle single item by ID in canonical slice
                for i := range m.items {
                    if m.items[i].ID == v.ID {
                        m.items[i].Selected = !m.items[i].Selected
                        break
                    }
                }
                // Keep cursor on same row after rebuild
                m.rebuildListItems()
                if idx < len(m.list.Items()) {
                    m.list.Select(idx)
                }
                return m, nil

            case groupRow:
                groupID := v.GroupID
                if groupID == "" {
                    return m, nil
                }

                // Decide whether to select-all or deselect-all
                anyUnselected := false
                for _, it := range m.items {
                    if it.GroupID == groupID && !it.Selected {
                        anyUnselected = true
                        break
                    }
                }

                for i := range m.items {
                    if m.items[i].GroupID == groupID {
                        m.items[i].Selected = anyUnselected
                    }
                }

                // Rebuild & reselect the group row
                m.rebuildListItems()
                for i, li := range m.list.Items() {
                    if gr, ok := li.(groupRow); ok && gr.GroupID == groupID {
                        m.list.Select(i)
                        break
                    }
                }
                return m, nil
            }

        case "g":
            // Toggle grouping on/off (visual only)
            m.grouped = !m.grouped

            m.delegate.grouped = m.grouped
            m.list.SetDelegate(m.delegate)

            curID := ""
            // Try to remember the current item/group to keep focus reasonable
            if idx := m.list.Index(); idx >= 0 && idx < len(m.list.Items()) {
                switch v := m.list.Items()[idx].(type) {
                case Item:
                    curID = v.ID
                case groupRow:
                    curID = v.GroupID
                }
            }

            m.rebuildListItems()

            // Try to restore cursor to something related
            if curID != "" {
                for i, li := range m.list.Items() {
                    switch v := li.(type) {
                    case Item:
                        if v.ID == curID {
                            m.list.Select(i)
                            goto doneSelect
                        }
                    case groupRow:
                        if v.GroupID == curID {
                            m.list.Select(i)
                            goto doneSelect
                        }
                    }
                }
            }
        doneSelect:
            return m, nil
        }
    }

    var cmd tea.Cmd
    m.list, cmd = m.list.Update(msg)
    return m, cmd
}

func (m model) View() string {
    if m.done {
        return ""
    }
    view := m.list.View()
    help := "\n[↑/↓] move  [space] select item/group  [g] toggle groups  [enter] confirm  [q] quit"
    return view + help
}

// Run starts the TUI and returns the selected items.
func Run(items []Item, groups []*Group) ([]Item, error) {
    p := tea.NewProgram(
        newModel(items, groups),
        tea.WithOutput(os.Stdout),
        tea.WithAltScreen(), // optional, but nicer full-screen UI
    )

    m, err := p.Run()
    if err != nil {
        return nil, err
    }

    finalModel := m.(model)

    if finalModel.aborted {
        return nil, ErrAborted
    }

    var selected []Item
    for _, it := range finalModel.items {
        if it.Selected {
            selected = append(selected, it)
        }
    }

    return selected, nil
}
