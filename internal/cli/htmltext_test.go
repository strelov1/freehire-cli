package cli

import "testing"

func TestDescriptionText(t *testing.T) {
	cases := []struct{ name, in, want string }{
		{"plain text passes through", "  Build things.\nShip them.  ", "Build things.\nShip them."},
		{
			"paragraphs sit a blank line apart",
			"<p>We build tools.</p><p>You ship them.</p>",
			"We build tools.\n\nYou ship them.",
		},
		{
			// The shape a real posting arrived in (Perkbox, 2026-10-20): every
			// bullet wrapped in its own <div><ul><li><p>.
			"nested list wrappers collapse to one bullet each",
			"<h3>Requirements</h3><div><ul><li><p>Go and gRPC.</p></li></ul></div><div><ul><li><p>AWS.</p></li></ul></div><p>Apply now.</p>",
			"Requirements\n\n- Go and gRPC.\n- AWS.\n\nApply now.",
		},
		{"implicitly closed items still split", "<ul><li>One<li>Two</ul>", "- One\n- Two"},
		{"entities decode", "<p>R&amp;D &amp; Q&amp;A</p>", "R&D & Q&A"},
		{"inline markup keeps its spaces", "<p>Use <b>Go</b> and <i>Rust</i>.</p>", "Use Go and Rust."},
		{"br breaks a line", "<p>Line one<br>Line two</p>", "Line one\nLine two"},
		{"nested lists indent", "<ul><li>Backend<ul><li>Go</li></ul></li></ul>", "- Backend\n  - Go"},
	}
	for _, tc := range cases {
		if got := descriptionText(tc.in); got != tc.want {
			t.Errorf("%s:\n got %q\nwant %q", tc.name, got, tc.want)
		}
	}
}
