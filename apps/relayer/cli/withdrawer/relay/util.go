package relay

import (
	"encoding/binary"

	"github.com/DigitalLabs-web3/neo-go-evm/pkg/core/block"
	"github.com/DigitalLabs-web3/neo-go-evm/pkg/crypto/hash"
	"github.com/ethereum/go-ethereum/common"
)

func proveTx(block *block.Block, txid common.Hash) ([]byte, error) {
	hashes := make([]common.Hash, len(block.Transactions))
	for i, tx := range block.Transactions {
		hashes[i] = tx.Hash()
	}
	tree, err := hash.NewMerkleTree(hashes)
	if err != nil {
		return nil, err
	}
	proofs, path, err := tree.Prove(txid)
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

func reverse[S ~[]E, E any](s S) {
	for i, j := 0, len(s)-1; i < j; i, j = i+1, j-1 {
		s[i], s[j] = s[j], s[i]
	}
}
