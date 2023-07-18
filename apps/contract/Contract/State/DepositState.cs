using System;
using Neo;

namespace Bridge
{
    public class DepositState
    {
        public UInt256 TxHash;
        public UInt160 From;
        public UInt64 Amount;
        public UInt160 To;

        public void Serialize(BufferWriter writer)
        {
            writer.WriteUint256(TxHash);
            writer.WriteUint160(From);
            writer.WriteUInt64(Amount);
            writer.WriteUint160(To);
        }

        public void Deserialize(BufferReader reader)
        {
            TxHash = reader.ReadUint256();
            From = reader.ReadUint160();
            Amount = reader.ReadUint64();
            To = reader.ReadUint160();
        }

        public byte[] ToByteArray()
        {
            var writer = new BufferWriter();
            Serialize(writer);
            return writer.GetBytes();
        }

        public static DepositState FromByteArray(byte[] data)
        {
            var state = new DepositState();
            var reader = new BufferReader(data);
            state.Deserialize(reader);
            return state;
        }
    }
}
