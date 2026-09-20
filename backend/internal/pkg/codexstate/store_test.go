package codexstate

import (
	"strings"
	"testing"
	"time"
)

func TestPeekPrefersBoundQualityToken(t *testing.T) {
	m := NewManager()
	now := time.Now()
	quality := makeFernetToken(now.Unix(), 40)
	// Pad to ~292 by repeating after parse... ParseToken uses actual encoded length.
	// Build tokens of desired lengths by adding extra payload bytes.
	quality = makeTokenOfLen(now.Unix(), 292)
	degraded := makeTokenOfLen(now.Unix(), 312)

	if !strings.HasPrefix(quality, "gAAAAA") {
		t.Fatalf("quality token prefix: %q", quality[:6])
	}
	m.CaptureBatch(1, "gpt-5.4", []string{degraded, quality}, "probe", now)
	got := m.Peek(1, "gpt-5.4", now)
	if got != quality {
		t.Fatalf("peek=%q want quality token", got)
	}
}

func TestNeedsRefreshAndExpiry(t *testing.T) {
	m := NewManager()
	old := time.Now().Add(-36 * time.Minute)
	tok := makeTokenOfLen(old.Unix(), 292)
	m.Capture(2, "gpt-5.4", tok, "probe", old)
	if !m.NeedsRefresh(2, "gpt-5.4", time.Now()) {
		t.Fatal("expected prefetch after 35m")
	}
	expired := time.Now().Add(-41 * time.Minute)
	tok = makeTokenOfLen(expired.Unix(), 292)
	m.Capture(3, "gpt-5.4", tok, "probe", expired)
	if m.Peek(3, "gpt-5.4", time.Now()) != "" {
		t.Fatal("expired token must not be injected")
	}
}

func TestAccountSwitchClearsPool(t *testing.T) {
	m := NewManager()
	now := time.Now()
	tok := makeTokenOfLen(now.Unix(), 292)
	m.BindChatGPTAccount(7, "acct-a")
	m.Capture(7, "gpt-5.4", tok, "probe", now)
	if m.Peek(7, "gpt-5.4", now) == "" {
		t.Fatal("token should remain after first account bind")
	}
	m.BindChatGPTAccount(7, "acct-b")
	if m.Peek(7, "gpt-5.4", now) != "" {
		t.Fatal("pool must clear on ChatGPT account switch")
	}
}

func makeTokenOfLen(issuedUnix int64, targetLen int) string {
	// 9 byte header encodes to 12 chars with raw URL encoding; add extra bytes.
	extra := 0
	tok := makeFernetToken(issuedUnix, extra)
	for len(tok) < targetLen {
		extra++
		tok = makeFernetToken(issuedUnix, extra)
		if extra > 400 {
			break
		}
	}
	return tok
}
