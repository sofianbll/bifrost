import { describe, expect, it } from "vitest";
import { compareVersions } from "./compareVersions";

describe("release notification version comparison", () => {
	it.each([
		["v2.2.3", "v2.2.3+sofian.1.abc123", 0],
		["v2.2.3+build.42", "2.2.3+sofian.2.def456", 0],
		["v2.2.4", "v2.2.3+sofian.1.abc123", 1],
		["v2.2.2", "v2.2.3+sofian.1.abc123", -1],
		["v2.2.3", "v2.2.3-prerelease1+sofian.1.abc123", 1],
		["v2.2.3-prerelease2+build.1", "v2.2.3-prerelease1+build.999", 1],
	])("compares %s to %s", (latest, current, expected) => {
		expect(compareVersions(latest, current)).toBe(expected);
	});
});