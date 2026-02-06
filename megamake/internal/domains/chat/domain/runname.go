package domain

import (
	"regexp"
	"strings"
	"time"
)

// RunNamePrefix is the stable prefix used in chat run names.
const RunNamePrefix = "chat-"

// BuildRunName returns a new run_name in the format:
//
//	YYYYMMDD_HHMMSSZ_chat-xxxxxxxx
//
// Where xxxxxxxx is 8 lowercase hex characters derived from randBytes.
// If randBytes is shorter than 4 bytes, the hex suffix is padded with zeros.
//
// Notes:
// - Uses UTC timestamps.
// - Intended to be stable and filesystem-friendly.
func BuildRunName(nowUTC time.Time, randBytes []byte) string {
	ts := formatUTCForRunName(nowUTC.UTC())
	sfx := hex8(randBytes)
	return ts + "_" + RunNamePrefix + sfx
}

// IsValidRunName returns true if runName matches the expected format.
// This is intentionally strict to keep run folder scanning safe and deterministic.
func IsValidRunName(runName string) bool {
	runName = strings.TrimSpace(runName)
	if runName == "" {
		return false
	}
	// Example: 20260206_153012Z_chat-8e3af90d
	re := regexp.MustCompile(`^\d{8}_\d{6}Z_chat-[0-9a-f]{8}$`)
	return re.MatchString(runName)
}

func formatUTCForRunName(t time.Time) string {
	// Same timestamp shape as artifact writer: 20260120_154233Z
	return t.UTC().Format("20060102_150405Z")
}

func hex8(b []byte) string {
	// Convert first 4 bytes into 8 lowercase hex chars.
	// If fewer than 4 bytes are present, missing bytes are treated as 0.
	var x [4]byte
	for i := 0; i < 4 && i < len(b); i++ {
		x[i] = b[i]
	}

	const hexdigits = "0123456789abcdef"
	out := make([]byte, 8)
	out[0] = hexdigits[(x[0]>>4)&0xF]
	out[1] = hexdigits[x[0]&0xF]
	out[2] = hexdigits[(x[1]>>4)&0xF]
	out[3] = hexdigits[x[1]&0xF]
	out[4] = hexdigits[(x[2]>>4)&0xF]
	out[5] = hexdigits[x[2]&0xF]
	out[6] = hexdigits[(x[3]>>4)&0xF]
	out[7] = hexdigits[x[3]&0xF]
	return string(out)
}
