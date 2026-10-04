import { test, expect } from "vitest";
import { fn1, fn2, fn3, fn4, fn5 } from "./functions";
test("known functions", () => {
  expect(fn1()).toBe(1);
  expect(fn2()).toBe(2);
  expect(fn3()).toBe(3);
  expect(fn4()).toBe(4);
  expect(fn5()).toBe(5);
});
