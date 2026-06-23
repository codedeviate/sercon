// Amounts are integer minor units (öre / cents), matching the provider APIs.
export type Money = number;
export function major(amountMinor: number): number { return amountMinor / 100; }
export function minor(amountMajor: number): number { return Math.round(amountMajor * 100); }
