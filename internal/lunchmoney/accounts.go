package lunchmoney

import "context"

// Account is the part of a manual or Plaid account lmao shows.
type Account struct {
	ID              int64   `json:"id"`
	Name            string  `json:"name"`
	DisplayName     *string `json:"display_name"`
	InstitutionName *string `json:"institution_name"`
	Mask            *string `json:"mask"` // Plaid accounts only
}

// Label is the account's name as the Lunch Money app shows it.
func (a Account) Label() string {
	if a.DisplayName != nil && *a.DisplayName != "" {
		return *a.DisplayName
	}
	return a.Name
}

// ManualAccounts returns the accounts the user maintains by hand.
func (c *Client) ManualAccounts(ctx context.Context) ([]Account, error) {
	var resp struct {
		Accounts []Account `json:"manual_accounts"`
	}
	err := c.do(ctx, "GET", "/manual_accounts", nil, nil, &resp)
	return resp.Accounts, err
}

// PlaidAccounts returns the accounts synced through Plaid.
func (c *Client) PlaidAccounts(ctx context.Context) ([]Account, error) {
	var resp struct {
		Accounts []Account `json:"plaid_accounts"`
	}
	err := c.do(ctx, "GET", "/plaid_accounts", nil, nil, &resp)
	return resp.Accounts, err
}

// Tag is a transaction tag.
type Tag struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

// Tags returns the user's tags.
func (c *Client) Tags(ctx context.Context) ([]Tag, error) {
	var resp struct {
		Tags []Tag `json:"tags"`
	}
	err := c.do(ctx, "GET", "/tags", nil, nil, &resp)
	return resp.Tags, err
}
