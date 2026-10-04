import { target } from "@core";
export function first() { target(); target(); }
export class Box { run() { return target(); } }
export const second = () => target();
target();
