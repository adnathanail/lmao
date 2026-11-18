package cmd

import (
	"math/big"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
)

// Basic ANSI colours, so the view follows the terminal's own theme.
var (
	styleReviewed = lipgloss.NewStyle().Foreground(lipgloss.Color("2"))
	styleError    = lipgloss.NewStyle().Foreground(lipgloss.Color("1"))
	styleWarn     = lipgloss.NewStyle().Foreground(lipgloss.Color("3"))
	styleIncome   = lipgloss.NewStyle().Foreground(lipgloss.Color("6"))
	styleSpend    = lipgloss.NewStyle().Foreground(lipgloss.Color("5"))
	styleDim      = lipgloss.NewStyle().Faint(true)
	styleHeading  = lipgloss.NewStyle().Bold(true)
	styleSelected = lipgloss.NewStyle().Reverse(true)
)

var spinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

var currencySymbols = map[string]string{
	"gbp": "£", "usd": "$", "eur": "€", "jpy": "¥", "inr": "₹",
	"cad": "CA$", "aud": "A$", "nzd": "NZ$", "chf": "CHF ",
}

// formatAmount formats a transaction amount (a decimal string, positive for
// money going out) as money, without a sign: credit says whether it's money
// coming in. Amounts that aren't numbers are returned as they are.
func formatAmount(amount, currency string) (text string, credit bool) {
	r, ok := new(big.Rat).SetString(strings.TrimSpace(amount))
	if !ok {
		return amount, false
	}
	credit = r.Sign() < 0
	digits := r.Abs(r).FloatString(2)
	whole, frac, _ := strings.Cut(digits, ".")
	var b strings.Builder
	for i, d := range whole {
		if i > 0 && (len(whole)-i)%3 == 0 {
			b.WriteByte(',')
		}
		b.WriteRune(d)
	}
	text = b.String() + "." + frac
	if sym, ok := currencySymbols[strings.ToLower(currency)]; ok {
		return sym + text, credit
	}
	if currency != "" {
		text += " " + strings.ToUpper(currency)
	}
	return text, credit
}

// listAmount is an amount as the list shows it: money coming in has a "+".
func listAmount(amount, currency string) (string, bool) {
	text, credit := formatAmount(amount, currency)
	if credit {
		text = "+" + text
	}
	return text, credit
}

// formatTime shows an API timestamp in local time, or as it is if it doesn't
// parse.
func formatTime(s string) string {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return s
	}
	return t.Local().Format("2006-01-02 15:04")
}

func plural(n int, one, many string) string {
	if n == 1 {
		return "1 " + one
	}
	return itoa(n) + " " + many
}
