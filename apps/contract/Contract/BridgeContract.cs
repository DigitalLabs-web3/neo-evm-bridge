using System;
using System.ComponentModel;
using System.Numerics;
using Neo;
using Neo.Cryptography.ECC;
using Neo.SmartContract;
using Neo.SmartContract.Framework;
using Neo.SmartContract.Framework.Attributes;
using Neo.SmartContract.Framework.Native;
using Neo.SmartContract.Framework.Services;

namespace Bridge
{
    [DisplayName("BridgeContract")]
    [ManifestExtra("Author", "NGD")]
    [ManifestExtra("Email", "developer@neo.ngd.org")]
    [ManifestExtra("Description", "This is a bridge contract between Neo and Neo evm layer")]
    public class BridgeContract : SmartContract
    {
        private const ulong DepositThreshold = 100000000; //1GAS
        private const ulong BaseBonus = 3000000; //0.03GAS
        private const byte depositedPrefix = 0x01;
        private static readonly byte[] OwnerKey = new byte[] { 0x02 };
        private static readonly byte[] ValidatorsKey = new byte[] { 0x03 };
        private static readonly byte[] DepositIdKey = new byte[] { 0x04 };
        private const byte PrefxHeader = 0x05;
        private const byte PrefixStateRoot = 0x06;
        private const byte PrefixWithdraw = 0x07;


        private const byte L2PrefixLock = 0x06;
        private const UInt64 L2ChainId = 10086;
        [InitialValue("0xE500000000000000000000000000000000000000", ContractParameterType.Hash160)]
        private static readonly UInt160 L2BridgeAddress = default;

        public delegate void OnDeployedDelegate(UInt160 owner);
        public static event OnDeployedDelegate OnDeployed;
        public delegate void OnDepositedDelegate(BigInteger id, UInt160 from, BigInteger amount, UInt160 to);
        public static event OnDepositedDelegate OnDeposited;
        public delegate void OnValidatorsChangedDelegate(ECPoint[] validators);
        public static event OnValidatorsChangedDelegate OnValidatorsChanged;
        public delegate void OnWithdrawedDelegate(UInt160 from, BigInteger amount, UInt160 to);
        public static event OnWithdrawedDelegate OnWithdrawed;
        public delegate void OnRewardDelegate(UInt160 relayer, BigInteger amount);
        public static event OnRewardDelegate OnRewarded;

        public static string Name()
        {
            return "BridgeContract";
        }

        public static void _deploy(object _, bool isUpdate)
        {
            if (Runtime.CallingScriptHash != ContractManagement.Hash)
                throw new Exception($"{nameof(_deploy)} can't be called directly");
            if (!isUpdate)
            {
                var owner = ((Transaction)Runtime.ScriptContainer).Sender;
                Storage.Put(Storage.CurrentContext, OwnerKey, owner);
                Storage.Put(Storage.CurrentContext, DepositIdKey, 1);
                OnDeployed(owner);
            }
        }

        private static BigInteger NewDepositId()
        {
            var context = Storage.CurrentContext;
            var id = (BigInteger)Storage.Get(context, DepositIdKey);
            Storage.Put(context, DepositIdKey, id + 1);
            return id;
        }

        public static void OnNEP17Payment(UInt160 from, UInt64 amount, object data)
        {
            if (Runtime.CallingScriptHash != GAS.Hash || from == null)
                throw new Exception("only accept gas");
            Deposit(from, amount, data);
        }

        private static void Deposit(UInt160 from, UInt64 amount, object data)
        {
            var to = (UInt160)data;
            if (!to.IsValid || to.IsZero)
                throw new Exception("invalid address on l2");
            if (amount < DepositThreshold)
                throw new Exception($"deposit threshold ({DepositThreshold / 100000000}GAS) unreached ");
            var depositedMap = new StorageMap(depositedPrefix);
            var txHash = ((Transaction)Runtime.ScriptContainer).Hash;
            var state = new DepositState
            {
                TxHash = txHash,
                From = from,
                Amount = amount,
                To = to,
            };
            var id = NewDepositId();
            depositedMap.Put((ByteString)id, (ByteString)state.ToByteArray());
            OnDeposited(id, from, amount, to);
        }

        private static bool OwnerCheck()
        {
            var owner = (UInt160)Storage.Get(Storage.CurrentContext, OwnerKey);
            return Runtime.CheckWitness(owner);
        }

        public static void DesignateValidators(ECPoint[] pks)
        {
            if (!OwnerCheck())
                throw new Exception("permission denied");
            if (!Util.ECPointsCheck(pks))
                throw new Exception("invalid public key");
            var ps = Util.ECPointUnique(pks);
            if (pks.Length != 1 && pks.Length != 4 && pks.Length != 7)// Consistency check with side chain config
                throw new Exception("invalid validators count");
            var txHash = ((Transaction)Runtime.ScriptContainer).Hash;
            var state = new ValidatorsState
            {
                TxHash = txHash,
                Validators = ps,
            };
            Storage.Put(Storage.CurrentContext, ValidatorsKey, state.ToByteArray());
            OnValidatorsChanged(ps);
        }

        public static void Update(ByteString nefFile, string manifest)
        {
            ContractManagement.Update(nefFile, manifest, null);
        }

        public static UInt160 Owner()
        {
            return (UInt160)Storage.Get(Storage.CurrentContext, OwnerKey);
        }

        public static ECPoint[] Validators()
        {
            var raw = (byte[])Storage.Get(Storage.CurrentContext, ValidatorsKey);
            if (raw != null && raw.Length > 0)
            {
                var state = ValidatorsState.FromByteArray(raw);
                return state.Validators;
            }
            return new ECPoint[0];
        }

        /************** withdraw ***************/
        private static byte[] CreateHeaderKey(UInt32 index)
        {
            return Helper.Concat(new byte[] { PrefxHeader }, Util.UInt32ToLittleEndian(index));
        }

        private static byte[] CreateStateRootKey(UInt32 index)
        {
            return Helper.Concat(new byte[] { PrefixStateRoot }, Util.UInt32ToLittleEndian(index));
        }

        private static byte[] CreateWithdrawKey(byte[] burnId)
        {
            return Helper.Concat(new byte[] { PrefixWithdraw }, burnId);
        }

        public static void SyncHeader(byte[] rawHeader)
        {
            var header = Header.FromByteArray(rawHeader);
            ECPoint[] validators = Validators();
            if (!header.Witness.IsSignedBy(validators))
                throw new Exception("invalid consensus");
            if (!header.Verify(L2ChainId))
                throw new Exception("invalid signatures");
            var h = GetHeader(header.Index);
            if (h is not null) throw new Exception("already exists");
            Storage.Put(Storage.CurrentContext, CreateHeaderKey(header.Index), rawHeader);
        }

        private static Header GetHeader(UInt32 index)
        {
            var rawHeader = Storage.Get(Storage.CurrentContext, CreateHeaderKey(index));
            if (rawHeader is null) return null;
            return Header.FromByteArray((byte[])rawHeader);
        }

        public static void SyncStateRoot(byte[] rawStateRoot)
        {
            var stateroot = StateRoot.FromByteArray(rawStateRoot);
            if (!stateroot.Witness.IsSignedBy(Validators()))
                throw new Exception("invalid consensus");
            if (!stateroot.Verify(L2ChainId))
                throw new Exception("invalid signatures");
            var h = GetStateRoot(stateroot.Index);
            if (h is not null) throw new Exception("already exists");
            Storage.Put(Storage.CurrentContext, CreateStateRootKey(stateroot.Index), rawStateRoot);
        }

        private static StateRoot GetStateRoot(UInt32 index)
        {
            var rawStateRoot = Storage.Get(Storage.CurrentContext, CreateStateRootKey(index));
            if (rawStateRoot is null) return null;
            return StateRoot.FromByteArray((byte[])rawStateRoot);
        }

        public static void Withdraw(UInt32 hindex, UInt256 txHash, UInt32 rindex, byte[] merkleProof, byte[] stateProof)
        {
            var header = GetHeader(hindex);
            if (header is null) throw new Exception("header not found");
            if (rindex < hindex) throw new Exception("invalid state root index");
            var stateroot = GetStateRoot(rindex);
            if (stateroot is null) throw new Exception("state root not found");
            var (ok1, path, hashes) = ParseMerkleProof(merkleProof);
            if (!ok1)
                throw new Exception("invalid merkle proof");
            if (!MerkleTree.VerifyProof(header.MerkleRoot, txHash, path, hashes))
                throw new Exception("invalid tx hash");
            var (ok2, key, value) = MPT.VerifyProof(stateProof, stateroot.RootHash);
            if (!ok2)
                throw new Exception("invalid state proof");
            var burnId = ParseBurnId(key);
            var state = DepositState.FromByteArray(value);
            if (state.TxHash != txHash)
                throw new Exception("tx hash unmatch");
            if (state.Amount < DepositThreshold)
                throw new Exception($"threshold ({DepositThreshold / 100000000}GAS) unreached");
            var withdrawKey = CreateWithdrawKey(burnId);
            var withdrawed = Storage.Get(Storage.CurrentContext, withdrawKey);
            if (withdrawed is not null)
                throw new Exception("already withdrawed");
            var selfBalance = GAS.BalanceOf(Runtime.ExecutingScriptHash);
            if (selfBalance < state.Amount)
                throw new Exception("insufficient deposited balance for withdraw");
            var tx = (Transaction)(Runtime.ScriptContainer);
            var withdrawedState = new WithdrawedState
            {
                TxHash = tx.Hash,
            };
            Storage.Put(Storage.CurrentContext, withdrawKey, withdrawedState.ToByteArray());
            var relayer = tx.Sender;
            var actualAmount = state.Amount - BaseBonus;
            var withdrawOk = GAS.Transfer(Runtime.ExecutingScriptHash, state.To, state.Amount - BaseBonus, state.From);
            if (!withdrawOk)
                throw new Exception("can't withdraw");
            var rewardOk = GAS.Transfer(Runtime.ExecutingScriptHash, relayer, BaseBonus, "relay bonus");
            if (!rewardOk)
                throw new Exception("can't reward");
            OnWithdrawed(state.From, actualAmount, state.To);
            OnRewarded(relayer, BaseBonus);
        }

        private static byte[] ParseBurnId(byte[] key)
        {
            //20 contract address + 1 lock prefix + 8 lock id
            if (key.Length < 29)
                throw new Exception("invalid key");
            var contract = (UInt160)key[0..20];
            if (!Util.ByteStringEqual(contract, L2BridgeAddress))
                throw new Exception("invalid l2 bridge address");
            if (L2PrefixLock != key[20])
                throw new Exception("invalid lock prefix");
            return key[21..];
        }

        private static (bool, uint, UInt256[]) ParseMerkleProof(byte[] mproof)
        {
            uint path = 0;
            var hashes = new UInt256[0];
            if (mproof.Length < 4)
                return (false, path, hashes);
            path = Util.UInt32FromLittleEndian(mproof[0..4]);
            var count = mproof.Length / 32;
            hashes = new UInt256[count];
            for (int i = 0; i < count; i++)
            {
                var start = 4 + i * 32;
                var end = 4 + (i + 1) * 32;
                if (end > mproof.Length) return (false, path, hashes);
                hashes[i] = (UInt256)mproof[start..end];
            }
            return (true, path, hashes);
        }

        public static UInt256 GetWithdrawed(UInt64 burnId)
        {
            var bytes = Util.UInt64ToLittleEndian(burnId);
            var key = CreateWithdrawKey(bytes);
            var withdrawed = Storage.Get(Storage.CurrentContext, key);
            if (withdrawed is null)
                return null;
            return WithdrawedState.FromByteArray((byte[])withdrawed).TxHash;
        }
    }
}
