using System;
using Neo;
using Neo.SmartContract.Framework.Services;

namespace Bridge
{
    public class BufferReader
    {
        private readonly byte[] buffer;
        private int offset;

        public BufferReader(byte[] b)
        {
            buffer = b;
            offset = 0;
        }

        public byte ReadByte()
        {
            if (buffer.Length <= offset)
                throw new Exception($"[BufferReader] unexpected EOF offset={offset}, buffer_length={buffer.Length}");
            return buffer[offset++];
        }

        public byte[] ReadBytes(int n)
        {
            if (buffer.Length < offset + n)
                throw new Exception($"[BufferReader] unexpected EOF offset={offset}, buffer_length={buffer.Length}");
            var value = buffer[offset..(offset + n)];
            offset += n;
            return value;
        }

        public UInt16 ReadUint16()
        {
            var raw = ReadBytes(2);
            var value = Util.UInt16FromLittleEndian(raw);
            offset += 2;
            return value;
        }

        public UInt32 ReadUint32()
        {
            var raw = ReadBytes(4);
            var value = Util.UInt32FromLittleEndian(raw);
            return value;
        }

        public UInt64 ReadUint64()
        {
            var raw = ReadBytes(8);
            var value = Util.UInt64FromLittleEndian(raw);
            return value;
        }

        public UInt64 ReadVarUint()
        {
            var b = ReadByte();
            return b switch
            {
                0xfd => ReadUint16(),
                0xfe => ReadUint32(),
                0xff => ReadUint64(),
                _ => b,
            };
        }

        public UInt160 ReadUint160()
        {
            var b = ReadBytes(20);
            return (UInt160)b;
        }

        public UInt256 ReadUint256()
        {
            var b = ReadBytes(32);
            return (UInt256)b;
        }

        public byte[] ReadVarBytes()
        {
            var len = (int)ReadVarUint();
            if (buffer.Length < offset + len)
                throw new Exception($"[BufferReader] unexpected EOF offset={offset}, buffer_length={buffer.Length}");
            return ReadBytes(len);
        }

        public byte[] Readed()
        {
            return buffer[..offset];
        }
    }
}
