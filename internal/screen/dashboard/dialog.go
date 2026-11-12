package dashboard

// The dialog opens from a row (click it, or enter).
//
// For a container it offers what fits its state: a stopped one can be
// started or removed; a running one can be stopped or restarted, and a small
// terminal under the header follows what it prints.
//
// For an image it offers to remove it (with all its tags) or, when it has
// several, to drop only the one in the row. When a container uses the image,
// Docker won't remove it: the dialog says which container, and takes you
// there.
//
// Removing asks for a second press first.

import (
	"regexp"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"

	"docker-tool/internal/bridge"
	"docker-tool/internal/models"
)

type dlgKind int

const (
	containerDlg dlgKind = iota
	imageDlg
)

type phase int

const (
	phaseReady      phase = iota // container not running (or an image): its actions are offered
	phaseStarting                // start sent, Docker hasn't answered yet
	phaseRunning                 // running: its output is followed
	phaseFailed                  // Docker refused the start; the terminal says why
	phaseStopping                // stop sent; Docker answers once the container is down
	phaseRestarting              // restart sent; same wait as a stop, then a start
	phaseRemoving                // remove sent
)

const (
	dialogOpen  = 0.22 // seconds for the dialog to unfold
	lineIn      = 0.25 // a new log line printing in
	maxLogLines = 2000 // older lines are dropped
	maxLogRunes = 2000 // a longer line is cut
	toastFor    = 3.0  // seconds a "removed web" note stays under the box
)

type lineKind int

const (
	stdoutLine lineKind = iota
	stderrLine
	noteLine // something went wrong (orange)
	infoLine // anchors talking, not the container ("output ended")
)

type logLine struct {
	text string
	kind lineKind
	at   float64 // when it arrived, for printing in
}

// action is something a dialog button does.
type action int

const (
	actNone action = iota
	actStart
	actStop
	actRestart
	actRemove
	actUntag
	actGoTo // to the container that uses the image
)

// button is one of the dialog's actions. loud is the one to do next
// (orange); the others are quiet.
type button struct {
	act   action
	label string
	icon  rune // drawn in colour over the label's first glyph
	loud  bool
}

// dialog is the open dialog, for a container or an image.
type dialog struct {
	kind    dlgKind
	c       models.DockerModel // refreshed with the list
	img     models.ImageModel  // refreshed with the list
	phase   phase
	born    float64
	phaseAt float64
	stream  *bridge.LogStream
	opening bool // OpenLogs sent, no answer yet
	ended   bool // the output stream ended
	logs    []logLine
	back    int    // rows scrolled up from the newest; 0 follows the output
	hover   action // button under the mouse
	onClose bool   // the mouse is on esc ✕
	armed   action // a remove waiting for its second press
	failMsg string // "Docker didn't stop it." after a refused action
	blocker string // ID of a container that uses the image
}

// busy is true while Docker is working on something the dialog asked.
func (d *dialog) busy() bool {
	switch d.phase {
	case phaseStarting, phaseStopping, phaseRestarting, phaseRemoving:
		return true
	}
	return false
}

func isUp(c models.DockerModel) bool { return c.State == "running" || c.State == "restarting" }

// imageName is how an image is called on screen: its first tag.
func imageName(im models.ImageModel) string {
	if len(im.Tags) > 0 {
		return im.Tags[0]
	}
	return "<none>"
}

// buttons are the actions the dialog offers right now, left to right.
func (m *Model) buttons() []button {
	d := m.dlg
	if d.busy() {
		return nil
	}
	rm := button{act: actRemove, label: " ✕  REMOVE ", icon: '✕'}
	if d.armed == actRemove {
		rm = button{act: actRemove, label: " ✕  CONFIRM REMOVE ", loud: true}
	}
	if d.kind == imageDlg {
		var bs []button
		if c, ok := m.containerByID(d.blocker); ok {
			bs = append(bs, button{act: actGoTo, label: " →  GO TO " + c.Name + " ", loud: d.armed == actNone})
		}
		bs = append(bs, rm)
		if len(d.img.Tags) > 1 {
			bs = append(bs, button{act: actUntag, label: " −  UNTAG ", icon: '−'})
		}
		return bs
	}
	switch d.phase {
	case phaseRunning:
		return []button{{act: actStop, label: " ■  STOP ", icon: '■'}, {act: actRestart, label: " ↻  RESTART ", icon: '↻'}}
	case phaseFailed:
		return []button{{act: actStart, label: " ↻  RETRY ", loud: d.armed == actNone}, rm}
	}
	return []button{{act: actStart, label: " ▶  START ", loud: d.armed == actNone}, rm}
}

func (m *Model) offers(a action) bool {
	if m.dlg == nil {
		return false
	}
	for _, b := range m.buttons() {
		if b.act == a {
			return true
		}
	}
	return false
}

// dlgLayout is where the dialog and its parts sit. It is centred on the
// logbook, so the lighthouse beam still finds the selected row's edge.
type dlgLayout struct {
	x0, y0, x1, y1    int
	inL, inR          int // text runs from inL to before inR
	headRow, stateRow int
	actRow, ruleRow   int // actRow: the buttons and what is happening
	termTop, termRows int
	termL, termR      int // log text runs from termL to before termR
	btns              []placedButton
	sayX              int    // where the message after the buttons starts
	close             [3]int // esc ✕: from, to (exclusive), row
}

type placedButton struct {
	button
	x0, x1 int
}

const (
	closeLabel = " esc ✕ "
	busyWidth  = 10 // the halftone progress that stands in for the buttons
)

func (m *Model) dialogLayout() dlgLayout {
	l := m.lay
	var w dlgLayout
	bw, bh := l.bx1-l.bx0+1, l.by1-l.by0+1
	dw, dh := min(bw-4, 104), min(bh-2, 32)
	w.x0, w.y0 = l.bx0+(bw-dw)/2, l.by0+(bh-dh)/2
	w.x1, w.y1 = w.x0+dw-1, w.y0+dh-1
	w.inL, w.inR = w.x0+3, w.x1-2
	gap := 1
	if dh < 18 {
		gap = 0
	}
	w.headRow = w.y0 + 1 + gap
	w.stateRow = w.headRow + 1
	w.actRow = w.stateRow + 1 + gap
	w.ruleRow = w.actRow + 1 + gap
	w.termTop = w.ruleRow + 1
	w.termRows = max(1, w.y1-gap-w.termTop)
	w.termL, w.termR = w.x0+5, w.x1-3

	x := w.inL
	if m.dlg != nil {
		for _, b := range m.buttons() {
			n := len([]rune(b.label))
			w.btns = append(w.btns, placedButton{b, x, x + n})
			x += n + 2
		}
		if m.dlg.busy() {
			x += busyWidth + 2
		}
	}
	w.sayX = x + 1
	n := len([]rune(closeLabel))
	w.close = [3]int{w.x1 - 2 - n, w.x1 - 2, w.y0}
	return w
}

// openDialog opens the dialog for the selected row.
func (m *Model) openDialog() tea.Cmd {
	r, ok := m.selected()
	if !ok || (r.c == nil && r.img == nil) {
		return nil
	}
	m.closeDialog()
	d := &dialog{born: m.now, phaseAt: m.now}
	m.dlg, m.hover = d, -1
	m.busy(dialogOpen + 0.4)
	if r.img != nil {
		d.kind, d.img = imageDlg, *r.img
		m.describeImage()
		return nil
	}
	d.c = *r.c
	if isUp(d.c) {
		d.phase = phaseRunning
		return m.openLogs()
	}
	return nil
}

// describeImage fills the image dialog's panel: its names, digests and who
// uses it.
func (m *Model) describeImage() {
	im := m.dlg.img
	for _, t := range im.Tags {
		m.addLine(stdoutLine, t)
	}
	if len(im.Tags) == 0 {
		m.addLine(infoLine, "no tags: a dangling image, removed by its ID")
	}
	for _, dg := range im.Digests {
		m.addLine(infoLine, dg)
	}
	if users := m.imageUsers(im); len(users) > 0 {
		names := make([]string, len(users))
		for i, c := range users {
			names[i] = c.Name
		}
		m.addLine(infoLine, "used by "+strings.Join(names, ", "))
	}
}

func (m *Model) closeDialog() {
	if m.dlg != nil && m.dlg.stream != nil {
		m.dlg.stream.Close()
	}
	m.dlg = nil
}

func (m *Model) openLogs() tea.Cmd {
	d := m.dlg
	d.opening = true
	if m.docker == nil {
		return nil
	}
	return bridge.OpenLogs(m.docker, d.c.ID)
}

// fresh resets a container dialog for a new run of the container: no output,
// no errors. The stream that opens next brings the recent output.
func (m *Model) fresh(p phase) {
	d := m.dlg
	if d.stream != nil {
		d.stream.Close()
	}
	*d = dialog{kind: d.kind, c: d.c, img: d.img, born: d.born, phase: p, phaseAt: m.now}
}

// do runs a button's action, if the dialog offers it now.
func (m *Model) do(a action) tea.Cmd {
	d := m.dlg
	if d == nil || !m.offers(a) {
		return nil
	}
	if a != actRemove {
		d.armed = actNone
	}
	switch a {
	case actStart:
		m.fresh(phaseStarting)
		return m.call(func() tea.Cmd { return bridge.StartContainer(m.docker, d.c.ID) })
	case actStop:
		d.phase, d.phaseAt, d.failMsg = phaseStopping, m.now, ""
		return m.call(func() tea.Cmd { return bridge.StopContainer(m.docker, d.c.ID) })
	case actRestart:
		d.phase, d.phaseAt, d.failMsg = phaseRestarting, m.now, ""
		return m.call(func() tea.Cmd { return bridge.RestartContainer(m.docker, d.c.ID) })
	case actRemove:
		if d.armed != actRemove {
			if d.kind == imageDlg {
				if users := m.imageUsers(d.img); len(users) > 0 {
					d.blocker, d.failMsg = users[0].ID, "" // Docker would refuse: say why now
					m.addLine(noteLine, users[0].Name+" uses this image. Remove that container first, then the image.")
					return nil
				}
			}
			d.armed = actRemove // ask for a second press
			return nil
		}
		d.armed, d.phase, d.phaseAt, d.failMsg = actNone, phaseRemoving, m.now, ""
		if d.kind == imageDlg {
			return m.call(func() tea.Cmd { return bridge.RemoveImage(m.docker, d.img) })
		}
		return m.call(func() tea.Cmd { return bridge.RemoveContainer(m.docker, d.c.ID) })
	case actUntag:
		d.phase, d.phaseAt, d.failMsg = phaseRemoving, m.now, ""
		tag := d.img.Tags[0]
		return m.call(func() tea.Cmd { return bridge.RemoveImageTag(m.docker, d.img.ID, tag) })
	case actGoTo:
		id := d.blocker
		m.closeDialog()
		m.selectContainer(id)
		return m.openDialog()
	}
	return nil
}

// call sends a bridge command, unless there is no Docker client (tests feed
// the answers themselves).
func (m *Model) call(cmd func() tea.Cmd) tea.Cmd {
	m.busy(0.5)
	if m.docker == nil {
		return nil
	}
	return cmd()
}

// failed records a refused action: the phase to go back to, the message
// beside the buttons, and Docker's reason in the panel.
func (m *Model) failed(p phase, msg string, err error) tea.Cmd {
	d := m.dlg
	d.phase, d.phaseAt, d.failMsg = p, m.now, msg
	text := err.Error()
	if d.kind == containerDlg {
		text = strings.ReplaceAll(text, d.c.ID, shortID(d.c.ID))
	} else {
		text = strings.ReplaceAll(text, strings.TrimPrefix(d.img.ID, "sha256:"), d.img.ShortID)
	}
	m.addLine(noteLine, text)
	return tea.Batch(m.Load(), m.kick())
}

// done closes the dialog after a removal and leaves a note under the box.
func (m *Model) done(note string) tea.Cmd {
	m.closeDialog()
	m.toast, m.toastAt = note, m.now
	m.busy(toastFor + 0.1)
	return tea.Batch(m.Load(), m.kick())
}

// dialogMsg handles the bridge messages that belong to the dialog. An
// answer for a dialog that was closed (or reopened on something else) only
// refreshes the list.
func (m *Model) dialogMsg(msg tea.Msg) tea.Cmd {
	d := m.dlg
	mine := func(id string, p ...phase) bool {
		if d == nil || (d.c.ID != id && d.img.ID != id) {
			return false
		}
		for _, q := range p {
			if d.phase == q {
				return true
			}
		}
		return false
	}
	switch msg := msg.(type) {
	case bridge.StartedMsg:
		if !mine(msg.ID, phaseStarting) {
			return m.Load()
		}
		if msg.Err != nil {
			return m.failed(phaseFailed, "Docker didn't start it.", msg.Err)
		}
		d.phase, d.phaseAt = phaseRunning, m.now
		d.c.State, d.c.Status = "running", "just started" // until the list refreshes
		m.busy(flashDur)
		return tea.Batch(m.openLogs(), m.Load(), m.kick())

	case bridge.StoppedMsg:
		if !mine(msg.ID, phaseStopping) {
			return m.Load()
		}
		if msg.Err != nil {
			return m.failed(phaseRunning, "Docker didn't stop it.", msg.Err)
		}
		d.phase, d.phaseAt = phaseReady, m.now
		d.c.State, d.c.Status = "exited", "just stopped"
		m.busy(flashDur)
		return tea.Batch(m.Load(), m.kick())

	case bridge.RestartedMsg:
		if !mine(msg.ID, phaseRestarting) {
			return m.Load()
		}
		if msg.Err != nil {
			return m.failed(phaseRunning, "Docker didn't restart it.", msg.Err)
		}
		m.fresh(phaseRunning) // the old stream ended with the stop; follow the new run
		d.c.State, d.c.Status = "running", "just restarted"
		m.busy(flashDur)
		return tea.Batch(m.openLogs(), m.Load(), m.kick())

	case bridge.ContainerRemovedMsg:
		if !mine(msg.ID, phaseRemoving) {
			return m.Load()
		}
		if msg.Err != nil {
			return m.failed(phaseReady, "Docker didn't remove it.", msg.Err)
		}
		return m.done("removed " + d.c.Name)

	case bridge.ImageRemovedMsg:
		if !mine(msg.ID, phaseRemoving) {
			return m.Load()
		}
		for _, t := range msg.Untagged {
			m.addLine(infoLine, "untagged "+t)
			for i, have := range d.img.Tags {
				if have == t {
					d.img.Tags = append(d.img.Tags[:i:i], d.img.Tags[i+1:]...)
					break
				}
			}
		}
		switch {
		case msg.Err != nil:
			if id := m.userIn(msg.Err.Error()); id != "" {
				d.blocker = id // a container uses it: GO TO takes you there
				return m.failed(phaseReady, "", msg.Err)
			}
			return m.failed(phaseReady, "Docker didn't remove it.", msg.Err)
		case msg.Deleted:
			name := imageName(d.img)
			if len(msg.Untagged) > 0 {
				name = msg.Untagged[0]
			}
			return m.done("removed " + name)
		}
		d.phase, d.phaseAt = phaseReady, m.now
		return tea.Batch(m.Load(), m.kick())

	case bridge.LogsOpenedMsg:
		if d == nil || d.c.ID != msg.ID || !d.opening || d.stream != nil {
			if msg.Stream != nil {
				msg.Stream.Close() // nobody is reading it anymore
			}
			return nil
		}
		d.opening = false
		if msg.Err != nil {
			d.ended = true
			m.addLine(noteLine, msg.Err.Error())
			return m.kick()
		}
		d.stream = msg.Stream
		return msg.Stream.Next()

	case bridge.LogsMsg:
		if d == nil || msg.Stream != d.stream {
			return nil // a stream that was closed
		}
		for _, l := range msg.Lines {
			k := stdoutLine
			if l.Stderr {
				k = stderrLine
			}
			m.addLine(k, l.Text)
		}
		if !msg.Done {
			return tea.Batch(msg.Stream.Next(), m.kick())
		}
		d.stream, d.ended = nil, true
		if msg.Err != nil {
			m.addLine(noteLine, msg.Err.Error())
		} else {
			m.addLine(infoLine, "output ended")
		}
		return tea.Batch(m.Load(), m.kick()) // it probably stopped: refresh its state
	}
	return nil
}

// addLine appends one line to the panel. Scrolled back, the view stays on
// the same lines instead of following.
func (m *Model) addLine(kind lineKind, text string) {
	d := m.dlg
	text = cleanLog(text)
	if d.back > 0 {
		w := m.dialogLayout()
		d.back += len(wrap(text, w.termR-w.termL))
	}
	d.logs = append(d.logs, logLine{text: text, kind: kind, at: m.now})
	if len(d.logs) > maxLogLines*5/4 {
		d.logs = append([]logLine(nil), d.logs[len(d.logs)-maxLogLines:]...)
	}
	m.busy(lineIn + 0.05)
}

// dialogSynced updates the dialog from a fresh list. When a container's
// output has ended and it is no longer up, the dialog offers its actions
// for a stopped container.
func (m *Model) dialogSynced() {
	d := m.dlg
	if d == nil {
		return
	}
	if d.kind == imageDlg {
		for _, im := range m.images {
			if im.ID == d.img.ID {
				d.img = im
				break
			}
		}
		return
	}
	if c, ok := m.containerByID(d.c.ID); ok {
		d.c = c
	}
	if d.phase == phaseRunning && d.ended && !isUp(d.c) {
		d.phase, d.phaseAt = phaseReady, m.now
	}
}

func (m *Model) containerByID(id string) (models.DockerModel, bool) {
	if id == "" {
		return models.DockerModel{}, false
	}
	for _, c := range m.containers {
		if c.ID == id {
			return c, true
		}
	}
	return models.DockerModel{}, false
}

// imageUsers are the containers created from an image, by one of its tags
// or by its ID.
func (m *Model) imageUsers(im models.ImageModel) []models.DockerModel {
	tags := map[string]bool{}
	for _, t := range im.Tags {
		tags[withTag(t)] = true
	}
	hex := strings.TrimPrefix(im.ID, "sha256:")
	var out []models.DockerModel
	for _, c := range m.containers {
		ref := c.Image
		byID := ref == im.ID || (len(ref) >= 12 && strings.HasPrefix(hex, strings.TrimPrefix(ref, "sha256:")))
		if tags[withTag(ref)] || byID {
			out = append(out, c)
		}
	}
	return out
}

// withTag adds the ":latest" Docker assumes when a reference names no tag.
func withTag(ref string) string {
	if strings.LastIndex(ref, ":") <= strings.LastIndex(ref, "/") {
		return ref + ":latest"
	}
	return ref
}

// usesImage finds the container in Docker's "container 3f57d9401f8d is
// using its referenced image" refusal.
var usesImage = regexp.MustCompile(`container ([0-9a-f]{12,64})`)

// userIn returns the ID of a known container named in a Docker error, or "".
func (m *Model) userIn(errText string) string {
	for _, sm := range usesImage.FindAllStringSubmatch(errText, -1) {
		for _, c := range m.containers {
			if strings.HasPrefix(c.ID, sm[1]) {
				return c.ID
			}
		}
	}
	return ""
}

func (m *Model) dialogKey(k tea.KeyMsg) tea.Cmd {
	d, ks := m.dlg, m.dkeys
	w := m.dialogLayout()
	switch {
	case key.Matches(k, ks.Close):
		if d.armed != actNone {
			d.armed = actNone // esc takes back a remove first
		} else {
			m.closeDialog()
		}
	case key.Matches(k, ks.Start):
		return tea.Batch(m.do(actStart), m.kick())
	case key.Matches(k, ks.Stop):
		return tea.Batch(m.do(actStop), m.kick())
	case key.Matches(k, ks.Restart):
		return tea.Batch(m.do(actRestart), m.kick())
	case key.Matches(k, ks.Remove):
		return tea.Batch(m.do(actRemove), m.kick())
	case key.Matches(k, ks.Untag):
		return tea.Batch(m.do(actUntag), m.kick())
	case key.Matches(k, ks.GoTo):
		return tea.Batch(m.do(actGoTo), m.kick())
	case key.Matches(k, ks.Up):
		m.scrollLogs(1)
	case key.Matches(k, ks.Down):
		m.scrollLogs(-1)
	case key.Matches(k, ks.PageUp):
		m.scrollLogs(max(1, w.termRows-1))
	case key.Matches(k, ks.PageDown):
		m.scrollLogs(-max(1, w.termRows-1))
	case key.Matches(k, ks.Top):
		m.scrollLogs(1 << 30)
	case key.Matches(k, ks.Follow):
		d.back = 0
	}
	return m.kick()
}

func (m *Model) dialogMouse(e tea.MouseMsg) tea.Cmd {
	d := m.dlg
	w := m.dialogLayout()
	d.hover, d.onClose = actNone, false
	for _, b := range w.btns {
		if e.Y == w.actRow && e.X >= b.x0 && e.X < b.x1 {
			d.hover = b.act
		}
	}
	if e.Y == w.close[2] && e.X >= w.close[0] && e.X < w.close[1] {
		d.onClose = true
	}
	switch {
	case e.Button == tea.MouseButtonWheelUp:
		m.scrollLogs(3)
	case e.Button == tea.MouseButtonWheelDown:
		m.scrollLogs(-3)
	case e.Action == tea.MouseActionPress && e.Button == tea.MouseButtonLeft:
		switch {
		case d.hover != actNone:
			return tea.Batch(m.do(d.hover), m.kick())
		case d.onClose:
			m.closeDialog()
		}
	}
	return m.kick()
}

// scrollLogs moves the panel n rows back in time (negative: forward).
func (m *Model) scrollLogs(n int) {
	d := m.dlg
	w := m.dialogLayout()
	d.back = max(0, d.back+n)
	_, d.back = d.tail(w.termR-w.termL, w.termRows)
}
