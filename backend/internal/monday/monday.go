// Package monday wraps Monday.com's GraphQL API for the manual
// "fetch from Monday" project-lookup action — see
// docs/equiptra-monday-lookup-addendum.md.
//
// Column shapes below were confirmed against real items on board 739667801
// ("Master Sheet") before writing this, not assumed from generic docs:
//
//   - tags9 ("Client") is actually a `dropdown` column, not a `Tags` column
//     as the addendum's config table labels it — same field mapping, just a
//     factual correction. Its `value` is {"ids":[...],"changed_at":...} and
//     its `text` is Monday's own pre-joined display label. Multi-select
//     genuinely happens in production data (confirmed one real item with
//     two client tags, e.g. "Man City Events, City TV") — the addendum's
//     single-tag assumption does NOT always hold. Resolved by using `.text`
//     directly rather than parsing `.value`, since Monday already joins
//     multiple tags into one readable string.
//   - date5 ("Date") is a `timeline` column: value is
//     {"from":"YYYY-MM-DD","to":"YYYY-MM-DD","changed_at":...}. Maps
//     cleanly to start_date/end_date.
//   - location ("Location") is a `location` column with a rich structured
//     value (lat/lng/city/street/country/placeId) — but its `.text` field
//     is already a clean formatted address string, so that's used directly
//     for delivery_address rather than reconstructing one from the parts.
//
// Also worth noting: order numbers on this board are NOT the addendum's
// `###-###` example format — real items are named things like "5376-06" and
// "5377-03" (variable digit counts each side of the dash). No fixed-width
// format is validated here; the order number is passed through as typed and
// matched via Monday's own exact-match item-name filter.
//
// Ambiguous matches are a real, current scenario on this board (not just
// defensive coding for a hypothetical) — confirmed multiple items sharing
// the same name today (e.g. "5377-03" appears 3 times).
package monday

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"
)

const apiURL = "https://api.monday.com/v2"

// ColumnMap configures which Monday column IDs back each Equiptra project
// field. order_number isn't included here — it's matched against the item's
// built-in name, queried as item.name rather than a column_values entry.
type ColumnMap struct {
	Name            string `json:"name"`             // "Fixture" text column -> projects.name
	Client          string `json:"client"`           // dropdown column -> projects.client
	DateRange       string `json:"date_range"`       // timeline column -> start_date/end_date
	ClientReference string `json:"client_reference"` // "PO Number" text column
	DeliveryAddress string `json:"delivery_address"` // location column
}

// defaultColumnMap matches board 739667801 as verified live — see the
// package doc comment. MONDAY_PROJECTS_COLUMN_MAP overrides this if the
// board's columns ever change without needing a code deploy.
var defaultColumnMap = ColumnMap{
	Name:            "text",
	Client:          "tags9",
	DateRange:       "date5",
	ClientReference: "text8",
	DeliveryAddress: "location",
}

const defaultBoardID = "739667801"

type Client struct {
	token      string
	boardID    string
	columns    ColumnMap
	httpClient *http.Client
}

// NewClient builds a client from environment variables. Returns nil if
// MONDAY_API_TOKEN is unset — the lookup feature is simply disabled rather
// than treated as a startup error, same convention as
// storage.NewSupabaseClient.
//
//	MONDAY_API_TOKEN             server-side only — never sent to the frontend, never logged
//	MONDAY_PROJECTS_BOARD_ID     defaults to 739667801 ("Master Sheet") if unset
//	MONDAY_PROJECTS_COLUMN_MAP   JSON object overriding defaultColumnMap fields; defaults used if unset or malformed
func NewClient() *Client {
	token := os.Getenv("MONDAY_API_TOKEN")
	if token == "" {
		return nil
	}
	boardID := os.Getenv("MONDAY_PROJECTS_BOARD_ID")
	if boardID == "" {
		boardID = defaultBoardID
	}
	columns := defaultColumnMap
	if raw := os.Getenv("MONDAY_PROJECTS_COLUMN_MAP"); raw != "" {
		var override ColumnMap
		if err := json.Unmarshal([]byte(raw), &override); err != nil {
			log.Printf("MONDAY_PROJECTS_COLUMN_MAP is malformed, using built-in defaults: %v", err)
		} else {
			columns = override
		}
	}
	return &Client{
		token:      token,
		boardID:    boardID,
		columns:    columns,
		httpClient: &http.Client{Timeout: 10 * time.Second},
	}
}

// ProjectLookup is the mapped result of a successful lookup — no database
// write happens here, this only returns data for the frontend to pre-fill a
// form with.
type ProjectLookup struct {
	Name            string  `json:"name"`
	Client          string  `json:"client,omitempty"`
	StartDate       *string `json:"start_date,omitempty"`
	EndDate         *string `json:"end_date,omitempty"`
	ClientReference string  `json:"client_reference,omitempty"`
	DeliveryAddress string  `json:"delivery_address,omitempty"`
}

// ErrNotFound and ErrAmbiguous are returned as-is (via errors.Is) so the
// handler can tell "no match" (404, never blocks manual entry) apart from
// "ambiguous match" (409, needs the user to resolve in Monday) and from a
// genuine API failure (502).
var (
	ErrNotFound  = errors.New("no matching item on the Monday board")
	ErrAmbiguous = errors.New("multiple items on the Monday board match this order number")
)

type graphQLRequest struct {
	Query     string         `json:"query"`
	Variables map[string]any `json:"variables,omitempty"`
}

type graphQLError struct {
	Message string `json:"message"`
}

type columnValue struct {
	ID    string  `json:"id"`
	Text  string  `json:"text"`
	Value *string `json:"value"`
}

type mondayItem struct {
	ID           string        `json:"id"`
	Name         string        `json:"name"`
	ColumnValues []columnValue `json:"column_values"`
}

type graphQLResponse struct {
	Data struct {
		Boards []struct {
			ItemsPage struct {
				Items []mondayItem `json:"items"`
			} `json:"items_page"`
		} `json:"boards"`
	} `json:"data"`
	Errors []graphQLError `json:"errors"`
}

type timelineValue struct {
	From string `json:"from"`
	To   string `json:"to"`
}

// LookupByOrderNumber matches the item whose built-in name exactly equals
// orderNumber (Monday's own any_of filter on the name column does an exact
// match, confirmed live — a partial order number returns zero results, not
// a fuzzy match).
func (c *Client) LookupByOrderNumber(ctx context.Context, orderNumber string) (*ProjectLookup, error) {
	columnIDs := []string{c.columns.Name, c.columns.Client, c.columns.DateRange, c.columns.ClientReference, c.columns.DeliveryAddress}
	quotedIDs := make([]string, len(columnIDs))
	for i, id := range columnIDs {
		quotedIDs[i] = fmt.Sprintf("%q", id)
	}

	// orderNumber is the only part of this query built from user input, so
	// it's passed as a GraphQL variable rather than interpolated into the
	// query text — board ID and column IDs are fixed server config.
	query := fmt.Sprintf(`query($orderNumber: CompareValue!) {
		boards(ids: [%s]) {
			items_page(limit: 5, query_params: {rules: [{column_id: "name", compare_value: $orderNumber, operator: any_of}]}) {
				items {
					id
					name
					column_values(ids: [%s]) {
						id
						text
						value
					}
				}
			}
		}
	}`, c.boardID, strings.Join(quotedIDs, ", "))

	body, err := json.Marshal(graphQLRequest{
		Query:     query,
		Variables: map[string]any{"orderNumber": []string{orderNumber}},
	})
	if err != nil {
		return nil, fmt.Errorf("building request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("building request: %w", err)
	}
	req.Header.Set("Authorization", c.token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("calling Monday API: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading Monday response: %w", err)
	}

	var parsed graphQLResponse
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return nil, fmt.Errorf("parsing Monday response (status %d): %w", resp.StatusCode, err)
	}
	if len(parsed.Errors) > 0 {
		return nil, fmt.Errorf("Monday API error: %s", parsed.Errors[0].Message)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("Monday API returned status %d", resp.StatusCode)
	}
	if len(parsed.Data.Boards) == 0 {
		return nil, fmt.Errorf("board %s not found or not accessible with this token", c.boardID)
	}

	items := parsed.Data.Boards[0].ItemsPage.Items
	if len(items) == 0 {
		return nil, ErrNotFound
	}
	if len(items) > 1 {
		return nil, ErrAmbiguous
	}

	return c.mapItem(items[0])
}

func (c *Client) mapItem(item mondayItem) (*ProjectLookup, error) {
	byID := make(map[string]columnValue, len(item.ColumnValues))
	for _, cv := range item.ColumnValues {
		byID[cv.ID] = cv
	}

	result := &ProjectLookup{
		Name:            byID[c.columns.Name].Text,
		Client:          byID[c.columns.Client].Text,
		ClientReference: byID[c.columns.ClientReference].Text,
		DeliveryAddress: byID[c.columns.DeliveryAddress].Text,
	}
	if result.Name == "" {
		// The "Fixture" text column is empty on some items — fall back to
		// the item's own name (the order number) rather than handing the
		// frontend a blank project name.
		result.Name = item.Name
	}

	if dateCV, ok := byID[c.columns.DateRange]; ok && dateCV.Value != nil {
		var tl timelineValue
		if err := json.Unmarshal([]byte(*dateCV.Value), &tl); err != nil {
			return nil, fmt.Errorf("parsing date column: %w", err)
		}
		if tl.From != "" {
			result.StartDate = &tl.From
		}
		if tl.To != "" {
			result.EndDate = &tl.To
		}
	}

	return result, nil
}
