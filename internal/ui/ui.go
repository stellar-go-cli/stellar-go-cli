package ui

import (
	"fmt"
	"os"
	"strings"
	"time"
)

// ANSI color codes
const (
	Reset = "\033[0m"
	Bold  = "\033[1m"
	Dim   = "\033[2m"

	Black   = "\033[30m"
	Red     = "\033[31m"
	Green   = "\033[32m"
	Yellow  = "\033[33m"
	Blue    = "\033[34m"
	Magenta = "\033[35m"
	Cyan    = "\033[36m"
	White   = "\033[37m"

	BrightRed     = "\033[91m"
	BrightGreen   = "\033[92m"
	BrightYellow  = "\033[93m"
	BrightBlue    = "\033[94m"
	BrightMagenta = "\033[95m"
	BrightCyan    = "\033[96m"
	BrightWhite   = "\033[97m"

	Gold  = "\033[38;5;220m"
	Teal  = "\033[38;5;51m"
	Coral = "\033[38;5;209m"
)

func isTerminal() bool {
	fi, err := os.Stdout.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}

func colorize(color, text string) string {
	if !isTerminal() {
		return text
	}
	return color + text + Reset
}

func Gold_(s string) string    { return colorize(Gold, s) }
func Teal_(s string) string    { return colorize(Teal, s) }
func Green_(s string) string   { return colorize(BrightGreen, s) }
func Red_(s string) string     { return colorize(BrightRed, s) }
func Yellow_(s string) string  { return colorize(BrightYellow, s) }
func Cyan_(s string) string    { return colorize(BrightCyan, s) }
func Blue_(s string) string    { return colorize(BrightBlue, s) }
func Magenta_(s string) string { return colorize(BrightMagenta, s) }
func Dim_(s string) string     { return colorize(Dim, s) }
func Bold_(s string) string    { return colorize(Bold, s) }
func White_(s string) string   { return colorize(BrightWhite, s) }

// ─────────────────────────────────────────────
// Banner
// ─────────────────────────────────────────────

func PrintBanner() {
	fmt.Println()
	fmt.Println(Gold_("  ███╗   ███╗ ██████╗ ███████╗ █████╗ ██████╗ ████████╗"))
	fmt.Println(Gold_("  ████╗ ████║██╔═══██╗╚══███╔╝██╔══██╗██╔══██╗╚══██╔══╝"))
	fmt.Println(Gold_("  ██╔████╔██║██║   ██║  ███╔╝ ███████║██████╔╝   ██║   "))
	fmt.Println(Gold_("  ██║╚██╔╝██║██║   ██║ ███╔╝  ██╔══██║██╔══██╗   ██║   "))
	fmt.Println(Gold_("  ██║ ╚═╝ ██║╚██████╔╝███████╗██║  ██║██║  ██║   ██║   "))
	fmt.Println(Gold_("  ╚═╝     ╚═╝ ╚═════╝ ╚══════╝╚═╝  ╚═╝╚═╝  ╚═╝   ╚═╝  "))
	fmt.Println()
	fmt.Printf("  %s %s  %s\n",
		Teal_("Orchestrated Agreements CLI"),
		Dim_("|"),
		Dim_("v0.1.0-mvp · OG Technologies EU"),
	)
	fmt.Println(Dim_("  ─────────────────────────────────────────────────────"))
	fmt.Println()
}

// ─────────────────────────────────────────────
// Spinner
// ─────────────────────────────────────────────

type Spinner struct {
	msg    string
	done   chan struct{}
	frames []string
}

func NewSpinner(msg string) *Spinner {
	return &Spinner{
		msg:    msg,
		done:   make(chan struct{}),
		frames: []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"},
	}
}

func (s *Spinner) Start() {
	go func() {
		i := 0
		for {
			select {
			case <-s.done:
				fmt.Printf("\r%s\r", strings.Repeat(" ", 80))
				return
			default:
				fmt.Printf("\r  %s %s", Teal_(s.frames[i%len(s.frames)]), s.msg)
				time.Sleep(80 * time.Millisecond)
				i++
			}
		}
	}()
}

func (s *Spinner) Stop(success bool, msg string) {
	close(s.done)
	time.Sleep(100 * time.Millisecond)
	if success {
		fmt.Printf("  %s %s\n", Green_("✓"), msg)
	} else {
		fmt.Printf("  %s %s\n", Red_("✗"), msg)
	}
}

// ─────────────────────────────────────────────
// Output helpers
// ─────────────────────────────────────────────

func Success(msg string) {
	fmt.Printf("  %s %s\n", Green_("✓"), msg)
}

func Error(msg string) {
	fmt.Printf("  %s %s\n", Red_("✗"), Red_(msg))
}

func Info(msg string) {
	fmt.Printf("  %s %s\n", Teal_("→"), msg)
}

func Warn(msg string) {
	fmt.Printf("  %s %s\n", Yellow_("⚠"), msg)
}

func Header(title string) {
	fmt.Println()
	fmt.Printf("  %s\n", Bold_(Gold_(title)))
	fmt.Printf("  %s\n", Dim_(strings.Repeat("─", len(title)+2)))
}

func SectionLabel(label string) {
	fmt.Println()
	dots := strings.Repeat("·", 40-len(label))
	fmt.Printf("  %s %s\n", Dim_(label), Dim_(dots))
}

func KV(key, value string) {
	fmt.Printf("  %-22s %s\n", Dim_(key+":"), White_(value))
}

func KVColor(key, value, color string) {
	fmt.Printf("  %-22s %s\n", Dim_(key+":"), colorize(color, value))
}

func Separator() {
	fmt.Println(Dim_("  ─────────────────────────────────────────────────────"))
}

func PrintStep(n int, label string) {
	fmt.Printf("\n  %s %s\n",
		colorize(Gold, fmt.Sprintf("[%02d]", n)),
		Bold_(label),
	)
}

// Link displays a clickable URL in supported terminals
func Link(url, text string) {
	if isTerminal() {
		// Use terminal hyperlink escape sequence for modern terminals
		// Format: \033]8;;URL\033\TEXT\033]8;;\033\
		hyperlink := fmt.Sprintf("\033]8;;%s\033\\%s\033]8;;\033\\", url, text)
		fmt.Printf("  %s %s\n", Teal_("🔗"), hyperlink)
	} else {
		// Fallback for non-terminal environments or CI/CD
		fmt.Printf("  %s %s: %s\n", Teal_("🔗"), text, url)
	}
}

// ─────────────────────────────────────────────
// Table
// ─────────────────────────────────────────────

type Table struct {
	Headers []string
	Rows    [][]string
}

func NewTable(headers ...string) *Table {
	return &Table{Headers: headers}
}

func (t *Table) AddRow(cols ...string) {
	t.Rows = append(t.Rows, cols)
}

func (t *Table) Print() {
	// Compute column widths
	widths := make([]int, len(t.Headers))
	for i, h := range t.Headers {
		widths[i] = len(h)
	}
	for _, row := range t.Rows {
		for i, cell := range row {
			if i < len(widths) && len(cell) > widths[i] {
				widths[i] = len(cell)
			}
		}
	}

	// Header
	fmt.Print("  ")
	for i, h := range t.Headers {
		fmt.Printf("%-*s  ", widths[i], Dim_(h))
	}
	fmt.Println()

	// Separator
	fmt.Print("  ")
	for _, w := range widths {
		fmt.Print(Dim_(strings.Repeat("─", w+2)))
	}
	fmt.Println()

	// Rows
	for _, row := range t.Rows {
		fmt.Print("  ")
		for i, cell := range row {
			if i < len(widths) {
				fmt.Printf("%-*s  ", widths[i], cell)
			}
		}
		fmt.Println()
	}
	fmt.Println()
}

// ─────────────────────────────────────────────
// Prompt
// ─────────────────────────────────────────────

func Prompt(label string) string {
	fmt.Printf("  %s %s ", Teal_("?"), label)
	var input string
	fmt.Scanln(&input)
	return strings.TrimSpace(input)
}

func Confirm(label string) bool {
	fmt.Printf("  %s %s [y/N]: ", Teal_("?"), label)
	var input string
	fmt.Scanln(&input)
	return strings.ToLower(strings.TrimSpace(input)) == "y"
}

func SelectNetwork(defaultNetwork string) string {
	fmt.Printf("  %s Select network:\n", Teal_("?"))
	fmt.Printf("    1) %s\n", Green_("Testnet")+" (stellar-testnet)")
	fmt.Printf("    2) %s\n", Yellow_("Mainnet")+" (stellar-mainnet)")
	fmt.Printf("  %s Enter choice [1-2]: ", Teal_("?"))

	var choice string
	fmt.Scanln(&choice)

	choice = strings.TrimSpace(choice)
	switch choice {
	case "1", "testnet", "stellar-testnet":
		return "stellar-testnet"
	case "2", "mainnet", "stellar-mainnet":
		return "stellar-mainnet"
	default:
		if choice == "" {
			return defaultNetwork
		}
		fmt.Printf("  %s Invalid choice. Using default: %s\n", Yellow_("⚠"), defaultNetwork)
		return defaultNetwork
	}
}

func PromptSupply(defaultSupply string) string {
	for {
		fmt.Printf("  %s Total supply [%s]: ", Teal_("?"), defaultSupply)
		var input string
		fmt.Scanln(&input)

		input = strings.TrimSpace(input)
		if input == "" {
			return defaultSupply
		}

		// Validate that supply is a positive number
		if isValidSupply(input) {
			return input
		}

		fmt.Printf("  %s Invalid supply. Please enter a positive number.\n", Red_("✗"))
	}
}

func isValidSupply(supply string) bool {
	// Check if it's a valid positive number (integer or decimal)
	if supply == "" || supply == "0" {
		return false
	}

	// Simple validation - check if it's a valid number format
	// This is a basic check; in production you'd want more robust validation
	for _, char := range supply {
		if !((char >= '0' && char <= '9') || char == '.' || char == ',') {
			return false
		}
	}

	return true
}
