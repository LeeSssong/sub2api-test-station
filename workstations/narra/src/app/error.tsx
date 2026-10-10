"use client";

import { useEffect } from "react";
import Link from "next/link";
import { Home, RefreshCw } from "lucide-react";

export default function GlobalError({
  error,
  reset,
}: {
  error: Error & { digest?: string };
  reset: () => void;
}) {
  useEffect(() => {
    console.error("[GlobalError]", error);
  }, [error]);

  return (
    <main className="flex min-h-[80vh] items-center justify-center px-5 py-12">
      <div className="studio-card mx-auto w-full max-w-md rounded-[2rem] p-8 text-center">
        <div className="mb-5 inline-flex rounded-full bg-rose-50 p-4 ring-1 ring-rose-100">
          <RefreshCw className="size-7 text-rose-500" aria-hidden />
        </div>
        <h1 className="editorial-title text-3xl font-semibold text-[var(--ink)]">
          出错了
        </h1>
        <p className="mt-3 text-sm leading-relaxed text-[var(--ink-soft)]">
          {error.message || "页面加载时遇到了未知问题。"}
        </p>
        {error.digest ? (
          <p className="mt-2 font-mono text-[10px] tracking-wide text-[var(--ink-soft)]/60">
            {error.digest}
          </p>
        ) : null}
        <div className="mt-6 flex flex-wrap justify-center gap-3">
          <button
            type="button"
            onClick={reset}
            className="inline-flex cursor-pointer items-center gap-2 rounded-full bg-[var(--ink)] px-5 py-2.5 text-sm font-medium text-white shadow-md transition-all duration-200 ease-out hover:bg-[var(--accent)] hover:shadow-lg"
          >
            <RefreshCw className="size-4" aria-hidden />
            重试
          </button>
          <Link
            href="/"
            className="inline-flex cursor-pointer items-center gap-2 rounded-full border border-[var(--line)] px-5 py-2.5 text-sm font-medium text-[var(--ink)] transition-all duration-200 ease-out hover:border-[var(--accent)] hover:text-[var(--accent)]"
          >
            <Home className="size-4" aria-hidden />
            回首页
          </Link>
        </div>
      </div>
    </main>
  );
}
