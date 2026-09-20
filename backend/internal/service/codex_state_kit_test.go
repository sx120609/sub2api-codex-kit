package service

import (
	"context"
	"encoding/base64"
	"encoding/binary"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/codexstate"
	"github.com/stretchr/testify/require"
)

func kitTestToken(issuedUnix int64, extra int) string {
	for ; extra < 400; extra++ {
		raw := make([]byte, 9+extra)
		raw[0] = 0x80
		binary.BigEndian.PutUint64(raw[1:9], uint64(issuedUnix))
		tok := base64.RawURLEncoding.EncodeToString(raw)
		if codexstate.MatchingLen(len(tok), codexstate.QualityLen292) {
			return tok
		}
	}
	tPanic := make([]byte, 9)
	tPanic[0] = 0x80
	binary.BigEndian.PutUint64(tPanic[1:9], uint64(issuedUnix))
	return base64.RawURLEncoding.EncodeToString(tPanic)
}

func TestApplyCodexStateKitHTTPReplacesOnlyWhenClientSentHeader(t *testing.T) {
	svc := &OpenAIGatewayService{}
	svc.ensureCodexStateKit()
	account := &Account{
		ID:       9,
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
		Status:   StatusActive,
		Extra:    map[string]any{codexStateKitEnabledExtraKey: true},
		Credentials: map[string]any{
			"chatgpt_account_id": "acct-9",
		},
	}
	token := kitTestToken(time.Now().Unix(), 80)
	require.True(t, svc.codexStateKit.mgr.Capture(9, "gpt-5.4", token, "probe", time.Now()))

	body := []byte(`{"model":"gpt-5.4"}`)
	h := http.Header{}
	svc.applyCodexStateKitHTTP(nil, account, h, body)
	require.Empty(t, h.Get(codexstate.HeaderName), "first request must not inject")

	h.Set(codexstate.HeaderName, "gAAAAAclient-old-value-that-will-not-parse")
	svc.applyCodexStateKitHTTP(nil, account, h, body)
	require.Equal(t, token, h.Get(codexstate.HeaderName))
}

func TestIngestCodexStateKitTokensRequiresAccount(t *testing.T) {
	svc := &OpenAIGatewayService{}
	_, err := svc.IngestCodexStateKitTokens(context.Background(), 0, CodexStateKitIngest{
		Tokens: []string{"gAAAAA"},
	})
	require.Error(t, err)
}

func TestFilterIngestTokensSkipsDegraded(t *testing.T) {
	quality := kitTestToken(time.Now().Unix(), 80)
	require.True(t, absInt(len(quality)-292) <= 4)
	degraded := kitDegradedToken(time.Now().Unix())
	require.True(t, len(degraded) >= 308 && len(degraded) <= 316)

	kept, accepted, skipped, degradedCount := filterIngestTokens(
		[]string{quality, degraded, "not-a-token"},
		"codex-state-kit",
		false,
	)
	require.Equal(t, 1, accepted)
	require.Equal(t, 2, skipped)
	require.Equal(t, 1, degradedCount)
	require.Equal(t, []string{quality}, kept)

	kept, accepted, skipped, degradedCount = filterIngestTokens(
		[]string{degraded},
		"codex-state-kit",
		true,
	)
	require.Equal(t, 1, accepted)
	require.Equal(t, 0, skipped)
	require.Equal(t, 1, degradedCount)
	require.Equal(t, []string{degraded}, kept)
}

func TestFlattenIngestBatchesAndMergeModels(t *testing.T) {
	batches := flattenIngestBatches(CodexStateKitIngest{
		Model:  "gpt-5.6-sol",
		Tokens: []string{"a"},
		Batches: []CodexStateKitIngestBatch{
			{Model: "gpt-5.4", Tokens: []string{"b"}},
		},
	})
	require.Len(t, batches, 2)
	require.Equal(t, "gpt-5.6-sol", batches[0].Model)
	require.Equal(t, []string{"gpt-5.4", "gpt-5.6-sol"}, mergeKitModels([]string{"gpt-5.4", "gpt-5.4"}, "gpt-5.6-sol", "gpt-5.4"))
}

func kitDegradedToken(issuedUnix int64) string {
	for extra := 80; extra < 500; extra++ {
		raw := make([]byte, 9+extra)
		raw[0] = 0x80
		binary.BigEndian.PutUint64(raw[1:9], uint64(issuedUnix))
		tok := base64.RawURLEncoding.EncodeToString(raw)
		if len(tok) >= 308 && len(tok) <= 316 {
			return tok
		}
	}
	return strings.Repeat("A", 312)
}

func absInt(n int) int {
	if n < 0 {
		return -n
	}
	return n
}
