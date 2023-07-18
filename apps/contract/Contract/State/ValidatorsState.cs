using System;
using Neo;
using Neo.Cryptography.ECC;
using Neo.SmartContract.Framework;

namespace Bridge
{
    public class ValidatorsState
    {
        public UInt256 TxHash;
        public ECPoint[] Validators;

        public void Serialize(BufferWriter writer)
        {
            writer.WriteUint256(TxHash);
            writer.WriteVarUint((ulong)Validators.Length);
            foreach (var p in Validators)
                writer.WriteVarBytes((byte[])p);
        }

        public void Deserialize(BufferReader reader)
        {
            TxHash = reader.ReadUint256();
            var length = (int)reader.ReadVarUint();
            Validators = new ECPoint[length];
            for (int i = 0; i < length; i++)
                Validators[i] = (ECPoint)reader.ReadVarBytes();
        }

        public byte[] ToByteArray()
        {
            var writer = new BufferWriter();
            Serialize(writer);
            return writer.GetBytes();
        }

        public static ValidatorsState FromByteArray(byte[] data)
        {
            var state = new ValidatorsState();
            var reader = new BufferReader(data);
            state.Deserialize(reader);
            return state;
        }
    }
}
