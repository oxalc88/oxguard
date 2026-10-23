export function shipping(weight: number): number {
  if (!Number.isFinite(weight)) throw new Error("finite weight required");
  if (weight < 0) throw new Error("negative weight");
  if (weight === 0) return 0;
  if (weight <= 1) return 5;
  if (weight <= 5) return 10;
  return 20;
}
