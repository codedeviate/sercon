declare const crypto: any;
declare const text: any;

const B64 = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/";

// base64Bytes base64-encodes a raw byte array. (text.str.base64Encode is
// UTF-8-string-only and corrupts bytes > 127, so we cannot use it for digests.)
export function base64Bytes(bytes: number[]): string {
  let out = "";
  for (let i = 0; i < bytes.length; i += 3) {
    const b0 = bytes[i];
    const b1 = i + 1 < bytes.length ? bytes[i + 1] : 0;
    const b2 = i + 2 < bytes.length ? bytes[i + 2] : 0;
    out += B64[b0 >> 2];
    out += B64[((b0 & 3) << 4) | (b1 >> 4)];
    out += i + 1 < bytes.length ? B64[((b1 & 15) << 2) | (b2 >> 6)] : "=";
    out += i + 2 < bytes.length ? B64[b2 & 63] : "=";
  }
  return out;
}

export function hexToBytes(hex: string): number[] {
  const out: number[] = [];
  for (let i = 0; i + 1 < hex.length; i += 2) out.push(parseInt(hex.substr(i, 2), 16));
  return out;
}

// Qliro: base64 of the raw SHA-256 digest of input.
export function sha256Base64(input: string): string {
  return base64Bytes(hexToBytes(crypto.hash.sha256(input)));
}

// Svea: SHA-512 of input as UPPERCASE hex.
export function sha512HexUpper(input: string): string {
  return crypto.hash.sha512(input).toUpperCase();
}

// Basic auth header value for user:pass (ASCII credentials).
export function basicAuth(user: string, pass: string): string {
  return "Basic " + text.str.base64Encode(`${user}:${pass}`);
}
