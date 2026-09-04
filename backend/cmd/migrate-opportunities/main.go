// Command migrate-opportunities imports currently-live CurrentRMS
// opportunities into Equiptra as real projects + booking history — only
// what's live right now (filtermode=live), not a historical archive.
//
// One opportunity -> one project. Each Principal/Accessory opportunity_item
// whose underlying CurrentRMS product matches products.legacy_id -> one
// booking_request (quantity_requested = the item's own requested quantity,
// not just the count of item_assets under it — CurrentRMS doesn't always
// generate an item_asset row for every unit requested). Each item_asset
// under it with a real stock_level_id (matched against assets.legacy_id)
// -> one booking_allocation. item_assets tagged "Group Booking" or
// "Non-Stock Booking" (CurrentRMS's own sentinels for "committed at the
// product level, no specific serial/non-inventory line" — confirmed via
// live inspection, not documented) get no allocation; the booking_request's
// quantity alone represents them, same as an unallocated request in our own
// system.
//
// Confirmed via live inspection before writing this (see chat — not
// re-derived from docs):
//   - The list endpoint's opportunities don't carry line items; detail must
//     be fetched per-opportunity with
//     ?include[]=opportunity_items&include[]=item_assets&include[]=member.
//     item_assets nests inside each opportunity_items entry (June 2026 API
//     change), not as a top-level array.
//   - opportunity_items are hierarchical (Group/Principal/Accessory via
//     opportunity_item_type_name); Group rows are folder headers with no
//     item_id and are skipped.
//   - The client is opportunity.member.name (via ?include[]=member) — NOT
//     opportunity.participants, which are internal LDMtv staff.
//   - The asset-matching key is item_asset.stock_level_id (NOT
//     item_asset.id), verified 1:1 against assets.legacy_id, along with
//     stock_level_asset_number matching assets.asset_number.
//
// Status mappings agreed with Ric before building (see chat) — spot-check a
// couple of known live projects against these once imported, since they're
// inferred from state/status names, not from CurrentRMS docs:
//
//	Opportunity (state_name, status_name) -> projects.status
//	  Quotation / Provisional -> tentative
//	  Order      / Open       -> confirmed
//	  Order      / Active     -> in_progress
//	  (anything else: skip the whole opportunity, logged loudly)
//
//	item_asset.status_name -> booking_allocations.status
//	  Provisional / Reserved / Allocated -> allocated
//	  Booked Out                         -> checked_out
//	  Checked In                         -> returned
//
// Idempotent: each imported project's notes field is stamped
// "Imported from CurrentRMS opportunity #<id>." and checked before
// creating, so re-running skips opportunities already imported rather than
// duplicating them. Use -dry-run to preview counts without writing, and
// -limit to cap how many opportunities are processed while testing.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"equiptra/internal/db"
	"equiptra/internal/models"
)

const baseURL = "https://api.current-rms.com/api/v1"

// --- CurrentRMS response shapes — only the fields this tool uses ---

type crmsListResponse struct {
	Opportunities []struct {
		ID int64 `json:"id"`
	} `json:"opportunities"`
	Meta struct {
		TotalRowCount int `json:"total_row_count"`
		RowCount      int `json:"row_count"`
		Page          int `json:"page"`
		PerPage       int `json:"per_page"`
	} `json:"meta"`
}

type crmsDetailResponse struct {
	Opportunity crmsOpportunity `json:"opportunity"`
}

type crmsMember struct {
	Name string `json:"name"`
}

type crmsOpportunity struct {
	ID               int64                 `json:"id"`
	Subject          string                `json:"subject"`
	Number           string                `json:"number"`
	Reference        string                `json:"reference"`
	StartsAt         string                `json:"starts_at"`
	EndsAt           string                `json:"ends_at"`
	StateName        string                `json:"state_name"`
	StatusName       string                `json:"status_name"`
	Member           *crmsMember           `json:"member"`
	OpportunityItems []crmsOpportunityItem `json:"opportunity_items"`
}

type crmsOpportunityItem struct {
	ID                      int64           `json:"id"`
	ItemID                  *int64          `json:"item_id"`
	ItemType                *string         `json:"item_type"`
	OpportunityItemTypeName string          `json:"opportunity_item_type_name"`
	Name                    string          `json:"name"`
	Quantity                string          `json:"quantity"`
	ItemAssets              []crmsItemAsset `json:"item_assets"`
}

type crmsItemAsset struct {
	StockLevelID          int64  `json:"stock_level_id"`
	StockLevelAssetNumber string `json:"stock_level_asset_number"`
	StatusName            string `json:"status_name"`
}

var crmsSentinelAssetNumbers = map[string]bool{
	"Group Booking":     true,
	"Non-Stock Booking": true,
}

// --- CurrentRMS client ---

type crmsClient struct {
	apiKey    string
	subdomain string
	http      *http.Client
}

func (c *crmsClient) do(path string, out interface{}) error {
	req, err := http.NewRequest(http.MethodGet, baseURL+path, nil)
	if err != nil {
		return err
	}
	req.Header.Set("X-AUTH-TOKEN", c.apiKey)
	req.Header.Set("X-SUBDOMAIN", c.subdomain)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("CurrentRMS returned %d: %s", resp.StatusCode, string(body))
	}
	return json.Unmarshal(body, out)
}

// liveOpportunityIDs paginates through GET /opportunities?filtermode=live —
// per_page defaulted to 12 in testing, so a live set larger than that needs
// real pagination, not a single fetch.
func (c *crmsClient) liveOpportunityIDs() ([]int64, error) {
	var ids []int64
	page := 1
	for {
		var resp crmsListResponse
		if err := c.do(fmt.Sprintf("/opportunities?filtermode=live&page=%d", page), &resp); err != nil {
			return nil, err
		}
		for _, o := range resp.Opportunities {
			ids = append(ids, o.ID)
		}
		if len(resp.Opportunities) == 0 || len(ids) >= resp.Meta.TotalRowCount {
			break
		}
		page++
	}
	return ids, nil
}

func (c *crmsClient) opportunityDetail(id int64) (*crmsOpportunity, error) {
	var resp crmsDetailResponse
	path := fmt.Sprintf("/opportunities/%d?include[]=opportunity_items&include[]=item_assets&include[]=member", id)
	if err := c.do(path, &resp); err != nil {
		return nil, err
	}
	return &resp.Opportunity, nil
}

// --- status mappings agreed before building — see package doc comment ---

func mapProjectStatus(stateName, statusName string) (models.ProjectStatus, bool) {
	switch {
	case stateName == "Quotation" && statusName == "Provisional":
		return models.ProjectStatusTentative, true
	case stateName == "Order" && statusName == "Open":
		return models.ProjectStatusConfirmed, true
	case stateName == "Order" && statusName == "Active":
		return models.ProjectStatusInProgress, true
	default:
		return "", false
	}
}

func mapAllocationStatus(statusName string) (models.BookingAllocationStatus, bool) {
	switch statusName {
	case "Provisional", "Reserved", "Allocated":
		return models.BookingAllocationStatusAllocated, true
	case "Booked Out":
		return models.BookingAllocationStatusCheckedOut, true
	case "Checked In":
		return models.BookingAllocationStatusReturned, true
	default:
		return "", false
	}
}

// recomputeRequestStatus mirrors handlers.recomputeBookingRequestStatus's
// logic (not exported, so duplicated here — see booking_requests.go) so an
// imported request's status reflects its allocations the same way the live
// app would compute it, rather than defaulting to the 'draft' column
// default.
func recomputeRequestStatus(quantityRequested int, allocationStatuses []models.BookingAllocationStatus) models.BookingRequestStatus {
	var activeCount, checkedOutCount, totalCount int
	for _, s := range allocationStatuses {
		totalCount++
		if s == models.BookingAllocationStatusAllocated || s == models.BookingAllocationStatusCheckedOut {
			activeCount++
		}
		if s == models.BookingAllocationStatusCheckedOut {
			checkedOutCount++
		}
	}
	switch {
	case totalCount > 0 && activeCount == 0:
		return models.BookingRequestStatusReturned
	case activeCount >= quantityRequested && checkedOutCount >= activeCount && activeCount > 0:
		return models.BookingRequestStatusOut
	case activeCount > 0:
		return models.BookingRequestStatusPartiallyAllocated
	default:
		return models.BookingRequestStatusReserved
	}
}

func parseCRMSTime(s string) (time.Time, error) {
	return time.Parse(time.RFC3339, s)
}

type stats struct {
	opportunitiesSeen                int
	projectsCreated                  int
	projectsSkippedExist             int
	projectsSkippedStatus            []string
	requestsCreated                  int
	itemsSkippedNoProduct            []string
	allocationsCreated               int
	allocationsSkippedGroupBooking   int
	allocationsSkippedUnmappedStatus []string
	allocationsSkippedNoAssetMatch   []string
}

func main() {
	dryRun := flag.Bool("dry-run", false, "preview counts without writing to the database")
	limit := flag.Int("limit", 0, "only process the first N live opportunities — 0 means no limit")
	flag.Parse()

	apiKey := os.Getenv("CURRENTRMS_API_KEY")
	subdomain := os.Getenv("CURRENTRMS_SUBDOMAIN")
	if apiKey == "" || subdomain == "" {
		log.Fatal("CURRENTRMS_API_KEY/CURRENTRMS_SUBDOMAIN not set")
	}
	crms := &crmsClient{apiKey: apiKey, subdomain: subdomain, http: &http.Client{Timeout: 20 * time.Second}}

	ctx := context.Background()
	pool, err := db.Connect(ctx)
	if err != nil {
		log.Fatalf("db connect: %v", err)
	}
	defer pool.Close()

	ids, err := crms.liveOpportunityIDs()
	if err != nil {
		log.Fatalf("listing live opportunities: %v", err)
	}
	log.Printf("found %d live opportunities", len(ids))
	if *limit > 0 && len(ids) > *limit {
		ids = ids[:*limit]
	}

	s := &stats{}
	for _, id := range ids {
		s.opportunitiesSeen++
		opp, err := crms.opportunityDetail(id)
		if err != nil {
			log.Printf("opportunity #%d: fetch failed, skipping: %v", id, err)
			continue
		}
		importOpportunity(ctx, pool, opp, *dryRun, s)
	}

	log.Printf("")
	log.Printf("=== SUMMARY ===")
	log.Printf("opportunities seen:            %d", s.opportunitiesSeen)
	log.Printf("projects created:              %d", s.projectsCreated)
	log.Printf("projects skipped (already imported): %d", s.projectsSkippedExist)
	log.Printf("projects skipped (unmapped status):  %d", len(s.projectsSkippedStatus))
	for _, m := range s.projectsSkippedStatus {
		log.Printf("    %s", m)
	}
	log.Printf("booking_requests created:      %d", s.requestsCreated)
	log.Printf("opportunity_items skipped (no matching product): %d", len(s.itemsSkippedNoProduct))
	for _, m := range s.itemsSkippedNoProduct {
		log.Printf("    %s", m)
	}
	log.Printf("booking_allocations created:   %d", s.allocationsCreated)
	log.Printf("item_assets skipped (Group Booking / Non-Stock Booking, no specific serial): %d", s.allocationsSkippedGroupBooking)
	log.Printf("item_assets skipped (unmapped item_asset status): %d", len(s.allocationsSkippedUnmappedStatus))
	for _, m := range s.allocationsSkippedUnmappedStatus {
		log.Printf("    %s", m)
	}
	log.Printf("item_assets skipped (real asset tag, but no matching assets.legacy_id — FLAG): %d", len(s.allocationsSkippedNoAssetMatch))
	for _, m := range s.allocationsSkippedNoAssetMatch {
		log.Printf("    %s", m)
	}
	if *dryRun {
		log.Printf("")
		log.Printf("DRY RUN — nothing was written")
	}
}

func importOpportunity(ctx context.Context, pool *pgxpool.Pool, opp *crmsOpportunity, dryRun bool, s *stats) {
	marker := fmt.Sprintf("Imported from CurrentRMS opportunity #%d.", opp.ID)

	var existingID int64
	err := pool.QueryRow(ctx, `SELECT id FROM projects WHERE notes = $1`, marker).Scan(&existingID)
	if err == nil {
		log.Printf("opportunity #%d (%q): already imported as project %d, skipping", opp.ID, opp.Subject, existingID)
		s.projectsSkippedExist++
		return
	}

	status, ok := mapProjectStatus(opp.StateName, opp.StatusName)
	if !ok {
		msg := fmt.Sprintf("opportunity #%d (%q): unmapped state/status %q/%q", opp.ID, opp.Subject, opp.StateName, opp.StatusName)
		log.Printf("%s — SKIPPED", msg)
		s.projectsSkippedStatus = append(s.projectsSkippedStatus, msg)
		return
	}

	startsAt, err := parseCRMSTime(opp.StartsAt)
	if err != nil {
		log.Printf("opportunity #%d (%q): bad starts_at %q, skipping: %v", opp.ID, opp.Subject, opp.StartsAt, err)
		return
	}
	endsAt, err := parseCRMSTime(opp.EndsAt)
	if err != nil {
		log.Printf("opportunity #%d (%q): bad ends_at %q, skipping: %v", opp.ID, opp.Subject, opp.EndsAt, err)
		return
	}
	if endsAt.Before(startsAt) {
		log.Printf("opportunity #%d (%q): ends_at before starts_at, skipping", opp.ID, opp.Subject)
		return
	}

	var client *string
	if opp.Member != nil && opp.Member.Name != "" {
		client = &opp.Member.Name
	}
	var reference *string
	if opp.Reference != "" {
		reference = &opp.Reference
	}
	var number *string
	if opp.Number != "" {
		number = &opp.Number
	}

	log.Printf("opportunity #%d (%q): state=%s status=%s -> project status=%s, %d opportunity_items",
		opp.ID, opp.Subject, opp.StateName, opp.StatusName, status, len(opp.OpportunityItems))

	if dryRun {
		s.projectsCreated++
		planOpportunityItems(ctx, pool, opp, 0, startsAt, endsAt, true, s)
		return
	}

	var projectID int64
	err = pool.QueryRow(ctx, `
		INSERT INTO projects (name, client, start_date, end_date, status, client_reference, order_number, notes)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING id`,
		opp.Subject, client, startsAt, endsAt, status, reference, number, marker,
	).Scan(&projectID)
	if err != nil {
		log.Printf("opportunity #%d (%q): project insert failed: %v", opp.ID, opp.Subject, err)
		return
	}
	s.projectsCreated++
	planOpportunityItems(ctx, pool, opp, projectID, startsAt, endsAt, false, s)
}

// planOpportunityItems handles both the real write path and the dry-run
// preview path (writeToDB=false skips every INSERT but still walks the same
// logic so dry-run counts match what a real run would do).
func planOpportunityItems(ctx context.Context, pool *pgxpool.Pool, opp *crmsOpportunity, projectID int64, startsAt, endsAt time.Time, dryRun bool, s *stats) {
	for _, item := range opp.OpportunityItems {
		if item.OpportunityItemTypeName == "Group" || item.ItemID == nil {
			continue // folder/header row, no product of its own
		}
		quantity, err := strconv.ParseFloat(item.Quantity, 64)
		if err != nil || quantity <= 0 {
			continue
		}

		var productID int64
		err = pool.QueryRow(ctx, `SELECT id FROM products WHERE legacy_id = $1`, *item.ItemID).Scan(&productID)
		if err != nil {
			msg := fmt.Sprintf("opportunity #%d: item %q (CurrentRMS product id %d, type %s) has no matching products.legacy_id",
				opp.ID, item.Name, *item.ItemID, stringOrEmpty(item.ItemType))
			s.itemsSkippedNoProduct = append(s.itemsSkippedNoProduct, msg)
			continue
		}

		var requestID int64
		if !dryRun {
			err = pool.QueryRow(ctx, `
				INSERT INTO booking_requests (project_id, product_id, quantity_requested, date_out, date_in, status)
				VALUES ($1, $2, $3, $4, $5, $6)
				RETURNING id`,
				projectID, productID, int(quantity), startsAt, endsAt, models.BookingRequestStatusReserved,
			).Scan(&requestID)
			if err != nil {
				log.Printf("opportunity #%d: booking_request insert failed for %q: %v", opp.ID, item.Name, err)
				continue
			}
		}
		s.requestsCreated++

		var allocationStatuses []models.BookingAllocationStatus
		for _, ia := range item.ItemAssets {
			if crmsSentinelAssetNumbers[ia.StockLevelAssetNumber] {
				s.allocationsSkippedGroupBooking++
				continue
			}
			allocStatus, ok := mapAllocationStatus(ia.StatusName)
			if !ok {
				msg := fmt.Sprintf("opportunity #%d: item %q asset %s has unmapped item_asset status %q",
					opp.ID, item.Name, ia.StockLevelAssetNumber, ia.StatusName)
				s.allocationsSkippedUnmappedStatus = append(s.allocationsSkippedUnmappedStatus, msg)
				continue
			}

			var assetID int64
			err := pool.QueryRow(ctx, `SELECT id FROM assets WHERE legacy_id = $1 AND product_id = $2`, ia.StockLevelID, productID).Scan(&assetID)
			if err != nil {
				msg := fmt.Sprintf("opportunity #%d: item %q asset_number=%s (CurrentRMS stock_level_id=%d) has no matching assets.legacy_id under product %d — FLAGGED, not silently skipped",
					opp.ID, item.Name, ia.StockLevelAssetNumber, ia.StockLevelID, productID)
				s.allocationsSkippedNoAssetMatch = append(s.allocationsSkippedNoAssetMatch, msg)
				continue
			}

			allocationStatuses = append(allocationStatuses, allocStatus)
			if dryRun {
				s.allocationsCreated++
				continue
			}

			var checkedOutAt, checkedInAt *time.Time
			var inspectionPassed *bool
			trueVal := true
			if allocStatus == models.BookingAllocationStatusCheckedOut || allocStatus == models.BookingAllocationStatusReturned {
				checkedOutAt = &startsAt
				inspectionPassed = &trueVal // chk_checkout_requires_inspection
			}
			if allocStatus == models.BookingAllocationStatusReturned {
				checkedInAt = &endsAt
			}
			_, err = pool.Exec(ctx, `
				INSERT INTO booking_allocations (booking_request_id, asset_id, status, checked_out_at, inspection_passed, checked_in_at)
				VALUES ($1, $2, $3, $4, $5, $6)`,
				requestID, assetID, allocStatus, checkedOutAt, inspectionPassed, checkedInAt,
			)
			if err != nil {
				log.Printf("opportunity #%d: booking_allocation insert failed for asset %d: %v", opp.ID, assetID, err)
				continue
			}
			s.allocationsCreated++
		}

		if !dryRun && len(allocationStatuses) > 0 {
			newStatus := recomputeRequestStatus(int(quantity), allocationStatuses)
			if _, err := pool.Exec(ctx, `UPDATE booking_requests SET status = $1 WHERE id = $2`, newStatus, requestID); err != nil {
				log.Printf("opportunity #%d: booking_request status update failed: %v", opp.ID, err)
			}
		}
	}
}

func stringOrEmpty(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
