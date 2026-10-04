package cli

import (
	"os/exec"
	"runtime"
)

// openBrowser launches the system's default browser on a URL. A var, not a
// func, so TestAuthLoginOAuthValidatesAndWritesCreds can swap in a stub that
// drives the local OAuth callback directly instead of a real browser.
var openBrowser = defaultOpenBrowser

func defaultOpenBrowser(url string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	return cmd.Start()
}
