package relay

import (
	"encoding/hex"
	"fmt"
	"testing"

	"github.com/DigitalLabs-web3/neo-go-evm/pkg/crypto/hash"
	"github.com/ethereum/go-ethereum/common"
	"github.com/joeqian10/neo3-gogogo/helper"
	"github.com/stretchr/testify/assert"
)

func TestXxx(t *testing.T) {
	h1, err := helper.UInt256FromString("da063b503e8dd9c3115e1016188e2c2bca82a1c46fd4835acd307375fa2eb0a8")
	assert.NoError(t, err)
	h2 := common.BytesToHash(h1.ToByteArray())
	fmt.Println(hex.EncodeToString(h2[:]))
	fmt.Println(hex.EncodeToString(h1.ToByteArray()))
	h2 = common.HexToHash("da063b503e8dd9c3115e1016188e2c2bca82a1c46fd4835acd307375fa2eb0a8")
	fmt.Println(hex.EncodeToString(h2[:]))
}

func TestHeaderHash(t *testing.T) {
	d, err := hex.DecodeString("00000000fa27856e23abb9723920997f5eacda51fd776e02068f0dce15a64e3ad9600a2b4ecf58c50be97c005fab9bc5734f68cfdf6343fbcdba0e3cc84dd7147188d7c12a854118840100005ee7dec4aa43b84f61230000008e2eaaa3e95fec87ee999f540dafab2b49f05e5d")
	assert.NoError(t, err)
	h := hash.Sha256(d)
	fmt.Println(h)
}

func TestHash160(t *testing.T) {
	b, _ := hex.DecodeString("110c21031af734da2f203c86107d1a65e9c4eaa393322d4aaedc7797990f50097497114411419ed0dc3a")
	h1 := hash.Hash160(b)
	fmt.Println(h1)
}
