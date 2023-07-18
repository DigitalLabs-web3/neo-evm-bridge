using Neo;
using Neo.Cryptography.ECC;
using Neo.SmartContract.Framework;
using Neo.SmartContract.Framework.Native;
using Neo.SmartContract.Framework.Services;

namespace Bridge
{
    public class Witness
    {
        public byte[] InvocationScript;
        public byte[] VerificationScript;

        private ECPoint[] signers;
        private int m;
        private byte[][] signatures;

        public UInt160 Address()
        {
            return (UInt160)CryptoLib.Ripemd160((ByteString)VerificationScript);
        }

        public void Deserialize(BufferReader reader)
        {
            InvocationScript = reader.ReadVarBytes();
            ParseInvocationScript();
            VerificationScript = reader.ReadVarBytes();
            ParseVerificationScript();
        }

        private void ParseInvocationScript()
        {
            var reader = new BufferReader(InvocationScript);
            var count = reader.ReadVarUint();
            signatures = new byte[count][];
            for (int i = 0; i < (int)count; i++)
                signatures[i] = reader.ReadVarBytes();
        }

        private void ParseVerificationScript()
        {
            var reader = new BufferReader(VerificationScript);
            m = (int)reader.ReadVarUint();
            var count = (int)reader.ReadVarUint();
            signers = new ECPoint[count];
            for (int i = 0; i < count; i++)
                signers[i] = (ECPoint)reader.ReadBytes(33);
            if (m != Util.CalculateBFTCount(count))
                throw new System.Exception("invalid verification script");
        }

        public bool IsSignedBy(ECPoint[] validators)
        {
            var l1 = signers.Length;
            int l2 = validators.Length;
            if (l1 != l2) return false;
            int signed = 0;
            foreach (var p in signers)
                foreach (var validator in validators)
                    if (Util.ECPointEqual(p, validator))
                    {
                        signed++;
                        break;
                    }
            return signed == signers.Length;
        }

        public bool VerifyMessage(byte[] message)
        {
            int signed = 0;
            foreach (var p in signers)
                foreach (var signature in signatures)
                {
                    if (CryptoLib.VerifyWithECDsa((ByteString)message, p, (ByteString)signature, NamedCurve.secp256k1))
                        signed++;
                }
            return signed == m;
        }
    }
}
