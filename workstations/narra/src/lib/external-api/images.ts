import "server-only";

const MAX_B64_RESPONSE_IMAGE_BYTES = 25 * 1024 * 1024;

type ExternalImageResponseFormat = "url" | "b64_json";

type GeneratedImageForResponse = {
  height: number | null;
  url: string;
  width: number | null;
};

function readDataUrlBase64(url: string) {
  const match = url.match(/^data:([^;,]+)?(;base64)?,(.*)$/i);
  if (!match) return null;

  const isBase64 = Boolean(match[2]);
  const payload = match[3] ?? "";
  if (isBase64) return payload.trim();

  return Buffer.from(decodeURIComponent(payload), "utf8").toString("base64");
}

async function imageUrlToBase64(url: string) {
  const dataUrlBase64 = readDataUrlBase64(url);
  if (dataUrlBase64) return dataUrlBase64;

  const response = await fetch(url);
  if (!response.ok) {
    throw new Error(`读取生成图片失败，无法返回 b64_json（HTTP ${response.status}）`);
  }

  const contentLength = Number(response.headers.get("content-length") ?? 0);
  if (contentLength > MAX_B64_RESPONSE_IMAGE_BYTES) {
    throw new Error("生成图片超过 b64_json 响应大小限制");
  }

  const arrayBuffer = await response.arrayBuffer();
  if (arrayBuffer.byteLength > MAX_B64_RESPONSE_IMAGE_BYTES) {
    throw new Error("生成图片超过 b64_json 响应大小限制");
  }

  return Buffer.from(arrayBuffer).toString("base64");
}

export async function formatImageGenerationData(
  images: GeneratedImageForResponse[],
  responseFormat: ExternalImageResponseFormat,
) {
  if (responseFormat === "url") {
    return images.map((image) => ({
      height: image.height,
      url: image.url,
      width: image.width,
    }));
  }

  return Promise.all(
    images.map(async (image) => ({
      b64_json: await imageUrlToBase64(image.url),
      height: image.height,
      url: image.url,
      width: image.width,
    })),
  );
}
