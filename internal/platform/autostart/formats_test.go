package autostart

import (
	"strings"
	"testing"
)

func TestDarwinRegistrationEscapesProgramArgument(t *testing.T) {
	registration := string(darwinRegistration(`/Applications/Sluice & Tools/Sluice "Desktop"`))

	for _, want := range []string{
		`<string>/Applications/Sluice &amp; Tools/Sluice &#34;Desktop&#34;</string>`,
		`<string>--background</string>`,
		`<key>RunAtLoad</key>`,
		`<key>LimitLoadToSessionType</key>`,
	} {
		if !strings.Contains(registration, want) {
			t.Errorf("registration does not contain %q:\n%s", want, registration)
		}
	}
}

func TestLinuxRegistrationQuotesReservedCharacters(t *testing.T) {
	registration := string(linuxRegistration(`/opt/Sluice $HOME/100%/sluice\\desktop`))
	want := `Exec="/opt/Sluice \$HOME/100%%/sluice\\\\desktop" --background`
	if !strings.Contains(registration, want) {
		t.Fatalf("registration does not contain %q:\n%s", want, registration)
	}
	if !strings.Contains(registration, "X-GNOME-Autostart-enabled=true\n") {
		t.Fatalf("registration does not explicitly enable GNOME autostart:\n%s", registration)
	}
}

func TestWindowsRunCommandQuotesExecutable(t *testing.T) {
	got := windowsRunCommand(`C:\\Program Files\\Sluice\\sluice-desktop.exe`)
	want := `"C:\\Program Files\\Sluice\\sluice-desktop.exe" --background`
	if got != want {
		t.Fatalf("windowsRunCommand() = %q, want %q", got, want)
	}
}
