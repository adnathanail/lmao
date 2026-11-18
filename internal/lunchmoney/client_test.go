package lunchmoney

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

type staticToken string

func (t staticToken) Token(context.Context) (string, error) { return string(t), nil }

func testClient(t *testing.T, h http.HandlerFunc) *Client {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	c := New(srv.URL, staticToken("test-token"))
	c.sleep = func(context.Context, time.Duration) error { return nil }
	return c
}

func TestUnreviewedTransactionsPages(t *testing.T) {
	var calls int
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		if got := r.Header.Get("Authorization"); got != "Bearer test-token" {
			t.Errorf("Authorization = %q", got)
		}
		if r.URL.Path != "/transactions" {
			t.Errorf("path = %q", r.URL.Path)
		}
		q := r.URL.Query()
		if q.Get("status") != "unreviewed" || q.Get("include_metadata") != "true" {
			t.Errorf("query = %v", q)
		}
		offset, _ := strconv.Atoi(q.Get("offset"))
		id := offset/pageSize + 1
		fmt.Fprintf(w, `{"transactions":[{"id":%d,"amount":"1.0000","status":"unreviewed","category_id":null}],"has_more":%v}`, id, id < 3)
	})
	txs, err := c.UnreviewedTransactions(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if calls != 3 || len(txs) != 3 || txs[2].ID != 3 {
		t.Fatalf("calls = %d, txs = %+v", calls, txs)
	}
	if txs[0].CategoryID != nil {
		t.Errorf("CategoryID = %v, want nil", *txs[0].CategoryID)
	}
}

func TestUpdateTransactionBody(t *testing.T) {
	id := int64(42)
	notes := ""
	tests := []struct {
		name   string
		change Change
		want   string
	}{
		{"review", Change{Status: StatusReviewed}, `{"status":"reviewed"}`},
		{"category", Change{Status: StatusReviewed, SetCategory: true, CategoryID: &id}, `{"category_id":42,"status":"reviewed"}`},
		{"clear category", Change{Status: StatusUnreviewed, SetCategory: true}, `{"category_id":null,"status":"unreviewed"}`},
		{"clear notes", Change{Notes: &notes}, `{"notes":""}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
				if r.Method != "PUT" || r.URL.Path != "/transactions/7" {
					t.Errorf("%s %s", r.Method, r.URL.Path)
				}
				body, _ := io.ReadAll(r.Body)
				if string(body) != tt.want {
					t.Errorf("body = %s, want %s", body, tt.want)
				}
				w.WriteHeader(http.StatusCreated)
				fmt.Fprint(w, `{"id":7,"status":"reviewed","notes":"hi"}`)
			})
			tx, err := c.UpdateTransaction(context.Background(), 7, tt.change)
			if err != nil {
				t.Fatal(err)
			}
			if tx.ID != 7 || tx.Status != StatusReviewed || tx.NotesText() != "hi" {
				t.Errorf("tx = %+v", tx)
			}
		})
	}
}

func TestErrorResponse(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprint(w, `{"message":"Request Validation Failure","errors":[{"errMsg":"category_id 9 does not exist"}]}`)
	})
	_, err := c.UpdateTransaction(context.Background(), 1, Change{Status: StatusReviewed})
	if err == nil || !strings.Contains(err.Error(), "category_id 9 does not exist") || !strings.Contains(err.Error(), "400") {
		t.Fatalf("err = %v", err)
	}
	if IsUnauthorized(err) {
		t.Error("IsUnauthorized for a 400")
	}
}

func TestRateLimitRetriesOnce(t *testing.T) {
	var calls atomic.Int32
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) == 1 {
			w.Header().Set("Retry-After", "7")
			w.WriteHeader(http.StatusTooManyRequests)
			fmt.Fprint(w, `{"message":"Too Many Requests","errors":[{"errMsg":"Too many requests, please try again later."}]}`)
			return
		}
		fmt.Fprint(w, `{"tags":[{"id":1,"name":"work"}]}`)
	})
	var waited time.Duration
	c.OnRateLimit = func(d time.Duration) { waited = d }
	tags, err := c.Tags(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(tags) != 1 || waited != 7*time.Second || calls.Load() != 2 {
		t.Fatalf("tags = %v, waited = %v, calls = %d", tags, waited, calls.Load())
	}
}

func TestRateLimitGivesUp(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	})
	_, err := c.Tags(context.Background())
	if err == nil || !strings.Contains(err.Error(), "429") {
		t.Fatalf("err = %v", err)
	}
}

func TestCategoriesFlattensGroups(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"categories": []any{
			map[string]any{"id": 1, "name": "Food", "is_group": true, "children": []any{
				map[string]any{"id": 2, "name": "Groceries", "group_id": 1},
				map[string]any{"id": 3, "name": "Eating out", "group_id": 1},
			}},
			map[string]any{"id": 4, "name": "Rent", "children": []any{}},
		}})
	})
	cats, err := c.Categories(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, cat := range cats {
		names = append(names, cat.Name)
		if cat.Children != nil {
			t.Errorf("%s has children", cat.Name)
		}
	}
	if got := strings.Join(names, ","); got != "Food,Groceries,Eating out,Rent" {
		t.Fatalf("names = %s", got)
	}
}

func TestRetryAfter(t *testing.T) {
	for h, want := range map[string]time.Duration{
		"":     time.Minute,
		"0":    time.Second,
		"30":   30 * time.Second,
		"9999": maxRetryWait,
		"junk": time.Minute,
	} {
		if got := retryAfter(h); got != want {
			t.Errorf("retryAfter(%q) = %v, want %v", h, got, want)
		}
	}
}
