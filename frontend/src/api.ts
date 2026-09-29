// 统一 fetch 封装：同源 Cookie、JSON 序列化、错误信封解包。
export class ApiError extends Error {
  code: string;
  status: number;
  extra?: Record<string, unknown>;

  constructor(status: number, code: string, message: string, extra?: Record<string, unknown>) {
    super(message);
    this.name = "ApiError";
    this.status = status;
    this.code = code;
    this.extra = extra;
  }
}

interface ApiOptions {
  method?: string;
  json?: unknown;
}

export async function api<T = unknown>(path: string, opts: ApiOptions = {}): Promise<T> {
  const init: RequestInit = {
    method: opts.method ?? "GET",
    credentials: "same-origin",
    headers: {},
  };
  if (opts.json !== undefined) {
    (init.headers as Record<string, string>)["Content-Type"] = "application/json";
    init.body = JSON.stringify(opts.json);
  }
  const res = await fetch(path, init);
  const text = await res.text();
  let data: unknown = null;
  if (text) {
    try {
      data = JSON.parse(text);
    } catch {
      // 非 JSON 响应按原始文本处理
    }
  }
  if (!res.ok) {
    const err = (data as { error?: { code?: string; message?: string; extra?: Record<string, unknown> } })?.error;
    throw new ApiError(res.status, err?.code ?? "http_error", err?.message ?? `请求失败（HTTP ${res.status}）`, err?.extra);
  }
  return data as T;
}

export function fmtTime(ms: number | null | undefined): string {
  if (!ms) return "—";
  return new Date(ms).toLocaleString("zh-CN", { hour12: false });
}

export function fmtDate(iso: string | undefined): string {
  if (!iso) return "—";
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return iso;
  return d.toLocaleString("zh-CN", { hour12: false });
}
