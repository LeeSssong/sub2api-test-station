async function deriveKey(secret: string) {
  const seed = await crypto.subtle.digest(
    "SHA-256",
    new TextEncoder().encode(secret),
  );

  return crypto.subtle.importKey("raw", seed, "AES-GCM", false, [
    "encrypt",
    "decrypt",
  ]);
}

function toBase64(bytes: Uint8Array) {
  return Buffer.from(bytes).toString("base64");
}

function fromBase64(value: string) {
  return Uint8Array.from(Buffer.from(value, "base64"));
}

export async function encryptProviderSecret(value: string, secret: string) {
  const iv = crypto.getRandomValues(new Uint8Array(12));
  const key = await deriveKey(secret);
  const encrypted = await crypto.subtle.encrypt(
    { iv, name: "AES-GCM" },
    key,
    new TextEncoder().encode(value),
  );

  return `${toBase64(iv)}:${toBase64(new Uint8Array(encrypted))}`;
}

export async function decryptProviderSecret(value: string, secret: string) {
  const [ivBase64, encryptedBase64] = value.split(":");
  const iv = fromBase64(ivBase64);
  const encrypted = fromBase64(encryptedBase64);
  const key = await deriveKey(secret);
  const decrypted = await crypto.subtle.decrypt(
    { iv, name: "AES-GCM" },
    key,
    encrypted,
  );

  return new TextDecoder().decode(decrypted);
}
