package service

import (
	"context"
	"crypto/sha256"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
	"golang.org/x/net/publicsuffix"
)

const openAIGuardianRoutingPinLimit = 4096

type openAIGuardianRoutingPin struct {
	endpoint   *url.URL
	jar        *cookiejar.Jar
	credential [sha256.Size]byte
}

// A pin is tied to the bearer and workspace used on the wire. If credentials
// rotate between the parent response and the review, the review simply dials
// without an old route hint.
func openAIGuardianRoutingCredential(headers http.Header) [sha256.Size]byte {
	return sha256.Sum256([]byte(headers.Get("Authorization") + "\x00" + headers.Get("ChatGPT-Account-Id")))
}

func openAIGuardianRoutingEndpoint(rawURL string) *url.URL {
	endpoint, err := url.Parse(rawURL)
	if err != nil || endpoint.Scheme != "wss" || endpoint.User != nil {
		return nil
	}
	host := strings.ToLower(endpoint.Hostname())
	switch {
	case host == "chatgpt.com", host == "chat.openai.com", host == "chatgpt-staging.com":
	case strings.HasSuffix(host, ".chatgpt.com"), strings.HasSuffix(host, ".chatgpt-staging.com"):
	default:
		return nil
	}
	port := endpoint.Port()
	endpoint.Scheme = "https"
	endpoint.Host = host
	if port != "" && port != "443" {
		endpoint.Host = net.JoinHostPort(host, port)
	}
	return endpoint
}

func captureOpenAIGuardianRoutingPin(account *Account, target string, sent, response http.Header) *openAIGuardianRoutingPin {
	if !account.UsesOpenAICodexProtocol() || sent.Get("Authorization") == "" {
		return nil
	}
	endpoint := openAIGuardianRoutingEndpoint(target)
	if endpoint == nil {
		return nil
	}
	var routingHeaders [2]string
	for _, raw := range response.Values("Set-Cookie") {
		name, _, ok := strings.Cut(raw, "=")
		if !ok {
			continue
		}
		var slot int
		switch strings.TrimSpace(name) {
		case "__oailb":
			slot = 0
		case "__cflb":
			slot = 1
		default:
			continue
		}
		if routingHeaders[slot] != "" || len(raw) > 8192 {
			return nil
		}
		routingHeaders[slot] = raw
	}
	var cookies []*http.Cookie
	for _, raw := range routingHeaders {
		if cookie, err := http.ParseSetCookie(raw); err == nil {
			cookies = append(cookies, cookie)
		}
	}
	if len(cookies) == 0 {
		return nil
	}
	jar, err := cookiejar.New(&cookiejar.Options{PublicSuffixList: publicsuffix.List})
	if err != nil {
		return nil
	}
	jar.SetCookies(endpoint, cookies)
	if len(jar.Cookies(endpoint)) == 0 {
		return nil
	}
	return &openAIGuardianRoutingPin{endpoint: endpoint, jar: jar, credential: openAIGuardianRoutingCredential(sent)}
}

func (pin *openAIGuardianRoutingPin) endpointFor(target string, headers http.Header) *url.URL {
	if pin == nil || pin.credential != openAIGuardianRoutingCredential(headers) {
		return nil
	}
	endpoint := openAIGuardianRoutingEndpoint(target)
	if endpoint == nil || endpoint.Host != pin.endpoint.Host || endpoint.EscapedPath() != pin.endpoint.EscapedPath() {
		return nil
	}
	return endpoint
}

func (pin *openAIGuardianRoutingPin) apply(target string, headers http.Header) {
	endpoint := pin.endpointFor(target, headers)
	if endpoint == nil {
		return
	}
	// Outbound Codex handshakes have no client-sourced Cookie header. Copy
	// only the independently captured routing cookies.
	request := http.Request{Header: headers}
	for _, cookie := range pin.jar.Cookies(endpoint) {
		request.AddCookie(cookie)
	}
}

type openAIGuardianRoutingKey struct {
	caller, group, account int64
	responseID             string
}

type openAIGuardianRoutingBinding struct {
	pin       *openAIGuardianRoutingPin
	thread    string
	expiresAt time.Time
}

type openAIGuardianRoutingStore struct {
	mu       sync.Mutex
	bindings map[openAIGuardianRoutingKey]openAIGuardianRoutingBinding
	next     int
	order    []openAIGuardianRoutingKey
}

type openAIGuardianSourceThreadKey struct{}

func openAIGuardianSourceThread(c *gin.Context, body []byte) string {
	if c == nil || c.Request == nil {
		return ""
	}
	metadata := gjson.GetBytes(body, "client_metadata")
	threads := append([]string(nil), c.Request.Header.Values("Thread-Id")...)
	for _, embedded := range []string{c.GetHeader(codexTurnMetadataHeader), metadata.Get("x-codex-turn-metadata").String()} {
		threads = append(threads, gjson.Get(embedded, "thread_id").String())
	}
	threads = append(threads, metadata.Get("thread_id").String())
	thread := ""
	for _, candidate := range threads {
		candidate = strings.TrimSpace(candidate)
		if candidate == "" {
			continue
		}
		if len(candidate) > 256 || (thread != "" && candidate != thread) {
			return ""
		}
		thread = candidate
	}
	return thread
}

func openAIGuardianRoutingScope(c *gin.Context, account *Account, responseID string) openAIGuardianRoutingKey {
	if c == nil || account == nil || account.ID <= 0 || len(responseID) > 256 ||
		ClassifyOpenAIPreviousResponseIDKind(responseID) != OpenAIPreviousResponseIDKindResponseID {
		return openAIGuardianRoutingKey{}
	}
	var groupID int64
	apiKey, _ := c.Get("api_key")
	key, _ := apiKey.(*APIKey)
	if key == nil || key.ID <= 0 {
		return openAIGuardianRoutingKey{}
	}
	if key.GroupID != nil {
		groupID = *key.GroupID
	}
	return openAIGuardianRoutingKey{caller: key.ID, group: groupID, account: account.ID, responseID: responseID}
}

func (s *OpenAIGatewayService) bindOpenAIGuardianParentRouting(c *gin.Context, account *Account, pin *openAIGuardianRoutingPin, eventType, responseID, thread string) {
	if pin == nil || eventType != "response.created" || thread == "" {
		return
	}
	key := openAIGuardianRoutingScope(c, account, responseID)
	if key.caller == 0 || len(pin.jar.Cookies(pin.endpoint)) == 0 {
		return
	}
	store := &s.openaiGuardianRouting
	store.mu.Lock()
	defer store.mu.Unlock()
	if store.bindings == nil {
		store.bindings = make(map[openAIGuardianRoutingKey]openAIGuardianRoutingBinding)
		store.order = make([]openAIGuardianRoutingKey, openAIGuardianRoutingPinLimit)
	}
	if _, found := store.bindings[key]; !found {
		delete(store.bindings, store.order[store.next])
		store.order[store.next] = key
		store.next = (store.next + 1) % len(store.order)
	}
	store.bindings[key] = openAIGuardianRoutingBinding{pin: pin, thread: thread, expiresAt: time.Now().Add(s.openAIWSResponseStickyTTL())}
}

func (s *OpenAIGatewayService) openAIGuardianReviewerRouting(ctx context.Context, c *gin.Context, account *Account, target string, headers http.Header, parentResponseID string) *openAIGuardianRoutingPin {
	if c == nil || c.Request == nil || !account.UsesOpenAICodexProtocol() {
		return nil
	}
	reviewer := c.Request.Header.Values("X-Codex-Guardian")
	if len(reviewer) != 1 || reviewer[0] != "reviewer" {
		return nil
	}
	affinity, ok := openAIGuardianParentAffinityFromContext(ctx)
	if !ok {
		return nil
	}
	key := openAIGuardianRoutingScope(c, account, parentResponseID)
	if key.caller == 0 {
		return nil
	}
	store := &s.openaiGuardianRouting
	store.mu.Lock()
	binding := store.bindings[key]
	store.mu.Unlock()
	// Keep the same pin identity on a live reviewer socket after cookie
	// expiry. A new dial still checks the jar and sends only live cookies.
	if binding.thread != affinity.parentThread || !time.Now().Before(binding.expiresAt) || binding.pin.endpointFor(target, headers) == nil {
		return nil
	}
	return binding.pin
}

const (
	codexAutoReviewModel      = "codex-auto-review"
	openAISubagentHeader      = "x-openai-subagent"
	codexParentThreadIDHeader = "x-codex-parent-thread-id"
	codexTurnMetadataHeader   = "x-codex-turn-metadata"
)

type openAIGuardianParentAffinityContextKey struct{}

type openAIGuardianParentAffinity struct {
	parentThread       string
	currentSessionHash string
	legacySessionHash  string
}

// WithOpenAIGuardianParentAffinity records a Codex review request's parent
// thread as a routing hint. The hint is resolved against the current group's
// sticky-session namespace later; client headers never carry an account ID.
func WithOpenAIGuardianParentAffinity(ctx context.Context, c *gin.Context, body []byte, requestedModel string) context.Context {
	if ctx == nil || c == nil || !strings.EqualFold(strings.TrimSpace(requestedModel), codexAutoReviewModel) {
		return ctx
	}

	headerMetadata := c.GetHeader(codexTurnMetadataHeader)
	bodyMetadata := openAIRequestPayloadView(body).Get("client_metadata.x-codex-turn-metadata").String()
	if !hasUnambiguousOpenAICodexReviewSubagent(
		c.GetHeader(openAISubagentHeader),
		codexSubagentKindFromMetadata(headerMetadata),
		codexSubagentKindFromMetadata(bodyMetadata),
	) {
		return ctx
	}

	parentID := ""
	for _, candidate := range []string{
		strings.TrimSpace(c.GetHeader(codexParentThreadIDHeader)),
		codexParentThreadIDFromMetadata(headerMetadata),
		codexParentThreadIDFromMetadata(bodyMetadata),
	} {
		if candidate == "" {
			continue
		}
		if parentID != "" && parentID != candidate {
			return ctx
		}
		parentID = candidate
	}
	if parentID == "" {
		return ctx
	}

	currentHash, legacyHash := deriveOpenAISessionHashes(parentID)
	if currentHash == "" {
		return ctx
	}
	return context.WithValue(ctx, openAIGuardianParentAffinityContextKey{}, openAIGuardianParentAffinity{
		parentThread:       parentID,
		currentSessionHash: currentHash,
		legacySessionHash:  legacyHash,
	})
}

func codexParentThreadIDFromMetadata(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" || !gjson.Valid(raw) {
		return ""
	}
	return strings.TrimSpace(gjson.Get(raw, "parent_thread_id").String())
}

func codexSubagentKindFromMetadata(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" || !gjson.Valid(raw) {
		return ""
	}
	return strings.TrimSpace(gjson.Get(raw, "subagent_kind").String())
}

func hasUnambiguousOpenAICodexReviewSubagent(candidates ...string) bool {
	subagent := ""
	for _, candidate := range candidates {
		candidate = strings.ToLower(strings.TrimSpace(candidate))
		if candidate == "" {
			continue
		}
		if subagent != "" && subagent != candidate {
			return false
		}
		subagent = candidate
	}
	return subagent == "guardian" || subagent == "review"
}

func openAIGuardianParentAffinityFromContext(ctx context.Context) (openAIGuardianParentAffinity, bool) {
	if ctx == nil {
		return openAIGuardianParentAffinity{}, false
	}
	affinity, ok := ctx.Value(openAIGuardianParentAffinityContextKey{}).(openAIGuardianParentAffinity)
	return affinity, ok && affinity.currentSessionHash != ""
}

func preserveOpenAIGuardianParentBinding(ctx context.Context, sessionHash string) bool {
	affinity, ok := openAIGuardianParentAffinityFromContext(ctx)
	if !ok {
		return false
	}
	sessionHash = strings.TrimSpace(sessionHash)
	return sessionHash != "" && (sessionHash == affinity.currentSessionHash || sessionHash == affinity.legacySessionHash)
}

func (s *OpenAIGatewayService) resolveOpenAIGuardianParentAccountID(ctx context.Context, groupID *int64) int64 {
	if s == nil || s.cache == nil {
		return 0
	}
	affinity, ok := openAIGuardianParentAffinityFromContext(ctx)
	if !ok {
		return 0
	}
	lookupCtx := withOpenAILegacySessionHash(ctx, affinity.legacySessionHash)
	accountID, err := s.getStickySessionAccountID(lookupCtx, groupID, affinity.currentSessionHash)
	if err != nil || accountID <= 0 {
		return 0
	}
	return accountID
}
