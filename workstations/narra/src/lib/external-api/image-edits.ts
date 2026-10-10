import "server-only";

import { downloadExternalImage } from "@/lib/external-api/source-images";
import { externalImageEditSchema } from "@/lib/validators";

const MAX_EDIT_IMAGE_BYTES = 10 * 1024 * 1024;
const ALLOWED_EDIT_IMAGE_TYPES = new Set([
  "image/gif",
  "image/jpeg",
  "image/png",
  "image/webp",
]);

type SourceImage = {
  data: Buffer;
  fileName: string;
  mimeType: string;
};

function numberField(value: FormDataEntryValue | null) {
  if (typeof value !== "string" || value.trim() === "") return undefined;
  const parsed = Number(value);
  return Number.isFinite(parsed) ? parsed : undefined;
}

function stringField(value: FormDataEntryValue | null) {
  return typeof value === "string" ? value : undefined;
}

function readImageUrl(value: unknown): string | null {
  if (typeof value === "string") return value;
  if (!value || typeof value !== "object") return null;

  const record = value as Record<string, unknown>;
  const candidate =
    record.url ??
    record.image_url ??
    record.imageUrl ??
    record.input_image ??
    record.inputImage;

  if (typeof candidate === "string") return candidate;
  if (candidate && typeof candidate === "object") {
    return readImageUrl(candidate);
  }

  return null;
}

function readJsonImageUrls(raw: Record<string, unknown>) {
  const values = [
    raw.image,
    raw.images,
    raw["image[]"],
    raw.image_url,
    raw.imageUrl,
  ];
  const urls: string[] = [];

  for (const value of values) {
    const items = Array.isArray(value) ? value : [value];
    for (const item of items) {
      const url = readImageUrl(item);
      if (url) urls.push(url);
    }
  }

  return urls;
}

function jsonBodyForSchema(raw: Record<string, unknown>) {
  return {
    count: raw.count,
    model: raw.model,
    moderation: raw.moderation,
    n: raw.n,
    negative_prompt: raw.negative_prompt,
    negativePrompt: raw.negativePrompt,
    output_compression: raw.output_compression,
    output_format: raw.output_format,
    outputCompression: raw.outputCompression,
    outputFormat: raw.outputFormat,
    prompt: raw.prompt,
    quality: raw.quality,
    response_format: raw.response_format,
    seed: raw.seed,
    size: raw.size,
  };
}

function formBodyForSchema(formData: FormData) {
  return {
    count: numberField(formData.get("count")),
    model: stringField(formData.get("model")),
    moderation: stringField(formData.get("moderation")),
    n: numberField(formData.get("n")),
    negative_prompt: stringField(formData.get("negative_prompt")),
    negativePrompt: stringField(formData.get("negativePrompt")),
    output_compression: numberField(formData.get("output_compression")),
    output_format: stringField(formData.get("output_format")),
    outputCompression: numberField(formData.get("outputCompression")),
    outputFormat: stringField(formData.get("outputFormat")),
    prompt: stringField(formData.get("prompt")),
    quality: stringField(formData.get("quality")),
    response_format: stringField(formData.get("response_format")),
    seed: numberField(formData.get("seed")),
    size: stringField(formData.get("size")),
  };
}

function formFiles(formData: FormData) {
  return [
    ...formData.getAll("image[]"),
    ...formData.getAll("image"),
    ...formData.getAll("images"),
    ...formData.getAll("referenceImages"),
  ].filter((value): value is File => value instanceof File);
}

async function fileToSourceImage(file: File, index: number): Promise<SourceImage> {
  if (file.size <= 0) {
    throw new Error("参考图不能为空");
  }
  if (file.size > MAX_EDIT_IMAGE_BYTES) {
    throw new Error("参考图不能超过 10MB");
  }

  const mimeType = (file.type || "image/png").toLowerCase();
  if (!ALLOWED_EDIT_IMAGE_TYPES.has(mimeType)) {
    throw new Error("参考图仅支持 png、jpeg、webp 或 gif 图片");
  }

  const data = Buffer.from(await file.arrayBuffer());
  if (data.byteLength > MAX_EDIT_IMAGE_BYTES) {
    throw new Error("参考图不能超过 10MB");
  }

  return {
    data,
    fileName: file.name || `source-${index + 1}.png`,
    mimeType,
  };
}

async function parseJsonEditBody(request: Request) {
  const raw = (await request.json()) as unknown;
  if (!raw || typeof raw !== "object" || Array.isArray(raw)) {
    throw new Error("请求体必须是 JSON 对象");
  }

  const record = raw as Record<string, unknown>;
  const body = externalImageEditSchema.parse(jsonBodyForSchema(record));
  const urls = readJsonImageUrls(record);
  const sourceImages = await Promise.all(
    urls.map((url, index) => downloadExternalImage(url, index)),
  );

  return { body, sourceImages };
}

async function parseFormEditBody(request: Request) {
  const formData = await request.formData();
  const body = externalImageEditSchema.parse(formBodyForSchema(formData));
  const files = formFiles(formData);
  const sourceImages = await Promise.all(
    files.map((file, index) => fileToSourceImage(file, index)),
  );

  return { body, sourceImages };
}

export async function parseExternalImageEditRequest(request: Request) {
  const contentType = request.headers.get("content-type")?.toLowerCase() ?? "";
  const result = contentType.includes("multipart/form-data")
    ? await parseFormEditBody(request)
    : await parseJsonEditBody(request);

  if (result.sourceImages.length === 0) {
    throw new Error("images/edits 至少需要 1 张参考图");
  }

  return result;
}
