package relay

import (
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"math/big"
	"strings"
	"time"

	"github.com/DigitalLabs-web3/neo-evm-bridge/relayer/config"
	"github.com/DigitalLabs-web3/neo-evm-bridge/relayer/constantclient"
	"github.com/DigitalLabs-web3/neo-go-evm/pkg/crypto/keys"
	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/nspcc-dev/neo-go/pkg/core/block"
	"github.com/nspcc-dev/neo-go/pkg/smartcontract/trigger"
	"github.com/nspcc-dev/neo-go/pkg/util"
	"github.com/nspcc-dev/neo-go/pkg/vm/stackitem"
	"github.com/nspcc-dev/neo-go/pkg/vm/vmstate"

	sstate "github.com/DigitalLabs-web3/neo-go-evm/pkg/core/state"
	"github.com/DigitalLabs-web3/neo-go-evm/pkg/core/transaction"
	sresult "github.com/DigitalLabs-web3/neo-go-evm/pkg/rpc/response/result"
	"github.com/DigitalLabs-web3/neo-go-evm/pkg/wallet"
	"github.com/nspcc-dev/neo-go/pkg/core/state"
)

const (
	DepositPrefix                     = 0x01
	ValidatorsKey                     = 0x03
	StateValidatorRole                = 4
	BlockTimeSeconds                  = 15
	MaxStateRootGetRange              = 57600
	MintThreshold                     = 100000000
	RoleManagementContract            = "49cf4e5378ffcd4dec034fd98a174c5491e395e2"
	BridgeContractName                = "Bridge"
	CCMSyncHeader                     = "syncHeader"
	CCMSyncStateRoot                  = "syncStateRoot"
	CCMSyncValidators                 = "syncValidators"
	CCMSyncStateRootValidatorsAddress = "syncStateRootValidatorsAddress"
	CCMRequestMint                    = "requestMint"
	CCMAlreadySyncedError             = "already synced"

	DepositedEventName            = "OnDeposited"
	ValidatorsDesignatedEventName = "OnValidatorsChanged"
)

type Relayer struct {
	cfg                           *config.Config
	lastHeader                    *block.Header
	lastStateRoot                 *state.MPTRoot
	roleManagementContractAddress util.Uint160
	client                        *constantclient.ConstantClient
	bridge                        *sstate.NativeContract
	account                       *wallet.Account
	best                          bool
}

func NewRelayer(cfg *config.Config, acc *wallet.Account) (*Relayer, error) {
	roleManagement, err := util.Uint160DecodeStringLE(RoleManagementContract)
	if err != nil {
		return nil, err
	}
	client := constantclient.New(cfg.MainSeeds, cfg.SideSeeds)
	bridge, err := client.Eth_NativeContract(BridgeContractName)
	if err != nil {
		return nil, fmt.Errorf("can't get bridge contract %w", err)
	}
	return &Relayer{
		cfg:                           cfg,
		roleManagementContractAddress: roleManagement,
		client:                        client,
		bridge:                        bridge,
		account:                       acc,
		best:                          false,
	}, nil
}

func (l *Relayer) Run() {
	for i := l.cfg.Start; l.cfg.End == 0 || i < l.cfg.End; {
		if l.best {
			time.Sleep(15 * time.Second)
		}
		log.Printf("syncing block, index=%d", i)
		block, _ := l.client.GetBlock(i)
		if block == nil {
			if !l.best {
				h, err := l.client.GetBlockCount()
				if err != nil {
					panic(err)
				}
				if i >= h {
					l.best = true
				}
			}
			continue
		}
		batch := new(taskBatch)
		batch.block = block
		batch.isJoint = l.isJointHeader(&block.Header)
		if batch.isJoint {
			log.Printf("joint header, index=%d, hash=%s\n", block.Index, block.Hash())
		}
		for _, tx := range block.Transactions {
			log.Printf("syncing tx, hash=%s\n", tx.Hash())
			applicationlog, err := l.client.GetApplicationLog(tx.Hash())
			if applicationlog == nil {
				panic(fmt.Errorf("can't get application log, err: %w", err))
			}
			for _, execution := range applicationlog.Executions {
				if execution.Trigger == trigger.Application && execution.VMState == vmstate.Halt {
					for _, nevent := range execution.Events {
						event := &nevent
						if l.isBridgeContract(event) {
							if isDepositEvent(event) {
								requestId, from, amount, to, err := l.parseDepositEvent(event)
								if err != nil {
									panic(err)
								}
								log.Printf("deposit event, index=%d, tx=%s, id=%d, from=%s, amount=%d, to=%s\n", block.Index, tx.Hash(), requestId, from, amount, to)
								if amount < MintThreshold {
									log.Printf("threshold unreached, id=%d, from=%s, amount=%d, to=%s\n", requestId, from, amount, to)
									continue
								}
								batch.addTask(depositTask{
									txid:      tx.Hash(),
									requestId: requestId,
								})
							} else if isDesignateValidatorsEvent(event) {
								pks, err := l.parseDesignateValidatorsEvent(event)
								if err != nil {
									panic(err)
								}
								log.Printf("validators designate event, index=%d, tx=%s, pks=%s\n", block.Index, tx.Hash(), pks)
								batch.addTask(validatorsDesignateTask{
									txid: tx.Hash(),
								})
							}
						} else if l.isRoleManagement(event) {
							isStateValidatorsDesignate, index, err := l.parseStateValidatorsDesignatedEvent(event)
							if err != nil {
								panic(err)
							}
							if isStateValidatorsDesignate {
								log.Printf("state validators designate event, index=%d, tx=%s,index=%d\n", block.Index, tx.Hash(), index)
								batch.addTask(stateValidatorsChangeTask{
									txid:  tx.Hash(),
									index: index,
								})
							}
						}
					}
				}
			}
		}
		err := l.sync(batch)
		if err != nil {
			panic(fmt.Errorf("can't sync block %d: %w", i, err))
		}
		if batch.isJoint || len(batch.tasks) > 0 {
			l.best = false
		}
		l.lastHeader = &block.Header
		i++
	}
}

func (l *Relayer) isJointHeader(header *block.Header) bool {
	if l.lastHeader == nil && header.Index > 0 {
		block, _ := l.client.GetBlock(uint32(header.Index) - 1)
		l.lastHeader = &block.Header
	}
	return header.Index == 0 || l.lastHeader.NextConsensus != header.NextConsensus
}

func (l *Relayer) isRoleManagement(event *state.NotificationEvent) bool {

	return event.ScriptHash == l.roleManagementContractAddress
}

func (l *Relayer) parseStateValidatorsDesignatedEvent(event *state.NotificationEvent) (bool, uint32, error) {
	if event.Name != "Designation" {
		return false, 0, nil
	}
	arr := event.Item.Value().([]stackitem.Item)
	if len(arr) != 2 {
		return false, 0, errors.New("invalid role deposite event arguments count")
	}
	role, err := arr[0].TryInteger()
	if err != nil {
		return false, 0, fmt.Errorf("can't parse role: %w", err)
	}
	if role.Int64() != StateValidatorRole {
		return false, 0, nil
	}
	index, err := arr[1].TryInteger()
	if err != nil {
		return false, 0, fmt.Errorf("can't parse index: %w", err)
	}
	return true, uint32(index.Uint64()), nil
}

func isDepositEvent(event *state.NotificationEvent) bool {
	return event.Name == DepositedEventName
}

func (l *Relayer) parseDepositEvent(event *state.NotificationEvent) (requestId uint64, from util.Uint160, amount uint64, to util.Uint160, err error) {
	arr := event.Item.Value().([]stackitem.Item)
	if len(arr) != 4 {
		err = errors.New("invalid deposited event arguments count")
		return
	}
	id, err := arr[0].TryInteger()
	if err != nil {
		err = fmt.Errorf("can't parse request id: %w", err)
		return
	}
	requestId = id.Uint64()
	if arr[1].Type() != stackitem.ByteArrayT {
		err = errors.New("invalid from type in deposit event")
		return
	}
	b, err := arr[1].TryBytes()
	if err != nil {
		err = fmt.Errorf("can't parse from: %w", err)
		return
	}
	bf, err := util.Uint160DecodeBytesBE(b)
	if err != nil {
		err = fmt.Errorf("can't parse from: %w", err)
		return
	}
	from = bf
	if arr[2].Type() != stackitem.IntegerT {
		panic("invalid amount type in deposit event")
	}
	amt, err := arr[2].TryInteger()
	if err != nil {
		err = fmt.Errorf("can't parse amount: %w", err)
		return
	}
	amount = amt.Uint64()
	if arr[3].Type() != stackitem.ByteArrayT {
		err = errors.New("invalid to type in deposit event")
		return
	}
	b, err = arr[3].TryBytes()
	if err != nil {
		err = fmt.Errorf("can't parse to: %w", err)
		return
	}
	bt, err := util.Uint160DecodeBytesBE(b)
	if err != nil {
		err = fmt.Errorf("can't parse to: %w", err)
		return
	}
	to = bt
	return requestId, from, amount, to, nil
}

func isDesignateValidatorsEvent(event *state.NotificationEvent) bool {
	return event.Name == ValidatorsDesignatedEventName
}

func (l *Relayer) parseDesignateValidatorsEvent(event *state.NotificationEvent) (pks keys.PublicKeys, err error) {
	arr := event.Item.Value().([]stackitem.Item)
	if len(arr) != 1 {
		err = errors.New("invalid validators change arguments count")
		return
	}
	arr = arr[0].Value().([]stackitem.Item)
	pks = make([]*keys.PublicKey, len(arr))
	for i, p := range arr {
		if p.Type() != stackitem.ByteArrayT {
			err = errors.New("invalid ecpoint type in validators change event")
			return
		}
		pk, e := p.TryBytes()
		if e != nil {
			err = fmt.Errorf("can't parse ecpoint base64: %w", e)
			return
		}
		pt, e := keys.NewPublicKeyFromBytes(pk, btcec.S256())
		if e != nil {
			err = fmt.Errorf("can't parse ecpoint, pk=%s, err=%w", hex.EncodeToString(pk), e)
			return
		}
		pks[i] = pt
	}
	return pks, nil
}

func (l *Relayer) isBridgeContract(notification *state.NotificationEvent) bool {
	return notification.ScriptHash == l.cfg.BridgeContract
}

func (l *Relayer) sync(batch *taskBatch) error {
	transactions := []*types.Transaction{}
	if batch.isJoint || len(batch.tasks) > 0 {
		tx, err := l.createHeaderSyncTransaction(&batch.block.Header)
		if err != nil {
			return err
		}
		if tx != nil { //synced already
			transactions = append(transactions, tx)
		}
	}
	err := l.commitTransactions(transactions)
	if err != nil {
		return err
	}
	transactions = transactions[:0]
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
	err = l.commitTransactions(transactions)
	if err != nil {
		return err
	}
	transactions = transactions[:0]
	for _, t := range batch.tasks {
		var (
			key      []byte
			method   string
			contract util.Uint160 = l.cfg.BridgeContract
		)
		switch v := t.(type) {
		case depositTask:
			method = CCMRequestMint
			key = append([]byte{DepositPrefix}, big.NewInt(int64(v.requestId)).Bytes()...)
		case validatorsDesignateTask:
			method = CCMSyncValidators
			key = []byte{ValidatorsKey}
		case stateValidatorsChangeTask:
			method = CCMSyncStateRootValidatorsAddress
			key = make([]byte, 5)
			key[0] = StateValidatorRole
			binary.BigEndian.PutUint32(key[1:], v.index+1)
			contract = l.roleManagementContractAddress
		default:
			return errors.New("unkown task")
		}
		tx, err := l.createStateSyncTransaction(method, batch.block, t.TxId(), stateroot, contract, key)
		if err != nil {
			return err
		}
		if tx == nil { //synced already
			continue
		}
		err = l.commitTransactions([]*types.Transaction{tx})
		if err != nil {
			return err
		}
	}
	return nil
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
		stateroot, err := l.client.GetStateRoot(stateIndex)
		if err != nil {
			if l.best { // wait next block, verified stateroot approved in next block
				time.Sleep(15 * time.Second)
				continue
			}
			return nil, fmt.Errorf("can't get state root,  %w", err)
		}
		if len(stateroot.Witness) == 0 {
			stateIndex++
			continue
		}
		log.Printf("verified state root found, index=%d", stateIndex)
		l.lastStateRoot = stateroot
		return stateroot, nil
	}
	return nil, errors.New("can't get verified state root, exceeds MaxStateRootGetRange")
}

func (l *Relayer) invokeObjectSync(method string, object []byte) (*types.Transaction, error) {
	data, err := l.bridge.Abi.Pack(method, object)
	if err != nil {
		return nil, fmt.Errorf("can't pack sync object, method=%s: %w", method, err)
	}
	return l.createEthLayerTransaction(data)
}

func (l *Relayer) createHeaderSyncTransaction(rpcHeader *block.Header) (*types.Transaction, error) {
	b, err := blockHeaderToBytes(mainHeaderToSideHeader(rpcHeader))
	if err != nil {
		return nil, fmt.Errorf("can't encode block header: %w", err)
	}
	tx, err := l.invokeObjectSync(CCMSyncHeader, b)
	if err != nil {
		if strings.Contains(err.Error(), CCMAlreadySyncedError) {
			log.Println("skip synced header")
			return nil, nil
		} else {
			return nil, fmt.Errorf("can't %s, header=%s: %w", CCMSyncHeader, rpcHeader.Hash(), err)
		}
	}
	log.Printf("created %s tx, txid=%s\n", CCMSyncHeader, tx.Hash())
	return tx, nil
}

func (l *Relayer) createStateRootSyncTransaction(stateroot *state.MPTRoot) (*types.Transaction, error) {
	b, err := staterootToBytes(mainStateRootToSideStateRoot(stateroot))
	if err != nil {
		return nil, fmt.Errorf("can't encode stateroot: %w", err)
	}
	tx, err := l.invokeObjectSync(CCMSyncStateRoot, b)
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

func (l *Relayer) invokeStateSync(method string, index uint32, txid util.Uint256, txproof []byte, rootIndex uint32, stateproof []byte) (*types.Transaction, error) {
	data, err := l.bridge.Abi.Pack(method, index, big.NewInt(0).SetBytes(common.BytesToHash(txid.BytesBE()).Bytes()), txproof, rootIndex, stateproof)
	if err != nil {
		return nil, err
	}
	return l.createEthLayerTransaction(data)
}

func (l *Relayer) createStateSyncTransaction(method string, block *block.Block, txid util.Uint256, stateroot *state.MPTRoot, contract util.Uint160, key []byte) (*types.Transaction, error) {
	txproof, err := proveTx(block, txid) // TODO: merkle tree reuse
	if err != nil {
		return nil, fmt.Errorf("can't build tx proof: %w", err)
	}
	stateproof, err := l.client.GetProof(stateroot.Root, contract, key)
	if err != nil {
		return nil, fmt.Errorf("can't get state proof %w", err)
	}
	tx, err := l.invokeStateSync(method, uint32(block.Index), txid, txproof, stateroot.Index, stateproof)
	if err != nil {
		if strings.Contains(err.Error(), CCMAlreadySyncedError) {
			log.Printf("%s skip synced\n", method)
			return nil, nil
		}
		if method == CCMSyncValidators && strings.Contains(err.Error(), "synced validators outdated") {
			log.Printf("%s skip synced validators", method)
			return nil, nil
		}
		if method == CCMRequestMint && strings.Contains(err.Error(), "already minted") {
			log.Printf("%s skip synced mint", method)
			return nil, nil
		}
		return nil, err
	}
	log.Printf("created %s tx, txid=%s\n", method, tx.Hash())
	return tx, nil
}

func (l *Relayer) createEthLayerTransaction(data []byte) (*types.Transaction, error) {
	var err error
	chainId := l.client.Eth_ChainId()
	gasPrice := l.client.Eth_GasPrice()
	nonce := l.client.Eth_GetTransactionCount(l.account.Address)
	ltx := &types.LegacyTx{
		Nonce:    nonce,
		To:       &(l.bridge.Address),
		GasPrice: gasPrice,
		Value:    big.NewInt(0),
		Data:     data,
	}
	tx := &transaction.Transaction{
		Transaction: *types.NewTx(ltx),
	}
	gas, err := l.client.Eth_EstimateGas(&sresult.TransactionObject{
		From:     l.account.Address,
		To:       tx.To(),
		GasPrice: tx.GasPrice(),
		Value:    tx.Value(),
		Data:     tx.Data(),
	})
	if err != nil {
		return nil, err
	}
	ltx.Gas = gas
	tx.Transaction = *types.NewTx(ltx)
	err = l.account.SignTx(chainId, tx)
	if err != nil {
		return nil, fmt.Errorf("can't sign tx: %w", err)
	}
	return &tx.Transaction, nil
}

func (l *Relayer) commitTransactions(transactions []*types.Transaction) error {
	if len(transactions) == 0 {
		return nil
	}
	appending := make([]common.Hash, len(transactions))
	for i, tx := range transactions {
		b, err := tx.MarshalBinary()
		if err != nil {
			return err
		}
		h, err := l.client.Eth_SendRawTransaction(b)
		if err != nil {
			return err
		}
		appending[i] = h
	}
	retry := 10
	for retry > 0 {
		time.Sleep(BlockTimeSeconds * time.Second)
		rest := make([]common.Hash, 0, len(appending))
		for _, h := range appending {
			txResp := l.client.Eth_GetTransactionByHash(h)
			if txResp == nil {
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
	block   *block.Block
	isJoint bool
	tasks   []task
}

func (b taskBatch) Index() uint32 {
	return uint32(b.block.Index)
}

func (b *taskBatch) addTask(t task) {
	b.tasks = append(b.tasks, t)
}

type task interface {
	TxId() util.Uint256
}

type depositTask struct {
	txid      util.Uint256
	requestId uint64
}

func (t depositTask) TxId() util.Uint256 {
	return t.txid
}

type validatorsDesignateTask struct {
	txid util.Uint256
}

func (t validatorsDesignateTask) TxId() util.Uint256 {
	return t.txid
}

type stateValidatorsChangeTask struct {
	txid  util.Uint256
	index uint32
}

func (t stateValidatorsChangeTask) TxId() util.Uint256 {
	return t.txid
}
