package lunchmoney

import "context"

// Category is a category or a category group.
type Category struct {
	ID          int64      `json:"id"`
	Name        string     `json:"name"`
	Description *string    `json:"description"`
	IsIncome    bool       `json:"is_income"`
	IsGroup     bool       `json:"is_group"`
	GroupID     *int64     `json:"group_id"`
	Archived    bool       `json:"archived"`
	Children    []Category `json:"children"`
}

// Categories returns the categories in the order the Lunch Money app shows
// them: each group followed by its categories, and categories outside a
// group on their own. Groups have IsGroup set and no Children (their
// categories follow them instead).
func (c *Client) Categories(ctx context.Context) ([]Category, error) {
	var resp struct {
		Categories []Category `json:"categories"`
	}
	if err := c.do(ctx, "GET", "/categories", nil, nil, &resp); err != nil {
		return nil, err
	}
	var out []Category
	for _, cat := range resp.Categories {
		children := cat.Children
		cat.Children = nil
		out = append(out, cat)
		out = append(out, children...)
	}
	return out, nil
}
