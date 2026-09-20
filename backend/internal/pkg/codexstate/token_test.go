package codexstate

import (
	"encoding/base64"
	"encoding/binary"
	"strings"
	"testing"
	"time"
)

func makeFernetToken(issuedUnix int64, extra int) string {
	raw := make([]byte, 9+extra)
	raw[0] = 0x80
	binary.BigEndian.PutUint64(raw[1:9], uint64(issuedUnix))
	return base64.RawURLEncoding.EncodeToString(raw)
}

func TestIssuedUnix(t *testing.T) {
	now := time.Now().Unix()
	tok := makeFernetToken(now, 20)
	if !strings.HasPrefix(tok, "gAAAAA") && len(tok) > 0 {
		// RawURL encoding of 0x80 + timestamp typically starts with gAAAAA
	}
	got, ok := IssuedUnix(tok)
	if !ok {
		t.Fatalf("expected parse ok, token=%q", tok)
	}
	if got != now {
		t.Fatalf("issued=%d want %d", got, now)
	}
}

func TestParseTokenRejectsNonFernet(t *testing.T) {
	if _, ok := ParseToken("not-a-token", "test"); ok {
		t.Fatal("expected reject")
	}
	if _, ok := ParseToken(strings.Repeat("a", 312), "test"); ok {
		t.Fatal("expected reject unprefixed blob")
	}
}

func TestIsDegradedAndQuality(t *testing.T) {
	if !IsDegraded(strings.Repeat("x", 312)) {
		t.Fatal("312 should be degraded")
	}
	if IsDegraded(strings.Repeat("x", 292)) {
		t.Fatal("292 should not be degraded")
	}
	if q, ok := CanonicalQualityLen(292); !ok || q != QualityLen292 {
		t.Fatalf("292 quality got %d %v", q, ok)
	}
	if q, ok := CanonicalQualityLen(330); !ok || q != QualityLen332 {
		t.Fatalf("330 should map to 332, got %d %v", q, ok)
	}
	if _, ok := CanonicalQualityLen(312); ok {
		t.Fatal("312 is not quality")
	}
}

func TestMatchingLenSlack(t *testing.T) {
	if !MatchingLen(288, 292) || !MatchingLen(296, 292) {
		t.Fatal("±4 around 292 should match")
	}
	if MatchingLen(287, 292) {
		t.Fatal("287 should not match 292")
	}
}
