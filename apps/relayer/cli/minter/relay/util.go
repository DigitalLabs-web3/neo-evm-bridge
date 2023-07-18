package relay

import (
	"encoding/binary"

	sblock "github.com/DigitalLabs-web3/neo-go-evm/pkg/core/block"
	sstate "github.com/DigitalLabs-web3/neo-go-evm/pkg/core/state"
	"github.com/DigitalLabs-web3/neo-go-evm/pkg/core/transaction"
	"github.com/DigitalLabs-web3/neo-go-evm/pkg/crypto/hash"
	sio "github.com/DigitalLabs-web3/neo-go-evm/pkg/io"
	"github.com/ethereum/go-ethereum/common"
	"github.com/nspcc-dev/neo-go/pkg/core/block"
	"github.com/nspcc-dev/neo-go/pkg/core/state"
	"github.com/nspcc-dev/neo-go/pkg/util"
)

func proveTx(block *block.Block, txid util.Uint256) ([]byte, error) {
	hashes := make([]common.Hash, len(block.Transactions))
	for i, tx := range block.Transactions {
		hashes[i] = common.BytesToHash(tx.Hash().BytesBE())
	}
	tree, err := hash.NewMerkleTree(hashes)
	if err != nil {
		return nil, err
	}
	proofs, path, err := tree.Prove(common.BytesToHash(txid.BytesBE()))
	if err != nil {
		return nil, err
	}
	proof := make([]byte, len(proofs)*common.HashLength+4)
	binary.LittleEndian.PutUint32(proof, path)
	for i, hash := range proofs {
		copy(proof[4+i*common.HashLength:], hash[:])
	}
	return proof, nil
}

func mainHeaderToSideHeader(h *block.Header) *sblock.Header {
	header := sblock.Header{
		Version:       h.Version,
		PrevHash:      common.BytesToHash(h.PrevHash.BytesBE()),
		MerkleRoot:    common.BytesToHash(h.MerkleRoot.BytesBE()),
		Timestamp:     h.Timestamp,
		Nonce:         h.Nonce,
		Index:         h.Index,
		NextConsensus: common.BytesToAddress(h.NextConsensus.BytesBE()),
		PrimaryIndex:  h.PrimaryIndex,
		Witness: transaction.Witness{
			VerificationScript: h.Script.VerificationScript,
			InvocationScript:   h.Script.InvocationScript,
		},
	}
	return &header
}

func blockHeaderToBytes(header *sblock.Header) ([]byte, error) {
	return sio.ToByteArray(header)
}

func mainStateRootToSideStateRoot(s *state.MPTRoot) *sstate.MPTRoot {
	stateroot := &sstate.MPTRoot{
		Version: s.Version,
		Index:   s.Index,
		Root:    common.BytesToHash(s.Root.BytesBE()),
		Witness: transaction.Witness{
			VerificationScript: s.Witness[0].VerificationScript,
			InvocationScript:   s.Witness[0].InvocationScript,
		},
	}
	return stateroot
}

func staterootToBytes(stateroot *sstate.MPTRoot) ([]byte, error) {
	return sio.ToByteArray(stateroot)
}

func reverse[S ~[]E, E any](s S) {
	for i, j := 0, len(s)-1; i < j; i, j = i+1, j-1 {
		s[i], s[j] = s[j], s[i]
	}
}
