package cli

import (
	"fmt"
	"os/exec"
	"runtime"
)

// openBrowser opens url in the default browser for the host OS. It is the
// production value of App.OpenURL; tests substitute a recorder.
func openBrowser(url string) error {
	var cmd string
	var args []string
	switch runtime.GOOS {
	case "darwin":
		cmd = "open"
	case "windows":
		cmd, args = "rundll32", []string{"url.dll,FileProtocolHandler"}
	default: // linux, bsd, etc.
		cmd = "xdg-open"
	}
	args = append(args, url)
	if _, err := exec.LookPath(cmd); err != nil {
		return fmt.Errorf("could not find %q to open a browser; visit %s manually", cmd, url)
	}
	return exec.Command(cmd, args...).Start()
}
