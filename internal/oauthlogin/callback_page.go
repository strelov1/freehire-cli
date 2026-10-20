package oauthlogin

// The page the browser lands on after the authorize screen. It cannot say
// "signed in": the code exchange happens after this response, in the terminal,
// which is where the real outcome is reported. Styled like freehire's own
// consent card (freehire's internal/api/handler/consent_page.go) so the round
// trip does not end on a bare page.

const callbackPageHead = `<!doctype html>
<html lang="en">
<head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>freehire CLI</title><style>
:root{--bg:oklch(.997 .003 130);--fg:oklch(.13 0 0);--card:oklch(1 0 0);--muted:oklch(.43 0 0);--border:oklch(.92 0 0);--tile:oklch(.96 0 0);color-scheme:light dark}
@media (prefers-color-scheme:dark){:root{--bg:oklch(.16 .006 110);--fg:oklch(.985 0 0);--card:oklch(.2 0 0);--muted:oklch(.72 0 0);--border:oklch(1 0 0/10%);--tile:oklch(1 0 0/6%)}}
*{box-sizing:border-box}
body{margin:0;min-height:100vh;display:flex;align-items:center;justify-content:center;padding:1.5rem;background:var(--bg);color:var(--fg);font:15px/1.5 ui-sans-serif,system-ui,-apple-system,"Segoe UI",Roboto,"Helvetica Neue",Arial,sans-serif}
.card{width:100%;max-width:28rem;background:var(--card);border:1px solid var(--border);border-radius:.875rem;overflow:hidden}
.brand{display:flex;align-items:center;justify-content:center;gap:.5rem;padding:1rem;border-bottom:1px solid var(--border);font-weight:600;letter-spacing:-.01em}
.mark{width:1.375rem;height:1.375rem;fill:currentColor}
.body{padding:1.75rem 2rem 2rem;text-align:center}
.icon{width:2.75rem;height:2.75rem;margin:0 auto 1rem;display:flex;align-items:center;justify-content:center;border-radius:.625rem;background:var(--tile)}
h1{margin:0;font-size:1.375rem;line-height:1.3;letter-spacing:-.02em}
p{margin:.75rem 0 0;color:var(--muted)}
code{font:.875rem ui-monospace,SFMono-Regular,Menlo,monospace;color:var(--fg)}
</style></head>
<body>
<main class="card">
  <div class="brand"><svg class="mark" viewBox="0 0 512 512" aria-hidden="true"><path fill-rule="evenodd" clip-rule="evenodd" d="M256 56C366.457 56 456 145.543 456 256C456 366.457 366.457 456 256 456C145.543 456 56 366.457 56 256C56 145.543 145.543 56 256 56ZM256 166L346 256L256 346L166 256L256 166Z"/></svg><span>freehire</span></div>
  <div class="body">
`

const callbackPageTail = `  </div>
</main>
</body>
</html>`

// terminalIcon is a prompt glyph: the next step happens in the terminal.
const terminalIcon = `<div class="icon"><svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="m5 8 4 4-4 4M12 16h7"/></svg></div>
`

const (
	callbackApprovedPage = callbackPageHead + terminalIcon + `    <h1>Return to the terminal</h1>
    <p>freehire sent your approval back. You can close this tab — the <code>freehire</code> CLI finishes signing you in.</p>
` + callbackPageTail

	callbackRefusedPage = callbackPageHead + terminalIcon + `    <h1>Connection cancelled</h1>
    <p>No access was granted. You can close this tab and run <code>freehire auth login</code> again if you change your mind.</p>
` + callbackPageTail
)

// callbackPage picks the page for what the redirect carried. Neither embeds a
// query value, so nothing here needs escaping.
func callbackPage(errorParam string) string {
	if errorParam != "" {
		return callbackRefusedPage
	}
	return callbackApprovedPage
}
