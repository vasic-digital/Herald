// Wave 5 Task 5 — shared attachment helpers for scenarios #5/#6/#7.
//
// Attachment payloads are generated inline at scenario-run time rather
// than committed under qaherald/testdata/, for two reasons:
//   1. The Go test binary stays tiny — no embedded 11MB blob.
//   2. Each helper documents the bytes-on-wire shape; a reviewer can
//      verify "yes, this is the minimum-viable PNG header" without
//      hexdump-ing a checked-in binary.
//
// All helpers return (bytes, contentType, filename) so the scenarios
// can pipe straight into o.TG.Upload + the transcript writer's
// AttachFile.
package scenario

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
)

// minimalPNGBytes returns a 1x1 black-pixel PNG. ~70 bytes. Telegram
// accepts this as a tele.Photo upload (the bot API does not validate
// dimensions; a 1x1 photo with a real PNG magic number suffices for
// the round-trip sha256 check).
func minimalPNGBytes() ([]byte, string, string) {
	img := image.NewRGBA(image.Rect(0, 0, 1, 1))
	img.Set(0, 0, color.Black)
	var buf bytes.Buffer
	_ = png.Encode(&buf, img)
	return buf.Bytes(), "image/png", "qa-sample.png"
}

// minimalPDFBytes returns the smallest syntactically valid PDF
// pherald + Telegram accept as a tele.Document with mime
// application/pdf. The %PDF-1.4 + %%EOF magic envelopes a minimal
// xref + trailer structure. ~250 bytes.
//
// The bytes here are a real PDF — opening this file in a PDF reader
// would render a blank page. We do not commit it as testdata because
// the inline generation is more reproducible across hosts (no risk of
// a stray editor stripping bytes).
func minimalPDFBytes() ([]byte, string, string) {
	body := []byte("%PDF-1.4\n" +
		"1 0 obj<</Type/Catalog/Pages 2 0 R>>endobj\n" +
		"2 0 obj<</Type/Pages/Count 1/Kids[3 0 R]>>endobj\n" +
		"3 0 obj<</Type/Page/Parent 2 0 R/MediaBox[0 0 612 792]/Contents 4 0 R>>endobj\n" +
		"4 0 obj<</Length 0>>stream\nendstream\nendobj\n" +
		"xref\n0 5\n" +
		"0000000000 65535 f\n" +
		"0000000009 00000 n\n" +
		"0000000053 00000 n\n" +
		"0000000098 00000 n\n" +
		"0000000160 00000 n\n" +
		"trailer<</Size 5/Root 1 0 R>>\n" +
		"startxref\n200\n%%EOF\n")
	return body, "application/pdf", "qa-sample.pdf"
}

// largeDocumentBytes returns an 11MB blob of repeating 0xAB octets
// with content-type application/octet-stream. Used by the oversized-
// payload variant inside scenario #6 to assert pherald returns 413 (or
// the operator-configured cap status) for over-quota uploads.
func largeDocumentBytes() ([]byte, string, string) {
	return bytes.Repeat([]byte{0xAB}, 11*1024*1024), "application/octet-stream", "qa-oversized.bin"
}

// minimalOGGBytes returns a single-page OGG container ("OggS" magic +
// minimum header). Telegram's bot API accepts this as a tele.Voice
// when the contentType is audio/ogg.
//
// The header here is hand-crafted to be the smallest valid OGG page:
// capture pattern "OggS", version 0, header type 0x02 (BOS), 8-byte
// granule position, 4-byte stream serial, 4-byte page sequence,
// 4-byte CRC32 (zero — many decoders accept this for the BOS page),
// and one segment of length 0. Total ~28 bytes.
func minimalOGGBytes() ([]byte, string, string) {
	body := []byte{
		// Capture pattern
		'O', 'g', 'g', 'S',
		// Version
		0x00,
		// Header type (BOS = 0x02)
		0x02,
		// Granule position (8 bytes, all zero — beginning of stream)
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		// Bitstream serial (4 bytes)
		0x01, 0x00, 0x00, 0x00,
		// Page sequence (4 bytes)
		0x00, 0x00, 0x00, 0x00,
		// CRC32 (4 bytes — set to zero; many decoders skip CRC
		// verification on BOS pages, which is fine for our magic-byte
		// upload smoke)
		0x00, 0x00, 0x00, 0x00,
		// Number of page segments
		0x00,
	}
	return body, "audio/ogg", "qa-sample.ogg"
}
