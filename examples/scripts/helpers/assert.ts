export function check(cond: boolean, msg: string): void {
  if (!cond) throw new Error(msg);
}
