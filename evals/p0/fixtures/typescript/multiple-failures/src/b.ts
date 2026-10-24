export function discount(total: number): number {
  if (!Number.isFinite(total)) throw new Error("finite total required");
  if (total < 0) throw new Error("negative total");
  if (total < 20) return 0;
  if (total < 100) return 2;
  if (total < 500) return 10;
  return 20;
}
