export function formatNumber(value: number) {
  return new Intl.NumberFormat("vi-VN").format(value);
}

export function formatPercent(value: number, digits = 0) {
  return new Intl.NumberFormat("vi-VN", {
    style: "percent",
    maximumFractionDigits: digits,
  }).format(value / 100);
}

export function formatMiB(value: number) {
  if (value >= 1024) return `${new Intl.NumberFormat("vi-VN", { maximumFractionDigits: 1 }).format(value / 1024)} GiB`;
  return `${formatNumber(value)} MiB`;
}

export function formatDateTime(value?: string) {
  if (!value) return "—";
  return new Intl.DateTimeFormat("vi-VN", {
    dateStyle: "short",
    timeStyle: "medium",
  }).format(new Date(value));
}

export function relativeTime(value?: string) {
  if (!value) return "chưa có dữ liệu";
  const diffSeconds = Math.round((new Date(value).getTime() - Date.now()) / 1000);
  const formatter = new Intl.RelativeTimeFormat("vi", { numeric: "auto" });
  const divisions = [
    { amount: 60, unit: "second" as const },
    { amount: 60, unit: "minute" as const },
    { amount: 24, unit: "hour" as const },
    { amount: 7, unit: "day" as const },
    { amount: 4.345, unit: "week" as const },
    { amount: 12, unit: "month" as const },
    { amount: Number.POSITIVE_INFINITY, unit: "year" as const },
  ];
  let duration = diffSeconds;
  for (const division of divisions) {
    if (Math.abs(duration) < division.amount) return formatter.format(Math.round(duration), division.unit);
    duration /= division.amount;
  }
  return "—";
}

export function shortId(value?: string, length = 12) {
  if (!value) return "—";
  return value.length > length ? `${value.slice(0, length)}…` : value;
}
