import type { LucideIcon } from "lucide-react";
import type { ReactNode } from "react";

import { cn } from "@/lib/utils";

type EmptyStateProps = {
  icon?: LucideIcon;
  title: string;
  description?: string;
  action?: ReactNode;
  className?: string;
};

export function EmptyState({
  icon: Icon,
  title,
  description,
  action,
  className,
}: EmptyStateProps) {
  return (
    <div
      className={cn(
        "studio-card flex flex-col items-center rounded-[2rem] border border-dashed border-[var(--line)] p-10 text-center",
        className,
      )}
    >
      {Icon ? (
        <div className="mb-4 inline-flex rounded-full bg-[var(--surface-strong)]/60 p-4 ring-1 ring-[var(--line)]">
          <Icon className="size-6 text-[var(--ink-soft)]" aria-hidden />
        </div>
      ) : null}
      <h2 className="text-xl font-semibold text-[var(--ink)]">{title}</h2>
      {description ? (
        <p className="mt-3 max-w-md text-sm leading-relaxed text-[var(--ink-soft)]">
          {description}
        </p>
      ) : null}
      {action ? <div className="mt-6">{action}</div> : null}
    </div>
  );
}
