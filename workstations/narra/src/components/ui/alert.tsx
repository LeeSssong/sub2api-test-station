import { AlertCircle, AlertTriangle, CheckCircle2, Info, X } from "lucide-react";
import type { ReactNode } from "react";

import { cn } from "@/lib/utils";

export type AlertVariant = "error" | "warning" | "success" | "info";

type AlertProps = {
  variant?: AlertVariant;
  title?: string;
  children: ReactNode;
  onDismiss?: () => void;
  className?: string;
};

const variantStyles: Record<
  AlertVariant,
  {
    container: string;
    icon: string;
    Icon: typeof AlertCircle;
  }
> = {
  error: {
    container: "border-rose-200 bg-rose-50/80 text-rose-700",
    icon: "text-rose-500",
    Icon: AlertCircle,
  },
  warning: {
    container: "border-amber-200 bg-amber-50/80 text-amber-800",
    icon: "text-amber-500",
    Icon: AlertTriangle,
  },
  success: {
    container: "border-emerald-200 bg-emerald-50/80 text-emerald-800",
    icon: "text-emerald-600",
    Icon: CheckCircle2,
  },
  info: {
    container: "border-sky-200 bg-sky-50/80 text-sky-800",
    icon: "text-sky-600",
    Icon: Info,
  },
};

export function Alert({
  variant = "error",
  title,
  children,
  onDismiss,
  className,
}: AlertProps) {
  const styles = variantStyles[variant];
  const Icon = styles.Icon;
  return (
    <div
      role={variant === "error" ? "alert" : "status"}
      aria-live={variant === "error" ? "assertive" : "polite"}
      className={cn(
        "flex items-start gap-3 rounded-2xl border px-4 py-3 text-sm backdrop-blur-sm",
        styles.container,
        className,
      )}
    >
      <Icon
        className={cn("mt-0.5 size-4 shrink-0", styles.icon)}
        aria-hidden
      />
      <div className="min-w-0 flex-1 leading-relaxed">
        {title ? <p className="font-medium">{title}</p> : null}
        <div className={title ? "mt-1 text-xs" : undefined}>{children}</div>
      </div>
      {onDismiss ? (
        <button
          type="button"
          onClick={onDismiss}
          aria-label="关闭"
          className="shrink-0 cursor-pointer rounded-md p-1 transition hover:bg-black/5"
        >
          <X className="size-4" aria-hidden />
        </button>
      ) : null}
    </div>
  );
}
