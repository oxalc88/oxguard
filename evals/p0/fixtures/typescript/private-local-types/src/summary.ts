export function summarize(groups: Record<string, Array<{ orderId: string; lines: Array<{ sku: string; quantity: number; price: number }> }>>): Record<string, number> {
  const totals: Record<string, number> = {};
  for (const [store, orders] of Object.entries(groups)) {
    totals[store] = orders.reduce((sum, order) => sum + order.lines.reduce((subtotal, line) => subtotal + line.quantity * line.price, 0), 0);
  }
  return totals;
}
