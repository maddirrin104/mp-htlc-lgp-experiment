package main

import (
	"crypto/ecdsa"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
)

type signReq struct {
	HashHex string `json:"hash_hex"`
}

type signResp struct {
	R string `json:"r"`
	S string `json:"s"`
}

type addrResp struct {
	Address string `json:"address"`
	Pubkey  string `json:"pubkey_uncompressed"`
}

func mustPK() (*ecdsa.PrivateKey, common.Address) {
	pk := strings.TrimSpace(os.Getenv("SIGNER_PK"))
	if pk == "" {
		log.Fatal("missing SIGNER_PK (in mock mode this is your TSS aggregate key placeholder)")
	}
	pk = strings.TrimPrefix(pk, "0x")
	k, err := crypto.HexToECDSA(pk)
	if err != nil {
		log.Fatalf("bad SIGNER_PK: %v", err)
	}
	addr := crypto.PubkeyToAddress(k.PublicKey)
	return k, addr
}

func main() {
	listen := strings.TrimSpace(os.Getenv("LISTEN"))
	if listen == "" {
		listen = ":8080"
	}

	k, addr := mustPK()
	pubUncompressed := crypto.FromECDSAPub(&k.PublicKey)

	h := http.NewServeMux()
	h.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		_, _ = w.Write([]byte("ok"))
	})

	h.HandleFunc("/address", func(w http.ResponseWriter, r *http.Request) {
		out := addrResp{Address: addr.Hex(), Pubkey: "0x" + hex.EncodeToString(pubUncompressed)}
		b, _ := json.Marshal(out)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(b)
	})

	h.HandleFunc("/signHash", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(405)
			return
		}
		var req signReq
		dec := json.NewDecoder(r.Body)
		if err := dec.Decode(&req); err != nil {
			w.WriteHeader(400)
			_, _ = w.Write([]byte("bad json"))
			return
		}
		hh := strings.TrimPrefix(strings.TrimSpace(req.HashHex), "0x")
		hash, err := hex.DecodeString(hh)
		if err != nil || len(hash) != 32 {
			w.WriteHeader(400)
			_, _ = w.Write([]byte("hash_hex must be 32 bytes"))
			return
		}

		// IMPORTANT: This is mock mode. Replace this handler with a real threshold signing protocol.
		sig, err := crypto.Sign(hash, k) // 65 bytes: r||s||v(0/1)
		if err != nil {
			w.WriteHeader(500)
			_, _ = w.Write([]byte("sign fail"))
			return
		}
		r := sig[0:32]
		s := sig[32:64]
		out := signResp{R: "0x" + hex.EncodeToString(r), S: "0x" + hex.EncodeToString(s)}
		b, _ := json.Marshal(out)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(b)
	})

	log.Printf("tss-signer(mock) listening on %s\naddress=%s", listen, addr.Hex())
	log.Fatal(http.ListenAndServe(listen, h))
}
