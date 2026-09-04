import { useEffect, useMemo, useState } from "react";
import { api, ContactSource } from "../api";

export function ContactSourcePicker({
  defaultValue = "",
  name = "source",
}: {
  defaultValue?: string;
  name?: string;
}) {
  const [sources, setSources] = useState<ContactSource[]>([]);
  const [value, setValue] = useState(defaultValue);
  const [creating, setCreating] = useState(false);
  const names = useMemo(
    () => new Set(sources.map((source) => source.name.toLowerCase())),
    [sources],
  );

  useEffect(() => {
    api<{ sources: ContactSource[] }>("/api/v1/contact-sources")
      .then((response) => setSources(response.sources ?? []))
      .catch(() => setSources([]));
  }, []);

  useEffect(() => {
    if (
      defaultValue &&
      sources.length &&
      !names.has(defaultValue.toLowerCase())
    ) {
      setCreating(true);
    }
  }, [defaultValue, names, sources.length]);

  return (
    <label>
      Source
      <select
        aria-label="Contact source"
        value={creating ? "__new__" : value}
        onChange={(event) => {
          if (event.target.value === "__new__") {
            setCreating(true);
            setValue("");
          } else {
            setCreating(false);
            setValue(event.target.value);
          }
        }}
      >
        <option value="">Choose a source</option>
        {sources.map((source) => (
          <option value={source.name} key={source.id}>
            {source.name}
          </option>
        ))}
        <option value="__new__">Create a new source...</option>
      </select>
      {creating && (
        <input
          aria-label="New contact source"
          value={value}
          onChange={(event) => setValue(event.target.value)}
          placeholder="Trade show, partner, neighborhood..."
          maxLength={80}
          autoFocus
        />
      )}
      <input type="hidden" name={name} value={value} />
    </label>
  );
}
