package screen

import "slices"

const (
	openLevels = 3
	openAll    = "all"
)

type foldedSpan struct {
	spanRow
	kids   int
	below  int
	open   bool
	toggle string
}

type spanFold struct {
	rows  []foldedSpan
	total int
	all   bool
}

func parentsOf(rows []spanRow) []int {
	parents := make([]int, len(rows))
	var stack []int
	for i, r := range rows {
		for len(stack) > r.depth {
			stack = stack[:len(stack)-1]
		}
		parents[i] = -1
		if len(stack) > 0 {
			parents[i] = stack[len(stack)-1]
		}
		stack = append(stack, i)
	}
	return parents
}

func (st *state) spanToggle(id string, open, forced bool) string {
	cur := slices.DeleteFunc(slices.Clone(st.query["open"]), func(v string) bool { return v == id })
	set := map[string][]string{"open": cur}
	switch {
	case !open:
		set["open"] = append(cur, id)
	case forced:
		set["span"] = nil
	}
	return st.href(set)
}

func (st *state) foldSpans(rows []spanRow) spanFold {
	asked := st.query["open"]
	fold := spanFold{total: len(rows), all: slices.Contains(asked, openAll)}
	parents := parentsOf(rows)
	kids := make([]int, len(rows))
	below := make([]int, len(rows))
	for i := len(rows) - 1; i >= 0; i-- {
		if p := parents[i]; p >= 0 {
			kids[p]++
			below[p] += below[i] + 1
		}
	}
	forced := map[int]bool{}
	for i, r := range rows {
		if r.id != st.query.Get("span") || r.id == "" {
			continue
		}
		for p := parents[i]; p >= 0; p = parents[p] {
			forced[p] = true
		}
	}
	open := make([]bool, len(rows))
	shown := make([]bool, len(rows))
	for i, r := range rows {
		open[i] = fold.all || r.depth < openLevels || forced[i] || slices.Contains(asked, r.id)
		shown[i] = parents[i] < 0 || shown[parents[i]] && open[parents[i]]
		if !shown[i] {
			continue
		}
		row := foldedSpan{spanRow: r, kids: kids[i], below: below[i], open: open[i]}
		if kids[i] > 0 && r.depth >= openLevels && !fold.all {
			row.toggle = st.spanToggle(r.id, open[i], forced[i])
		}
		fold.rows = append(fold.rows, row)
	}
	return fold
}

func (st *state) spanAllHref(all bool) string {
	if all {
		return st.href(map[string][]string{"open": {openAll}})
	}
	return st.href(map[string][]string{"open": nil})
}
