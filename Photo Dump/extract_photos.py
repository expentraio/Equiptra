"""
Equiptra — CurrentRMS photo extraction script.

Pulls product photos out of CurrentRMS via their API before your renewal lapses,
and saves them locally with a naming convention that can be matched back to
products later once Equiptra is built.

Naming convention: {product_id}_{sanitized-product-name}.{ext}
  e.g. 281_1005BAC.jpg

Usage:
    pip install requests
    python extract_photos.py

You'll need:
  - Your CurrentRMS API key (System Setup > Integrations > API)
  - Your subdomain (the part before .current-rms.com in your login URL)
  - The Products CSV export you already have (for the list of product IDs/names)

STEP 1 (optional sanity check): run `python3 extract_photos.py inspect <id>`
on any product ID to confirm your API key/subdomain work and that the product
actually has a photo set, before running the full batch.

STEP 2: run `python3 extract_photos.py` with no arguments to download every
product's photo and write a manifest tracking what succeeded/failed/had none.
"""

import csv
import os
import re
import sys
import time
import json
import requests

# ---- CONFIGURE THESE ----
API_KEY = "XLhVpGtfWgeqDDoF71va"
SUBDOMAIN = "ldmtv"  # e.g. if you log in at ldmtv.current-rms.com, this is "ldmtv"
PRODUCTS_CSV = "Current-Product-20260829-1979654-66amv8.csv"
OUTPUT_DIR = "product_photos"
# --------------------------

BASE_URL = "https://api.current-rms.com/api/v1"
HEADERS = {
    "X-AUTH-TOKEN": API_KEY,
    "X-SUBDOMAIN": SUBDOMAIN,
    "Content-Type": "application/json",
}


def sanitize(name: str) -> str:
    """Make a product name safe for use in a filename."""
    name = re.sub(r"[^\w\-. ]", "", name)
    name = name.strip().replace(" ", "-")
    return name[:80]  # keep filenames sane length


def inspect_one_product(product_id: str):
    """Fetch a single product and print the raw response. Used this to confirm
    the photo lives at product.icon.url — a presigned S3 URL that expires after
    a few hours, so it must be downloaded immediately, not stored for later."""
    url = f"{BASE_URL}/products/{product_id}"
    resp = requests.get(url, headers=HEADERS)
    print(f"Status: {resp.status_code}")
    if resp.status_code != 200:
        print("Response body:", resp.text[:500])
        return
    data = resp.json()
    icon = data.get("product", {}).get("icon")
    print("icon field:", json.dumps(icon, indent=2)[:500] if icon else "No icon set on this product")


# Confirmed via inspect_one_product(): the photo URL lives at product.icon.url.
# It's a presigned S3 URL that expires after a few hours — must be downloaded
# right after fetching, never stored/reused as a link.


def download_all_photos():
    os.makedirs(OUTPUT_DIR, exist_ok=True)
    manifest_path = os.path.join(OUTPUT_DIR, "_manifest.csv")

    with open(PRODUCTS_CSV, encoding="utf-8-sig") as f:
        products = list(csv.DictReader(f))

    with open(manifest_path, "w", newline="", encoding="utf-8") as mf:
        writer = csv.writer(mf)
        writer.writerow(["product_id", "product_name", "filename", "status"])

        for p in products:
            pid = p["Id"].strip()
            name = p["Name"].strip()
            safe_name = sanitize(name)

            url = f"{BASE_URL}/products/{pid}"
            try:
                resp = requests.get(url, headers=HEADERS, timeout=15)
            except requests.RequestException as e:
                writer.writerow([pid, name, "", f"request_failed: {e}"])
                continue

            if resp.status_code != 200:
                writer.writerow([pid, name, "", f"http_{resp.status_code}"])
                continue

            data = resp.json()
            icon = data.get("product", {}).get("icon")
            photo_url = icon.get("url") if icon else None

            if not photo_url:
                writer.writerow([pid, name, "", "no_photo"])
                continue

            ext = photo_url.split(".")[-1].split("?")[0][:4]
            filename = f"{pid}_{safe_name}.{ext}"
            filepath = os.path.join(OUTPUT_DIR, filename)

            # Download immediately — this URL is presigned and expires in a
            # few hours, so there's no "come back later" option.
            try:
                img_resp = requests.get(photo_url, timeout=20)
                if img_resp.status_code == 200:
                    with open(filepath, "wb") as imgf:
                        imgf.write(img_resp.content)
                    writer.writerow([pid, name, filename, "downloaded"])
                    print(f"Saved {filename}")
                else:
                    writer.writerow([pid, name, "", f"image_http_{img_resp.status_code}"])
            except requests.RequestException as e:
                writer.writerow([pid, name, "", f"image_request_failed: {e}"])

            time.sleep(0.3)  # be polite to their API, avoid rate limiting

    print(f"\nDone. Manifest written to {manifest_path}")


if __name__ == "__main__":
    if len(sys.argv) > 1 and sys.argv[1] == "inspect":
        # Run: python extract_photos.py inspect <product_id>
        pid = sys.argv[2] if len(sys.argv) > 2 else "281"
        inspect_one_product(pid)
    else:
        download_all_photos()
