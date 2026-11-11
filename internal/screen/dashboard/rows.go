package dashboard

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"docker-tool/internal/models"
	"docker-tool/internal/screen/theme"
)

// row is one line of the list, already formatted.
type row struct {
	id    string
	look  theme.Look // zero Glyph when the row has no state
	cells []string   // one per column of its section
	dim   bool       // dangling image and the like
	c     *models.DockerModel
	img   *models.ImageModel
}

// column describes one column of a section's list.
type column struct {
	title string
	min   int  // width it never shrinks below
	max   int  // width it never grows past (0: no cap)
	flex  int  // share of the spare width
	right bool // right-aligned (sizes)
	from  int  // only shown when the list is at least this wide
}

var sectionColumns = [numTabs][]column{
	containersTab: {
		{title: "NAME", min: 12, max: 32, flex: 3},
		{title: "IMAGE", min: 14, max: 40, flex: 3, from: 88},
		{title: "STATE", min: 10},
		{title: "STATUS", min: 14, max: 26, flex: 2},
		{title: "PORTS", min: 10, max: 60, flex: 3},
	},
	imagesTab: {
		{title: "REPOSITORY", min: 16, max: 48, flex: 4},
		{title: "TAG", min: 8, max: 24, flex: 1},
		{title: "IMAGE ID", min: 12},
		{title: "SIZE", min: 8, right: true},
		{title: "CREATED", min: 13, max: 16, flex: 1, from: 70},
	},
	podsTab: {
		{title: "NAME", min: 16, max: 48, flex: 3},
		{title: "NAMESPACE", min: 12, max: 24, flex: 1},
		{title: "STATE", min: 10},
		{title: "RESTARTS", min: 8, right: true},
		{title: "AGE", min: 8},
	},
}

// placed is a column given its position on screen.
type placed struct {
	column
	idx  int // index into row.cells
	x, w int
}

const colGap = 2

// place fits a section's columns into width cells starting at x0: columns
// too wide for the terminal drop out, the spare width goes to flex columns.
func place(cols []column, x0, width int) []placed {
	var out []placed
	need, flex := 0, 0
	for i, c := range cols {
		if width < c.from {
			continue
		}
		if len(out) > 0 {
			need += colGap
		}
		need += c.min
		flex += c.flex
		out = append(out, placed{column: c, idx: i, w: c.min})
	}
	extra := max(0, width-need)
	given, lastFlex := 0, -1
	for i := range out {
		if out[i].flex > 0 && flex > 0 {
			add := extra * out[i].flex / flex
			out[i].w += add
			given += add
			lastFlex = i
		}
	}
	if lastFlex >= 0 {
		out[lastFlex].w += extra - given
	}
	for i := range out {
		if out[i].max > 0 {
			out[i].w = min(out[i].w, out[i].max)
		}
	}
	x := x0
	for i := range out {
		out[i].x = x
		x += out[i].w + colGap
	}
	return out
}

// rows returns a section's rows, filtered.
func (m *Model) rows(s section) []row {
	var out []row
	f := strings.ToLower(m.filter)
	keep := func(parts ...string) bool {
		if f == "" {
			return true
		}
		return strings.Contains(strings.ToLower(strings.Join(parts, " ")), f)
	}
	switch s {
	case containersTab:
		for i := range m.containers {
			c := &m.containers[i]
			look := theme.ContainerLook(c.State, c.Health)
			if !keep(c.Name, c.Image, look.Word, c.Ports) {
				continue
			}
			out = append(out, row{id: c.ID, look: look, c: c,
				cells: []string{c.Name, c.Image, look.Word, shortStatus(c.Status), shortPorts(c.Ports)}})
		}
	case imagesTab:
		for i := range m.images {
			im := &m.images[i]
			repo, tag := "<none>", "<none>"
			if len(im.Tags) > 0 {
				repo, tag = splitTag(im.Tags[0])
			}
			if !keep(strings.Join(im.Tags, " "), im.ShortID) {
				continue
			}
			out = append(out, row{id: im.ID, img: im, dim: len(im.Tags) == 0,
				cells: []string{repo, tag, im.ShortID, humanSize(im.Size), ago(im.CreatedAt, m.wall)}})
		}
	}
	return out
}

// shortStatus drops the "(healthy)" or "(Paused)" tail: the STATE column
// already says it.
func shortStatus(s string) string {
	if i := strings.LastIndex(s, " ("); i > 0 && strings.HasSuffix(s, ")") && !strings.HasPrefix(s, "Exited") && !strings.HasPrefix(s, "Restarting") {
		return s[:i]
	}
	return s
}

// shortPorts drops the any-address prefix: "0.0.0.0:8080->80/tcp" reads as
// "8080->80/tcp".
func shortPorts(p string) string {
	if p == "" {
		return "—"
	}
	p = strings.ReplaceAll(p, "0.0.0.0:", "")
	return strings.ReplaceAll(p, ":::", "")
}

// selected returns the row under the selection.
func (m *Model) selected() (row, bool) {
	rows := m.rows(m.sec)
	if len(rows) == 0 {
		return row{}, false
	}
	return rows[min(m.sel[m.sec], len(rows)-1)], true
}

// sortContainers puts the ones that are up first, then by name.
func sortContainers(cs []models.DockerModel) {
	rank := func(state string) int {
		switch state {
		case "running", "restarting":
			return 0
		case "paused":
			return 1
		}
		return 2
	}
	sort.SliceStable(cs, func(i, j int) bool {
		if a, b := rank(cs[i].State), rank(cs[j].State); a != b {
			return a < b
		}
		return cs[i].Name < cs[j].Name
	})
}

// sortImages puts the newest first.
func sortImages(is []models.ImageModel) {
	sort.SliceStable(is, func(i, j int) bool { return is[i].CreatedAt.After(is[j].CreatedAt) })
}

// splitTag splits "registry:5000/app:v1" into "registry:5000/app" and "v1".
func splitTag(ref string) (repo, tag string) {
	i := strings.LastIndex(ref, ":")
	if i < 0 || strings.Contains(ref[i:], "/") {
		return ref, "latest"
	}
	return ref[:i], ref[i+1:]
}

// humanSize formats bytes the way docker does (SI units).
func humanSize(b int64) string {
	if b < 1000 {
		return fmt.Sprintf("%d B", b)
	}
	f, unit := float64(b), ""
	for _, u := range []string{"kB", "MB", "GB", "TB"} {
		f, unit = f/1000, u
		if f < 1000 {
			break
		}
	}
	if f < 10 {
		return fmt.Sprintf("%.1f %s", f, unit)
	}
	return fmt.Sprintf("%.0f %s", f, unit)
}

// ago says how long before now t was, in the largest unit that fits.
func ago(t, now time.Time) string {
	if t.IsZero() || now.IsZero() {
		return "—"
	}
	d := now.Sub(t)
	n := func(v float64, unit string) string {
		k := int(v)
		if k == 1 {
			return "1 " + unit + " ago"
		}
		return fmt.Sprintf("%d %ss ago", k, unit)
	}
	day := 24 * time.Hour
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return n(d.Minutes(), "minute")
	case d < 2*day:
		return n(d.Hours(), "hour")
	case d < 14*day:
		return n(float64(d/day), "day")
	case d < 60*day:
		return n(float64(d/(7*day)), "week")
	case d < 730*day:
		return n(float64(d/(30*day)), "month")
	}
	return n(float64(d/(365*day)), "year")
}

// clip shortens s to w cells, ending in an ellipsis when cut.
func clip(s string, w int) string {
	r := []rune(s)
	if len(r) <= w {
		return s
	}
	if w <= 0 {
		return ""
	}
	return string(r[:w-1]) + "…"
}
