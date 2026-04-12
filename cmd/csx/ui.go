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
	accent  colors.Color
	success colors.Color
	warning colors.Color
	danger  colors.Color
	muted   colors.Color
}

func newCLIUI(out io.Writer) *cliUI {
	width := 100.0
	if columns, err := strconv.Atoi(strings.TrimSpace(os.Getenv("COLUMNS"))); err == nil && columns >= 60 {
		width = float64(columns - 8)
	}
	return &cliUI{
		out:     out,
		text:    textfmt.NewTerminal(),
		width:   width,
		accent:  mustColor("#7C3AED"),
		success: mustColor("#059669"),
		warning: mustColor("#D97706"),
		danger:  mustColor("#DC2626"),
		muted:   mustColor("#6B7280"),
	}
}

func (u *cliUI) println(parts ...string) {
	_, _ = fmt.Fprintln(u.out, strings.Join(parts, " "))
}

func (u *cliUI) section(title string) {
	u.println(u.paint(u.accent, title))
}

func (u *cliUI) info(format string, args ...any) {
	u.println(u.paint(u.accent, "•"), fmt.Sprintf(format, args...))
}

func (u *cliUI) successf(format string, args ...any) {
	u.println(u.paint(u.success, "✓"), fmt.Sprintf(format, args...))
}

func (u *cliUI) warnf(format string, args ...any) {
	u.println(u.paint(u.warning, "!"), fmt.Sprintf(format, args...))
}

func (u *cliUI) errorf(format string, args ...any) {
	u.println(u.paint(u.danger, "x"), fmt.Sprintf(format, args...))
}

func (u *cliUI) kv(key, value string) {
	u.println(u.label(key+":"), value)
}

func (u *cliUI) label(value string) string {
	return u.paint(u.muted, value)
}

func (u *cliUI) path(path string, width float64) string {
	return u.text.ElidePath(path, width)
}

func (u *cliUI) snippet(snippet string, width float64) string {
	clean := strings.TrimSpace(strings.ReplaceAll(snippet, "\t", "    "))
	if clean == "" {
		return ""
	}
	return u.text.ElideEnd(clean, width)
}

func (u *cliUI) paint(c colors.Color, value string) string {
	r, g, b, _ := c.RGBA()
	return fmt.Sprintf("\x1b[38;2;%d;%d;%dm%s\x1b[0m", toByte(r), toByte(g), toByte(b), value)
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
