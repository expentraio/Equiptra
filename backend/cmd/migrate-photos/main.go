// Command migrate-photos bulk-uploads the CurrentRMS product photos already
// pulled locally (brief §7) to S3 and populates products.image_url,
// matching each file to its product by the product_id embedded in the
// manifest/filename (which is the product's legacy_id in this app).
// Safe to re-run — re-uploads and overwrites image_url unconditionally.
package main

import (
	"context"
	"encoding/csv"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"equiptra/internal/db"
	"equiptra/internal/storage"
)

var extToContentType = map[string]string{
	".jpg":  "image/jpeg",
	".jpeg": "image/jpeg",
	".jfif": "image/jpeg",
	".png":  "image/png",
	".webp": "image/webp",
}

func main() {
	manifestPath := flag.String("manifest", "../Photo Dump/product_photos/_manifest.csv", "path to the photo manifest CSV")
	photosDir := flag.String("photos-dir", "", "directory containing the photo files (default: manifest's directory)")
	flag.Parse()

	dir := *photosDir
	if dir == "" {
		dir = filepath.Dir(*manifestPath)
	}

	ctx := context.Background()
	pool, err := db.Connect(ctx)
	if err != nil {
		log.Fatalf("db connect: %v", err)
	}
	defer pool.Close()

	s3Client, err := storage.NewClient(ctx)
	if err != nil {
		log.Fatalf("s3 client: %v", err)
	}
	if s3Client == nil {
		log.Fatal("S3_BUCKET is not set — nowhere to upload to")
	}

	f, err := os.Open(*manifestPath)
	if err != nil {
		log.Fatalf("opening manifest: %v", err)
	}
	defer f.Close()

	rows, err := csv.NewReader(f).ReadAll()
	if err != nil {
		log.Fatalf("reading manifest: %v", err)
	}
	if len(rows) == 0 {
		log.Fatal("manifest is empty")
	}
	header := rows[0]
	colIdx := make(map[string]int, len(header))
	for i, h := range header {
		colIdx[strings.TrimSpace(h)] = i
	}

	var uploaded, notFound, skipped int
	for _, row := range rows[1:] {
		status := row[colIdx["status"]]
		if status != "downloaded" {
			skipped++
			continue
		}
		legacyIDStr := row[colIdx["product_id"]]
		filename := row[colIdx["filename"]]
		legacyID, err := strconv.Atoi(legacyIDStr)
		if err != nil {
			log.Printf("skipping row with bad product_id %q: %v", legacyIDStr, err)
			skipped++
			continue
		}

		ext := strings.ToLower(filepath.Ext(filename))
		contentType, ok := extToContentType[ext]
		if !ok {
			log.Printf("product legacy_id=%d skipped: unsupported extension %q", legacyID, ext)
			skipped++
			continue
		}

		body, err := os.ReadFile(filepath.Join(dir, filename))
		if err != nil {
			log.Printf("product legacy_id=%d skipped: reading file: %v", legacyID, err)
			skipped++
			continue
		}

		key := fmt.Sprintf("products/%d%s", legacyID, ext)
		getURL, err := s3Client.PutObject(ctx, key, contentType, body)
		if err != nil {
			log.Printf("product legacy_id=%d upload failed: %v", legacyID, err)
			skipped++
			continue
		}

		tag, err := pool.Exec(ctx, `UPDATE products SET image_url = $1, updated_at = now() WHERE legacy_id = $2`, getURL, legacyID)
		if err != nil {
			log.Printf("product legacy_id=%d db update failed: %v", legacyID, err)
			skipped++
			continue
		}
		if tag.RowsAffected() == 0 {
			log.Printf("product legacy_id=%d: no matching product row (not found)", legacyID)
			notFound++
			continue
		}
		uploaded++
	}

	fmt.Printf("photos: uploaded %d, product not found %d, skipped %d\n", uploaded, notFound, skipped)
}
