// 轻量 UI 基础设施：toast、模态框、确认弹窗、空态/加载态、图标。
import { useEffect, useState, type ReactNode } from "react";

/* ---------- Toast ---------- */

type ToastType = "ok" | "err" | "warn";

export function toast(message: string, type: ToastType = "ok") {
  window.dispatchEvent(new CustomEvent("app-toast", { detail: { message, type } }));
}

interface ToastItem {
  id: number;
  message: string;
  type: ToastType;
}

export function ToastHost() {
  const [items, setItems] = useState<ToastItem[]>([]);
  useEffect(() => {
    let seq = 0;
    const onToast = (e: Event) => {
      const { message, type } = (e as CustomEvent<{ message: string; type: ToastType }>).detail;
      const id = ++seq;
      setItems((prev) => [...prev, { id, message, type }]);
      // 错误类不自动消失前也先给足阅读时间
      const ttl = type === "err" ? 6000 : 3500;
      setTimeout(() => setItems((prev) => prev.filter((t) => t.id !== id)), ttl);
    };
    window.addEventListener("app-toast", onToast);
    return () => window.removeEventListener("app-toast", onToast);
  }, []);
  return (
    <>
      {items.map((t) => (
        <div key={t.id} className={`toast ${t.type}`} role="status">
          {t.type === "ok" ? "✓" : t.type === "err" ? "✕" : "!"} {t.message}
        </div>
      ))}
    </>
  );
}

/* ---------- 模态框 ---------- */

export function Modal({
  open,
  title,
  children,
  onClose,
  width = 440,
}: {
  open: boolean;
  title: string;
  children: ReactNode;
  onClose?: () => void;
  width?: number;
}) {
  useEffect(() => {
    if (!open || !onClose) return;
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") onClose();
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [open, onClose]);
  if (!open) return null;
  return (
    <div className="modal-mask" onMouseDown={(e) => onClose && e.target === e.currentTarget && onClose()}>
      <div className="modal" style={{ width }} role="dialog" aria-label={title}>
        <div className="modal-title">{title}</div>
        {children}
      </div>
    </div>
  );
}

export function ConfirmModal({
  open,
  title,
  text,
  confirmText = "确认",
  danger = false,
  onCancel,
  onConfirm,
}: {
  open: boolean;
  title: string;
  text?: string;
  confirmText?: string;
  danger?: boolean;
  onCancel: () => void;
  onConfirm: () => void;
}) {
  return (
    <Modal open={open} title={title} onClose={onCancel} width={420}>
      {text ? <div className="modal-text">{text}</div> : null}
      <div className="modal-actions">
        <button className="btn btn-ghost btn-sm" autoFocus onClick={onCancel}>
          取消
        </button>
        <button className={`btn btn-sm ${danger ? "btn-danger-solid" : "btn-primary"}`} onClick={onConfirm}>
          {confirmText}
        </button>
      </div>
    </Modal>
  );
}

/* ---------- 状态组件 ---------- */

export function Loading({ text = "加载中…" }: { text?: string }) {
  return <div className="page-loading">{text}</div>;
}

export function EmptyState({ text, big = "◻" }: { text: string; big?: string }) {
  return (
    <div className="empty-state">
      <div className="big">{big}</div>
      {text}
    </div>
  );
}

export function Badge({ kind, children }: { kind: "green" | "gray" | "red" | "blue" | "amber" | "violet"; children: ReactNode }) {
  const cls = kind === "violet" ? "badge badge-violet" : `badge badge-${kind}`;
  return <span className={cls}>{children}</span>;
}

export function Switch({
  checked,
  onChange,
  label,
}: {
  checked: boolean;
  onChange: (next: boolean) => void;
  label?: string;
}) {
  return (
    <label className="switch" title={label}>
      <input type="checkbox" role="switch" aria-checked={checked} checked={checked} onChange={(e) => onChange(e.target.checked)} />
      <span className="slider" />
    </label>
  );
}

/* ---------- 图标 ---------- */

function svgProps() {
  return { width: 20, height: 20, viewBox: "0 0 24 24", fill: "none", stroke: "currentColor", strokeWidth: 2, strokeLinecap: "round" as const, strokeLinejoin: "round" as const };
}

export const Icons = {
  github: () => (
    <svg width={20} height={20} viewBox="0 0 24 24" fill="currentColor" aria-hidden="true">
      <path d="M12 .5C5.65.5.5 5.65.5 12c0 5.08 3.29 9.39 7.86 10.91.58.11.79-.25.79-.55 0-.27-.01-1.17-.02-2.12-3.2.7-3.88-1.36-3.88-1.36-.52-1.33-1.28-1.68-1.28-1.68-1.04-.71.08-.7.08-.7 1.15.08 1.76 1.19 1.76 1.19 1.03 1.76 2.69 1.25 3.35.96.1-.75.4-1.25.72-1.54-2.55-.29-5.24-1.28-5.24-5.68 0-1.26.45-2.28 1.19-3.09-.12-.29-.52-1.46.11-3.05 0 0 .97-.31 3.17 1.18a11 11 0 0 1 2.89-.39c.98 0 1.97.13 2.89.39 2.2-1.49 3.16-1.18 3.16-1.18.63 1.59.24 2.76.12 3.05.74.81 1.18 1.83 1.18 3.09 0 4.41-2.69 5.38-5.25 5.67.41.35.77 1.04.77 2.1 0 1.52-.01 2.74-.01 3.11 0 .31.2.67.8.55A11.5 11.5 0 0 0 23.5 12C23.5 5.65 18.35.5 12 .5z" />
    </svg>
  ),
  clock: () => (
    <svg {...svgProps()}>
      <circle cx="12" cy="12" r="9" />
      <path d="M12 7v5l3 3" />
    </svg>
  ),
  calendar: () => (
    <svg {...svgProps()}>
      <rect x="3" y="5" width="18" height="16" rx="2" />
      <path d="M8 3v4M16 3v4M3 10h18" />
    </svg>
  ),
  refresh: () => (
    <svg {...svgProps()} width={12} height={12}>
      <path d="M21 12a9 9 0 1 1-2.6-6.4" />
      <path d="M21 3v6h-6" />
    </svg>
  ),
  clockBig: () => (
    <svg {...svgProps()} width={64} height={64} strokeWidth={1.4}>
      <circle cx="12" cy="12" r="9" />
      <path d="M12 7v5l3 3" />
    </svg>
  ),
  calendarBig: () => (
    <svg {...svgProps()} width={64} height={64} strokeWidth={1.4}>
      <rect x="3" y="5" width="18" height="16" rx="2" />
      <path d="M8 3v4M16 3v4M3 10h18" />
      <path d="M8 15h3" />
    </svg>
  ),
  shield: () => (
    <svg {...svgProps()}>
      <path d="M12 3l7 3v5c0 4.5-3 8-7 10-4-2-7-5.5-7-10V6z" />
      <path d="M9 12l2 2 4-4" />
    </svg>
  ),
  users: () => (
    <svg {...svgProps()}>
      <circle cx="9" cy="8" r="3.2" />
      <path d="M3.5 19c.6-3 2.8-4.5 5.5-4.5S14 16 14.5 19" />
      <path d="M16 8.2a3 3 0 0 1 0 5.6M17.5 19c-.2-1.2-.6-2.2-1.2-3" />
    </svg>
  ),
  gauge: () => (
    <svg {...svgProps()}>
      <path d="M4 14a8 8 0 1 1 16 0" />
      <path d="M12 14l3.5-4.5" />
      <path d="M2 19h20" />
    </svg>
  ),
  activity: () => (
    <svg {...svgProps()}>
      <path d="M3 12h4l3-7 4 14 3-7h4" />
    </svg>
  ),
  list: () => (
    <svg {...svgProps()}>
      <path d="M9 6h11M9 12h11M9 18h11" />
      <path d="M4 6h.01M4 12h.01M4 18h.01" />
    </svg>
  ),
  key: () => (
    <svg {...svgProps()}>
      <circle cx="8" cy="15" r="4" />
      <path d="M10.8 12.2 20 3M16 7l3 3" />
    </svg>
  ),
  logout: () => (
    <svg {...svgProps()} width={16} height={16}>
      <path d="M9 21H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h4" />
      <path d="M16 17l5-5-5-5M21 12H9" />
    </svg>
  ),
};
