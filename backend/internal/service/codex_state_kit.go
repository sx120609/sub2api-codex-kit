package service

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/codexstate"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
)

const (
	codexStateKitEnabledExtraKey  = "codex_state_kit_enabled"
	codexStateKitBoundLenExtraKey = "codex_state_kit_bound_token_len"
	codexStateKitModelsExtraKey   = "codex_state_kit_models"
	codexStateKitDefaultHarvest   = 10
	codexStateKitDefaultInterval  = 30 * time.Second
)

type CodexStateKitUpdate struct {
	Enabled       *bool    `json:"enabled"`
	BoundTokenLen *int     `json:"bound_token_len"`
	ClearBoundLen bool     `json:"clear_bound_token_len"`
	Models        []string `json:"models"`
}

type CodexStateKitStatus struct {
	codexstate.View
	GlobalEnabled bool `json:"globalEnabled"`
}

type codexStateKitRuntime struct {
	svc    *OpenAIGatewayService
	mgr    *codexstate.Manager
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

func newCodexStateKitRuntime(svc *OpenAIGatewayService) *codexStateKitRuntime {
	return &codexStateKitRuntime{
		svc: svc,
		mgr: codexstate.NewManager(),
	}
}

func (s *OpenAIGatewayService) startCodexStateKit() {
	if s == nil {
		return
	}
	if s.codexStateKit == nil {
		s.codexStateKit = newCodexStateKitRuntime(s)
	}
	if s.cfg != nil && s.cfg.Gateway.CodexStateKit.Enabled {
		s.codexStateKit.Start()
	}
}

func (s *OpenAIGatewayService) StopCodexStateKit() {
	if s == nil || s.codexStateKit == nil {
		return
	}
	s.codexStateKit.Stop()
}

func (r *codexStateKitRuntime) Start() {
	if r == nil || r.cancel != nil {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	r.cancel = cancel
	r.wg.Add(1)
	go func() {
		defer r.wg.Done()
		r.loop(ctx)
	}()
}

func (r *codexStateKitRuntime) Stop() {
	if r == nil || r.cancel == nil {
		return
	}
	r.cancel()
	r.wg.Wait()
	r.cancel = nil
}

func (r *codexStateKitRuntime) globalEnabled() bool {
	if r == nil || r.svc == nil || r.svc.cfg == nil {
		return true
	}
	return r.svc.cfg.Gateway.CodexStateKit.Enabled
}

func (r *codexStateKitRuntime) harvestConcurrency() int {
	n := 0
	if r != nil && r.svc != nil && r.svc.cfg != nil {
		n = r.svc.cfg.Gateway.CodexStateKit.HarvestConcurrency
	}
	if n <= 0 {
		n = codexStateKitDefaultHarvest
	}
	if n > 20 {
		n = 20
	}
	return n
}

func (r *codexStateKitRuntime) checkInterval() time.Duration {
	secs := 0
	if r != nil && r.svc != nil && r.svc.cfg != nil {
		secs = r.svc.cfg.Gateway.CodexStateKit.CheckIntervalSeconds
	}
	if secs <= 0 {
		return codexStateKitDefaultInterval
	}
	return time.Duration(secs) * time.Second
}

func (r *codexStateKitRuntime) defaultModel() string {
	if r == nil || r.svc == nil || r.svc.cfg == nil {
		return "gpt-5.4"
	}
	model := strings.TrimSpace(r.svc.cfg.Gateway.CodexStateKit.DefaultModel)
	if model == "" {
		return "gpt-5.4"
	}
	return model
}

func (r *codexStateKitRuntime) loop(ctx context.Context) {
	timer := time.NewTimer(2 * time.Second)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
			r.harvestAll(ctx)
			timer.Reset(r.checkInterval())
		}
	}
}

func (r *codexStateKitRuntime) harvestAll(ctx context.Context) {
	if !r.globalEnabled() || r.svc == nil || r.svc.accountRepo == nil {
		return
	}
	accounts, err := r.svc.accountRepo.ListByPlatform(ctx, PlatformOpenAI)
	if err != nil {
		slog.Warn("codex_state_kit list accounts failed", "err", err)
		return
	}
	for i := range accounts {
		account := accounts[i]
		if !accountKitEnabled(&account) {
			continue
		}
		full, err := r.svc.accountRepo.GetByID(ctx, account.ID)
		if err != nil || full == nil {
			r.harvestAccount(ctx, &account)
			continue
		}
		r.harvestAccount(ctx, full)
	}
}

func accountKitEnabled(account *Account) bool {
	if account == nil || !account.IsOpenAIOAuthLike() || !account.IsActive() {
		return false
	}
	enabled, ok := account.Extra[codexStateKitEnabledExtraKey].(bool)
	return ok && enabled
}

func (r *codexStateKitRuntime) harvestAccount(ctx context.Context, account *Account) {
	if r == nil || account == nil {
		return
	}
	r.mgr.BindChatGPTAccount(account.ID, account.GetChatGPTAccountID())
	if bound := accountKitBoundLen(account); bound != nil {
		r.mgr.SetBoundLen(account.ID, bound)
	}
	models := r.modelsForAccount(account)
	now := time.Now()
	for _, model := range models {
		if ctx.Err() != nil {
			return
		}
		if !r.mgr.NeedsRefresh(account.ID, model, now) {
			continue
		}
		r.harvestModel(ctx, account, model)
	}
}

func (r *codexStateKitRuntime) modelsForAccount(account *Account) []string {
	seen := map[string]struct{}{}
	var models []string
	add := func(model string) {
		model = strings.TrimSpace(model)
		if model == "" {
			return
		}
		if _, ok := seen[model]; ok {
			return
		}
		seen[model] = struct{}{}
		models = append(models, model)
	}
	for _, model := range accountKitModels(account) {
		add(model)
	}
	for _, model := range r.mgr.ActiveModels(account.ID, time.Now()) {
		add(model)
	}
	if len(models) == 0 {
		add(r.defaultModel())
	}
	return models
}

func accountKitBoundLen(account *Account) *int {
	if account == nil || account.Extra == nil {
		return nil
	}
	raw, ok := account.Extra[codexStateKitBoundLenExtraKey]
	if !ok || raw == nil {
		return nil
	}
	n := extraIntValue(raw, 0)
	if n != codexstate.QualityLen292 && n != codexstate.QualityLen332 && n != codexstate.DegradedLen {
		return nil
	}
	return &n
}

func accountKitModels(account *Account) []string {
	if account == nil || account.Extra == nil {
		return nil
	}
	switch raw := account.Extra[codexStateKitModelsExtraKey].(type) {
	case []string:
		return raw
	case []any:
		out := make([]string, 0, len(raw))
		for _, item := range raw {
			if s, ok := item.(string); ok && strings.TrimSpace(s) != "" {
				out = append(out, strings.TrimSpace(s))
			}
		}
		return out
	default:
		return nil
	}
}

func extraIntValue(raw any, fallback int) int {
	switch v := raw.(type) {
	case int:
		return v
	case int32:
		return int(v)
	case int64:
		return int(v)
	case float64:
		return int(v)
	case json.Number:
		n, err := v.Int64()
		if err != nil {
			return fallback
		}
		return int(n)
	default:
		return fallback
	}
}

func (r *codexStateKitRuntime) harvestModel(ctx context.Context, account *Account, model string) {
	concurrency := r.harvestConcurrency()
	tokens := make([]string, 0, concurrency)
	var mu sync.Mutex
	var wg sync.WaitGroup
	sem := make(chan struct{}, concurrency)
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			token, err := r.fetchOne(ctx, account, model)
			if err != nil || token == "" {
				return
			}
			if codexstate.IsDegraded(token) {
				mu.Lock()
				tokens = append(tokens, token)
				mu.Unlock()
				return
			}
			mu.Lock()
			tokens = append(tokens, token)
			mu.Unlock()
		}()
	}
	wg.Wait()
	if len(tokens) == 0 {
		return
	}
	r.mgr.RecordDistribution(account.ID, model, codexstate.Histogram(tokens))
	matched := r.mgr.CaptureBatch(account.ID, model, tokens, "probe", time.Now())
	slog.Info("codex_state_kit harvested",
		"account_id", account.ID,
		"model", model,
		"tokens", len(tokens),
		"matched", matched,
	)
}

func (r *codexStateKitRuntime) fetchOne(ctx context.Context, account *Account, model string) (string, error) {
	if r.svc == nil || r.svc.openAITokenProvider == nil {
		return "", nil
	}
	token, err := r.svc.openAITokenProvider.GetAccessToken(ctx, account)
	if err != nil || strings.TrimSpace(token) == "" {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, chatgptCodexURL, bytes.NewReader(codexstate.ProbeBody(model)))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("OpenAI-Beta", "responses=experimental")
	req.Host = "chatgpt.com"
	if err := resolveAndSetOpenAIChatGPTAccountHeaders(ctx, r.svc.accountRepo, req.Header, account); err != nil {
		return "", err
	}
	enforceCodexIdentityHeadersWithUA(req.Header, r.svc.codexIdentityOverrideUA(account))

	proxyURL := ""
	if account.Proxy != nil {
		proxyURL = account.Proxy.URL()
	}
	resp, err := r.svc.doOpenAIUpstream(req, proxyURL, account)
	if err != nil {
		return "", err
	}
	defer func() {
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}()
	headerToken := strings.TrimSpace(resp.Header.Get(codexstate.HeaderName))
	if headerToken == "" || !strings.HasPrefix(headerToken, "gAAAAA") {
		return "", nil
	}
	return headerToken, nil
}

func (s *OpenAIGatewayService) applyCodexStateKitHTTP(c *gin.Context, account *Account, h http.Header, body []byte) {
	if s == nil || s.codexStateKit == nil || h == nil || account == nil {
		return
	}
	if !s.codexStateKit.globalEnabled() || !accountKitEnabled(account) {
		return
	}
	model := strings.TrimSpace(gjson.GetBytes(body, "model").String())
	if model != "" {
		s.codexStateKit.mgr.RegisterModel(account.ID, model)
		s.codexStateKit.mgr.BindChatGPTAccount(account.ID, account.GetChatGPTAccountID())
	}
	if strings.TrimSpace(h.Get(openAICodexTurnStateHeader)) == "" || model == "" {
		return
	}
	token := s.codexStateKit.mgr.Peek(account.ID, model, time.Now())
	if token == "" {
		return
	}
	h.Set(openAICodexTurnStateHeader, token)
}

func (s *OpenAIGatewayService) applyCodexStateKitWS(c *gin.Context, account *Account, turnState, model string) string {
	if s == nil || s.codexStateKit == nil || account == nil {
		return turnState
	}
	if !s.codexStateKit.globalEnabled() || !accountKitEnabled(account) {
		return turnState
	}
	if strings.TrimSpace(turnState) == "" {
		return turnState
	}
	model = strings.TrimSpace(model)
	if model == "" {
		return turnState
	}
	s.codexStateKit.mgr.RegisterModel(account.ID, model)
	s.codexStateKit.mgr.BindChatGPTAccount(account.ID, account.GetChatGPTAccountID())
	if token := s.codexStateKit.mgr.Peek(account.ID, model, time.Now()); token != "" {
		return token
	}
	return turnState
}

func openaiRequestModelFromContext(c *gin.Context) string {
	if c == nil {
		return ""
	}
	if raw, ok := c.Get("openai_parsed_request_body"); ok {
		switch body := raw.(type) {
		case map[string]any:
			if model, ok := body["model"].(string); ok {
				return strings.TrimSpace(model)
			}
		}
	}
	return ""
}

func (s *OpenAIGatewayService) captureCodexStateKitFromUpstream(account *Account, model, token string) {
	if s == nil || s.codexStateKit == nil || account == nil {
		return
	}
	if !s.codexStateKit.globalEnabled() || !accountKitEnabled(account) {
		return
	}
	model = strings.TrimSpace(model)
	token = strings.TrimSpace(token)
	if model == "" || token == "" {
		return
	}
	s.codexStateKit.mgr.BindChatGPTAccount(account.ID, account.GetChatGPTAccountID())
	s.codexStateKit.mgr.RegisterModel(account.ID, model)
	s.codexStateKit.mgr.Capture(account.ID, model, token, "upstream", time.Now())
}

func (s *OpenAIGatewayService) ensureCodexStateKit() *codexStateKitRuntime {
	if s == nil {
		return nil
	}
	if s.codexStateKit == nil {
		s.codexStateKit = newCodexStateKitRuntime(s)
	}
	return s.codexStateKit
}

func (s *OpenAIGatewayService) GetCodexStateKitStatus(ctx context.Context, accountID int64) (*CodexStateKitStatus, error) {
	if s == nil || s.accountRepo == nil {
		return nil, ErrAccountNotFound
	}
	s.ensureCodexStateKit()
	account, err := s.accountRepo.GetByID(ctx, accountID)
	if err != nil {
		return nil, err
	}
	if account == nil || !account.IsOpenAIOAuthLike() {
		return nil, ErrAccountNotFound
	}
	view := s.codexStateKit.mgr.View(accountID, time.Now())
	view.Enabled = accountKitEnabled(account)
	if bound := accountKitBoundLen(account); bound != nil {
		s.codexStateKit.mgr.SetBoundLen(accountID, bound)
		view.BoundTokenLen = *bound
	}
	return &CodexStateKitStatus{
		View:          view,
		GlobalEnabled: s.codexStateKit.globalEnabled(),
	}, nil
}

func (s *OpenAIGatewayService) RefreshCodexStateKitAccount(ctx context.Context, accountID int64) (*CodexStateKitStatus, error) {
	s.ensureCodexStateKit()
	account, err := s.accountRepo.GetByID(ctx, accountID)
	if err != nil {
		return nil, err
	}
	if account == nil || !account.IsOpenAIOAuthLike() {
		return nil, ErrAccountNotFound
	}
	if !s.codexStateKit.globalEnabled() {
		return s.GetCodexStateKitStatus(ctx, accountID)
	}
	s.codexStateKit.harvestAccount(ctx, account)
	return s.GetCodexStateKitStatus(ctx, accountID)
}

func (s *OpenAIGatewayService) UpdateCodexStateKitAccount(ctx context.Context, accountID int64, patch CodexStateKitUpdate) (*CodexStateKitStatus, error) {
	s.ensureCodexStateKit()
	account, err := s.accountRepo.GetByID(ctx, accountID)
	if err != nil {
		return nil, err
	}
	if account == nil || !account.IsOpenAIOAuthLike() {
		return nil, ErrAccountNotFound
	}
	updates := map[string]any{}
	if patch.Enabled != nil {
		updates[codexStateKitEnabledExtraKey] = *patch.Enabled
	}
	if patch.ClearBoundLen {
		updates[codexStateKitBoundLenExtraKey] = nil
		s.codexStateKit.mgr.SetBoundLen(accountID, nil)
	} else if patch.BoundTokenLen != nil {
		updates[codexStateKitBoundLenExtraKey] = *patch.BoundTokenLen
		s.codexStateKit.mgr.SetBoundLen(accountID, patch.BoundTokenLen)
	}
	if patch.Models != nil {
		updates[codexStateKitModelsExtraKey] = patch.Models
	}
	if len(updates) > 0 {
		if err := s.accountRepo.UpdateExtra(ctx, accountID, updates); err != nil {
			return nil, err
		}
	}
	return s.GetCodexStateKitStatus(ctx, accountID)
}
