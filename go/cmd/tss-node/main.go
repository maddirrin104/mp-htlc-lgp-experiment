package main

import (
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"math/big"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	"github.com/bnb-chain/tss-lib/v2/common"
	"github.com/bnb-chain/tss-lib/v2/ecdsa/keygen"
	"github.com/bnb-chain/tss-lib/v2/ecdsa/signing"
	"github.com/bnb-chain/tss-lib/v2/tss"

	"github.com/ethereum/go-ethereum/crypto"

	"mp-htlc-lgp/experiment/internal/tssnet"
)

var (
	coordinatorURL = flag.String("coordinator", "ws://tss-coordinator:9000/ws", "coordinator websocket URL")
	clusterSession = flag.String("session", "cluster", "coordinator room/session name")
	partyStr       = flag.String("party", "P1", "this node party id")
	dataDir        = flag.String("data", "/data", "data directory")
	gatewayParty   = flag.String("gateway", "G", "gateway party id")
)

type runtime struct {
	mu        sync.Mutex
	busy      bool
	party     tss.Party
	idMap     map[string]*tss.PartyID
	parties   []string
	threshold int
}

func main() {
	flag.Parse()
	log.SetFlags(log.LstdFlags | log.Lmicroseconds)

	u, err := url.Parse(*coordinatorURL)
	if err != nil {
		log.Fatalf("bad coordinator url: %v", err)
	}

	c, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	if err != nil {
		log.Fatalf("dial: %v", err)
	}
	defer c.Close()

	// hello
	_ = c.WriteMessage(websocket.TextMessage, tssnet.MustJSON(tssnet.WSMessage{Type: "hello", Session: *clusterSession, Party: *partyStr, Role: "node"}))

	rt := &runtime{}

	for {
		_, b, err := c.ReadMessage()
		if err != nil {
			log.Fatalf("read: %v", err)
		}
		var m tssnet.WSMessage
		if err := json.Unmarshal(b, &m); err != nil {
			continue
		}
		switch m.Type {
		case "send":
			handleWire(rt, m)
		case "cmd":
			handleCmd(rt, c, m)
		}
	}
}

func handleCmd(rt *runtime, c *websocket.Conn, m tssnet.WSMessage) {
	// only act if we're in the party set (if provided)
	if len(m.Parties) > 0 {
		mine := false
		for _, p := range m.Parties {
			if p == *partyStr {
				mine = true
				break
			}
		}
		if !mine {
			return
		}
	}

	rt.mu.Lock()
	if rt.busy {
		rt.mu.Unlock()
		log.Printf("busy; ignoring cmd=%s", m.Cmd)
		return
	}
	rt.busy = true
	rt.mu.Unlock()

	go func() {
		defer func() {
			rt.mu.Lock()
			rt.busy = false
			rt.party = nil
			rt.idMap = nil
			rt.mu.Unlock()
		}()

		switch m.Cmd {
		case "keygen":
				if err := runKeygen(rt, c, m.Parties, m.Threshold); err != nil {
					sendResult(c, tssnet.WSMessage{Type: "cmd", Session: *clusterSession, Party: *partyStr, Parties: []string{*gatewayParty}, Cmd: "keygen_result", Ok: false, Err: errString(err)})
				}
		case "sign":
			err := runSign(rt, c, m.Parties, m.Threshold, m.HashHex)
			// runSign itself will send sign_result (with r,s) if ok
			if err != nil {
				sendResult(c, tssnet.WSMessage{Type: "cmd", Session: *clusterSession, Party: *partyStr, Parties: []string{*gatewayParty}, Cmd: "sign_result", Ok: false, Err: errString(err)})
			}
		default:
			log.Printf("unknown cmd: %s", m.Cmd)
		}
	}()
}

func handleWire(rt *runtime, m tssnet.WSMessage) {
	rt.mu.Lock()
	p := rt.party
	idMap := rt.idMap
	rt.mu.Unlock()
	if p == nil || idMap == nil {
		return
	}
	wireBytes, err := base64.StdEncoding.DecodeString(m.PayloadB64)
	if err != nil {
		return
	}
	from := idMap[m.From]
	if from == nil {
		return
	}
	_, terr := p.UpdateFromBytes(wireBytes, from, m.Bcast)
	if terr != nil {
		log.Printf("UpdateFromBytes error: %v", terr)
	}
}

func runKeygen(rt *runtime, c *websocket.Conn, parties []string, threshold int) error {
	if len(parties) == 0 {
		return errors.New("empty parties")
	}
	if threshold <= 0 || threshold >= len(parties) {
		return fmt.Errorf("bad threshold=%d for n=%d", threshold, len(parties))
	}
	thisID := strings.TrimSpace(*partyStr)
	partyIDs, idMap, thisParty, err := makeParties(parties, thisID)
	if err != nil {
		return err
	}
	ctx := tss.NewPeerContext(partyIDs)
	params := tss.NewParameters(tss.S256(), ctx, thisParty, len(partyIDs), threshold)

	outCh := make(chan tss.Message, 1024)
	endCh := make(chan keygen.LocalPartySaveData, 1)

	// omit preParams => library computes in round 1
	local := keygen.NewLocalParty(params, outCh, endCh)

	rt.mu.Lock()
	rt.party = local
	rt.idMap = idMap
	rt.parties = parties
	rt.threshold = threshold
	rt.mu.Unlock()

	go func() {
		if err := local.Start(); err != nil {
			log.Printf("party.Start error: %v", err)
		}
	}()

	// forward outCh messages
	for {
		select {
		case msg := <-outCh:
			wire, routing, err := msg.WireBytes()
			if err != nil {
				continue
			}
			to := routeToStrings(parties, routing, thisID)
			sendWire(c, to, routing.From.Id, routing.IsBroadcast, wire)
		case save := <-endCh:
			// persist key shares locally
			if err := persistKeygen(*dataDir, save); err != nil {
				log.Printf("persist keygen error: %v", err)
			}
			pub := crypto.FromECDSAPub(save.ECDSAPub)
			addr := crypto.PubkeyToAddress(*save.ECDSAPub).Hex()
			// send a richer result to gateway
			sendResult(c, tssnet.WSMessage{Type: "cmd", Session: *clusterSession, Party: thisID, Parties: []string{*gatewayParty}, Cmd: "keygen_result", Ok: true, PubKeyHex: "0x" + hex.EncodeToString(pub), AddrHex: addr})
			return nil
		case <-time.After(30 * time.Minute):
			return errors.New("keygen timeout")
		}
	}
}

func runSign(rt *runtime, c *websocket.Conn, parties []string, threshold int, hashHex string) error {
	if len(parties) == 0 {
		return errors.New("empty parties")
	}
	thisID := strings.TrimSpace(*partyStr)
	partyIDs, idMap, thisParty, err := makeParties(parties, thisID)
	if err != nil {
		return err
	}
	ctx := tss.NewPeerContext(partyIDs)
	params := tss.NewParameters(tss.S256(), ctx, thisParty, len(partyIDs), threshold)

	keyData, err := loadKeygen(*dataDir)
	if err != nil {
		return err
	}
	hashBytes, err := decode32(hashHex)
	if err != nil {
		return err
	}
	msgInt := new(big.Int).SetBytes(hashBytes)

	outCh := make(chan tss.Message, 1024)
	endCh := make(chan *common.SignatureData, 1)
	local := signing.NewLocalParty(msgInt, params, keyData, outCh, endCh)

	rt.mu.Lock()
	rt.party = local
	rt.idMap = idMap
	rt.parties = parties
	rt.threshold = threshold
	rt.mu.Unlock()

	go func() {
		if err := local.Start(); err != nil {
			log.Printf("sign party.Start error: %v", err)
		}
	}()

	for {
		select {
		case msg := <-outCh:
			wire, routing, err := msg.WireBytes()
			if err != nil {
				continue
			}
			to := routeToStrings(parties, routing, thisID)
			sendWire(c, to, routing.From.Id, routing.IsBroadcast, wire)
		case sig := <-endCh:
			if sig == nil {
				return errors.New("nil signature")
			}
			sendResult(c, tssnet.WSMessage{Type: "cmd", Session: *clusterSession, Party: thisID, Parties: []string{*gatewayParty}, Cmd: "sign_result", Ok: true, RHex: "0x" + hex.EncodeToString(sig.R), SHex: "0x" + hex.EncodeToString(sig.S)})
			return nil
		case <-time.After(10 * time.Minute):
			return errors.New("sign timeout")
		}
	}
}

func sendWire(c *websocket.Conn, to []string, from string, bcast bool, wire []byte) {
	m := tssnet.WSMessage{Type: "send", Session: *clusterSession, Party: *partyStr, From: from, To: to, Bcast: bcast, PayloadB64: base64.StdEncoding.EncodeToString(wire)}
	_ = c.WriteMessage(websocket.TextMessage, tssnet.MustJSON(m))
}

func sendResult(c *websocket.Conn, m tssnet.WSMessage) {
	_ = c.WriteMessage(websocket.TextMessage, tssnet.MustJSON(m))
}

func routeToStrings(all []string, routing *tss.MessageRouting, self string) []string {
	if routing == nil {
		return []string{"*"}
	}
	if routing.IsBroadcast {
		to := make([]string, 0, len(all))
		for _, p := range all {
			if p != self {
				to = append(to, p)
			}
		}
		return to
	}
	to := make([]string, 0, len(routing.To))
	for _, p := range routing.To {
		if p != nil {
			to = append(to, p.Id)
		}
	}
	return to
}

func makeParties(parties []string, self string) ([]*tss.PartyID, map[string]*tss.PartyID, *tss.PartyID, error) {
	partyIDs := make([]*tss.PartyID, 0, len(parties))
	idMap := map[string]*tss.PartyID{}
	var this *tss.PartyID
	for i, id := range parties {
		id = strings.TrimSpace(id)
		uid := big.NewInt(int64(i + 1))
		pid := tss.NewPartyID(id, id, uid)
		partyIDs = append(partyIDs, pid)
		idMap[id] = pid
		if id == self {
			this = pid
		}
	}
	if this == nil {
		return nil, nil, nil, fmt.Errorf("self party %s not in parties", self)
	}
	return partyIDs, idMap, this, nil
}

func mustPartyIDs(parties []string) []*tss.PartyID {
	ids, _, _, _ := makeParties(parties, parties[0]) // hack: returns list; ignore this
	return ids
}

func persistKeygen(dir string, save keygen.LocalPartySaveData) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(save, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "keygen.json"), b, 0o600)
}

func loadKeygen(dir string) (keygen.LocalPartySaveData, error) {
	b, err := os.ReadFile(filepath.Join(dir, "keygen.json"))
	if err != nil {
		return keygen.LocalPartySaveData{}, err
	}
	var save keygen.LocalPartySaveData
	if err := json.Unmarshal(b, &save); err != nil {
		return keygen.LocalPartySaveData{}, err
	}
	return save, nil
}

func decode32(h string) ([]byte, error) {
	h = strings.TrimPrefix(h, "0x")
	b, err := hex.DecodeString(h)
	if err != nil {
		return nil, err
	}
	if len(b) != 32 {
		return nil, fmt.Errorf("hash must be 32 bytes, got %d", len(b))
	}
	return b, nil
}

func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
