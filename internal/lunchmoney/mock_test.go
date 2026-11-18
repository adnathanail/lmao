package lunchmoney

import (
	"context"
	"os"
	"testing"
)

// TestMockServer runs the client against Lunch Money's mock server
// (https://mock.lunchmoney.dev/v2), which accepts any token of 11 or more
// characters. It needs the network, so it only runs with LMAO_TEST_MOCK=1.
func TestMockServer(t *testing.T) {
	if os.Getenv("LMAO_TEST_MOCK") == "" {
		t.Skip("set LMAO_TEST_MOCK=1 to run against mock.lunchmoney.dev")
	}
	c := New("https://mock.lunchmoney.dev/v2", staticToken("lmao-mock-token"))
	ctx := context.Background()

	txs, err := c.UnreviewedTransactions(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("%d unreviewed transactions", len(txs))
	for _, tx := range txs {
		if tx.ID == 0 || tx.Date == "" || tx.Amount == "" {
			t.Errorf("incomplete transaction: %+v", tx)
		}
	}

	cats, err := c.Categories(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("%d categories", len(cats))
	manual, err := c.ManualAccounts(ctx)
	if err != nil {
		t.Fatal(err)
	}
	plaid, err := c.PlaidAccounts(ctx)
	if err != nil {
		t.Fatal(err)
	}
	tags, err := c.Tags(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("%d manual accounts, %d Plaid accounts, %d tags", len(manual), len(plaid), len(tags))

	if len(txs) == 0 || len(cats) == 0 {
		return
	}
	var catID *int64
	for _, c := range cats {
		if !c.IsGroup {
			catID = &c.ID
			break
		}
	}
	notes := "from lmao"
	for _, change := range []Change{
		{Status: StatusReviewed},
		{Status: StatusReviewed, SetCategory: true, CategoryID: catID},
		{Status: StatusUnreviewed, SetCategory: true},
		{Notes: &notes},
	} {
		tx, err := c.UpdateTransaction(ctx, txs[0].ID, change)
		if err != nil {
			t.Errorf("%+v: %v", change, err)
			continue
		}
		if tx.ID != txs[0].ID {
			t.Errorf("%+v: returned transaction %d", change, tx.ID)
		}
	}
}
