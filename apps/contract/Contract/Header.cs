using System;
using Neo;
using Neo.SmartContract.Framework;
using Neo.SmartContract.Framework.Native;
using Neo.SmartContract.Framework.Services;

namespace Bridge
{
    public class Header
    {
        public UInt32 Version;
        public UInt256 PreHash;
        public UInt256 MerkleRoot;
        public UInt64 Timestamp;
        public UInt64 Nonce;
        public UInt32 Index;
        public byte PrimaryIndex;
        public UInt160 NextConsensus;
        public Witness Witness;

        public UInt256 Hash;

        public bool Verify(UInt64 chainId)
        {
            return Witness.VerifyMessage(GetSignData(chainId));
        }

        public byte[] GetSignData(UInt64 chainId)
        {
            var data = Helper.Concat(Util.UInt64ToLittleEndian(chainId), (byte[])Hash);
            return data;
        }

        private void DeserializeHashable(BufferReader reader)
        {
            Version = reader.ReadUint32();
            PreHash = reader.ReadUint256();
            MerkleRoot = reader.ReadUint256();
            Timestamp = reader.ReadUint64();
            Nonce = reader.ReadUint64();
            Index = reader.ReadUint32();
            PrimaryIndex = reader.ReadByte();
            NextConsensus = reader.ReadUint160();
        }

        public void Deserialize(BufferReader reader)
        {
            DeserializeHashable(reader);
            Hash = (UInt256)CryptoLib.Sha256((ByteString)reader.Readed());
            Witness = new Witness();
            Witness.Deserialize(reader);
        }

        public static Header FromByteArray(byte[] b)
        {
            var reader = new BufferReader(b);
            var hdr = new Header();
            hdr.Deserialize(reader);
            return hdr;
        }
    }
}
