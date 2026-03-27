package orchestrator

import (
	"context"
	"encoding/hex"
	"fmt"
	"log"
	"math/big"
	"sort"
	"strings"
	"sync"
	"time"

	tsscommon "github.com/bnb-chain/tss-lib/v2/common"
	kg "github.com/bnb-chain/tss-lib/v2/ecdsa/keygen"
	signing "github.com/bnb-chain/tss-lib/v2/ecdsa/signing"
	"github.com/bnb-chain/tss-lib/v2/tss"
	gethcrypto "github.com/ethereum/go-ethereum/crypto"

	"mphtlc-lgp-mpc/internal/config"
	"mphtlc-lgp-mpc/internal/policy"
	"mphtlc-lgp-mpc/internal/storage"
	"mphtlc-lgp-mpc/internal/tssengine"
	"mphtlc-lgp-mpc/internal/tssutil"
)

type session struct {
	id           string
	protocol     Protocol
	participants []string
	parties      tss.SortedPartyIDs // LƯU TRỮ CON TRỎ GỐC CỦA DANH SÁCH
	status       SessionStatus
	err          string
	startedAt    time.Time
	completedAt  *time.Time
	executionMS  int64
	digest       []byte
	digestHex    string
	signature    string
	recovered    string
	pubKeyHex    string
	ethAddress   string

	party       tss.Party
	outCh       chan tss.Message
	keygenEndCh chan *kg.LocalPartySaveData
	signEndCh   chan *tsscommon.SignatureData
	cancel      context.CancelFunc
}

type Manager struct {
	cfg          config.HTTPConfig
	localPartyID *tss.PartyID
	peerClient   *HTTPPeerClient
	mu           sync.RWMutex
	sessions     map[string]*session
	peers        map[string]string
}

func NewManager(cfg config.HTTPConfig) *Manager {
	peers := make(map[string]string, len(cfg.Peers))
	for _, p := range cfg.Peers {
		peers[p.ID] = p.BaseURL
	}

	return &Manager{
		cfg:          cfg,
		localPartyID: tssutil.BuildPartyID(cfg.NodeID),
		peerClient:   NewHTTPPeerClient(cfg.HTTPTimeout),
		sessions:     map[string]*session{},
		peers:        peers,
	}
}

func (m *Manager) Health() map[string]any {
	return map[string]any{
		"node_id": m.cfg.NodeID,
		"status":  "ok",
		"time":    time.Now().UTC(),
	}
}

func (m *Manager) StartKeygen(ctx context.Context, req KeygenStartRequest) (*SessionResponse, error) {
	initReq := SessionInitRequest{
		SessionID:       req.SessionID,
		Protocol:        ProtocolKeygen,
		Participants:    req.Participants,
		Threshold:       req.Threshold,
		InitiatorNodeID: m.cfg.NodeID,
	}

	if err := m.initOnAllPeers(ctx, req.Participants, initReq); err != nil {
		return nil, err
	}
	if err := m.startLocal(req.SessionID); err != nil {
		return nil, err
	}
	return m.GetSession(req.SessionID)
}

func (m *Manager) StartSign(ctx context.Context, req SignStartRequest) (*SessionResponse, error) {
	digest := gethcrypto.Keccak256([]byte(req.Message)) // raw keccak256, no EIP-191 prefix

	penCfg := config.PenaltyConfig{
		TimeLockSeconds:      req.TimeLockUnix - req.ProtocolStartUnix,
		PenaltyWindowSeconds: req.PenaltyWindowSec,
		ExpectedSignLatency:  time.Duration(req.EstimatedMPCMs) * time.Millisecond,
		SafetyMargin:         2 * time.Second,
		WarnOnly:             !req.StrictPolicy,
	}

	eval, err := policy.EvaluatePenaltyWindow(
		penCfg,
		time.Unix(req.ProtocolStartUnix, 0),
		time.Now(),
	)
	if err != nil {
		return nil, err
	}

	log.Printf(
		"[policy] session=%s decision=%s msg=%q projected_penalty=%.4f",
		req.SessionID, eval.Decision, eval.Message, eval.ProjectedPenalty,
	)

	if eval.Decision == policy.DecisionReject {
		return nil, fmt.Errorf("policy rejected signing: %s", eval.Message)
	}

	initReq := SessionInitRequest{
		SessionID:       req.SessionID,
		Protocol:        ProtocolSign,
		Participants:    req.Signers,
		Threshold:       req.Threshold,
		DigestHex:       hex.EncodeToString(digest),
		InitiatorNodeID: m.cfg.NodeID,
	}

	if err := m.initOnAllPeers(ctx, req.Signers, initReq); err != nil {
		return nil, err
	}
	if err := m.startLocal(req.SessionID); err != nil {
		return nil, err
	}
	return m.GetSession(req.SessionID)
}

func (m *Manager) InitLocalSession(req SessionInitRequest) (*SessionResponse, error) {
	m.mu.Lock()
	if _, ok := m.sessions[req.SessionID]; ok {
		m.mu.Unlock()
		return m.GetSession(req.SessionID)
	}

	sess, err := m.buildSessionLocked(req)
	if err != nil {
		m.mu.Unlock()
		return nil, err
	}
	m.sessions[req.SessionID] = sess
	m.mu.Unlock() // Mở khóa trước khi chạy tiến trình ngầm

	go m.runOutboundPump(sess)

	// SỬA LỖI DEADLOCK CHÍNH LÀ ĐÂY:
	// Nếu đây là Node được gọi (không phải người khởi xướng), nó phải tự động Start()
	if req.InitiatorNodeID != m.cfg.NodeID {
		go func() {
			if err := m.startLocal(req.SessionID); err != nil {
				log.Printf("[session] failed to auto-start peer session: %v", err)
			}
		}()
	}

	return m.GetSession(req.SessionID)
}

func (m *Manager) startLocal(sessionID string) error {
	m.mu.RLock()
	sess, ok := m.sessions[sessionID]
	m.mu.RUnlock()
	if !ok {
		return fmt.Errorf("unknown session %s", sessionID)
	}

	// Chống Start 2 lần gây lỗi
	m.mu.Lock()
	if sess.status == StatusRunning {
		m.mu.Unlock()
		return nil
	}
	sess.status = StatusRunning
	m.mu.Unlock()

	if err := sess.party.Start(); err != nil {
		m.failSession(sessionID, fmt.Errorf("party start: %w", err))
		return err
	}

	return nil
}

func (m *Manager) ReceiveMessage(req TSSMessageRequest) error {
	wire, err := hex.DecodeString(strings.TrimPrefix(req.WireHex, "0x"))
	if err != nil {
		return err
	}

	m.mu.RLock()
	sess, ok := m.sessions[req.SessionID]
	m.mu.RUnlock()
	if !ok {
		return fmt.Errorf("session %s not found", req.SessionID)
	}

	// SỬA LỖI: Trích xuất trực tiếp con trỏ gốc từ danh sách đã sắp xếp
	var fromPID *tss.PartyID
	for _, p := range sess.parties {
		if p.Id == req.FromID {
			fromPID = p
			break
		}
	}

	if fromPID == nil {
		return fmt.Errorf("sender %s not found in session parties", req.FromID)
	}

	_, tssErr := sess.party.UpdateFromBytes(wire, fromPID, req.Broadcast)
	if tssErr != nil {
		return tssErr
	}
	return nil
}

func (m *Manager) GetSession(id string) (*SessionResponse, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.sessionResponseLocked(id)
}

func (m *Manager) sessionResponseLocked(id string) (*SessionResponse, error) {
	s, ok := m.sessions[id]
	if !ok {
		return nil, fmt.Errorf("session %s not found", id)
	}

	return &SessionResponse{
		SessionID:        s.id,
		Protocol:         s.protocol,
		Status:           s.status,
		Error:            s.err,
		StartedAt:        s.startedAt,
		CompletedAt:      s.completedAt,
		ExecutionTimeMS:  s.executionMS,
		SignatureHex:     s.signature,
		DigestHex:        s.digestHex,
		RecoveredAddress: s.recovered,
		PublicKeyHex:     s.pubKeyHex,
		EthereumAddress:  s.ethAddress,
	}, nil
}

func (m *Manager) initOnAllPeers(ctx context.Context, participants []string, req SessionInitRequest) error {
	if _, err := m.InitLocalSession(req); err != nil {
		return err
	}

	for _, id := range participants {
		if id == m.cfg.NodeID {
			continue
		}
		base, ok := m.peers[id]
		if !ok {
			return fmt.Errorf("peer %s not configured", id)
		}
		if err := m.peerClient.PostJSON(ctx, base, "/api/v1/sessions/init", req); err != nil {
			return err
		}
	}
	return nil
}

func (m *Manager) buildSessionLocked(req SessionInitRequest) (*session, error) {
	ids := append([]string(nil), req.Participants...)
	sort.Strings(ids)

	sortedPIDs, err := m.buildSortedParties(ids)
	if err != nil {
		return nil, err
	}

	peerCtx := tss.NewPeerContext(sortedPIDs)

	// SỬA LỖI: Lấy chính xác con trỏ của Node Local từ danh sách
	var selfPID *tss.PartyID
	for _, p := range sortedPIDs {
		if p.Id == m.cfg.NodeID {
			selfPID = p
			break
		}
	}
	if selfPID == nil {
		return nil, fmt.Errorf("local node %s not found in sorted parties", m.cfg.NodeID)
	}

	params := tss.NewParameters(
		tss.EC(),
		peerCtx,
		selfPID,
		len(sortedPIDs),
		req.Threshold,
	)

	outCh := make(chan tss.Message, 256)
	sessCtx, cancel := context.WithTimeout(context.Background(), m.cfg.SessionTimeout)

	s := &session{
		id:           req.SessionID,
		protocol:     req.Protocol,
		participants: ids,
		parties:      sortedPIDs, // LƯU LẠI DANH SÁCH VÀO SESSION
		status:       StatusPending,
		startedAt:    time.Now().UTC(),
		outCh:        outCh,
		cancel:       cancel,
	}

	switch req.Protocol {
	case ProtocolSign:
		digest, err := hex.DecodeString(strings.TrimPrefix(req.DigestHex, "0x"))
		if err != nil {
			cancel()
			return nil, err
		}

		shareEnv, err := storage.LoadShare(m.cfg.SharePath)
		if err != nil {
			cancel()
			return nil, err
		}

		s.digest = digest
		s.digestHex = "0x" + hex.EncodeToString(digest)

		endCh := make(chan *tsscommon.SignatureData, 1)
		s.signEndCh = endCh
		s.party = signing.NewLocalParty(
			new(big.Int).SetBytes(digest),
			params,
			shareEnv.Share,
			outCh,
			endCh,
		)

		go m.awaitSignResult(sessCtx, s)

	case ProtocolKeygen:
		preCtx, preCancel := context.WithTimeout(context.Background(), m.cfg.PreParamTimeout)
		defer preCancel()

		preParams, err := kg.GeneratePreParamsWithContext(preCtx, m.cfg.PreParamConcurrency)
		if err != nil {
			cancel()
			return nil, err
		}

		endCh := make(chan *kg.LocalPartySaveData, 1)
		s.keygenEndCh = endCh
		s.party = kg.NewLocalParty(params, outCh, endCh, *preParams)

		go m.awaitKeygenResult(sessCtx, s)

	default:
		cancel()
		return nil, fmt.Errorf("unsupported protocol %s", req.Protocol)
	}

	return s, nil
}

func (m *Manager) awaitKeygenResult(ctx context.Context, s *session) {
	select {
	case <-ctx.Done():
		m.failSession(s.id, ctx.Err())
		return

	case save := <-s.keygenEndCh:
		if err := storage.SaveShare(m.cfg.SharePath, m.cfg.NodeID, *save); err != nil {
			m.failSession(s.id, err)
			return
		}

		pub := save.ECDSAPub.ToECDSAPubKey()
		completed := time.Now().UTC()
		execMS := time.Since(s.startedAt).Milliseconds()

		m.mu.Lock()
		s.status = StatusCompleted
		s.completedAt = &completed
		s.executionMS = execMS
		s.pubKeyHex = hex.EncodeToString(gethcrypto.FromECDSAPub(pub))
		s.ethAddress = gethcrypto.PubkeyToAddress(*pub).Hex()
		m.mu.Unlock()

		log.Printf(
			"[keygen] session=%s node=%s address=%s T_keygen_ms=%d",
			s.id, m.cfg.NodeID, s.ethAddress, execMS,
		)
		s.cancel()
	}
}

func (m *Manager) awaitSignResult(ctx context.Context, s *session) {
	select {
	case <-ctx.Done():
		m.failSession(s.id, ctx.Err())
		return

	case sig := <-s.signEndCh:
		compat := &tssengine.SignatureCompat{SignatureData: sig}

		recPub, err := compat.RecoverPubkey(s.digest)
		if err != nil {
			m.failSession(s.id, err)
			return
		}

		completed := time.Now().UTC()
		execMS := time.Since(s.startedAt).Milliseconds()
		signature := append(append(append([]byte{}, sig.R...), sig.S...), compat.Recovery())

		m.mu.Lock()
		s.status = StatusCompleted
		s.completedAt = &completed
		s.executionMS = execMS
		s.signature = "0x" + hex.EncodeToString(signature)
		s.recovered = gethcrypto.PubkeyToAddress(*recPub).Hex()
		m.mu.Unlock()

		log.Printf(
			"[sign] session=%s node=%s recovered=%s T_sign_ms=%d digest=%s",
			s.id, m.cfg.NodeID, s.recovered, execMS, s.digestHex,
		)
		s.cancel()
	}
}

func (m *Manager) failSession(id string, err error) {
	completed := time.Now().UTC()

	m.mu.Lock()
	defer m.mu.Unlock()

	if s, ok := m.sessions[id]; ok {
		s.status = StatusFailed
		s.err = err.Error()
		s.completedAt = &completed
		s.executionMS = time.Since(s.startedAt).Milliseconds()
	}

	log.Printf("[session] session=%s node=%s failed err=%v", id, m.cfg.NodeID, err)
}

func (m *Manager) runOutboundPump(s *session) {
	idle := time.NewTimer(m.cfg.TssTimeout)
	defer idle.Stop()

	for {
		select {
		case <-idle.C:
			m.failSession(s.id, fmt.Errorf("tss outbound idle timeout after %s", m.cfg.TssTimeout))
			return

		case <-time.After(200 * time.Millisecond):
			m.mu.RLock()
			st := s.status
			m.mu.RUnlock()
			if st == StatusCompleted || st == StatusFailed {
				return
			}

		case msg := <-s.outCh:
			if !idle.Stop() {
				select {
				case <-idle.C:
				default:
				}
			}
			idle.Reset(m.cfg.TssTimeout)

			if err := m.forwardMessage(s, msg); err != nil {
				m.failSession(s.id, err)
				return
			}
		}
	}
}

func (m *Manager) forwardMessage(s *session, msg tss.Message) error {
	wire, routing, err := msg.WireBytes()
	if err != nil {
		return err
	}

	req := TSSMessageRequest{
		SessionID: s.id,
		Protocol:  s.protocol,
		WireHex:   hex.EncodeToString(wire),
		FromID:    routing.From.Id,
		Broadcast: routing.IsBroadcast || len(routing.To) == 0,
	}

	targets := []string{}
	if req.Broadcast {
		for _, id := range s.participants {
			if id != m.cfg.NodeID {
				targets = append(targets, id)
			}
		}
	} else {
		for _, to := range routing.To {
			if to.Id != m.cfg.NodeID {
				targets = append(targets, to.Id)
			}
		}
		req.ToIDs = targets
	}

	for _, id := range targets {
		base, ok := m.peers[id]
		if !ok {
			return fmt.Errorf("peer %s base url missing", id)
		}

		go func(targetID, targetBase string) {
			ctx, cancel := context.WithTimeout(context.Background(), m.cfg.HTTPTimeout)
			defer cancel()
			if err := m.peerClient.PostJSON(ctx, targetBase, "/api/v1/tss/message", req); err != nil {
				log.Printf("[network] failed to send msg to %s: %v", targetID, err)
			}
		}(id, base)
	}

	return nil
}

func (m *Manager) buildSortedParties(participantIDs []string) (tss.SortedPartyIDs, error) {
	parties := tssutil.BuildPartyIDsFromStrings(participantIDs)

	if len(parties) == 0 {
		return nil, fmt.Errorf("empty party list")
	}

	found := false
	for _, p := range parties {
		if p.Id == m.cfg.NodeID {
			found = true
			break
		}
	}

	if !found {
		return nil, fmt.Errorf(
			"local node %s not found in participants: %v",
			m.cfg.NodeID,
			participantIDs,
		)
	}

	return parties, nil
}