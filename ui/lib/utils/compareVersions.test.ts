import assert from "node:assert/strict";
import test from "node:test";
import { compareVersions } from "./compareVersions.ts";

const cases: [string, string, number][] = [
  ["v2.2.6", "v2.2.6+sofian.1.abc123", 0],
  ["v2.2.6+build.42", "2.2.6+sofian.2.def456", 0],
  ["v2.2.7", "v2.2.6+sofian.1.abc123", 1],
  ["v2.2.5", "v2.2.6+sofian.1.abc123", -1],
  ["v2.2.6", "v2.2.6-prerelease1+sofian.1.abc123", 1],
  ["v2.2.6-prerelease2+build.1", "v2.2.6-prerelease1+build.999", 1],
];
for (const [latest, current, expected] of cases) {
  test(`compares ${latest} to ${current}`, () => {
    assert.equal(compareVersions(latest, current), expected);
  });
}
