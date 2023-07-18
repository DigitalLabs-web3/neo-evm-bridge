using System;
using Neo.Cryptography.ECC;
using Neo.SmartContract.Framework;
using Neo.SmartContract.Framework.Native;
using Neo.SmartContract.Framework.Services;

namespace Bridge
{
    public static class Util
    {
        public static void ArrayCopy(byte[] dest, int destIndex, byte[] source, int srcIndex, int length)
        {
            if (srcIndex >= source.Length || srcIndex + length > source.Length || srcIndex < 0 || length < 0 ||
            destIndex >= dest.Length || destIndex + length > dest.Length || destIndex < 0)
                throw new Exception("[Util] invalid range");
            for (int i = 0; i < length; i++)
                dest[destIndex + i] = source[srcIndex + i];
        }

        public static byte[] BytesAppend(byte[] a, byte x)
        {
            var r = new byte[a.Length + 1];
            int i = 0;
            for (; i < a.Length; i++)
                r[i] = a[i];
            r[i] = x;
            return r;
        }

        public static UInt16 UInt16FromLittleEndian(byte[] b)
        {
            if (b.Length < 2)
                throw new Exception("[Util] wrong uint16 length");
            return (UInt16)((UInt16)b[0] | (UInt16)b[1] << 8);
        }

        public static byte[] UInt16ToLittleEndian(UInt16 num)
        {
            var b = new byte[2];
            b[0] = (byte)(num & 0xff);
            b[1] = (byte)((num >> 8) & 0xff);
            return b;
        }

        public static byte[] UInt32ToLittleEndian(UInt32 num)
        {
            var b = new byte[4];
            for (int i = 0; i < 4; i++)
            {
                b[i] = (byte)(num & 0xff);
                num >>= 8;
            }
            return b;
        }

        public static UInt32 UInt32FromLittleEndian(byte[] b)
        {
            if (b.Length < 4)
                throw new Exception("[Util] wrong uint32 length");
            return (UInt32)((UInt32)b[0] | (UInt32)b[1] << 8 | (UInt32)b[2] << 16 | (UInt32)b[3] << 24);
        }

        public static UInt64 UInt64FromLittleEndian(byte[] b)
        {
            if (b.Length < 8)
                throw new Exception("[Util] wrong uint64 length");
            return (UInt64)((UInt64)b[0] | (UInt64)b[1] << 8 | (UInt64)b[2] << 16 | (UInt64)b[3] << 24
            | (UInt64)b[4] << 32 | (UInt64)b[5] << 40 | (UInt64)b[6] << 48 | (UInt64)b[7] << 56);
        }

        public static byte[] UInt64ToLittleEndian(UInt64 num)
        {
            var b = new byte[8];
            for (int i = 0; i < 8; i++)
            {
                b[i] = (byte)(num & 0xff);
                num >>= 8;
            }
            return b;
        }

        public static byte[] DoubleSha256(byte[] d)
        {
            return (byte[])CryptoLib.Sha256(CryptoLib.Sha256((ByteString)d));
        }

        public static bool StartWith(byte[] s, byte[] prefix)
        {
            if (prefix.Length == 0)
                throw new Exception("[Util] empty prefix");
            if (s.Length < prefix.Length)
                return false;
            for (int i = 0; i < prefix.Length; i++)
                if (s[i] != prefix[i]) return false;
            return true;
        }

        public static int CalculateBFTCount(int n)
        {
            return (2 * n + 1) / 3;
        }

        public static bool ECPointsCheck(ECPoint[] ps)
        {
            foreach (var p in ps)
                if (!p.IsValid || (p[0] != 2 && p[0] != 3)) return false;
            return true;
        }

        public static ECPoint[] ECPointUnique(ECPoint[] ps)
        {
            for (int i = 0; i < ps.Length; i++)
            {
                for (int j = i + 1; j < ps.Length; j++)
                {
                    if (ECPointEqual(ps[i], ps[j]))
                        ps = ECPointsRemove(ps, j);
                }
            }
            return ps;
        }

        public static bool ECPointEqual(ECPoint a, ECPoint b)
        {
            if (a.Length != b.Length) return false;
            for (int i = 0; i < a.Length; i++)
            {
                if (a[i] != b[i]) return false;
            }
            return true;
        }

        public static ECPoint[] ECPointsRemove(ECPoint[] ps, int index)
        {
            if (index >= ps.Length) throw new Exception($"{nameof(ECPointsRemove)} {nameof(index)}");
            var r = new ECPoint[ps.Length - 1];
            for (int i = 0, j = 0; i < ps.Length; i++)
            {
                if (i != index)
                    r[j++] = ps[i];
            }
            return r;
        }

        public static bool ByteStringEqual(ByteString a, ByteString b)
        {
            if (a.Length != b.Length) return false;
            for (int i = 0; i < a.Length; i++)
                if (a[i] != b[i]) return false;
            return true;
        }
    }
}
