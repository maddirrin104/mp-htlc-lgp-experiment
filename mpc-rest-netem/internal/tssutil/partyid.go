package tssutil

import (
	"math/big"

	"github.com/bnb-chain/tss-lib/v2/tss"
)

// BuildPartyID tạo PartyID theo cách tất định chỉ từ chính chuỗi id.
// QUAN TRỌNG: phải dùng đúng hàm này ở mọi nơi trong hệ thống.
func BuildPartyID(id string) *tss.PartyID {
	key := new(big.Int).SetBytes([]byte(id))
	moniker := id
	return tss.NewPartyID(id, moniker, key)
}

// BuildPartyIDsFromStrings tạo danh sách PartyID từ []string.
func BuildPartyIDsFromStrings(ids []string) tss.SortedPartyIDs {
	parties := make(tss.UnSortedPartyIDs, 0, len(ids))
	for _, id := range ids {
		parties = append(parties, BuildPartyID(id))
	}
	return tss.SortPartyIDs(parties)
}