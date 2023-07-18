package constantclient

import (
	"context"
	"fmt"
	"math/big"

	"github.com/DigitalLabs-web3/neo-go-evm/pkg/core/block"
	"github.com/DigitalLabs-web3/neo-go-evm/pkg/core/state"
	sio "github.com/DigitalLabs-web3/neo-go-evm/pkg/io"
	"github.com/DigitalLabs-web3/neo-go-evm/pkg/rpc/client"
	"github.com/DigitalLabs-web3/neo-go-evm/pkg/rpc/response"
	"github.com/DigitalLabs-web3/neo-go-evm/pkg/rpc/response/result"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	mblock "github.com/nspcc-dev/neo-go/pkg/core/block"
	mstate "github.com/nspcc-dev/neo-go/pkg/core/state"
	"github.com/nspcc-dev/neo-go/pkg/core/transaction"
	mio "github.com/nspcc-dev/neo-go/pkg/io"
	"github.com/nspcc-dev/neo-go/pkg/neorpc"
	mresult "github.com/nspcc-dev/neo-go/pkg/neorpc/result"
	"github.com/nspcc-dev/neo-go/pkg/rpcclient"
	"github.com/nspcc-dev/neo-go/pkg/util"
)

type ConstantClient struct {
	mainSeeds []string
	sideSeeds []string
	mIndex    int
	sIndex    int
	mClient   *rpcclient.Client
	sClient   *client.Client
}

func New(mseeds, sseeds []string) *ConstantClient {
	c := &ConstantClient{
		mainSeeds: mseeds,
		sideSeeds: sseeds,
		mIndex:    0,
		sIndex:    0,
		mClient:   nil,
		sClient:   nil,
	}
	c.ensureNewClient(true)
	c.ensureNewClient(false)
	return c
}

func newClient(index *int, count int, newClient func(index int) (interface{}, error)) interface{} {
	i := *index
	cli, err := newClient(*index)
	for err != nil {
		*index = (*index + 1) % count
		if *index == i {
			panic(fmt.Errorf("can't initialize client"))
		}
		cli, err = newClient(*index)
	}
	return cli
}

func (c *ConstantClient) ensureNewClient(isMain bool) {
	if isMain {
		c.mClient = newClient(&c.mIndex, len(c.mainSeeds), func(index int) (interface{}, error) {
			cli, err := rpcclient.New(context.Background(), c.mainSeeds[index], rpcclient.Options{})
			if err != nil {
				return nil, err
			}
			err = cli.Init()
			return cli, err
		}).(*rpcclient.Client)
	} else {
		c.sClient = newClient(&c.sIndex, len(c.sideSeeds), func(index int) (interface{}, error) {
			cli, err := client.New(context.Background(), c.sideSeeds[index], client.Options{})
			if err != nil {
				return nil, err
			}
			err = cli.Init()
			return cli, err
		}).(*client.Client)
	}
}

func isSideNetworkError(err error) bool {
	_, ok := err.(*neorpc.Error)
	return !ok
}

func isMainNetworkError(err error) bool {
	_, ok := err.(*response.Error)
	return !ok
}

func isNetworkError(err error, isMain bool) bool {
	if isMain {
		return isMainNetworkError(err)
	}
	return isSideNetworkError(err)
}

func (c *ConstantClient) ensureRequest(isMain bool, doRequest func() (interface{}, error)) (interface{}, error) {
	retry := 5
	var lasterr error
	if isMain {
		retry = len(c.mainSeeds)
	} else {
		retry = len(c.sideSeeds)
	}
	for retry > 0 {
		r, err := doRequest()
		if err != nil {
			retry--
			lasterr = err
			if isNetworkError(err, isMain) {
				c.ensureNewClient(false)
				continue
			}
		}
		return r, err
	}
	return nil, lasterr
}

func (c *ConstantClient) GetVersion() (*mresult.Version, error) {
	r, err := c.ensureRequest(true, func() (interface{}, error) {
		return c.mClient.GetVersion()
	})
	if err != nil {
		return nil, err
	}
	return r.(*mresult.Version), nil
}

func (c *ConstantClient) GetApplicationLog(txid util.Uint256) (*mresult.ApplicationLog, error) {
	r, err := c.ensureRequest(true, func() (interface{}, error) {
		return c.mClient.GetApplicationLog(txid, nil)
	})
	if err != nil {
		return nil, err
	}
	return r.(*mresult.ApplicationLog), nil
}

func (c *ConstantClient) GetBlock(index uint32) (*mblock.Block, error) {
	r, err := c.ensureRequest(true, func() (interface{}, error) {
		return c.mClient.GetBlockByIndex(index)
	})
	if err != nil {
		return nil, err
	}
	return r.(*mblock.Block), nil
}

func (c *ConstantClient) GetBlockCount() (uint32, error) {
	r, err := c.ensureRequest(true, func() (interface{}, error) {
		return c.mClient.GetBlockCount()
	})
	if err != nil {
		return 0, err
	}
	return r.(uint32), nil
}

func (c *ConstantClient) GetStateRoot(index uint32) (*mstate.MPTRoot, error) {
	r, err := c.ensureRequest(true, func() (interface{}, error) {
		return c.mClient.GetStateRootByHeight(index)
	})
	if err != nil {
		return nil, err
	}
	return r.(*mstate.MPTRoot), nil
}

func (c *ConstantClient) InvokeScript(script []byte, signer transaction.Signer) (*mresult.Invoke, error) {
	r, err := c.ensureRequest(true, func() (interface{}, error) {
		return c.mClient.InvokeScript(script, []transaction.Signer{signer})
	})
	if err != nil {
		return nil, err
	}
	return r.(*mresult.Invoke), nil
}

func (c *ConstantClient) CalculateNetworkFee(tx *transaction.Transaction) (int64, error) {
	r, err := c.ensureRequest(true, func() (interface{}, error) {
		return c.mClient.CalculateNetworkFee(tx)
	})
	if err != nil {
		return 0, err
	}
	return r.(int64), nil
}

func (c *ConstantClient) SendRawTransaction(tx *transaction.Transaction) (util.Uint256, error) {
	r, err := c.ensureRequest(true, func() (interface{}, error) {
		return c.mClient.SendRawTransaction(tx)
	})
	if err != nil {
		return util.Uint256{}, err
	}
	return r.(util.Uint256), nil
}

func (c *ConstantClient) GetRawTransaction(txid util.Uint256) (*transaction.Transaction, error) {
	r, err := c.ensureRequest(true, func() (interface{}, error) {
		return c.mClient.GetRawTransaction(txid)
	})
	if err != nil {
		return nil, err
	}
	return r.(*transaction.Transaction), nil
}

func proofToBytes(proof *mresult.ProofWithKey) []byte {
	w := mio.NewBufBinWriter()
	proof.EncodeBinary(w.BinWriter)
	return w.Bytes()
}

func (c *ConstantClient) GetProof(rootHash util.Uint256, contractHash util.Uint160, key []byte) ([]byte, error) {
	r, err := c.ensureRequest(true, func() (interface{}, error) {
		return c.mClient.GetProof(rootHash, contractHash, key)
	})
	if err != nil {
		return nil, err
	}
	res := r.(*mresult.ProofWithKey)
	return proofToBytes(res), nil
}

func (c *ConstantClient) Eth_NativeContract(name string) (*state.NativeContract, error) {
	r, err := c.ensureRequest(false, func() (interface{}, error) {
		return c.sClient.GetNativeContracts()
	})
	if err != nil {
		return nil, err
	}
	cs := r.([]state.NativeContract)
	for _, contract := range cs {
		if contract.Name == name {
			return &contract, nil
		}
	}
	return nil, nil
}

func (c *ConstantClient) Eth_ChainId() uint64 {
	r, _ := c.ensureRequest(false, func() (interface{}, error) {
		return c.sClient.Eth_ChainId()
	})
	return r.(uint64)
}

func (c *ConstantClient) Eth_GasPrice() *big.Int {
	r, _ := c.ensureRequest(false, func() (interface{}, error) {
		return c.sClient.Eth_GasPrice()
	})
	return r.(*big.Int)
}

func (c *ConstantClient) Eth_GetTransactionCount(address common.Address) uint64 {
	r, _ := c.ensureRequest(false, func() (interface{}, error) {
		return c.sClient.Eth_GetTransactionCount(address)
	})
	return r.(uint64)
}

func (c *ConstantClient) Eth_EstimateGas(tx *result.TransactionObject) (uint64, error) {
	r, err := c.ensureRequest(false, func() (interface{}, error) {
		return c.sClient.Eth_EstimateGas(tx)
	})
	if err != nil {
		return 0, err
	}
	return r.(uint64), nil
}

func (c *ConstantClient) Eth_SendRawTransaction(rawTx []byte) (common.Hash, error) {
	r, err := c.ensureRequest(false, func() (interface{}, error) {
		return c.sClient.Eth_SendRawTransaction(rawTx)
	})
	if err != nil {
		return common.Hash{}, err
	}
	return r.(common.Hash), nil
}

func (c *ConstantClient) Eth_GetTransactionByHash(hash common.Hash) *result.TransactionOutputRaw {
	r, _ := c.ensureRequest(false, func() (interface{}, error) {
		return c.sClient.Eth_GetTransactionByHash(hash)
	})
	return r.(*result.TransactionOutputRaw)
}

func (c *ConstantClient) Eth_GetBlockNumber() (uint64, error) {
	r, err := c.ensureRequest(false, func() (interface{}, error) {
		return c.sClient.Eth_BlockNumber()
	})
	if err != nil {
		return 0, err
	}
	return r.(uint64), err
}

func (c *ConstantClient) Eth_GetBlockByNumber(height uint32) (*block.Block, error) {
	r, err := c.ensureRequest(false, func() (interface{}, error) {
		return c.sClient.GetBlockByIndex(height)
	})
	if err != nil {
		return nil, err
	}
	return r.(*block.Block), err
}

func (c *ConstantClient) Eth_GetReceipt(txid common.Hash) *types.Receipt {
	r, _ := c.ensureRequest(false, func() (interface{}, error) {
		return c.sClient.Eth_GetTransactionReceipt(txid)
	})
	return r.(*types.Receipt)
}

func (c *ConstantClient) Eth_GetStateRoot(index uint32) (*state.MPTRoot, error) {
	r, err := c.ensureRequest(false, func() (interface{}, error) {
		return c.sClient.GetStateRootByHeight(index)
	})
	if err != nil {
		return nil, err
	}
	return r.(*state.MPTRoot), err
}

func sideProofToBytes(proof *result.ProofWithKey) []byte {
	w := sio.NewBufBinWriter()
	proof.EncodeBinary(w.BinWriter)
	return w.Bytes()
}

func (c *ConstantClient) Eth_GetProof(stateroot common.Hash, contract common.Address, key []byte) ([]byte, error) {
	r, err := c.ensureRequest(false, func() (interface{}, error) {
		return c.sClient.GetProof(stateroot, contract, key)
	})
	if err != nil {
		return nil, err
	}
	return sideProofToBytes(r.(*result.ProofWithKey)), nil
}
