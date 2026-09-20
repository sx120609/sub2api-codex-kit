// Package codexstate implements Codex Turn-State harvesting, pooling, and
// replacement rules ported from DouDOU-start/codex-state-kit.
package codexstate

import (
	"encoding/base64"
	"encoding/binary"
	"strings"
	"time"
)

const (
	HeaderName = "x-codex-turn-state"

	// MaxAge is the inject/peek validity window (40 minutes).
	MaxAge = 40 * time.Minute
	// PrefetchAge is when a cached token should be refreshed (35 minutes).
	PrefetchAge = 35 * time.Minute
	// ActiveWindow is how long a model stays in the harvest set after last use.
	ActiveWindow = 60 * time.Minute
	// PoolMaxAge drops pool entries older than this.
	PoolMaxAge = 2 * time.Hour

	QualityLen292    = 292
	QualityLen332    = 332
	DegradedLen      = 312
	maxPoolVariants  = 10
	lengthMatchSlack = 4
	degradedLenLow   = 308
	degradedLenHigh  = 316
)

// Token is a parsed x-codex-turn-state blob.
type Token struct {
	Value      string `json:"token"`
	IssuedUnix int64  `json:"issued_unix"`
	Len        int    `json:"len"`
	Source     string `json:"source"`
	CapturedAt string `json:"captured_at"`
}

// ParseToken validates a Fernet-like Turn-State blob.
func ParseToken(raw, source string) (Token, bool) {
	raw = strings.TrimSpace(raw)
	issued, ok := IssuedUnix(raw)
	if !ok {
		return Token{}, false
	}
	return Token{
		Value:      raw,
		IssuedUnix: issued,
		Len:        len(raw),
		Source:     source,
		CapturedAt: time.Now().UTC().Format(time.RFC3339Nano),
	}, true
}

// IssuedUnix extracts the Fernet timestamp from a gAAAAA… token.
func IssuedUnix(token string) (int64, bool) {
	token = strings.TrimSpace(token)
	if !strings.HasPrefix(token, "gAAAAA") {
		return 0, false
	}
	raw, err := decodeFernet(token)
	if err != nil || len(raw) < 9 || raw[0] != 0x80 {
		return 0, false
	}
	return int64(binary.BigEndian.Uint64(raw[1:9])), true
}

func decodeFernet(token string) ([]byte, error) {
	if b, err := base64.RawURLEncoding.DecodeString(token); err == nil {
		return b, nil
	}
	if b, err := base64.URLEncoding.DecodeString(token); err == nil {
		return b, nil
	}
	padded := token
	switch len(padded) % 4 {
	case 2:
		padded += "=="
	case 3:
		padded += "="
	}
	return base64.URLEncoding.DecodeString(padded)
}

func IsDegraded(token string) bool {
	n := len(strings.TrimSpace(token))
	return n >= degradedLenLow && n <= degradedLenHigh
}

func MatchingLen(actual, target int) bool {
	low := target - lengthMatchSlack
	if low < 0 {
		low = 0
	}
	return actual >= low && actual <= target+lengthMatchSlack
}

func CanonicalQualityLen(length int) (int, bool) {
	if MatchingLen(length, QualityLen292) {
		return QualityLen292, true
	}
	if MatchingLen(length, QualityLen332) {
		return QualityLen332, true
	}
	return 0, false
}

func Age(issuedUnix int64, now time.Time) time.Duration {
	if issuedUnix <= 0 {
		return MaxAge + time.Second
	}
	issued := time.Unix(issuedUnix, 0)
	if now.Before(issued) {
		return 0
	}
	return now.Sub(issued)
}
