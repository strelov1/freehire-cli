package cli

import (
	"regexp"
	"strings"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// descriptionText renders a posting's description — sanitized HTML from the
// API — as text a person reads in a terminal: paragraphs apart, list items as
// "- " bullets, entities decoded. A description that carries no markup (some
// sources store plain text) passes through untouched. It walks a parsed tree
// rather than pattern-matching tags, because board HTML often closes <li> and
// <p> implicitly and only a real parser applies those rules.
func descriptionText(s string) string {
	if !strings.Contains(s, "<") {
		return strings.TrimSpace(s)
	}
	doc, err := html.Parse(strings.NewReader(s))
	if err != nil {
		return strings.TrimSpace(s)
	}
	var w textWriter
	w.walk(doc)
	return strings.TrimSpace(w.b.String())
}

// textWriter accumulates text and the line breaks blocks ask for. A break is
// held as pending and only written before the next text, so nested blocks
// (<div><ul><li><p>) ask for breaks without stacking empty lines.
type textWriter struct {
	b       strings.Builder
	pending int // newlines owed before the next text: 1 = new line, 2 = blank line
	bullet  bool
	depth   int // list nesting, for bullet indentation
	inPre   bool
	// lastBullet records that the line written last was a bullet. Boards often
	// wrap every item in its own <ul>, and without this each one-item list's
	// surrounding blank line would split a single list into separate ones.
	lastBullet bool
}

var spaceRun = regexp.MustCompile(`\s+`)

func (w *textWriter) brk(n int) {
	if w.depth > 0 && n > 1 {
		n = 1 // list items stay on consecutive lines
	}
	w.pending = max(w.pending, n)
}

func (w *textWriter) text(t string) {
	if !w.inPre {
		t = spaceRun.ReplaceAllString(t, " ")
	}
	// Every newline is written from pending right before text, so a line has
	// just started exactly when nothing is written yet or a break is owed.
	atLineStart := w.b.Len() == 0 || w.pending > 0 || w.bullet
	if atLineStart && !w.inPre {
		t = strings.TrimLeft(t, " ")
	}
	if t == "" {
		return
	}
	if w.bullet && w.lastBullet {
		w.pending = min(w.pending, 1)
	}
	if w.b.Len() > 0 && w.pending > 0 {
		w.b.WriteString(strings.Repeat("\n", w.pending))
		w.lastBullet = false
	}
	w.pending = 0
	if w.bullet {
		w.b.WriteString(strings.Repeat("  ", max(w.depth-1, 0)) + "- ")
		w.bullet = false
		w.lastBullet = true
	}
	w.b.WriteString(t)
}

func (w *textWriter) walk(n *html.Node) {
	switch n.Type {
	case html.TextNode:
		w.text(n.Data)
		return
	case html.ElementNode:
		switch n.DataAtom {
		case atom.Script, atom.Style, atom.Head:
			return
		case atom.Br:
			w.brk(1)
			return
		}
	}

	breakAround := 0
	switch n.DataAtom {
	case atom.P, atom.H1, atom.H2, atom.H3, atom.H4, atom.H5, atom.H6,
		atom.Ul, atom.Ol, atom.Table, atom.Blockquote, atom.Pre, atom.Hr:
		breakAround = 2
	case atom.Div, atom.Section, atom.Article, atom.Tr, atom.Dt, atom.Dd:
		breakAround = 1
	case atom.Li:
		w.brk(1)
		w.bullet = true
	case atom.Td, atom.Th:
		w.text(" ")
	}
	if breakAround > 0 {
		w.brk(breakAround)
	}

	list := n.DataAtom == atom.Ul || n.DataAtom == atom.Ol
	if list {
		w.depth++
	}
	if n.DataAtom == atom.Pre {
		w.inPre = true
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		w.walk(c)
	}
	// Leave the list before asking for the break after it, or the blank line
	// that should separate it from the next paragraph is capped to a newline.
	if list {
		w.depth--
	}
	if n.DataAtom == atom.Pre {
		w.inPre = false
	}

	if breakAround > 0 {
		w.brk(breakAround)
	}
	if n.DataAtom == atom.Li {
		w.bullet = false
		w.brk(1)
	}
}
