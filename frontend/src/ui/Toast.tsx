import {
  createContext,
  useCallback,
  useContext,
  useMemo,
  useRef,
  useState,
  type ReactNode,
} from "react";
import styles from "./Toast.module.css";

interface ToastItem {
  id: number;
  title: string;
  detail?: string;
  tone: "info" | "error";
}

interface ToastApi {
  toast: (title: string, detail?: string) => void;
  toastError: (title: string, detail?: string) => void;
}

const ToastContext = createContext<ToastApi | null>(null);

export function useToast(): ToastApi {
  const ctx = useContext(ToastContext);
  if (!ctx) throw new Error("useToast must be used inside ToastProvider");
  return ctx;
}

export function ToastProvider({ children }: { children: ReactNode }) {
  const [items, setItems] = useState<ToastItem[]>([]);
  const nextId = useRef(0);

  const push = useCallback((title: string, detail: string | undefined, tone: "info" | "error") => {
    const id = nextId.current++;
    setItems((prev) => [...prev, { id, title, tone, ...(detail !== undefined ? { detail } : {}) }]);
    setTimeout(() => setItems((prev) => prev.filter((t) => t.id !== id)), 6000);
  }, []);

  const toastApi = useMemo<ToastApi>(
    () => ({
      toast: (title, detail) => push(title, detail, "info"),
      toastError: (title, detail) => push(title, detail, "error"),
    }),
    [push],
  );

  return (
    <ToastContext.Provider value={toastApi}>
      {children}
      <div className={styles.region} role="status" aria-live="polite">
        {items.map((t) => (
          <div key={t.id} className={[styles.toast, t.tone === "error" ? styles.error : ""].join(" ")}>
            <div className={styles.title}>{t.title}</div>
            {t.detail && <div className={styles.detail}>{t.detail}</div>}
          </div>
        ))}
      </div>
    </ToastContext.Provider>
  );
}
