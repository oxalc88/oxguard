import { a } from "./a";
import { b } from "./b";

export function original(x: number): number {
  return a(x) + b(x);
}
