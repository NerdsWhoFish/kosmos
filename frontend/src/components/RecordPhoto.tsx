import { ChangeEvent, useEffect, useState } from "react";
import { Camera } from "lucide-react";
import { api, Attachment } from "../api";

export function RecordPhoto({
  recordType,
  recordID,
  label,
  fallback,
}: {
  recordType: "account" | "contact";
  recordID: string;
  label: string;
  fallback: React.ReactNode;
}) {
  const [photo, setPhoto] = useState<Attachment | null>(null);
  const [error, setError] = useState("");

  useEffect(() => {
    api<{ attachments: Attachment[] }>(
      `/api/v1/attachments?recordType=${recordType}&recordId=${encodeURIComponent(recordID)}`,
    )
      .then((response) =>
        setPhoto(
          response.attachments.find((item) => item.kind === "photo") ?? null,
        ),
      )
      .catch(() => setPhoto(null));
  }, [recordID, recordType]);

  async function upload(event: ChangeEvent<HTMLInputElement>) {
    const file = event.target.files?.[0];
    if (!file) return;
    const data = new FormData();
    data.set("file", file);
    data.set("kind", "photo");
    data.set("recordType", recordType);
    data.set("recordId", recordID);
    const response = await fetch("/api/v1/attachments", {
      method: "POST",
      headers: { "X-Kosmos-CSRF": "1" },
      body: data,
    });
    if (!response.ok) {
      const body = (await response.json().catch(() => ({}))) as {
        error?: { message?: string };
      };
      setError(body.error?.message ?? "Could not upload photo");
      return;
    }
    const uploaded = (await response.json()) as Attachment;
    if (photo)
      await api(`/api/v1/attachments/${photo.id}`, { method: "DELETE" }).catch(
        () => undefined,
      );
    setPhoto(uploaded);
    setError("");
    event.target.value = "";
  }

  return (
    <span className="record-photo-wrap">
      <span className="record-avatar large">
        {photo ? (
          <img src={photo.viewUrl} alt={`${label} profile`} />
        ) : (
          fallback
        )}
      </span>
      <label className="photo-upload" title={`Change ${label} photo`}>
        <Camera size={14} />
        <span className="sr-only">Upload {label} photo</span>
        <input
          type="file"
          accept="image/jpeg,image/png,image/webp"
          onChange={upload}
        />
      </label>
      {error && <small className="photo-error">{error}</small>}
    </span>
  );
}
