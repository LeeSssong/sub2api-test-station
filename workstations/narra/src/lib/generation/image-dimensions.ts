// 解析 PNG/JPEG/WebP/GIF 头部以拿到真实像素尺寸。
// 仅消费图片字节流前数百字节，不会把全图解码到内存。
// 用途：渠道把 4K 静默降级到 1024 时，前端能拿出依据来对比。

export type ImageDimensions = {
  height: number;
  width: number;
};

const PNG_MAGIC = Buffer.from([0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a]);
const RIFF_MAGIC = Buffer.from("RIFF");
const WEBP_MAGIC = Buffer.from("WEBP");
const GIF_MAGIC_87 = Buffer.from("GIF87a");
const GIF_MAGIC_89 = Buffer.from("GIF89a");

function readPng(buffer: Buffer): ImageDimensions | null {
  if (buffer.length < 24) return null;
  if (!buffer.subarray(0, 8).equals(PNG_MAGIC)) return null;

  const width = buffer.readUInt32BE(16);
  const height = buffer.readUInt32BE(20);
  return width > 0 && height > 0 ? { height, width } : null;
}

function readJpeg(buffer: Buffer): ImageDimensions | null {
  if (buffer.length < 4) return null;
  if (buffer[0] !== 0xff || buffer[1] !== 0xd8) return null;

  let offset = 2;
  while (offset + 9 < buffer.length) {
    if (buffer[offset] !== 0xff) return null;
    const marker = buffer[offset + 1];
    offset += 2;

    // SOF0..SOF15 携带尺寸；排除 DHT/DRI/JPG 等非 SOF 段。
    const isStartOfFrame =
      marker >= 0xc0 && marker <= 0xcf && marker !== 0xc4 && marker !== 0xc8 && marker !== 0xcc;

    const segmentLength = buffer.readUInt16BE(offset);
    if (isStartOfFrame) {
      if (offset + 7 > buffer.length) return null;
      const height = buffer.readUInt16BE(offset + 3);
      const width = buffer.readUInt16BE(offset + 5);
      return width > 0 && height > 0 ? { height, width } : null;
    }
    offset += segmentLength;
  }
  return null;
}

function readWebp(buffer: Buffer): ImageDimensions | null {
  if (buffer.length < 30) return null;
  if (!buffer.subarray(0, 4).equals(RIFF_MAGIC)) return null;
  if (!buffer.subarray(8, 12).equals(WEBP_MAGIC)) return null;

  const fourCC = buffer.subarray(12, 16).toString("ascii");

  if (fourCC === "VP8 ") {
    // Lossy. 帧数据在 chunk 起始 + 6 处有 0x9d012a 的同步码，紧接着 16-bit width/height。
    const width = buffer.readUInt16LE(26) & 0x3fff;
    const height = buffer.readUInt16LE(28) & 0x3fff;
    return width > 0 && height > 0 ? { height, width } : null;
  }

  if (fourCC === "VP8L") {
    // Lossless. signature byte (0x2f) 之后是 14-bit width-1 与 height-1。
    if (buffer[20] !== 0x2f) return null;
    const bits =
      buffer[21] | (buffer[22] << 8) | (buffer[23] << 16) | (buffer[24] << 24);
    const width = (bits & 0x3fff) + 1;
    const height = ((bits >> 14) & 0x3fff) + 1;
    return width > 0 && height > 0 ? { height, width } : null;
  }

  if (fourCC === "VP8X") {
    // Extended. canvas size 是 24-bit little-endian width-1 / height-1。
    const width =
      ((buffer[24] | (buffer[25] << 8) | (buffer[26] << 16)) & 0xffffff) + 1;
    const height =
      ((buffer[27] | (buffer[28] << 8) | (buffer[29] << 16)) & 0xffffff) + 1;
    return width > 0 && height > 0 ? { height, width } : null;
  }

  return null;
}

function readGif(buffer: Buffer): ImageDimensions | null {
  if (buffer.length < 10) return null;
  const head = buffer.subarray(0, 6);
  if (!head.equals(GIF_MAGIC_87) && !head.equals(GIF_MAGIC_89)) return null;
  const width = buffer.readUInt16LE(6);
  const height = buffer.readUInt16LE(8);
  return width > 0 && height > 0 ? { height, width } : null;
}

export function readImageDimensions(buffer: Buffer): ImageDimensions | null {
  return (
    readPng(buffer) ?? readJpeg(buffer) ?? readWebp(buffer) ?? readGif(buffer)
  );
}

export function formatDimensions(dimensions: ImageDimensions) {
  return `${dimensions.width}x${dimensions.height}`;
}
