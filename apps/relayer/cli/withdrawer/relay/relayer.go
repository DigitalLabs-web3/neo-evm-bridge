package relay

import (
	"encoding/binary"
	"errors"
	"fmt"
	"log"
	"math/big"
	"strings"
	"time"

	"github.com/DigitalLabs-web3/neo-evm-bridge/relayer/config"
	"github.com/DigitalLabs-web3/neo-evm-bridge/relayer/constantclient"
	"github.com/DigitalLabs-web3/neo-go-evm/pkg/core/block"
	"github.com/DigitalLabs-web3/neo-go-evm/pkg/core/native"
	"github.com/DigitalLabs-web3/neo-go-evm/pkg/io"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/nspcc-dev/neo-go/pkg/config/netmode"
	"github.com/nspcc-dev/neo-go/pkg/core/transaction"
	"github.com/nspcc-dev/neo-go/pkg/smartcontract"
	"github.com/nspcc-dev/neo-go/pkg/util"

	"github.com/DigitalLabs-web3/neo-go-evm/pkg/core/state"
	"github.com/nspcc-dev/neo-go/pkg/wallet"
)

const (
	BurnPrefix           = 0x06
	ValidatorsKey        = 0x03
	StateValidatorRole   = 4
	BlockTimeSeconds     = 15
	MaxStateRootGetRange = 57600
	MintThreshold        = 100000000

	CCMSyncHeader    = "syncHeader"
	CCMSyncStateRoot = "syncStateRoot"
	CCMWithdraw      = "withdraw"

	CCMAlreadySyncedError = "already exists"
	CCMAlreadyWithdrawed  = "already withdrawed"

	BurnSignature           = "burn(address)"
	SyncValidatorsSignature = "syncValidators(uint32,uint256,bytes,uint32,bytes)"
)

var (
	BurnTopic          = common.BytesToHash(crypto.Keccak256([]byte(BurnSignature)))
	SyncValidatorTopic = common.BytesToHash(crypto.Keccak256([]byte(SyncValidatorsSignature)))

	MinWithdrawValue = big.NewInt(100000000) //1GAS
)

type Relayer struct {
	cfg                           *config.Config
	lastHeader                    *block.Header
	lastStateRoot                 *state.MPTRoot
	roleManagementContractAddress common.Address
	client                        *constantclient.ConstantClient
	bridge                        common.Address
	account                       *wallet.Account
	best                          bool
	netmode                       netmode.Magic
}

func NewRelayer(cfg *config.Config, acc *wallet.Account) (*Relayer, error) {
	client := constantclient.New(cfg.MainSeeds, cfg.SideSeeds)
	mver, err := client.GetVersion()
	if err != nil {
		return nil, err
	}
	return &Relayer{
		cfg:                           cfg,
		roleManagementContractAddress: native.DesignationAddress,
		client:                        client,
		account:                       acc,
		bridge:                        native.BridgeAddress,
		best:                          false,
		netmode:                       mver.Protocol.Network,
	}, nil
}

func (l *Relayer) Run() {
	for i := l.cfg.Start; l.cfg.End == 0 || i < l.cfg.End; {
		if l.best {
			time.Sleep(15 * time.Second)
		}
		block, err := l.client.Eth_GetBlockByNumber(i)
		if err != nil || block == nil {
			if !l.best {
				h, err := l.client.Eth_GetBlockNumber()
				if err != nil {
					panic(err)
				}
				if i >= uint32(h) {
					l.best = true
				}
			}
			continue
		}
		log.Printf("syncing block, index=%d", i)
		batch := new(taskBatch)
		batch.block = block
		for _, tx := range block.Transactions {
			log.Printf("syncing tx, hash=%s\n", tx.Hash())
			receipt := l.client.Eth_GetReceipt(tx.Hash())
			if receipt == nil {
				panic(fmt.Errorf("can't get receipt"))
			}
			if receipt.Status != 1 {
				continue
			}
			for _, rlog := range receipt.Logs {
				if l.isBridgeContract(rlog) && !rlog.Removed {
					if isBurnEvent(rlog) {
						to, amount, burnId, err := l.parseBurnEvent(rlog)
						if err != nil {
							panic(err)
						}
						log.Printf("burn event, index=%d, tx=%s, id=%d, amount=%s, to=%s\n", block.Index, tx.Hash(), burnId, amount.String(), to)
						if amount.Cmp(MinWithdrawValue) < 0 {
							log.Printf("threshold unreached, id=%d, from=%s, amount=%s, to=%s\n", burnId, tx.From(), amount.String(), to)
							continue
						}
						batch.addTask(burnTask{
							txid:   tx.Hash(),
							burnId: burnId,
						})
					}
				}
			}
		}
		err = l.sync(batch)
		if err != nil {
			panic(fmt.Errorf("can't sync block %d: %w", i, err))
		}
		l.lastHeader = &block.Header
		i++
	}
}

func isBurnEvent(rlog *types.Log) bool {
	return len(rlog.Topics) > 0 && rlog.Topics[0] == BurnTopic
}

func (l *Relayer) isBridgeContract(rlog *types.Log) bool {
	return rlog.Address == l.bridge
}

func (l *Relayer) parseBurnEvent(rlog *types.Log) (to common.Address, amount big.Int, burnId uint64, err error) {
	if len(rlog.Topics) < 3 {
		err = errors.New("invalid burn event topics count")
		return
	}
	to = common.BytesToAddress(rlog.Topics[1][:])
	burnId = binary.LittleEndian.Uint64(rlog.Topics[2][24:])
	amount = *big.NewInt(0).SetBytes(rlog.Data)
	return
}

func (l *Relayer) sync(batch *taskBatch) error {
	transactions := []*transaction.Transaction{}
	if len(batch.tasks) > 0 {
		tx, err := l.createHeaderSyncTransaction(&batch.block.Header)
		if err != nil {
			return err
		}
		if tx != nil { //synced already
			transactions = append(transactions, tx)
		}
	}
	var stateroot *state.MPTRoot
	if len(batch.tasks) > 0 {
		sr, err := l.getVerifiedStateRoot(batch.Index())
		if err != nil {
			return err
		}
		tx, err := l.createStateRootSyncTransaction(sr)
		if err != nil {
			return err
		}
		if tx != nil { //synced already
			transactions = append(transactions, tx)
		}
		stateroot = sr
	}
	err := l.commitTransactions(transactions)
	if err != nil {
		return err
	}
	transactions = transactions[:0]
	for _, t := range batch.tasks {
		key := make([]byte, 9)
		key[0] = native.PrefixBurn
		binary.LittleEndian.PutUint64(key[1:], t.burnId)
		tx, err := l.createStateSyncTransaction("withdraw", batch.block, t.txid, stateroot, l.bridge, key)
		if err != nil {
			return err
		}
		if tx == nil { //synced already
			continue
		}
		transactions = append(transactions, tx)
	}
	return l.commitTransactions(transactions)
}

func (l *Relayer) getVerifiedStateRoot(index uint32) (*state.MPTRoot, error) {
	if l.lastStateRoot != nil && l.lastStateRoot.Index >= index {
		return l.lastStateRoot, nil
	}
	if index < l.cfg.VerifiedRootStart {
		index = l.cfg.VerifiedRootStart
	}
	stateIndex := index
	for stateIndex < index+MaxStateRootGetRange {
		stateroot, err := l.client.Eth_GetStateRoot(stateIndex)
		if err != nil || stateroot == nil {
			if l.best { // wait next block, verified stateroot approved in next block
				time.Sleep(15 * time.Second)
				continue
			}
			return nil, fmt.Errorf("can't get state root, err: %w", err)
		}
		if len(stateroot.Witness.InvocationScript) == 0 {
			stateIndex++
			continue
		}
		log.Printf("verified state root found, index=%d", stateIndex)
		l.lastStateRoot = stateroot
		return stateroot, nil
	}
	return nil, errors.New("can't get verified state root, exceeds MaxStateRootGetRange")
}

func (l *Relayer) createHeaderSyncTransaction(header *block.Header) (*transaction.Transaction, error) {
	b, err := io.ToByteArray(header)
	if err != nil {
		return nil, fmt.Errorf("can't deserialize header, err: %w", err)
	}
	tx, err := l.createNeoTransaction(CCMSyncHeader, b)
	if err != nil {
		if strings.Contains(err.Error(), CCMAlreadySyncedError) {
			log.Println("skip synced header")
			return nil, nil
		} else {
			return nil, fmt.Errorf("can't %s, header=%s: %w", CCMSyncHeader, header.Hash(), err)
		}
	}
	log.Printf("created %s tx, txid=%s\n", CCMSyncHeader, tx.Hash())
	return tx, nil
}

func (l *Relayer) createStateRootSyncTransaction(stateroot *state.MPTRoot) (*transaction.Transaction, error) {
	b, err := io.ToByteArray(stateroot)
	if err != nil {
		return nil, fmt.Errorf("can't deserialize stateroot, err: %w", err)
	}
	tx, err := l.createNeoTransaction(CCMSyncStateRoot, b)
	if err != nil {
		if strings.Contains(err.Error(), CCMAlreadySyncedError) {
			log.Println("skip synced state root")
			return nil, nil
		} else {
			return nil, fmt.Errorf("can't sync state root: %w", err)
		}
	}
	log.Printf("created %s tx, txid=%s\n", CCMSyncStateRoot, tx.Hash())
	return tx, nil
}

func (l *Relayer) createStateSyncTransaction(method string, block *block.Block, txid common.Hash, stateroot *state.MPTRoot, contract common.Address, key []byte) (*transaction.Transaction, error) {
	txproof, err := proveTx(block, txid) // TODO: merkle tree reuse
	if err != nil {
		return nil, fmt.Errorf("can't build tx proof: %w", err)
	}
	stateproof, err := l.client.Eth_GetProof(stateroot.Root, contract, key)
	if err != nil {
		return nil, fmt.Errorf("can't get state proof %w", err)
	}
	tx, err := l.createNeoTransaction(method, uint32(block.Index), txid.Bytes(), stateroot.Index, txproof, stateproof)
	if err != nil {
		if strings.Contains(err.Error(), CCMAlreadySyncedError) {
			log.Printf("%s skip synced\n", method)
			return nil, nil
		}
		if method == CCMWithdraw && strings.Contains(err.Error(), CCMAlreadyWithdrawed) {
			log.Printf("%s skip already withdrawed", method)
			return nil, nil
		}
		return nil, err
	}
	log.Printf("created %s tx, txid=%s\n", method, tx.Hash())
	return tx, nil
}

func (l *Relayer) createNeoTransaction(method string, params ...interface{}) (*transaction.Transaction, error) {
	sb := smartcontract.NewBuilder()
	sb.InvokeMethod(l.cfg.BridgeContract, method, params...)
	script, err := sb.Script()
	if err != nil {
		return nil, fmt.Errorf("can't build script, err: %w", err)
	}
	invokeResult, err := l.client.InvokeScript(script, transaction.Signer{
		Account: l.account.ScriptHash(),
		Scopes:  transaction.CalledByEntry,
	})
	if err != nil {
		return nil, fmt.Errorf("failed invoke script, err: %w", err)
	}
	if invokeResult.State != "HALT" {
		return nil, fmt.Errorf("faild invoke script, err: %s", invokeResult.FaultException)
	}
	tx := transaction.New(script, invokeResult.GasConsumed)
	height, err := l.client.GetBlockCount()
	if err != nil {
		return nil, fmt.Errorf("can't get block count, err: %w", err)
	}
	tx.ValidUntilBlock = height + 240
	tx.Signers = []transaction.Signer{{
		Account: l.account.ScriptHash(),
		Scopes:  transaction.CalledByEntry,
	}}
	tx.Scripts = []transaction.Witness{{
		VerificationScript: l.account.GetVerificationScript(),
		InvocationScript:   []byte{},
	}}
	netFee, err := l.client.CalculateNetworkFee(tx)
	if err != nil {
		return nil, fmt.Errorf("can't calculate network fee, err: %w", err)
	}
	tx.NetworkFee = netFee
	err = l.account.SignTx(l.netmode, tx)
	if err != nil {
		return nil, fmt.Errorf("can't sign tx, err: %w", err)
	}
	return tx, nil
}

func (l *Relayer) commitTransactions(transactions []*transaction.Transaction) error {
	if len(transactions) == 0 {
		return nil
	}
	appending := make([]util.Uint256, len(transactions))
	for i, tx := range transactions {
		h, err := l.client.SendRawTransaction(tx)
		if err != nil {
			return err
		}
		appending[i] = h
	}
	retry := 10
	for retry > 0 {
		time.Sleep(BlockTimeSeconds * time.Second)
		rest := make([]util.Uint256, 0, len(appending))
		for _, h := range appending {
			tx, err := l.client.GetRawTransaction(h)
			if err != nil || tx == nil {
				rest = append(rest, h)
			}
		}
		if len(rest) == 0 {
			return nil
		}
		appending = rest
		retry--
	}
	return fmt.Errorf("can't commit transactions: %v", appending)
}

type taskBatch struct {
	block *block.Block
	tasks []burnTask
}

func (b taskBatch) Index() uint32 {
	return uint32(b.block.Index)
}

func (b *taskBatch) addTask(t burnTask) {
	b.tasks = append(b.tasks, t)
}

type burnTask struct {
	txid   common.Hash
	burnId uint64
}
