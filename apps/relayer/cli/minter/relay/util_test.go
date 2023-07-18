package relay

import (
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"testing"

	sblock "github.com/DigitalLabs-web3/neo-go-evm/pkg/core/block"
	sstate "github.com/DigitalLabs-web3/neo-go-evm/pkg/core/state"
	"github.com/DigitalLabs-web3/neo-go-evm/pkg/crypto/hash"
	sio "github.com/DigitalLabs-web3/neo-go-evm/pkg/io"
	"github.com/ethereum/go-ethereum/common"
	"github.com/nspcc-dev/neo-go/pkg/core/block"
	"github.com/nspcc-dev/neo-go/pkg/core/state"
	mio "github.com/nspcc-dev/neo-go/pkg/io"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRpcHeaderToBlockHeader(t *testing.T) {
	j := `{
			"hash": "0x298fd02666552849cf2c00a41b8d7681e201b1ca1d34d180c6100ac00938a16a",
			"size": 220,
			"version": 0,
			"previousblockhash": "0xe7b7e12ef4cbfcec9073c00e7974c198e3b49fcdefa5bf5826e4f5d38e006c83",
			"merkleroot": "0xa75cbad3873a480bfb352ae6e639b46a69cf6144eab52b7973afd716fb9e2e51",
			"time": 1679298172579,
			"nonce": "2BC280B2B70832EC",
			"index": 598,
			"primary": 0,
			"nextconsensus": "Ng45jeKWt4RaZCMJ8d14kM8aivCf4jxtvQ",
			"witnesses": [
				{
					"invocation": "DEBAlvVcaheRvQYF6XAiOshu2NFK7WCMSsuTS2NIjQ49pIjiNIB2L9STQ9un7KeiTMZW8c9AO+qSr/Ir557Dkr4r",
					"verification": "EQwhA2k15XPVCfGVUj+RZvnxoVRnfbI9Eopzh7nE9cVGfObQEUGe0Nw6"
				}
			],
			"confirmations": 284,
			"nextblockhash": "0x96c497765078f8a6607f24b0191acf668a1101b40fe5f740a7907aab8b7c245a"
		}`
	jh := new(block.Header)
	err := json.Unmarshal([]byte(j), jh)
	assert.NoError(t, err)
	h := mainHeaderToSideHeader(jh)
	assert.Equal(t, uint32(598), h.Index)
	b1, err := mainHeaderBytes(jh)
	assert.NoError(t, err)
	b2, err := sideHeaderBytes(h)
	assert.NoError(t, err)
	assert.Equal(t, hex.EncodeToString(b1), hex.EncodeToString(b2))
	t.Log(h.Hash())
}

func mainHeaderBytes(header *block.Header) ([]byte, error) {
	w := mio.NewBufBinWriter()
	header.EncodeBinary(w.BinWriter)
	if w.Err != nil {
		return nil, w.Err
	}
	return w.Bytes(), nil
}

func sideHeaderBytes(header *sblock.Header) ([]byte, error) {
	return sio.ToByteArray(header)
}

func TestMainStateRootToSideStateRoot(t *testing.T) {
	j := `{
        "version": 0,
        "index": 1481,
        "roothash": "0x72b5acdd3fb1fc50462bac16599ca0a89fe17fd0cb14fa04425131179afb13ab",
        "witnesses": [
            {
                "invocation": "DEDbtvRxT+hO08ef8SRY5Nkp1SPtG7lnKNWIdQerxxJUa3HXD9jyLRCBkOK5Z1tBAlibFH3xalUvyBT5yckfpUPK",
                "verification": "EQwhA2k15XPVCfGVUj+RZvnxoVRnfbI9Eopzh7nE9cVGfObQEUGe0Nw6"
            }
        ]
    }`
	s := new(state.MPTRoot)
	err := json.Unmarshal([]byte(j), s)
	assert.NoError(t, err)
	b1, err := mainStateRootBytes(s)
	assert.NoError(t, err)
	ss := mainStateRootToSideStateRoot(s)
	b2, err := sideStateRootBytes(ss)
	assert.NoError(t, err)
	assert.Equal(t, hex.EncodeToString(b1), hex.EncodeToString(b2))
}

func mainStateRootBytes(root *state.MPTRoot) ([]byte, error) {
	w := mio.NewBufBinWriter()
	root.EncodeBinary(w.BinWriter)
	if w.Err != nil {
		return nil, w.Err
	}
	return w.Bytes(), nil
}

func sideStateRootBytes(root *sstate.MPTRoot) ([]byte, error) {
	return sio.ToByteArray(root)
}

func TestMerkleTree(t *testing.T) {
	target := common.HexToHash("0xf1bec4606f5ac640f23d5bd9af6c4de328e621310b7a7880af7e538b5d045e60")
	t.Log(base64.StdEncoding.EncodeToString(target.Bytes()))
	hashes := []common.Hash{
		common.HexToHash("0xf55b3695d135944323f965c1481c8209135187a41a4d37048b387b4138f67d42"),
		common.HexToHash("0x4b3fdeb056ea6445623d95f57bd25a1f675ec49b0c5c4e9be1a9ad0803e41f36"),
		common.HexToHash("0x48812b6c9a3fc3ded44a8cb912d742eb873d4cf5145bf18ce87a72b92af290b2"),
		target,
	}
	tree, err := hash.NewMerkleTree(hashes)
	require.NoError(t, err)
	t.Log(tree.Root())
	t.Log(base64.StdEncoding.EncodeToString(tree.Root().Bytes()))
	proofs, path, err := tree.Prove(target)
	require.NoError(t, err)
	t.Log(path)
	proof := make([]byte, len(proofs)*common.HashLength+4)
	binary.LittleEndian.PutUint32(proof, path)
	for i, hash := range proofs {
		copy(proof[4+i*common.HashLength:], hash[:])
	}
	t.Log(base64.StdEncoding.EncodeToString(proof))
	t.Log(hash.VerifyMerkleProof(tree.Root(), target, proofs, path))
}
