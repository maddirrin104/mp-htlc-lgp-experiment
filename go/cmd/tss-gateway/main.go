package main

import (
	"encoding/json"
	"flag"
	"log"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	"mp-htlc-lgp/experiment/internal/tssnet"
)

var (
	coordinatorURL   = flag.String("coordinator", "ws://tss-coordinator:9000/ws", "coordinator websocket URL")
	clusterSession   = flag.String("session", "cluster", "coordinator room/session name")
	gatewayParty     = flag.String("party", "G", "gateway party id")
	defaultParties   = flag.String("parties", "P1,P2,P3", "comma-separated party IDs")
	defaultThreshold = flag.Int("threshold", 1, "threshold t for {t,n}")
	listenAddr       = flag.String("listen", ":9100", "http listen")
)

type server struct {
	wsMu sync.Mutex
	ws   *websocket.Conn

	// serialize requests: simplest & safest for experiments
	reqMu sync.Mutex

	in chan tssnet.WSMessage

	mu         sync.RWMutex
	lastPubKey string
	lastAddr   string
}

func main() {
	flag.Parse()
	log.SetFlags(log.LstdFlags | log.Lmicroseconds)

	s := &server{in: make(chan tssnet.WSMessage, 1024)}
	if err := s.connectWS(); err != nil {
		log.Fatalf("ws connect: %v", err)
	}
	go s.readLoop()

	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200); w.Write([]byte("ok")) })
	http.HandleFunc("/address", s.handleAddress)
	http.HandleFunc("/keygen", s.handleKeygen)
	http.HandleFunc("/signHash", s.handleSignHash)
	log.Printf("tss gateway listening on %s", *listenAddr)
	log.Fatal(http.ListenAndServe(*listenAddr, nil))
}

func (s *server) connectWS() error {
	u, err := url.Parse(*coordinatorURL)
	if err != nil {
		return err
	}
	c, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	if err != nil {
		return err
	}
	s.ws = c
	// hello
	return c.WriteMessage(websocket.TextMessage, tssnet.MustJSON(tssnet.WSMessage{Type: "hello", Session: *clusterSession, Party: *gatewayParty, Role: "gateway"}))
}

func partiesFromFlag() []string {
	parts := strings.Split(*defaultParties, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func (s *server) readLoop() {
	for {
		_, b, err := s.ws.ReadMessage()
		if err != nil {
			log.Fatalf("ws read: %v", err)
		}
		var m tssnet.WSMessage
		if err := json.Unmarshal(b, &m); err != nil {
			continue
		}
		if m.Type != "cmd" {
			continue
		}
		// cache address/pubkey on successful keygen
		if m.Cmd == "keygen_result" && m.Ok && m.AddrHex != "" {
			s.mu.Lock()
			s.lastPubKey = m.PubKeyHex
			s.lastAddr = m.AddrHex
			s.mu.Unlock()
		}
		select {
		case s.in <- m:
		default:
			// drop if overwhelmed
		}
	}
}

func (s *server) sendCmd(m tssnet.WSMessage) error {
	s.wsMu.Lock()
	defer s.wsMu.Unlock()
	return s.ws.WriteMessage(websocket.TextMessage, tssnet.MustJSON(m))
}

// --- HTTP handlers ---

func (s *server) handleAddress(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	s.mu.RLock()
	addr := s.lastAddr
	pub := s.lastPubKey
	s.mu.RUnlock()
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"address": addr, "pubkey": pub})
}

func (s *server) handleKeygen(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	s.reqMu.Lock()
	defer s.reqMu.Unlock()

	parties := partiesFromFlag()
	thr := *defaultThreshold
	start := time.Now()
	_ = s.sendCmd(tssnet.WSMessage{Type: "cmd", Session: *clusterSession, Party: *gatewayParty, Cmd: "keygen", Parties: parties, Threshold: thr})

	deadline := time.After(45 * time.Minute)
	byParty := map[string]tssnet.WSMessage{}
	for len(byParty) < len(parties) {
		select {
		case m := <-s.in:
			if m.Cmd != "keygen_result" {
				continue
			}
			byParty[m.Party] = m
		case <-deadline:
			w.WriteHeader(http.StatusGatewayTimeout)
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "err": "timeout", "received": len(byParty), "expected": len(parties)})
			return
		}
	}

	// sanity check: all ok, same addr
	var addr, pub string
	for _, p := range parties {
		m := byParty[p]
		if !m.Ok {
			w.WriteHeader(http.StatusBadGateway)
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "err": "party failed", "party": p, "detail": m.Err})
			return
		}
		if addr == "" {
			addr, pub = m.AddrHex, m.PubKeyHex
			continue
		}
		if m.AddrHex != "" && addr != "" && strings.ToLower(m.AddrHex) != strings.ToLower(addr) {
			w.WriteHeader(http.StatusBadGateway)
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "err": "address mismatch", "a": addr, "b": m.AddrHex, "party": p})
			return
		}
	}

	resp := map[string]any{
		"ok": true,
		"address": addr,
		"pubkey": pub,
		"threshold": thr,
		"parties": parties,
		"t_keygen_ms": time.Since(start).Milliseconds(),
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

func (s *server) handleSignHash(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	s.reqMu.Lock()
	defer s.reqMu.Unlock()

	var req struct {
		HashHex string `json:"hash_hex"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)
	if req.HashHex == "" {
		req.HashHex = r.URL.Query().Get("hash_hex")
	}
	if req.HashHex == "" {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "err": "missing hash_hex"})
		return
	}

	parties := partiesFromFlag()
	thr := *defaultThreshold
	start := time.Now()
	_ = s.sendCmd(tssnet.WSMessage{Type: "cmd", Session: *clusterSession, Party: *gatewayParty, Cmd: "sign", Parties: parties, Threshold: thr, HashHex: req.HashHex})

	deadline := time.After(15 * time.Minute)
	for {
		select {
		case m := <-s.in:
			if m.Cmd != "sign_result" {
				continue
			}
			if !m.Ok {
				w.WriteHeader(http.StatusBadGateway)
				_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "err": m.Err, "party": m.Party})
				return
			}
			resp := map[string]any{"ok": true, "r": m.RHex, "s": m.SHex, "party": m.Party, "t_sign_ms": time.Since(start).Milliseconds()}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(resp)
			return
		case <-deadline:
			w.WriteHeader(http.StatusGatewayTimeout)
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "err": "timeout"})
			return
		}
	}
}
