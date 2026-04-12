"use client";

export function LiveNowBadge() {
  return (
    <span className="inline-flex items-center gap-1 px-2 py-0.5 rounded-full bg-red-100 text-red-700 text-xs font-medium">
      <span className="w-2 h-2 rounded-full bg-red-500 animate-pulse" />
      Live Now
    </span>
  );
}
