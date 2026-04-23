import numbro from 'numbro';
export function formatBinarySize(bytes?: number): string {
  const num = Number(bytes);
  if (Number.isNaN(num)) {
    return '-';
  }
  return numbro(num).format({ output: 'byte', base: 'binary', mantissa: 2 });
}

/**
 * Local time in `yyyy-MM-dd HH:mm:ss` format. Built manually (not
 * `toLocaleString`) because locale-aware formatters use different
 * separators (slash, dot, CJK characters) across locales and we want
 * the column to align cross-browser. Returns '-' on bad input.
 */
export function formatDateTime(iso?: string): string {
  if (!iso) return '-';
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return '-';
  const pad = (n: number) => String(n).padStart(2, '0');
  return (
    `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())} ` +
    `${pad(d.getHours())}:${pad(d.getMinutes())}:${pad(d.getSeconds())}`
  );
}
