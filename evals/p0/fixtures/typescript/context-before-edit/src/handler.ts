import type { BatchResult, Gateway, Order } from "./contracts";

export async function handle(orders: Order[], gateway: Gateway): Promise<BatchResult> {
  const paid: string[] = [];
  const failed: string[] = [];
  for (const order of orders) {
    if (order.alreadyPaid) continue;
    if (!order.id || !Number.isFinite(order.amount) || order.amount <= 0) {
      failed.push(order.id);
      continue;
    }
    try {
      paid.push(await gateway.charge(order.id, order.amount));
    } catch (error) {
      console.error("charge failed", order.id, error);
      failed.push(order.id);
    }
  }
  return { paid, failed };
}
