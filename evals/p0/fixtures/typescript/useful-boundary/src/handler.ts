import { deliver, type Transport } from "./transport";

export async function handle(transport: Transport, orderId: string) {
  const receipt = await deliver(transport, orderId);
  return { orderId, receipt };
}
