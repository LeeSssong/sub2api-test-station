import Link from "next/link";
import { Compass, Home } from "lucide-react";

export default function NotFound() {
  return (
    <main className="flex min-h-[80vh] items-center justify-center px-5 py-12">
      <div className="studio-card mx-auto w-full max-w-md rounded-[2rem] p-8 text-center">
        <div className="mb-5 inline-flex rounded-full bg-[var(--surface-strong)]/60 p-4 ring-1 ring-[var(--line)]">
          <Compass className="size-7 text-[var(--accent)]" aria-hidden />
        </div>
        <p className="editorial-title text-6xl font-semibold tracking-tight text-[var(--ink)]">
          404
        </p>
        <h1 className="mt-2 text-lg font-medium text-[var(--ink)]">
          这里没有要找的页面
        </h1>
        <p className="mt-3 text-sm leading-relaxed text-[var(--ink-soft)]">
          它可能已经被移走，或者从来不存在。
        </p>
        <div className="mt-6 flex flex-wrap justify-center gap-3">
          <Link
            href="/"
            className="inline-flex cursor-pointer items-center gap-2 rounded-full bg-[var(--ink)] px-5 py-2.5 text-sm font-medium text-white shadow-md transition-all duration-200 ease-out hover:bg-[var(--accent)] hover:shadow-lg"
          >
            <Home className="size-4" aria-hidden />
            回首页
          </Link>
          <Link
            href="/works"
            className="inline-flex cursor-pointer items-center gap-2 rounded-full border border-[var(--line)] px-5 py-2.5 text-sm font-medium text-[var(--ink)] transition-all duration-200 ease-out hover:border-[var(--accent)] hover:text-[var(--accent)]"
          >
            去看看作品
          </Link>
        </div>
      </div>
    </main>
  );
}
