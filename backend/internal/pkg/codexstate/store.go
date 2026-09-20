package codexstate

import (
	"sort"
	"strings"
	"sync"
	"time"
)

// LenCount is a histogram bucket from one harvest round.
type LenCount struct {
	Len   int `json:"len"`
	Count int `json:"count"`
}

// PoolTokenInfo is a UI-facing pool entry.
type PoolTokenInfo struct {
	Len     int   `json:"len"`
	AgeSecs int64 `json:"ageSecs"`
	IsBound bool  `json:"isBound"`
	IsValid bool  `json:"isValid"`
}

// ModelView is the per-model status shown in the admin panel.
type ModelView struct {
	Model         string          `json:"model"`
	Status        string          `json:"status"`
	AgeSecs       *int64          `json:"ageSecs"`
	Len           *int            `json:"len"`
	CapturedAt    *string         `json:"capturedAt"`
	Distribution  []LenCount      `json:"distribution"`
	PoolTokens    []PoolTokenInfo `json:"poolTokens"`
	BoundOverride *int            `json:"boundOverride"`
}

// View is the admin status payload for one ChatGPT account.
type View struct {
	Status         string      `json:"status"`
	AgeSecs        *int64      `json:"ageSecs"`
	Len            *int        `json:"len"`
	Source         *string     `json:"source"`
	CapturedAt     *string     `json:"capturedAt"`
	Models         []ModelView `json:"models"`
	BoundTokenLen  int         `json:"boundTokenLen"`
	Enabled        bool        `json:"enabled"`
	AccountID      int64       `json:"accountId"`
	ChatGPTAccount string      `json:"chatgptAccountId,omitempty"`
}

type accountStore struct {
	tokens         map[string]Token
	pool           map[string][]Token
	activeModels   map[string]int64
	boundTokenLen  *int
	autoQualityLen *int
	modelBoundLens map[string]int
	distributions  map[string][]LenCount
	chatgptID      string
}

func newAccountStore() *accountStore {
	return &accountStore{
		tokens:         map[string]Token{},
		pool:           map[string][]Token{},
		activeModels:   map[string]int64{},
		modelBoundLens: map[string]int{},
		distributions:  map[string][]LenCount{},
	}
}

// Manager holds per-account Turn-State pools.
type Manager struct {
	mu       sync.Mutex
	accounts map[int64]*accountStore
}

func NewManager() *Manager {
	return &Manager{accounts: map[int64]*accountStore{}}
}

func (m *Manager) store(accountID int64) *accountStore {
	st, ok := m.accounts[accountID]
	if !ok {
		st = newAccountStore()
		m.accounts[accountID] = st
	}
	return st
}

func (m *Manager) BindChatGPTAccount(accountID int64, chatgptID string) {
	chatgptID = strings.TrimSpace(chatgptID)
	if accountID <= 0 || chatgptID == "" {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	st := m.store(accountID)
	if st.chatgptID == chatgptID {
		return
	}
	if st.chatgptID != "" {
		*st = *newAccountStore()
	}
	st.chatgptID = chatgptID
}

func (m *Manager) RegisterModel(accountID int64, model string) {
	model = strings.TrimSpace(model)
	if accountID <= 0 || model == "" {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.store(accountID).activeModels[model] = time.Now().Unix()
}

func (m *Manager) ActiveModels(accountID int64, now time.Time) []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	st := m.accounts[accountID]
	if st == nil {
		return nil
	}
	cutoff := now.Unix() - int64(ActiveWindow.Seconds())
	out := make([]string, 0, len(st.activeModels))
	for model, seen := range st.activeModels {
		if seen > cutoff {
			out = append(out, model)
		}
	}
	sort.Strings(out)
	return out
}

func (m *Manager) SetBoundLen(accountID int64, length *int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	st := m.store(accountID)
	st.boundTokenLen = length
	st.promoteAll(time.Now())
}

func (m *Manager) SetModelBoundLen(accountID int64, model string, length *int) {
	model = strings.TrimSpace(model)
	if model == "" {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	st := m.store(accountID)
	if length == nil {
		delete(st.modelBoundLens, model)
	} else {
		st.modelBoundLens[model] = *length
	}
	st.promoteModel(model, time.Now())
}

func (m *Manager) Peek(accountID int64, model string, now time.Time) string {
	model = strings.TrimSpace(model)
	if accountID <= 0 || model == "" {
		return ""
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	st := m.accounts[accountID]
	if st == nil {
		return ""
	}
	tok, ok := st.tokens[model]
	if !ok {
		return ""
	}
	if Age(tok.IssuedUnix, now) > MaxAge {
		return ""
	}
	return tok.Value
}

func (m *Manager) NeedsRefresh(accountID int64, model string, now time.Time) bool {
	model = strings.TrimSpace(model)
	m.mu.Lock()
	defer m.mu.Unlock()
	st := m.accounts[accountID]
	if st == nil {
		return true
	}
	tok, ok := st.tokens[model]
	if !ok {
		return true
	}
	return Age(tok.IssuedUnix, now) > PrefetchAge
}

func (m *Manager) CaptureBatch(accountID int64, model string, tokens []string, source string, now time.Time) int {
	model = strings.TrimSpace(model)
	if accountID <= 0 || model == "" {
		return 0
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	st := m.store(accountID)
	matched := 0
	target := st.boundLenFor(model)
	for _, raw := range tokens {
		if !st.storeToPool(model, raw, source, now) {
			continue
		}
		if MatchingLen(len(strings.TrimSpace(raw)), target) {
			matched++
		}
	}
	return matched
}

func (m *Manager) Capture(accountID int64, model, token, source string, now time.Time) bool {
	model = strings.TrimSpace(model)
	if accountID <= 0 || model == "" {
		return false
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.store(accountID).storeToPool(model, token, source, now)
}

func (m *Manager) RecordDistribution(accountID int64, model string, dist []LenCount) {
	model = strings.TrimSpace(model)
	if accountID <= 0 || model == "" {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.store(accountID).distributions[model] = dist
}

func (m *Manager) Invalidate(accountID int64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	st := m.accounts[accountID]
	if st == nil {
		return
	}
	st.tokens = map[string]Token{}
	st.pool = map[string][]Token{}
}

func (m *Manager) View(accountID int64, now time.Time) View {
	m.mu.Lock()
	defer m.mu.Unlock()
	st := m.accounts[accountID]
	view := View{Status: "empty", BoundTokenLen: QualityLen292, AccountID: accountID}
	if st == nil {
		return view
	}
	view.ChatGPTAccount = st.chatgptID
	view.BoundTokenLen = st.boundLen()
	models := make([]string, 0, len(st.activeModels)+len(st.tokens)+len(st.pool))
	seen := map[string]struct{}{}
	for model := range st.activeModels {
		if _, ok := seen[model]; ok {
			continue
		}
		seen[model] = struct{}{}
		models = append(models, model)
	}
	for model := range st.tokens {
		if _, ok := seen[model]; ok {
			continue
		}
		seen[model] = struct{}{}
		models = append(models, model)
	}
	for model := range st.pool {
		if _, ok := seen[model]; ok {
			continue
		}
		seen[model] = struct{}{}
		models = append(models, model)
	}
	sort.Strings(models)
	var newest *Token
	for _, model := range models {
		mv := st.modelView(model, now)
		view.Models = append(view.Models, mv)
		if tok, ok := st.tokens[model]; ok {
			if newest == nil || tok.IssuedUnix > newest.IssuedUnix {
				copyTok := tok
				newest = &copyTok
			}
		}
	}
	if newest != nil && Age(newest.IssuedUnix, now) <= MaxAge {
		age := int64(Age(newest.IssuedUnix, now).Seconds())
		length := newest.Len
		src := newest.Source
		captured := newest.CapturedAt
		view.Status = "ready"
		view.AgeSecs = &age
		view.Len = &length
		view.Source = &src
		view.CapturedAt = &captured
	} else if len(view.Models) > 0 {
		view.Status = "refreshing"
	}
	return view
}

func (s *accountStore) boundLen() int {
	if s.boundTokenLen != nil {
		return *s.boundTokenLen
	}
	if s.autoQualityLen != nil {
		return *s.autoQualityLen
	}
	return QualityLen292
}

func (s *accountStore) boundLenFor(model string) int {
	if v, ok := s.modelBoundLens[model]; ok {
		return v
	}
	return s.boundLen()
}

func (s *accountStore) storeToPool(model, raw, source string, now time.Time) bool {
	tok, ok := ParseToken(raw, source)
	if !ok {
		return false
	}
	entries := s.pool[model]
	replaced := false
	for i, existing := range entries {
		if MatchingLen(existing.Len, tok.Len) {
			entries[i] = tok
			replaced = true
			break
		}
	}
	if !replaced {
		entries = append(entries, tok)
	}
	if len(entries) > maxPoolVariants {
		sort.Slice(entries, func(i, j int) bool { return entries[i].IssuedUnix > entries[j].IssuedUnix })
		entries = entries[:maxPoolVariants]
	}
	s.pool[model] = entries
	s.observeQuality(tok.Len, now)
	target := s.boundLenFor(model)
	if MatchingLen(tok.Len, target) {
		s.tokens[model] = tok
	}
	s.cleanupPool(now)
	return true
}

func (s *accountStore) observeQuality(length int, now time.Time) {
	canon, ok := CanonicalQualityLen(length)
	if !ok {
		return
	}
	if s.autoQualityLen != nil && *s.autoQualityLen == canon {
		return
	}
	s.autoQualityLen = &canon
	if s.boundTokenLen == nil {
		s.promoteAll(now)
	}
}

func (s *accountStore) promoteAll(now time.Time) {
	for model := range s.pool {
		s.promoteModel(model, now)
	}
}

func (s *accountStore) promoteModel(model string, now time.Time) {
	target := s.boundLenFor(model)
	var best *Token
	for i := range s.pool[model] {
		tok := s.pool[model][i]
		if !MatchingLen(tok.Len, target) {
			continue
		}
		if Age(tok.IssuedUnix, now) > MaxAge {
			continue
		}
		if best == nil || tok.IssuedUnix > best.IssuedUnix {
			copyTok := tok
			best = &copyTok
		}
	}
	if best == nil {
		delete(s.tokens, model)
		return
	}
	s.tokens[model] = *best
}

func (s *accountStore) cleanupPool(now time.Time) {
	for model, entries := range s.pool {
		kept := entries[:0]
		for _, tok := range entries {
			if Age(tok.IssuedUnix, now) <= PoolMaxAge {
				kept = append(kept, tok)
			}
		}
		if len(kept) == 0 {
			delete(s.pool, model)
			continue
		}
		s.pool[model] = kept
	}
}

func (s *accountStore) modelView(model string, now time.Time) ModelView {
	mv := ModelView{Model: model, Status: "empty", Distribution: s.distributions[model]}
	if mv.Distribution == nil {
		mv.Distribution = []LenCount{}
	}
	target := s.boundLenFor(model)
	if override, ok := s.modelBoundLens[model]; ok {
		mv.BoundOverride = &override
	}
	if tok, ok := s.tokens[model]; ok {
		age := int64(Age(tok.IssuedUnix, now).Seconds())
		length := tok.Len
		captured := tok.CapturedAt
		mv.AgeSecs = &age
		mv.Len = &length
		mv.CapturedAt = &captured
		if Age(tok.IssuedUnix, now) <= MaxAge {
			mv.Status = "ready"
		} else {
			mv.Status = "expired"
		}
	}
	for _, tok := range s.pool[model] {
		age := int64(Age(tok.IssuedUnix, now).Seconds())
		mv.PoolTokens = append(mv.PoolTokens, PoolTokenInfo{
			Len:     tok.Len,
			AgeSecs: age,
			IsBound: MatchingLen(tok.Len, target),
			IsValid: Age(tok.IssuedUnix, now) <= MaxAge,
		})
	}
	if mv.PoolTokens == nil {
		mv.PoolTokens = []PoolTokenInfo{}
	}
	return mv
}
