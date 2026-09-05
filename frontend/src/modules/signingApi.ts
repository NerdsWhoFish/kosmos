import { api } from "../api";

export type SigningField = {
  id: string;
  type: "signature" | "date" | "name" | "text";
  label: string;
  page: number;
  x: number;
  y: number;
  width: number;
  height: number;
  required: boolean;
  signerId?: string;
};

export type SigningSigner = {
  id: string;
  name: string;
  email?: string;
  completedSignerName?: string;
  signedAt?: string;
  consent?: string;
  session?: SigningSession;
};

export type SigningSession = {
  ipAddress: string;
  userAgent: string;
  city?: string;
  region?: string;
  country?: string;
  capturedAt: string;
  source: "cloudflare" | "direct";
};

export type SigningRequest = {
  id: string;
  title: string;
  fileName: string;
  status: "draft" | "pending" | "completed" | "revoked";
  pages: { width: number; height: number }[];
  fields: SigningField[];
  revision: number;
  signerName: string;
  signerEmail: string;
  createdAt: string;
  updatedAt: string;
  expiresAt?: string;
  completedAt?: string;
  postSignExpiresAt?: string;
  downloadExpiresAt?: string;
  accessExpiresAt?: string;
  signers?: SigningSigner[];
  currentSignerId?: string;
  originalSHA256: string;
  flattened?: boolean;
  uploadedSHA256?: string;
  signedSHA256?: string;
  session?: SigningSession;
};

export const consentText =
  "I agree to use electronic records and signatures, have reviewed this document, and intend my signature to be binding.";

export function signingError(error: unknown) {
  return error instanceof Error ? error.message : "Something went wrong. Please try again.";
}
export const fieldLabels: Record<SigningField["type"], string> = {
  signature: "Signature",
  date: "Date signed",
  name: "Full name",
  text: "Text",
};

export function signingCredential(fragment: string) {
  const match = /^#([A-Za-z0-9_-]+)\.([A-Za-z0-9_-]+)$/.exec(fragment);
  return match ? { id: match[1], token: match[2] } : null;
}

export function signingHeaders(token?: string) {
  return token ? { "X-Kosmos-Signing-Token": token } : undefined;
}

export function signingAPI<T>(
  path: string,
  token?: string,
  init: RequestInit = {},
) {
  return api<T>(path, {
    ...init,
    referrerPolicy: "no-referrer",
    cache: "no-store",
    headers: { ...signingHeaders(token), ...init.headers },
  });
}

export function boundedField(field: SigningField, page?: { width: number; height: number }): SigningField {
  const width = Math.max(0.05, page ? 20 / page.width : 0, Math.min(1, field.width));
  const height = Math.max(0.015, page ? 15.6 / page.height : 0, Math.min(1, field.height));
  return {
    ...field,
    width,
    height,
    x: Math.max(0, Math.min(1 - width, field.x)),
    y: Math.max(0, Math.min(1 - height, field.y)),
  };
}

export async function pdfBytes(
  path: string,
  token?: string,
  signal?: AbortSignal,
) {
  const response = await fetch(path, {
    headers: signingHeaders(token),
    referrerPolicy: "no-referrer",
    cache: "no-store",
    signal,
  });
  if (!response.ok)
    throw new Error(
      "This PDF could not be loaded. Try again or ask the sender for a new link.",
    );
  return response.arrayBuffer();
}

export async function downloadPDF(
  path: string,
  fileName: string,
  token?: string,
) {
  const data = await pdfBytes(path, token);
  const url = URL.createObjectURL(
    new Blob([data], { type: "application/pdf" }),
  );
  const link = document.createElement("a");
  link.href = url;
  link.download = fileName;
  link.click();
  setTimeout(() => URL.revokeObjectURL(url), 30_000);
}
