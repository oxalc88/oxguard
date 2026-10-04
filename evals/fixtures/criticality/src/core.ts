export function target() { return 1; }
export function unused() { return 0; }
export function recursive(): number { return recursive(); }
