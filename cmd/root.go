// Package cmd is lmao's command line and full-screen view.
package cmd

import (
	"errors"
	"os"
	"strconv"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/term"
	"github.com/spf13/cobra"

	"github.com/adnathanail/lmao/internal/auth"
	"github.com/adnathanail/lmao/internal/lunchmoney"
)

var rootCmd = &cobra.Command{
	Use:   "lmao",
	Short: "Review your unreviewed Lunch Money transactions",
	Long: `lmao opens a full-screen list of your unreviewed Lunch Money transactions,
one month at a time, starting with the current one, with the selected
transaction's details beside it.

Keys:
  ↑/↓ j/k     select a transaction (PgUp/PgDn, g/G jump)
  r           mark it reviewed (it needs a category)
  c           set its category (and mark it reviewed)
  n           edit its notes
  u           undo the review (and category) made in this session
  o           open it in the Lunch Money web app
  ←/→ h/l     show the previous or next month
  s           flip the order (newest or oldest first), selecting the first
  J/K         scroll the details
  R           reload from Lunch Money
  q           quit

Environment:
  ` + auth.TokenEnv + `    an access token, from ` + auth.TokenURL + `
  ` + lunchmoney.BaseURLEnv + `  the API to use (default ` + lunchmoney.DefaultBaseURL + `)`,
	Version:       version,
	Args:          cobra.NoArgs,
	SilenceUsage:  true,
	SilenceErrors: true,
	RunE:          runBrowse,
}

// Execute runs lmao and returns the code to exit with.
func Execute() int {
	if err := rootCmd.Execute(); err != nil {
		lipgloss.Fprintln(os.Stderr, styleError.Render("error:"), err)
		return 1
	}
	return 0
}

func runBrowse(*cobra.Command, []string) error {
	tokens := auth.EnvSource{}
	if err := tokens.Check(); err != nil {
		return err
	}
	if !term.IsTerminal(os.Stdin.Fd()) || !term.IsTerminal(os.Stdout.Fd()) {
		return errors.New("lmao needs a terminal")
	}
	client := lunchmoney.New(os.Getenv(lunchmoney.BaseURLEnv), tokens)
	p := tea.NewProgram(newModel(client))
	client.OnRateLimit = func(wait time.Duration) { p.Send(rateLimitMsg{wait}) }
	_, err := p.Run()
	return err
}

func itoa(n int) string { return strconv.Itoa(n) }
