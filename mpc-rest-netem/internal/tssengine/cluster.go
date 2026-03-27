package tssengine

import (
	"context"
	"fmt"
	"math/big"
	"reflect"
	"time"

	tsscommon "github.com/bnb-chain/tss-lib/v2/common"
	kg "github.com/bnb-chain/tss-lib/v2/ecdsa/keygen"
	signing "github.com/bnb-chain/tss-lib/v2/ecdsa/signing"
	"github.com/bnb-chain/tss-lib/v2/tss"

	"mphtlc-lgp-mpc/internal/config"
	"mphtlc-lgp-mpc/internal/storage"
)

type Cluster struct {
	cfg config.ClusterConfig
}

type nodeRuntime struct {
	cfg   config.NodeConfig
	party tss.Party
	outCh chan tss.Message
}

func NewCluster(cfg config.ClusterConfig) *Cluster {
	return &Cluster{cfg: cfg}
}

func (c *Cluster) Config() config.ClusterConfig { return c.cfg }

func buildPartyIDs(cfgs []config.NodeConfig) tss.SortedPartyIDs {
	unsorted := make(tss.UnSortedPartyIDs, 0, len(cfgs))
	for _, node := range cfgs {
		unsorted = append(unsorted, tss.NewPartyID(node.ID, node.Moniker, big.NewInt(node.Key)))
	}
	return tss.SortPartyIDs(unsorted)
}

func pickSubset(sorted tss.SortedPartyIDs, wanted map[string]struct{}) tss.SortedPartyIDs {
	out := make(tss.SortedPartyIDs, 0, len(wanted))
	for _, pid := range sorted {
		if _, ok := wanted[pid.Id]; ok {
			out = append(out, pid)
		}
	}
	return out
}

func (c *Cluster) RunKeygen(ctx context.Context) (*KeygenResult, error) {
	pIDs := buildPartyIDs(c.cfg.Parties)
	peerCtx := tss.NewPeerContext(pIDs)
	preParams, err := c.generatePreParams(ctx)
	if err != nil {
		return nil, err
	}

	runtimes := make([]nodeRuntime, 0, len(c.cfg.Parties))
	endChs := make([]chan *kg.LocalPartySaveData, 0, len(c.cfg.Parties))
	for i, nodeCfg := range c.cfg.Parties {
		outCh := make(chan tss.Message, 128)
		endCh := make(chan *kg.LocalPartySaveData, 1)
		params := tss.NewParameters(tss.EC(), peerCtx, pIDs[i], len(pIDs), c.cfg.Threshold)
		party := kg.NewLocalParty(params, outCh, endCh, *preParams[i])
		runtimes = append(runtimes, nodeRuntime{cfg: nodeCfg, party: party, outCh: outCh})
		endChs = append(endChs, endCh)
	}

	if err := startAll(runtimes); err != nil {
		return nil, err
	}
	if err := routeUntilDone(ctx, runtimes, c.cfg.MessageTimeout, nil, nil); err != nil {
		return nil, err
	}

	results := make([]*kg.LocalPartySaveData, len(endChs))
	for i, ch := range endChs {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case res := <-ch:
			results[i] = res
			if err := storage.SaveShare(c.cfg.Parties[i].SharePath, c.cfg.Parties[i].ID, *res); err != nil {
				return nil, err
			}
		}
	}

	pub := results[0].ECDSAPub.ToECDSAPubKey()
	addr, pubHex, err := marshalPubKey(pub)
	if err != nil {
		return nil, err
	}
	return &KeygenResult{
		PublicKey:       pub,
		EthereumAddress: addr,
		PublicKeyHex:    pubHex,
		ParticipantIDs:  participantIDs(c.cfg.Parties),
	}, nil
}

func (c *Cluster) RunSigning(ctx context.Context, digest []byte, signerIDs []string) (*SignatureResult, error) {
	if len(digest) == 0 {
		return nil, fmt.Errorf("digest must not be empty")
	}
	wanted := make(map[string]struct{}, len(signerIDs))
	for _, id := range signerIDs {
		wanted[id] = struct{}{}
	}
	if len(wanted) < c.cfg.Threshold+1 {
		return nil, fmt.Errorf("need at least threshold+1 signers; have %d need %d", len(wanted), c.cfg.Threshold+1)
	}

	allIDs := buildPartyIDs(c.cfg.Parties)
	signerPIDs := pickSubset(allIDs, wanted)
	peerCtx := tss.NewPeerContext(signerPIDs)

	runtimes := make([]nodeRuntime, 0, len(signerPIDs))
	endChs := make([]chan *tsscommon.SignatureData, 0, len(signerPIDs))
	pubBySigner := make(map[string]string)
	for _, pid := range signerPIDs {
		nodeCfg, ok := c.findNode(pid.Id)
		if !ok {
			return nil, fmt.Errorf("signer %s not found in config", pid.Id)
		}
		env, err := storage.LoadShare(nodeCfg.SharePath)
		if err != nil {
			return nil, fmt.Errorf("load share for %s: %w", pid.Id, err)
		}
		if env.Share.ECDSAPub == nil {
			return nil, fmt.Errorf("missing ECDSA public key in share for %s", pid.Id)
		}
		addr, _, err := marshalPubKey(env.Share.ECDSAPub.ToECDSAPubKey())
		if err != nil {
			return nil, err
		}
		pubBySigner[pid.Id] = addr.Hex()

		outCh := make(chan tss.Message, 256)
		endCh := make(chan *tsscommon.SignatureData, 1)
		params := tss.NewParameters(tss.EC(), peerCtx, pid, len(signerPIDs), c.cfg.Threshold)
		party := signing.NewLocalParty(toBigInt32(digest), params, env.Share, outCh, endCh)
		runtimes = append(runtimes, nodeRuntime{cfg: nodeCfg, party: party, outCh: outCh})
		endChs = append(endChs, endCh)
	}

	if err := startAll(runtimes); err != nil {
		return nil, err
	}
	if err := routeUntilDone(ctx, runtimes, c.cfg.MessageTimeout, nil, nil); err != nil {
		return nil, err
	}

	var sig *tsscommon.SignatureData
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case sig = <-endChs[0]:
	}

	signature := append(append(append([]byte{}, sig.R...), sig.S...), ((&SignatureCompat{SignatureData: sig}).Recovery()))
	recoveredPub, err := ((&SignatureCompat{SignatureData: sig}).RecoverPubkey(digest))
	if err != nil {
		return nil, fmt.Errorf("recover pubkey: %w", err)
	}
	recoveredAddr, _, err := marshalPubKey(recoveredPub)
	if err != nil {
		return nil, err
	}

	return &SignatureResult{
		R:            fmt.Sprintf("0x%x", sig.R),
		S:            fmt.Sprintf("0x%x", sig.S),
		V:            ((&SignatureCompat{SignatureData: sig}).Recovery()),
		SignatureHex: fmt.Sprintf("0x%x", signature),
		DigestHex:    fmt.Sprintf("0x%x", digest),
		Recovered:    recoveredAddr,
		SignerIDs:    signerIDs,
	}, nil
}

func (c *Cluster) findNode(id string) (config.NodeConfig, bool) {
	for _, n := range c.cfg.Parties {
		if n.ID == id {
			return n, true
		}
	}
	return config.NodeConfig{}, false
}

func (c *Cluster) generatePreParams(ctx context.Context) ([]*kg.LocalPreParams, error) {
	out := make([]*kg.LocalPreParams, len(c.cfg.Parties))
	for i := range c.cfg.Parties {
		localCtx, cancel := context.WithTimeout(ctx, c.cfg.PreParamTimeout)
		defer cancel()
		pre, err := kg.GeneratePreParamsWithContext(localCtx, c.cfg.PreParamConcurrency)
		if err != nil {
			return nil, fmt.Errorf("generate preparams for party %d: %w", i, err)
		}
		out[i] = pre
	}
	return out, nil
}

func startAll(nodes []nodeRuntime) error {
	for _, n := range nodes {
		if err := n.party.Start(); err != nil {
			return fmt.Errorf("start party %s: %w", n.cfg.ID, err)
		}
	}
	return nil
}

func routeUntilDone(
	ctx context.Context,
	nodes []nodeRuntime,
	messageTimeout time.Duration,
	onMessage func(tss.Message),
	onIdle func(),
) error {
	partyByID := make(map[string]tss.Party, len(nodes))
	cases := make([]reflect.SelectCase, 0, len(nodes)+1)
	for _, n := range nodes {
		partyByID[n.cfg.ID] = n.party
		cases = append(cases, reflect.SelectCase{Dir: reflect.SelectRecv, Chan: reflect.ValueOf(n.outCh)})
	}
	ticker := time.NewTicker(messageTimeout)
	defer ticker.Stop()
	cases = append(cases, reflect.SelectCase{Dir: reflect.SelectRecv, Chan: reflect.ValueOf(ticker.C)})

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		chosen, recv, ok := reflect.Select(cases)
		if chosen == len(cases)-1 {
			if onIdle != nil {
				onIdle()
			}
			return nil
		}
		if !ok {
			continue
		}
		msg, ok := recv.Interface().(tss.Message)
		if !ok {
			continue
		}
		if onMessage != nil {
			onMessage(msg)
		}
		if err := deliverMessage(msg, partyByID); err != nil {
			return err
		}
	}
}

func deliverMessage(msg tss.Message, parties map[string]tss.Party) error {
	wireBytes, routing, err := msg.WireBytes()
	if err != nil {
		return fmt.Errorf("wire bytes: %w", err)
	}
	if routing.IsBroadcast || len(routing.To) == 0 {
		for id, party := range parties {
			if id == routing.From.Id {
				continue
			}
			if _, err := party.UpdateFromBytes(wireBytes, routing.From, routing.IsBroadcast); err != nil {
				return fmt.Errorf("deliver broadcast to %s: %w", id, err)
			}
		}
		return nil
	}
	for _, to := range routing.To {
		party, ok := parties[to.Id]
		if !ok {
			return fmt.Errorf("unknown recipient %s", to.Id)
		}
		if _, err := party.UpdateFromBytes(wireBytes, routing.From, routing.IsBroadcast); err != nil {
			return fmt.Errorf("deliver unicast to %s: %w", to.Id, err)
		}
	}
	return nil
}
