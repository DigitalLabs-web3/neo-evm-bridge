using System;
using Neo;
using Neo.SmartContract.Framework;
using Neo.SmartContract.Framework.Native;
using Neo.SmartContract.Framework.Services;

namespace Bridge
{
    public class MerkleTree
    {
        public static bool VerifyProof(UInt256 root, UInt256 target, UInt32 path, UInt256[] hashes)
        {
            return root == CalculateMerkleRoot(target, path, hashes);
        }

        private static UInt256 CalculateMerkleRoot(UInt256 target, UInt32 path, UInt256[] hashes)
        {
            byte[] scratch;
            var parent = (byte[])target;
            for (int i = 0; i < hashes.Length; i++)
            {
                if (((path >> i) & 1) == 1)
                    scratch = Helper.Concat(parent, hashes[i]);
                else
                    scratch = Helper.Concat((byte[])hashes[i], parent);
                parent = Util.DoubleSha256(scratch);
            }
            return (UInt256)parent;
        }
    }
}
