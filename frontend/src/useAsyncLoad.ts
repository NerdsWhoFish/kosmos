import { useCallback, useEffect, useRef, useState } from "react";

export function useAsyncLoad(initialLoading = true) {
  const [loading, setLoading] = useState(initialLoading);
  const [error, setError] = useState("");
  const request = useRef(0);

  useEffect(() => () => {
    request.current += 1;
  }, []);

  const reset = useCallback(() => {
    request.current += 1;
    setError("");
    setLoading(false);
  }, []);

  const run = useCallback(async <T,>(fetchData: () => Promise<T>, apply: (data: T) => void) => {
    const current = ++request.current;
    setError("");
    setLoading(true);
    try {
      const data = await fetchData();
      if (current === request.current) apply(data);
    } catch (reason) {
      if (current === request.current) {
        setError(
          reason instanceof Error ? reason.message : "Could not load data. Try again.",
        );
      }
    } finally {
      if (current === request.current) setLoading(false);
    }
  }, []);

  return { loading, error, setError, run, reset };
}
