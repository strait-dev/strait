#!/usr/bin/env node
import { runCli } from "../cli";
import { buildContext } from "../context";

await runCli(process.argv.slice(2), buildContext(process));
