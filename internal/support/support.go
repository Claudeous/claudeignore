package support

import (
	"math/rand/v2"
	"os/exec"
	"runtime"

	"github.com/charmbracelet/lipgloss"
)

// SupportURL is the project's support/sponsorship URL.
const SupportURL = "https://github.com/Claudeous/claudeignore#support"

var (
	textStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("13"))
	urlStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("14"))
)

// StyledMessage returns a styled terminal message encouraging support.
func StyledMessage() string {
	msg := textStyle.Render("💜 claudeignore is free & open source. Support development → ")
	url := urlStyle.Render(SupportURL)
	return msg + url
}

// ShouldShow returns true with roughly 20% probability (1-in-5 chance).
func ShouldShow() bool {
	return rand.IntN(5) == 0
}

// BrowserCommand returns the OS-appropriate command and args to open a URL.
func BrowserCommand(url string) (string, []string) {
	switch runtime.GOOS {
	case "darwin":
		return "open", []string{url}
	case "windows":
		return "cmd", []string{"/c", "start", url}
	default: // linux and others
		return "xdg-open", []string{url}
	}
}

// OpenBrowser opens SupportURL in the system's default browser.
func OpenBrowser() error {
	cmd, args := BrowserCommand(SupportURL)
	return exec.Command(cmd, args...).Start()
}
