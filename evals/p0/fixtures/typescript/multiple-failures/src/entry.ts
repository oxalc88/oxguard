import { shipping } from "./a";
import { discount } from "./b";

export function quote(weight: number, total: number): number {
  return total + shipping(weight) - discount(total);
}
