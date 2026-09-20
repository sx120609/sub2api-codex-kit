package service

import (
	"encoding/base64"
	"encoding/binary"
	"net/http"
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
