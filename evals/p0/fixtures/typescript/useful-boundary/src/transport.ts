export interface Transport {
  send(payload: string): Promise<string>;
}

export async function deliver(transport: Transport, orderId: string): Promise<string> {
  if (!orderId.trim()) throw new Error("order id required");
  try {
    return await transport.send(JSON.stringify({ order_id: orderId, version: 1 }));
  } catch (cause) {
    throw new Error("delivery unavailable", { cause });
  }
}
