using System;
using Neo;
using Neo.SmartContract.Framework;
using Neo.SmartContract.Framework.Native;

namespace Bridge
{
    public class StateRoot
    {
        public byte Version;
        public UInt32 Index;
        public UInt256 RootHash;
        public Witness Witness;

        public UInt256 Hash;

        public bool Verify(UInt64 chainId)
        {
            return Witness.VerifyMessage(GetSignData(chainId));
        }

        public byte[] GetSignData(UInt64 chainId)
        {
            return Helper.Concat(Util.UInt64ToLittleEndian(chainId), (byte[])Hash);
        }

        private void DeserializeHashable(BufferReader reader)
        {
            Version = reader.ReadByte();
            Index = reader.ReadUint32();
            RootHash = reader.ReadUint256();
        }

        public void Deserialize(BufferReader reader)
        {
            DeserializeHashable(reader);
            Hash = (UInt256)CryptoLib.Sha256((ByteString)reader.Readed());
            Witness = new Witness();
            Witness.Deserialize(reader);
        }

        public static StateRoot FromByteArray(byte[] b)
        {
            var reader = new BufferReader(b);
            var st = new StateRoot();
            st.Deserialize(reader);
            return st;
        }
    }
}
