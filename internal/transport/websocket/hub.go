package tradepush

import (
	"github.com/SPSingh09/zettabridge/internal/domain"
	"context"
	"encoding/json"
	"log"
	"sync"

	"github.com/gofiber/contrib/websocket"

	"github.com/SPSingh09/zettabridge/internal/store"
)

const sendBuffer = 16

// WebhookLookup resolves webhook ownership for fan-out.
type WebhookLookup interface {
	GetWebhookByID(ctx context.Context, id string) (*store.Webhook, error)
	ListOrgMembers(ctx context.Context, orgID string) ([]*store.OrgMemberProfile, error)
}

// Hub fans out trade inserts to connected WebSocket clients (in-process; single VM).
type Hub struct {
	pg WebhookLookup

	mu      sync.RWMutex
	clients map[string]map[*client]struct{} // userID -> clients
}

type client struct {
	userID string
	send   chan []byte
	// pong carries ping application-data from the read goroutine to the write
	// pump so that pong frames are sent by exactly one goroutine. The default
	// fasthttp/websocket ping handler writes directly to the connection from
	// the ReadMessage goroutine, which races with writePump and causes a panic.
	pong chan string
	done chan struct{}
	once sync.Once
}

// NewHub creates an in-process trade push hub.
func NewHub(pg WebhookLookup) *Hub {
	return &Hub{
		pg:      pg,
		clients: make(map[string]map[*client]struct{}),
	}
}

// Register adds a WebSocket connection for a user. Call the returned cleanup when
// the read side disconnects so the write pump goroutine can exit.
func (h *Hub) Register(userID string, conn *websocket.Conn) func() {
	c := &client{
		userID: userID,
		send:   make(chan []byte, sendBuffer),
		pong:   make(chan string, 1),
		done:   make(chan struct{}),
	}
	h.mu.Lock()
	if h.clients[userID] == nil {
		h.clients[userID] = make(map[*client]struct{})
	}
	h.clients[userID][c] = struct{}{}
	h.mu.Unlock()

	go h.writePump(conn, c)
	return func() { h.unregister(c) }
}

func (h *Hub) writePump(conn *websocket.Conn, c *client) {
	// Route pong responses through this goroutine so that ReadMessage (running
	// in the WsTrades goroutine) never writes to the connection directly. The
	// default ping handler calls WriteMessage from the read goroutine, which
	// races with data writes here and causes a "concurrent write" panic.
	conn.SetPingHandler(func(appData string) error {
		select {
		case c.pong <- appData:
		default: // drop if previous pong not yet sent; fine for keepalive
		}
		return nil
	})

	defer func() {
		h.unregister(c)
		_ = conn.Close()
	}()
	for {
		select {
		case <-c.done:
			return
		case data := <-c.pong:
			if err := conn.WriteMessage(websocket.PongMessage, []byte(data)); err != nil {
				return
			}
		case msg, ok := <-c.send:
			if !ok {
				return
			}
			if err := conn.WriteMessage(websocket.TextMessage, msg); err != nil {
				return
			}
		}
	}
}

func (h *Hub) unregister(c *client) {
	c.once.Do(func() {
		h.mu.Lock()
		if set := h.clients[c.userID]; set != nil {
			delete(set, c)
			if len(set) == 0 {
				delete(h.clients, c.userID)
			}
		}
		h.mu.Unlock()
		close(c.done)
	})
}

// OnTradeInserted implements queue.TradeNotifier.
func (h *Hub) OnTradeInserted(ctx context.Context, trade *store.Trade) {
	if trade == nil {
		return
	}
	wh, err := h.pg.GetWebhookByID(ctx, trade.WebhookID)
	if err != nil || wh == nil {
		log.Printf("tradepush: webhook lookup failed id=%s: %v", trade.WebhookID, err)
		return
	}

	payload, err := json.Marshal(map[string]interface{}{
		"type":  "trade",
		"trade": trade,
	})
	if err != nil {
		return
	}

	recipients := h.TradeRecipients(ctx, wh)

	for uid := range recipients {
		h.broadcast(uid, payload)
	}
}

// TradeRecipients returns user IDs that should receive a trade push for a webhook.
func (h *Hub) TradeRecipients(ctx context.Context, wh *store.Webhook) map[string]struct{} {
	recipients := map[string]struct{}{wh.UserID: {}}
	if wh.OrgID != nil {
		members, err := h.pg.ListOrgMembers(ctx, *wh.OrgID)
		if err != nil {
			log.Printf("tradepush: org members lookup failed org=%s: %v", *wh.OrgID, err)
		} else {
			for _, m := range members {
				if m != nil && m.Status == domain.StatusActive {
					recipients[m.UserID] = struct{}{}
				}
			}
		}
	}
	return recipients
}

// OnPaperMarksUpdated implements marketdata.MarkNotifier — pushes LTP updates
// to the paper account owner without requiring a linked webhook.
func (h *Hub) OnPaperMarksUpdated(ctx context.Context, account *store.PaperAccount, positions []*store.PaperPosition) {
	if account == nil || len(positions) == 0 {
		return
	}
	payload, err := json.Marshal(map[string]interface{}{
		"type":             "paper_mark",
		"paper_account_id": account.ID,
		"positions":        positions,
	})
	if err != nil {
		return
	}
	h.broadcast(account.UserID, payload)
}

func (h *Hub) broadcast(userID string, payload []byte) {
	h.mu.RLock()
	set := h.clients[userID]
	clients := make([]*client, 0, len(set))
	for c := range set {
		clients = append(clients, c)
	}
	h.mu.RUnlock()

	for _, c := range clients {
		select {
		case <-c.done:
		case c.send <- payload:
		default:
			// slow client — drop to avoid blocking worker
		}
	}
}

// ClientCount returns connected clients for tests.
func (h *Hub) ClientCount(userID string) int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.clients[userID])
}
