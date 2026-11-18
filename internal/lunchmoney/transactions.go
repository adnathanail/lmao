package lunchmoney

import (
	"context"
	"encoding/json"
	"net/url"
	"strconv"
)

// Transaction statuses.
const (
	StatusReviewed      = "reviewed"
	StatusUnreviewed    = "unreviewed"
	StatusDeletePending = "delete_pending"
)

// Transaction is a transaction object. Amount is a decimal string, positive
// for money going out (a debit) and negative for money coming in.
type Transaction struct {
	ID              int64           `json:"id"`
	Date            string          `json:"date"`
	Amount          string          `json:"amount"`
	Currency        string          `json:"currency"`
	ToBase          float64         `json:"to_base"`
	Payee           string          `json:"payee"`
	OriginalName    *string         `json:"original_name"`
	CategoryID      *int64          `json:"category_id"`
	Notes           *string         `json:"notes"`
	Status          string          `json:"status"`
	IsPending       bool            `json:"is_pending"`
	ManualAccountID *int64          `json:"manual_account_id"`
	PlaidAccountID  *int64          `json:"plaid_account_id"`
	RecurringID     *int64          `json:"recurring_id"`
	TagIDs          []int64         `json:"tag_ids"`
	ExternalID      *string         `json:"external_id"`
	Source          *string         `json:"source"`
	CreatedAt       string          `json:"created_at"`
	UpdatedAt       string          `json:"updated_at"`
	IsSplitParent   bool            `json:"is_split_parent"`
	SplitParentID   *int64          `json:"split_parent_id"`
	IsGroupParent   bool            `json:"is_group_parent"`
	GroupParentID   *int64          `json:"group_parent_id"`
	PlaidMetadata   json.RawMessage `json:"plaid_metadata,omitempty"`
	CustomMetadata  json.RawMessage `json:"custom_metadata,omitempty"`
}

// NotesText is the transaction's notes, or "" if it has none.
func (t Transaction) NotesText() string {
	if t.Notes == nil {
		return ""
	}
	return *t.Notes
}

// pageSize is the most transactions the API returns in one page.
const pageSize = 2000

// UnreviewedTransactions returns every transaction waiting to be reviewed,
// with metadata. Pending transactions, split parents and the transactions
// inside groups are left out, as the API does by default.
func (c *Client) UnreviewedTransactions(ctx context.Context) ([]Transaction, error) {
	var all []Transaction
	for offset := 0; ; offset += pageSize {
		q := url.Values{
			"status":           {StatusUnreviewed},
			"include_metadata": {"true"},
			"limit":            {strconv.Itoa(pageSize)},
			"offset":           {strconv.Itoa(offset)},
		}
		var page struct {
			Transactions []Transaction `json:"transactions"`
			HasMore      bool          `json:"has_more"`
		}
		if err := c.do(ctx, "GET", "/transactions", q, nil, &page); err != nil {
			return nil, err
		}
		all = append(all, page.Transactions...)
		if !page.HasMore || len(page.Transactions) == 0 {
			return all, nil
		}
	}
}

// Change is a set of changes to a transaction. Only the fields set are sent.
type Change struct {
	Status string // "" leaves the status as it is

	SetCategory bool
	CategoryID  *int64 // nil clears the category, when SetCategory

	Notes *string // "" clears the notes; nil leaves them as they are
}

// MarshalJSON sends only the fields being changed: the API returns 400 for
// any it doesn't know, and treats null as clearing a field.
func (c Change) MarshalJSON() ([]byte, error) {
	body := map[string]any{}
	if c.Status != "" {
		body["status"] = c.Status
	}
	if c.SetCategory {
		body["category_id"] = c.CategoryID
	}
	if c.Notes != nil {
		body["notes"] = *c.Notes
	}
	return json.Marshal(body)
}

// UpdateTransaction applies a change, and returns the updated transaction.
func (c *Client) UpdateTransaction(ctx context.Context, id int64, change Change) (Transaction, error) {
	var tx Transaction
	err := c.do(ctx, "PUT", "/transactions/"+strconv.FormatInt(id, 10), nil, change, &tx)
	return tx, err
}
