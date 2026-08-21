package autostart

import (
	"bytes"
	"encoding/xml"
	"strings"
)

const (
	applicationID = "ma.del.sluice.desktop"
	backgroundArg = "--background"
)

func darwinRegistration(executable string) []byte {
	var escaped bytes.Buffer
	_ = xml.EscapeText(&escaped, []byte(executable))
	return []byte(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key>
  <string>` + applicationID + `</string>
  <key>ProgramArguments</key>
  <array>
    <string>` + escaped.String() + `</string>
    <string>` + backgroundArg + `</string>
  </array>
  <key>RunAtLoad</key>
  <true/>
  <key>LimitLoadToSessionType</key>
  <string>Aqua</string>
</dict>
</plist>
`)
}

func linuxRegistration(executable string) []byte {
	return []byte(`[Desktop Entry]
Type=Application
Version=1.0
Name=Sluice
Comment=Start the Sluice synchronization client
Exec=` + desktopExecArgument(executable) + ` ` + backgroundArg + `
Terminal=false
X-GNOME-Autostart-enabled=true
`)
}

func desktopExecArgument(argument string) string {
	replacer := strings.NewReplacer(
		`\`, `\\`,
		`"`, `\"`,
		"`", "\\`",
		`$`, `\$`,
		`%`, `%%`,
	)
	return `"` + replacer.Replace(argument) + `"`
}

func windowsRunCommand(executable string) string {
	return quoteWindowsArgument(executable) + " " + backgroundArg
}

func quoteWindowsArgument(argument string) string {
	var quoted strings.Builder
	quoted.WriteByte('"')
	backslashes := 0
	for _, character := range argument {
		switch character {
		case '\\':
			backslashes++
		case '"':
			quoted.WriteString(strings.Repeat(`\`, backslashes*2+1))
			quoted.WriteRune(character)
			backslashes = 0
		default:
			quoted.WriteString(strings.Repeat(`\`, backslashes))
			quoted.WriteRune(character)
			backslashes = 0
		}
	}
	quoted.WriteString(strings.Repeat(`\`, backslashes*2))
	quoted.WriteByte('"')
	return quoted.String()
}
