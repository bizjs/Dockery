import numbro from 'numbro';
export function formatBinarySize(bytes?: number): string {
  const num = Number(bytes);
  if (Number.isNaN(num)) {
    return '-';
  }
  return numbro(num).format({ output: 'byte', base: 'binary', mantissa: 2 });
}
