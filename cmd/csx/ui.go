package main

import (
	"fmt"
	"io"
	"math"
	"os"
	"strconv"
	"strings"

	colors "github.com/SCKelemen/color"
	textfmt "github.com/SCKelemen/text"
)

type cliUI struct {
	out     io.Writer
	text    *textfmt.Text
	width   float64
	kvWidth float64
	accent  colors.Color
	success colors.Color
	warning colors.Color
	danger  colors.Color
	muted   colors.Color
}

func newCLIUI(out io.Writer) *cliUI {
	width := 80.0
	if columns, err := strconv.Atoi(strings.TrimSpace(os.Getenv("COLUMNS"))); err == nil && columns >= 40 {
		width = float64(columns)
	}
	return &cliUI{
		out:     out,
		text:    textfmt.NewTerminal(),
		width:   width,
		kvWidth: 14,
		accent:  mustColor("#7C3AED"),
		success: mustColor("#059669"),
		warning: mustColor("#D97706"),
		danger:  mustColor("#DC2626"),
		muted:   mustColor("#6B7280"),
	}
}

// println writes a line to the output.
func (u *cliUI) println(parts ...string) {
	_, _ = fmt.Fprintln(u.out, strings.Join(parts, " "))
}

// section renders a section header with a horizontal rule.
func (u *cliUI) section(title string) {
	titleWidth := u.text.Width(title)
	remaining := u.width - titleWidth - 2 // 2 for spaces
	if remaining < 4 {
		remaining = 4
	}
	rule := strings.Repeat("─", int(remaining))
	u.println(u.paint(u.accent, title), u.paint(u.muted, rule))
}

// info renders an informational line with a bullet.
func (u *cliUI) info(format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	u.println(u.paint(u.muted, "  •"), msg)
}

// successf renders a success line with a checkmark.
func (u *cliUI) successf(format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	u.println(u.paint(u.success, "  ✓"), msg)
}

// warnf renders a warning line.
func (u *cliUI) warnf(format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	u.println(u.paint(u.warning, "  !"), msg)
}

// errorf renders an error line.
func (u *cliUI) errorf(format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	u.println(u.paint(u.danger, "  ✗"), msg)
}

// kv renders a key-value pair with proper alignment.
// The key is right-padded to kvWidth using Unicode-aware measurement
// so CJK characters, emoji, and combining marks align correctly.
func (u *cliUI) kv(key, value string) {
	label := key + ":"
	labelWidth := u.text.Width(label)
	padding := u.kvWidth - labelWidth
	if padding < 1 {
		padding = 1
	}
	padStr := strings.Repeat(" ", int(math.Ceil(padding)))
	u.println("  " + u.paint(u.muted, label) + padStr + value)
}

// label paints a value in the muted color.
func (u *cliUI) label(value string) string {
	return u.paint(u.muted, value)
}

// path elides a file path to fit within the given width,
// using Unicode-aware measurement.
func (u *cliUI) path(path string, width float64) string {
	return u.text.ElidePath(path, width)
}

// snippet cleans and elides a code snippet to fit within the given width,
// using Unicode-aware measurement.
func (u *cliUI) snippet(snippet string, width float64) string {
	clean := strings.TrimSpace(strings.ReplaceAll(snippet, "	", "    "))
	if clean == "" {
		return ""
	}
	return u.text.ElideEnd(clean, width)
}

// divider renders a thin horizontal rule.
func (u *cliUI) divider() {
	u.println(u.paint(u.muted, "  "+strings.Repeat("─", int(u.width-4))))
}

// paint applies 24-bit ANSI color to a string.
func (u *cliUI) paint(c colors.Color, value string) string {
	r, g, b, _ := c.RGBA()
	return fmt.Sprintf("[38;2;%d;%d;%dm%s[0m", toByte(r), toByte(g), toByte(b), value)
}

// mustColor parses a hex color string. It returns black on invalid input
// instead of panicking, ensuring CLI startup never crashes on bad color values.
func mustColor(hex string) colors.Color {
	colorValue, err := colors.HexToRGB(hex)
	if err != nil {
		return colors.RGB(0, 0, 0)
	}
	return colorValue
}

func toByte(value float64) int {
	return int(math.Round(value * 255))
}
