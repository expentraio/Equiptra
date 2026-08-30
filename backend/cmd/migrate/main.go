// Command migrate is a one-off importer for the CurrentRMS CSV exports
// described in the build brief §5: products first, then assets joined to
// products by exact name match. Safe to re-run — upserts on legacy_id, so a
// re-export (e.g. after the country-of-origin backfill) can be re-imported.
package main

import (
	"context"
	"encoding/csv"
	"flag"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"equiptra/internal/db"
)

func main() {
	productsPath := flag.String("products", "../Current-Product-20260829-1979654-66amv8.csv", "path to the CurrentRMS products CSV export")
	assetsPath := flag.String("assets", "../asset_listing_report_08-29-2026_09-18-31.csv", "path to the CurrentRMS asset listing CSV export")
	flag.Parse()

	ctx := context.Background()
	pool, err := db.Connect(ctx)
	if err != nil {
		log.Fatalf("db connect: %v", err)
	}
	defer pool.Close()

	productIDByName, err := importProducts(ctx, pool, *productsPath)
	if err != nil {
		log.Fatalf("importing products: %v", err)
	}
	fmt.Printf("products: imported/updated %d\n", len(productIDByName))

	stats, err := importAssets(ctx, pool, *assetsPath, productIDByName)
	if err != nil {
		log.Fatalf("importing assets: %v", err)
	}
	fmt.Printf("assets: imported/updated %d serialized, %d bulk, %d skipped (no matching product)\n",
		stats.serialized, stats.bulk, stats.skipped)
}

// readRows reads a CSV into a slice of header->value maps, trimming a
// leading UTF-8 BOM from the first header if present.
func readRows(path string) ([]map[string]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	r := csv.NewReader(f)
	r.LazyQuotes = true

	header, err := r.Read()
	if err != nil {
		return nil, err
	}
	if len(header) > 0 {
		header[0] = strings.TrimPrefix(header[0], "\uFEFF")
	}

	var rows []map[string]string
	for {
		record, err := r.Read()
		if err != nil {
			break // io.EOF or malformed trailing line
		}
		row := make(map[string]string, len(header))
		for i, col := range header {
			if i < len(record) {
				row[col] = record[i]
			}
		}
		rows = append(rows, row)
	}
	return rows, nil
}

func nullableString(s string) *string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	return &s
}

func nullableFloat(s string) *float64 {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return nil
	}
	return &f
}

func importProducts(ctx context.Context, pool *pgxpool.Pool, path string) (map[string]int64, error) {
	rows, err := readRows(path)
	if err != nil {
		return nil, err
	}

	productIDByName := make(map[string]int64, len(rows))
	for _, row := range rows {
		legacyID, err := strconv.Atoi(strings.TrimSpace(row["Id"]))
		if err != nil {
			log.Printf("product row skipped: bad Id %q", row["Id"])
			continue
		}
		name := strings.TrimSpace(row["Name"])
		if name == "" {
			log.Printf("product legacy_id=%d skipped: blank name", legacyID)
			continue
		}

		var id int64
		err = pool.QueryRow(ctx, `
			INSERT INTO products (legacy_id, name, category, manufacturer, weight_kg,
			                       country_of_origin_code, is_accessory, barcode, description, active)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
			ON CONFLICT (legacy_id) DO UPDATE SET
				name = EXCLUDED.name,
				category = EXCLUDED.category,
				manufacturer = EXCLUDED.manufacturer,
				weight_kg = EXCLUDED.weight_kg,
				country_of_origin_code = EXCLUDED.country_of_origin_code,
				is_accessory = EXCLUDED.is_accessory,
				barcode = EXCLUDED.barcode,
				description = EXCLUDED.description,
				active = EXCLUDED.active,
				updated_at = now()
			RETURNING id`,
			legacyID,
			name,
			nullableString(row["Product Group"]),
			nullableString(row["Manufacturer"]),
			nullableFloat(row["Weight"]),
			nullableString(row["Country of Origin Code"]),
			strings.EqualFold(strings.TrimSpace(row["Accessory Only"]), "Yes"),
			nullableString(row["Barcode"]),
			nullableString(row["Description"]),
			strings.EqualFold(strings.TrimSpace(row["Active"]), "Yes"),
		).Scan(&id)
		if err != nil {
			return nil, fmt.Errorf("upserting product legacy_id=%d (%s): %w", legacyID, name, err)
		}
		productIDByName[name] = id
	}
	return productIDByName, nil
}

type assetImportStats struct {
	serialized int
	bulk       int
	skipped    int
}

// parseEffectiveDate parses CurrentRMS's "2024-12-03 00:00:00 UTC" timestamp
// format into a date.
func parseEffectiveDate(s string) *time.Time {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	t, err := time.Parse("2006-01-02 15:04:05 UTC", s)
	if err != nil {
		return nil
	}
	return &t
}

func importAssets(ctx context.Context, pool *pgxpool.Pool, path string, productIDByName map[string]int64) (assetImportStats, error) {
	rows, err := readRows(path)
	if err != nil {
		return assetImportStats{}, err
	}

	var stats assetImportStats
	for _, row := range rows {
		legacyID, err := strconv.Atoi(strings.TrimSpace(row["id"]))
		if err != nil {
			log.Printf("asset row skipped: bad id %q", row["id"])
			stats.skipped++
			continue
		}
		name := strings.TrimSpace(row["name"])
		productID, ok := productIDByName[name]
		if !ok {
			log.Printf("asset legacy_id=%d skipped: no product matches name %q", legacyID, name)
			stats.skipped++
			continue
		}

		assetNumber := nullableString(row["asset_number"])
		isBulk := assetNumber == nil

		quantity := 1
		if isBulk {
			if q := nullableFloat(row["quantity_held"]); q != nil {
				quantity = int(*q)
			} else {
				quantity = 0
			}
		}

		status := "active"
		if strings.TrimSpace(row["written_off"]) == "1.0" {
			status = "written_off"
		} else if strings.TrimSpace(row["sold"]) == "1.0" {
			status = "sold"
		}

		_, err = pool.Exec(ctx, `
			INSERT INTO assets (legacy_id, product_id, asset_number, serial_number, is_bulk, quantity,
			                     location, purchase_price, replacement_value, purchase_date, status)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
			ON CONFLICT (legacy_id) DO UPDATE SET
				product_id = EXCLUDED.product_id,
				asset_number = EXCLUDED.asset_number,
				serial_number = EXCLUDED.serial_number,
				is_bulk = EXCLUDED.is_bulk,
				quantity = EXCLUDED.quantity,
				location = EXCLUDED.location,
				purchase_price = EXCLUDED.purchase_price,
				replacement_value = EXCLUDED.replacement_value,
				purchase_date = EXCLUDED.purchase_date,
				status = EXCLUDED.status,
				updated_at = now()`,
			legacyID,
			productID,
			assetNumber,
			nullableString(row["serial_number"]),
			isBulk,
			quantity,
			nullableString(row["location"]),
			nullableFloat(row["purchase_price"]),
			nullableFloat(row["asset_replacement_value"]),
			parseEffectiveDate(row["effective_date"]),
			status,
		)
		if err != nil {
			return stats, fmt.Errorf("upserting asset legacy_id=%d (%s): %w", legacyID, name, err)
		}
		if isBulk {
			stats.bulk++
		} else {
			stats.serialized++
		}
	}
	return stats, nil
}
