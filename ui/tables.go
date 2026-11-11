package ui

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/NolanFogarty/courtside/backend"
	"github.com/charmbracelet/x/ansi"
)

// Table columns own both formatting and comparison, so numbers, dates and
// missing values behave consistently in every team and player table.
type dataColumn struct {
	key, label           string
	width, minWidth      int
	numeric, date, right bool
	precision            int
	percent              bool
}

func textColumn(key, label string, width int) dataColumn {
	return dataColumn{key: key, label: label, width: width, minWidth: min(width, 12)}
}
func numberColumn(key, label string, width, precision int) dataColumn {
	return dataColumn{key: key, label: label, width: width, minWidth: width, numeric: true, precision: precision}
}
func percentColumn(key, label string) dataColumn {
	c := numberColumn(key, label, 5, 1)
	c.percent = true
	return c
}
func dateColumn(key, label string, width int) dataColumn {
	c := textColumn(key, label, width)
	c.date = true
	return c
}
func (c dataColumn) value(r backend.Row) string {
	if !r.Has(c.key) {
		return "—"
	}
	if c.key == "NUM" {
		return r.Text(c.key)
	}
	if c.key == "SALARY" {
		if !r.IsNumeric(c.key) {
			return "—"
		}
		n := int64(r.Number(c.key))
		return money(&n)
	}
	if !c.numeric {
		return r.Text(c.key)
	}
	if !r.IsNumeric(c.key) {
		return "—"
	}
	n := r.Number(c.key)
	if c.percent {
		n *= 100
	}
	return fmt.Sprintf("%.*f", c.precision, n)
}

type columnSort struct {
	key    string
	desc   bool
	cursor int
}

func (s columnSort) cursorFor(cols []dataColumn) int {
	for i, c := range cols {
		if c.key == s.key {
			return i
		}
	}
	return 0
}
func (s *columnSort) apply(cols []dataColumn, direction string) {
	if len(cols) == 0 {
		return
	}
	s.cursor = clampInt(s.cursor, 0, len(cols)-1)
	c := cols[s.cursor]
	if direction == "up" {
		s.desc = false
	} else if direction == "down" {
		s.desc = true
	} else if s.key == c.key {
		s.desc = !s.desc
	} else {
		s.desc = c.numeric || c.date
	}
	s.key = c.key
}
func orderedRows(rows []backend.Row, cols []dataColumn, state columnSort) []backend.Row {
	out := append([]backend.Row(nil), rows...)
	var column *dataColumn
	for i := range cols {
		if cols[i].key == state.key {
			column = &cols[i]
			break
		}
	}
	if column == nil {
		return out
	}
	c := *column
	sort.SliceStable(out, func(i, j int) bool {
		a, b := out[i], out[j]
		// Unknown values stay at the end in either direction, never becoming zero.
		if !a.Has(c.key) {
			return false
		}
		if !b.Has(c.key) {
			return true
		}
		if c.numeric && (!a.IsNumeric(c.key) || !b.IsNumeric(c.key)) {
			if !a.IsNumeric(c.key) && !b.IsNumeric(c.key) {
				return false
			}
			if !a.IsNumeric(c.key) {
				return false
			}
			return true
		}
		comparison := 0
		if c.numeric {
			if a.Number(c.key) < b.Number(c.key) {
				comparison = -1
			} else if a.Number(c.key) > b.Number(c.key) {
				comparison = 1
			}
		} else if c.date {
			comparison = strings.Compare(sortDate(a.Text(c.key)), sortDate(b.Text(c.key)))
		} else {
			comparison = strings.Compare(strings.ToLower(a.Text(c.key)), strings.ToLower(b.Text(c.key)))
		}
		if state.desc {
			return comparison > 0
		}
		return comparison < 0
	})
	return out
}
func sortDate(s string) string {
	for _, layout := range []string{time.RFC3339, "2006-01-02", "Jan 02, 2006", "Jan 2, 2006", "01/02/2006"} {
		if d, e := time.Parse(layout, s); e == nil {
			return d.Format("2006-01-02T15:04:05Z")
		}
	}
	return s
}
func dataRow(values map[string]any) backend.Row {
	r := backend.Row{}
	for k, v := range values {
		b, _ := json.Marshal(v)
		r[k] = b
	}
	return r
}

type dataTableOptions struct {
	width, offset, limit, selected int
	columnOffset                   int
	sorting, wrap                  bool
	order                          columnSort
	empty                          string
	separatorRows                  map[int]bool // full-width rules in place of data rows
}

type visibleColumn struct{ index, width int }

func tableColumns(cols []dataColumn, width, offset int) []visibleColumn {
	if len(cols) == 0 {
		return nil
	}
	sizes := make([]int, len(cols))
	total := max(0, len(cols)-1)
	for i, c := range cols {
		sizes[i] = max(c.width, lipgloss.Width(c.label)+2)
		total += sizes[i]
	}
	// Shrink descriptive columns before hiding any data columns.
	for total > width {
		changed := false
		for i, c := range cols {
			if sizes[i] > max(c.minWidth, lipgloss.Width(c.label)+2) {
				sizes[i]--
				total--
				changed = true
				if total <= width {
					break
				}
			}
		}
		if !changed {
			break
		}
	}
	if total <= width {
		out := []visibleColumn{}
		for i, w := range sizes {
			out = append(out, visibleColumn{i, w})
		}
		return stretchColumns(out, width)
	}
	// Pin the identity column while paging horizontally through stats.
	first := min(sizes[0], max(1, width-8))
	out := []visibleColumn{{0, first}}
	used := first
	for i := 1 + clampInt(offset, 0, max(0, len(cols)-2)); i < len(cols); i++ {
		w := min(sizes[i], max(1, width-first-1))
		if used+1+w > width {
			break
		}
		out = append(out, visibleColumn{i, w})
		used += 1 + w
	}
	return stretchColumns(out, width)
}
func stretchColumns(cols []visibleColumn, width int) []visibleColumn {
	if len(cols) == 0 {
		return cols
	}
	used := len(cols) - 1
	for _, c := range cols {
		used += c.width
	}
	extra := max(0, width-used)
	for i := range cols {
		cols[i].width += extra / len(cols)
		if i < extra%len(cols) {
			cols[i].width++
		}
	}
	return cols
}
func revealColumn(cols []dataColumn, width, offset, cursor int) int {
	if cursor == 0 {
		return 0
	}
	offset = clampInt(offset, 0, max(0, len(cols)-2))
	if cursor <= offset {
		offset = cursor - 1
	}
	for off := offset; off < len(cols)-1; off++ {
		for _, v := range tableColumns(cols, width, off) {
			if v.index == cursor {
				return off
			}
		}
	}
	for off := 0; off < offset; off++ {
		for _, v := range tableColumns(cols, width, off) {
			if v.index == cursor {
				return off
			}
		}
	}
	return max(0, cursor-1)
}

func renderDataTable(title string, cols []dataColumn, rows []backend.Row, o dataTableOptions) string {
	width := max(1, o.width-4) // rounded border and one space of padding per side
	gutter := 0
	if o.selected >= 0 {
		gutter = 2
	}
	if o.sorting {
		o.columnOffset = revealColumn(cols, max(1, width-gutter), o.columnOffset, o.order.cursor)
	}
	visible := tableColumns(cols, max(1, width-gutter), o.columnOffset)
	// A full-width panel keeps tables and their borders aligned across sections.
	fit := func(s string) string { return pad(ansi.Truncate(s, width, "…"), width, false) }
	if len(visible) < len(cols) {
		title += "  · [/] columns"
	}
	if o.order.key != "" {
		shown := false
		label := strings.ReplaceAll(o.order.key, "_", " ")
		for _, c := range cols {
			if c.key == o.order.key {
				label = c.label
			}
		}
		for _, v := range visible {
			if cols[v.index].key == o.order.key {
				shown = true
			}
		}
		if !shown {
			arrow := " ↑"
			if o.order.desc {
				arrow = " ↓"
			}
			title += " · sorted by " + label + arrow
		}
	}
	start := clampInt(o.offset, 0, len(rows))
	end := len(rows)
	if o.limit > 0 {
		end = min(end, start+o.limit)
	}
	if start > 0 || end < len(rows) {
		title += fmt.Sprintf("  · %d–%d / %d", min(start+1, end), end, len(rows))
	}
	lines := []string{accentSty.Render(fit(title))}
	headers := strings.Repeat(" ", gutter)
	for i, v := range visible {
		if i > 0 {
			headers += " "
		}
		c := cols[v.index]
		label := c.label
		if c.key == o.order.key {
			if o.order.desc {
				label += " ↓"
			} else {
				label += " ↑"
			}
		}
		cell := pad(ansi.Truncate(label, v.width, ""), v.width, c.numeric || c.right)
		if o.sorting && v.index == o.order.cursor {
			headers += accentSty.Underline(true).Render(cell)
		} else if c.key == o.order.key {
			headers += accentSty.Render(cell)
		} else {
			headers += colHeaderSty.Render(cell)
		}
	}
	lines = append(lines, fit(headers), dimRowSty.Render(strings.Repeat("─", width)))
	for index := start; index < end; index++ {
		if o.separatorRows[index] {
			lines = append(lines, dimRowSty.Render(strings.Repeat("─", width)))
			continue
		}
		cells := [][]string{}
		height := 1
		for _, v := range visible {
			value := cols[v.index].value(rows[index])
			if o.wrap {
				value = ansi.Wrap(value, v.width, "")
			}
			parts := strings.Split(value, "\n")
			height = max(height, len(parts))
			cells = append(cells, parts)
		}
		for line := 0; line < height; line++ {
			row := ""
			if gutter > 0 {
				row = "  "
				if index == o.selected && line == 0 {
					row = "▶ "
				}
			}
			for i, v := range visible {
				if i > 0 {
					row += " "
				}
				value := ""
				if line < len(cells[i]) {
					value = cells[i][line]
				}
				row += pad(ansi.Truncate(value, v.width, "…"), v.width, cols[v.index].numeric || cols[v.index].right)
			}
			row = fit(row)
			if index == o.selected && row == ansi.Strip(row) {
				// Plain selected rows get the selection style. Rows the
				// caller already styled (e.g. schedule winner/loser
				// colors) stand on their own: wrapping them would bury
				// their colors under the outer style's reset.
				row = accentSty.Render(row)
			}
			lines = append(lines, row)
		}
	}
	if len(rows) == 0 {
		note := o.empty
		if note == "" {
			note = "No data available."
		}
		lines = append(lines, mutedSty.Render(fit(note)))
	}
	sty := panelSty
	if o.sorting || o.selected >= 0 {
		sty = focusedPanelSty
	}
	return sty.Render(strings.Join(lines, "\n"))
}
func infoTable(title string, fields [][2]string, width int) []string {
	rows := []backend.Row{}
	for _, f := range fields {
		if f[1] != "" {
			rows = append(rows, dataRow(map[string]any{"FIELD": f[0], "VALUE": f[1]}))
		}
	}
	cols := []dataColumn{textColumn("FIELD", "DETAIL", 22), textColumn("VALUE", "VALUE", max(12, width-27))}
	return strings.Split(renderDataTable(title, cols, rows, dataTableOptions{width: width, selected: -1, wrap: true}), "\n")
}
