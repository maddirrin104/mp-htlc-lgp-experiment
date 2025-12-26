package main

import (
	"encoding/json"
	"log"
	"net/http"
	"sync"

	"github.com/gorilla/websocket"

	"mp-htlc-lgp/experiment/internal/tssnet"
)

type hub struct {
	mu       sync.RWMutex
	sessions map[string]map[string]*websocket.Conn // session -> party -> conn
	roles    map[string]map[string]string          // session -> party -> role
	lastCmd  map[string]*tssnet.WSMessage          // session -> last cmd (best-effort)
}

func newHub() *hub {
	return &hub{
		sessions: map[string]map[string]*websocket.Conn{},
		roles:    map[string]map[string]string{},
		lastCmd:  map[string]*tssnet.WSMessage{},
	}
}

var upgrader = websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}

func (h *hub) add(session, party, role string, c *websocket.Conn) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if _, ok := h.sessions[session]; !ok {
		h.sessions[session] = map[string]*websocket.Conn{}
		h.roles[session] = map[string]string{}
	}
	h.sessions[session][party] = c
	h.roles[session][party] = role
	if cmd := h.lastCmd[session]; cmd != nil {
		_ = c.WriteMessage(websocket.TextMessage, tssnet.MustJSON(*cmd))
	}
}

func (h *hub) remove(session, party string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if m, ok := h.sessions[session]; ok {
		delete(m, party)
		delete(h.roles[session], party)
		if len(m) == 0 {
			delete(h.sessions, session)
			delete(h.roles, session)
			delete(h.lastCmd, session)
		}
	}
}

func (h *hub) send(session string, to []string, msg tssnet.WSMessage) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	m := h.sessions[session]
	if len(m) == 0 {
		return
	}
	b := tssnet.MustJSON(msg)
	if len(to) == 0 || (len(to) == 1 && to[0] == "*") {
		for _, c := range m {
			_ = c.WriteMessage(websocket.TextMessage, b)
		}
		return
	}
	for _, p := range to {
		if c, ok := m[p]; ok {
			_ = c.WriteMessage(websocket.TextMessage, b)
		}
	}
}

func (h *hub) handleWS(w http.ResponseWriter, r *http.Request) {
	c, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("upgrade: %v", err)
		return
	}
	defer c.Close()

	// hello
	_, hb, err := c.ReadMessage()
	if err != nil {
		return
	}
	var hello tssnet.WSMessage
	if err := json.Unmarshal(hb, &hello); err != nil {
		return
	}
	if hello.Type != "hello" || hello.Session == "" || hello.Party == "" {
		return
	}
	session := hello.Session
	party := hello.Party
	h.add(session, party, hello.Role, c)
	log.Printf("join session=%s party=%s role=%s", session, party, hello.Role)
	defer func() {
		h.remove(session, party)
		log.Printf("leave session=%s party=%s", session, party)
	}()

	for {
		_, b, err := c.ReadMessage()
		if err != nil {
			return
		}
		var m tssnet.WSMessage
		if err := json.Unmarshal(b, &m); err != nil {
			continue
		}
		if m.Session == "" {
			m.Session = session
		}
		switch m.Type {
		case "send":
			h.send(m.Session, m.To, m)
		case "cmd":
			// best-effort: remember last cmd for late joiners
			h.mu.Lock()
			cpy := m
			h.lastCmd[m.Session] = &cpy
			h.mu.Unlock()
			h.send(m.Session, m.Parties, m) // if Parties empty => nothing, nodes should join first
			if len(m.Parties) == 0 {
				h.send(m.Session, []string{"*"}, m)
			}
		case "ping":
			_ = c.WriteMessage(websocket.TextMessage, tssnet.MustJSON(tssnet.WSMessage{Type: "pong", Session: session, Party: party}))
		}
	}
}

func main() {
	h := newHub()
	http.HandleFunc("/ws", h.handleWS)
	log.Printf("tss coordinator listening on :9000/ws")
	log.Fatal(http.ListenAndServe(":9000", nil))
}
