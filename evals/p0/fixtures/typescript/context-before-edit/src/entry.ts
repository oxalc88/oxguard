import type { Gateway, Order } from "./contracts";
import { handle } from "./handler";

export function runBatch(orders: Order[], gateway: Gateway) {
  return handle(orders, gateway);
}
