export interface Order {
  id: string;
  amount: number;
  alreadyPaid?: boolean;
}

export interface Gateway {
  charge(id: string, amount: number): Promise<string>;
}

export interface BatchResult {
  paid: string[];
  failed: string[];
}
