export type ResourceRoute = {
  id: string;
  action: string;
};

export function resourceRoute(path: string, base: string): ResourceRoute {
  if (path === base) return { id: "", action: "list" };
  const remainder = path.startsWith(`${base}/`)
    ? path.slice(base.length + 1)
    : "";
  const parts = remainder
    .split("/")
    .filter(Boolean)
    .map((part) => decodeURIComponent(part));
  if (!parts.length) return { id: "", action: "list" };
  if (parts[0] === "new") return { id: "", action: "new" };
  return { id: parts[0], action: parts[1] || "view" };
}
