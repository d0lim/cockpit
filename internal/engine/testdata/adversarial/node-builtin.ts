import { readFileSync } from "node:fs";

export default () => readFileSync("/etc/hostname", "utf8");
