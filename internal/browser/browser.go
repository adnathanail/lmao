// Package browser opens URLs in the user's web browser.
package browser

import (
	"fmt"
	"net/url"
	"os/exec"
	"runtime"
	"strconv"
)

// TransactionURL is the Lunch Money web app's page for a transaction.
func TransactionURL(id int64) string {
	q := url.Values{"transaction_id": {strconv.FormatInt(id, 10)}}
	return "https://my.lunchmoney.app/transactions?" + q.Encode()
}

// Open opens u in the default browser, without waiting for it.
func Open(u string) error {
	name := "xdg-open"
	if runtime.GOOS == "darwin" {
		name = "open"
	}
	cmd := exec.Command(name, u)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("opening the browser with %s: %w", name, err)
	}
	// Reap it in the background, so it doesn't linger as a zombie.
	go func() { _ = cmd.Wait() }()
	return nil
}
