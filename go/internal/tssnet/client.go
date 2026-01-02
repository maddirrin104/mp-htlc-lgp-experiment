package tssnet

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type Client struct {
	BaseURL string
	HTTP    *http.Client
}

type addrResp struct {
	Address string `json:"address"`
	Pubkey  string `json:"pubkey_uncompressed"`
}

type signReq struct {
	HashHex string `json:"hash_hex"`
}

type signResp struct {
	R string `json:"r"`
	S string `json:"s"`
}

func New(baseURL string) *Client {
	baseURL = strings.TrimRight(baseURL, "/")
	return &Client{BaseURL: baseURL, HTTP: &http.Client{Timeout: 30 * time.Second}}
}

func (c *Client) GetAddress() (addr string, pubkey string, err error) {
	resp, err := c.HTTP.Get(c.BaseURL + "/address")
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return "", "", fmt.Errorf("/address status %d: %s", resp.StatusCode, string(b))
	}
	var out addrResp
	if err := json.Unmarshal(b, &out); err != nil {
		return "", "", err
	}
	return out.Address, out.Pubkey, nil
}

func (c *Client) SignHash(hash32 []byte) (r32 []byte, s32 []byte, err error) {
	if len(hash32) != 32 {
		return nil, nil, fmt.Errorf("hash must be 32 bytes")
	}
	reqBody, _ := json.Marshal(signReq{HashHex: "0x" + hex.EncodeToString(hash32)})
	resp, err := c.HTTP.Post(c.BaseURL+"/signHash", "application/json", bytes.NewReader(reqBody))
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return nil, nil, fmt.Errorf("/signHash status %d: %s", resp.StatusCode, string(b))
	}
	var out signResp
	if err := json.Unmarshal(b, &out); err != nil {
		return nil, nil, err
	}
	r, err := decode32(out.R)
	if err != nil {
		return nil, nil, fmt.Errorf("bad r: %w", err)
	}
	s, err := decode32(out.S)
	if err != nil {
		return nil, nil, fmt.Errorf("bad s: %w", err)
	}
	return r, s, nil
}

func decode32(h string) ([]byte, error) {
	h = strings.TrimSpace(h)
	h = strings.TrimPrefix(h, "0x")
	b, err := hex.DecodeString(h)
	if err != nil {
		return nil, err
	}
	if len(b) > 32 {
		return nil, fmt.Errorf("too long: %d", len(b))
	}
	// left-pad to 32
	out := make([]byte, 32)
	copy(out[32-len(b):], b)
	return out, nil
}
