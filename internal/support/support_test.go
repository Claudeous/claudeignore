package support

import (
	"runtime"
	"strings"
	"testing"
)

// Task 1: StyledMessage

func TestStyledMessage_NonEmpty(t *testing.T) {
	msg := StyledMessage()
	if msg == "" {
		t.Fatal("StyledMessage() returned empty string")
	}
}

func TestStyledMessage_ContainsSupportURL(t *testing.T) {
	msg := StyledMessage()
	if !strings.Contains(msg, SupportURL) {
		t.Errorf("StyledMessage() does not contain SupportURL %q; got %q", SupportURL, msg)
	}
}

func TestStyledMessage_ContainsEmoji(t *testing.T) {
	msg := StyledMessage()
	if !strings.Contains(msg, "💜") {
		t.Errorf("StyledMessage() does not contain 💜 emoji; got %q", msg)
	}
}

// Task 2: ShouldShow

func TestShouldShow_Distribution(t *testing.T) {
	const runs = 1000
	hits := 0
	for i := 0; i < runs; i++ {
		if ShouldShow() {
			hits++
		}
	}
	// Expect roughly 20% (200 hits). Allow generous range 100–300 to avoid flakiness.
	if hits < 100 || hits > 300 {
		t.Errorf("ShouldShow() hit %d/1000 times; expected between 100 and 300", hits)
	}
}

// Task 3: BrowserCommand

func TestBrowserCommand_Darwin(t *testing.T) {
	cmd, args := BrowserCommand("https://example.com")
	if runtime.GOOS != "darwin" {
		t.Skip("darwin-only test")
	}
	if cmd != "open" {
		t.Errorf("darwin: expected command %q, got %q", "open", cmd)
	}
	if len(args) != 1 || args[0] != "https://example.com" {
		t.Errorf("darwin: expected args [https://example.com], got %v", args)
	}
}

func TestBrowserCommand_Linux(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("linux-only test")
	}
	cmd, args := BrowserCommand("https://example.com")
	if cmd != "xdg-open" {
		t.Errorf("linux: expected command %q, got %q", "xdg-open", cmd)
	}
	if len(args) != 1 || args[0] != "https://example.com" {
		t.Errorf("linux: expected args [https://example.com], got %v", args)
	}
}

func TestBrowserCommand_Windows(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("windows-only test")
	}
	cmd, args := BrowserCommand("https://example.com")
	if cmd != "cmd" {
		t.Errorf("windows: expected command %q, got %q", "cmd", cmd)
	}
	if len(args) != 3 || args[0] != "/c" || args[1] != "start" || args[2] != "https://example.com" {
		t.Errorf("windows: expected args [/c start https://example.com], got %v", args)
	}
}

func TestBrowserCommand_CorrectForCurrentOS(t *testing.T) {
	url := "https://example.com"
	cmd, args := BrowserCommand(url)

	switch runtime.GOOS {
	case "darwin":
		if cmd != "open" {
			t.Errorf("darwin: expected %q, got %q", "open", cmd)
		}
		if len(args) != 1 || args[0] != url {
			t.Errorf("darwin: unexpected args %v", args)
		}
	case "windows":
		if cmd != "cmd" {
			t.Errorf("windows: expected %q, got %q", "cmd", cmd)
		}
		if len(args) < 3 || args[0] != "/c" || args[1] != "start" {
			t.Errorf("windows: unexpected args %v", args)
		}
	default:
		if cmd != "xdg-open" {
			t.Errorf("linux: expected %q, got %q", "xdg-open", cmd)
		}
		if len(args) != 1 || args[0] != url {
			t.Errorf("linux: unexpected args %v", args)
		}
	}
}
