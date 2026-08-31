// Command repair-photo-cache fixes the cache-control header on the 693
// already-backfilled product photos, all originally uploaded without a
// cache-control header, which made Supabase Storage default to
// `no-cache` — see docs/equiptra-image-caching-addendum.md.
//
// Re-uploads each photo to its *existing* object path
// ({legacy_id}{ext}, matching cmd/migrate-photos's original convention)
// with x-upsert, so the same bytes replace the same object at the same
// URL — just with the corrected header this time. No database changes:
// products.image_url already points at the right place, since neither
// the path nor the URL changes.
//
// Prefers the original local source files (Photo Dump/product_photos/);
// if one is missing, falls back to pulling the object's current bytes
// from Supabase Storage itself and re-uploading those unchanged.
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

	"equiptra/internal/storage"
)

const productPhotosBucket = "product-photos"

// photoCacheMaxAgeSeconds must match internal/handlers/product_photos.go's
// value — see that file's comment for the reasoning.
const photoCacheMaxAgeSeconds = 31536000

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
	limit := flag.Int("limit", 0, "only process the first N 'downloaded' rows — 0 means no limit (run the full repair)")
	flag.Parse()

	dir := *photosDir
	if dir == "" {
		dir = filepath.Dir(*manifestPath)
	}

	ctx := context.Background()
	supabaseClient := storage.NewSupabaseClient()
	if supabaseClient == nil {
		log.Fatal("SUPABASE_PROJECT_REF/SUPABASE_SERVICE_ROLE_KEY not set — nowhere to upload to")
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

	var repaired, failed, skipped, processed int
	for _, row := range rows[1:] {
		status := row[colIdx["status"]]
		if status != "downloaded" {
			skipped++
			continue
		}
		if *limit > 0 && processed >= *limit {
			break
		}
		processed++

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
		path := fmt.Sprintf("%d%s", legacyID, ext)

		body, err := os.ReadFile(filepath.Join(dir, filename))
		if err != nil {
			// Local source file gone — pull the object's current bytes from
			// storage instead and re-upload those unchanged.
			var dlContentType string
			body, dlContentType, err = supabaseClient.DownloadObject(ctx, productPhotosBucket, path)
			if err != nil {
				log.Printf("product legacy_id=%d skipped: local file missing and could not fetch existing object: %v", legacyID, err)
				failed++
				continue
			}
			if dlContentType != "" {
				contentType = dlContentType
			}
			log.Printf("product legacy_id=%d: local file missing, re-uploading existing stored bytes instead", legacyID)
		}

		publicURL, err := supabaseClient.UploadObject(ctx, productPhotosBucket, path, contentType, body, photoCacheMaxAgeSeconds)
		if err != nil {
			log.Printf("product legacy_id=%d repair failed: %v", legacyID, err)
			failed++
			continue
		}
		log.Printf("product legacy_id=%d: repaired -> %s", legacyID, publicURL)
		repaired++
	}

	fmt.Printf("cache repair: repaired %d, failed %d, skipped %d\n", repaired, failed, skipped)
}
