package relay

import (
	"crypto/elliptic"
	"encoding/binary"
	"errors"
	"fmt"
	"log"
	"math/big"
	"strings"
	"time"

	"github.com/ZhangTao1596/neo-evm-bridge/config"
	"github.com/ZhangTao1596/neo-evm-bridge/constantclient"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/nspcc-dev/neo-go/pkg/core/block"
	"github.com/nspcc-dev/neo-go/pkg/crypto/keys"
	"github.com/nspcc-dev/neo-go/pkg/smartcontract/trigger"
	"github.com/nspcc-dev/neo-go/pkg/util"
	"github.com/nspcc-dev/neo-go/pkg/vm/stackitem"
	"github.com/nspcc-dev/neo-go/pkg/vm/vmstate"

	sstate "github.com/neo-ngd/neo-go/pkg/core/state"
	"github.com/neo-ngd/neo-go/pkg/core/transaction"
	sresult "github.com/neo-ngd/neo-go/pkg/rpc/response/result"
	"github.com/neo-ngd/neo-go/pkg/wallet"
	"github.com/nspcc-dev/neo-go/pkg/core/state"
)

const (
	DepositPrefix                     = 0x01
	ValidatorsKey                     = 0x03
	StateValidatorRole                = 4
	BlockTimeSeconds                  = 15
	MaxStateRootTryCount              = 1000
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
}

func NewRelayer(cfg *config.Config, acc *wallet.Account) (*Relayer, error) {
	roleManagement, err := util.Uint160DecodeStringLE(RoleManagementContract)
	if err != nil {
		return nil, err
	}
	client := constantclient.New(cfg.MainSeeds, cfg.SideSeeds)
	bridge := client.Eth_NativeContract(BridgeContractName)
	if bridge == nil {
		return nil, errors.New("can't get bridge contract")
	}
	return &Relayer{
		cfg:                           cfg,
		roleManagementContractAddress: roleManagement,
		client:                        client,
		bridge:                        bridge,
		account:                       acc,
	}, nil
}

func (l *Relayer) Run() {
	for i := l.cfg.Start; i < l.cfg.End; {
		log.Printf("syncing block, index=%d", i)
		block := l.client.GetBlock(i)
		if block == nil {
			time.Sleep(15 * time.Second)
			continue
		}
		batch := new(taskBatch)
		batch.block = block
		batch.isJoint = l.isJointHeader(&block.Header)
		for _, tx := range block.Transactions {
			log.Printf("syncing tx, hash=%s\n", tx.Hash())
			applicationlog := l.client.GetApplicationLog(tx.Hash())
			for _, execution := range applicationlog.Executions {
				if execution.Trigger == trigger.Application && execution.VMState == vmstate.Halt {
					for _, nevent := range execution.Events {
						event := &nevent
						if l.isManageContract(event) {
							if isDepositEvent(event) {
								requestId, from, amount, to, err := l.parseDepositEvent(event)
								if err != nil {
									panic(err)
								}
								log.Printf("deposit event, id=%d, from=%s, amount=%d, to=%s\n", requestId, from, amount, to)
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
								log.Printf("validators designate event, pks=%s\n", pks)
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
								log.Printf("state validators designate event, index=%d\n", index)
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
		l.lastHeader = &block.Header
		i++
	}
}

func (l *Relayer) isJointHeader(header *block.Header) bool {
	if l.lastHeader == nil && header.Index > 0 {
		block := l.client.GetBlock(uint32(header.Index) - 1)
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
		pkb, e := p.TryBytes()
		if e != nil {
			err = fmt.Errorf("can't parse ecpoint base64: %w", e)
			return
		}
		pt, e := keys.NewPublicKeyFromBytes(pkb, elliptic.P256())
		if err != nil {
			err = fmt.Errorf("can't parse ecpoint: %w", e)
			return
		}
		pks[i] = pt
	}
	return pks, nil
}

func (l *Relayer) isManageContract(notification *state.NotificationEvent) bool {
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
	fmt.Println(len(batch.tasks))
	for _, t := range batch.tasks {
		var (
			key    []byte
			method string
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
			binary.BigEndian.PutUint32(key[1:], v.index)
		default:
			return errors.New("unkown task")
		}
		tx, err := l.createStateSyncTransaction(method, batch.block, t.TxId(), stateroot, key)
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
	for stateIndex := index; stateIndex < index+MaxStateRootTryCount; stateIndex++ {
		stateroot := l.client.GetStateRoot(stateIndex)
		if stateroot == nil {
			return nil, errors.New("can't get state root")
		}
		if len(stateroot.Witness) == 0 {
			continue
		}
		log.Printf("verified state root found, index=%d", stateIndex)
		l.lastStateRoot = stateroot
		return stateroot, nil
	}
	return nil, errors.New("can't get verified state root")
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
			return nil, fmt.Errorf("can't %s, header=%s, h=%s,: %w", CCMSyncHeader, rpcHeader.Hash(), rpcHeader.Hash(), err)
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

func (l *Relayer) createStateSyncTransaction(method string, block *block.Block, txid util.Uint256, stateroot *state.MPTRoot, key []byte) (*types.Transaction, error) {
	txproof, err := proveTx(block, txid) // TODO: merkle tree reuse
	if err != nil {
		return nil, fmt.Errorf("can't build tx proof: %w", err)
	}
	stateproof := l.client.GetProof(stateroot.Root, l.cfg.BridgeContract, key)
	if stateproof == nil {
		return nil, errors.New("can't get state proof")
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
	tx := &transaction.EthTx{
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
	err = l.account.SignTx(chainId, transaction.NewTx(tx))
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
	retry := 3
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
	return fmt.Errorf("can't commit transactions: [%v]", appending)
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
