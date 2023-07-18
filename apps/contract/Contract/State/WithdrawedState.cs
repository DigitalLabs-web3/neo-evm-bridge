using System;
using Neo;

namespace Bridge
{
    public class WithdrawedState
    {
        public UInt256 TxHash;

        public void Serialize(BufferWriter writer)
        {
            writer.WriteUint256(TxHash);
        }

        public void Deserialize(BufferReader reader)
        {
            TxHash = reader.ReadUint256();
        }

        public byte[] ToByteArray()
        {
            var writer = new BufferWriter();
            Serialize(writer);
            return writer.GetBytes();
        }

        public static WithdrawedState FromByteArray(byte[] data)
        {
            var state = new WithdrawedState();
            var reader = new BufferReader(data);
            state.Deserialize(reader);
            return state;
        }
    }
}
