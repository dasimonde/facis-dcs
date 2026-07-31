const assert = require("node:assert/strict");
const { EventEmitter } = require("node:events");
const fs = require("node:fs");
const path = require("node:path");
const test = require("node:test");
const { URL } = require("node:url");

const AsyncFunction = Object.getPrototypeOf(async function () {}).constructor;

function flowFunction(file, id) {
  const nodes = JSON.parse(fs.readFileSync(path.join(__dirname, file), "utf8"));
  return nodes.find((node) => node.id === id).func;
}

function memoryFS(initial = {}) {
  const files = new Map(Object.entries(initial));
  return {
    readFileSync(name) {
      if (!files.has(name)) throw new Error("ENOENT");
      return files.get(name);
    },
    writeFileSync(name, value) {
      files.set(name, String(value));
    },
    json(name) {
      return JSON.parse(files.get(name));
    },
  };
}

function transport(responses, calls) {
  return {
    request(target, options, callback) {
      const request = new EventEmitter();
      const responseBody = responses.get(target.href);
      calls.push({ url: target.href, options });
      request.write = () => {};
      request.destroy = (error) => request.emit("error", error);
      request.end = () => {
        queueMicrotask(() => {
          const response = new EventEmitter();
          response.statusCode = responseBody.status;
          callback(response);
          if (responseBody.body !== undefined) {
            response.emit("data", Buffer.from(JSON.stringify(responseBody.body)));
          }
          response.emit("end");
        });
      };
      return request;
    },
  };
}

function environment(values) {
  return { get: (name) => values[name] };
}

test("checkpoint publisher keeps internal and external timeout budgets separate", async () => {
  const calls = [];
  const responses = new Map([
    ["http://hydra/token", { status: 200, body: { access_token: "token" } }],
    ["http://dcs/pac/audit/checkpoint/head", { status: 200, body: { seq: 1, root: "root-1" } }],
    [
      "http://dcs/pac/audit/checkpoint/1",
      {
        status: 200,
        body: {
          seq: 1,
          root: "root-1",
          prev_root: null,
          leaf_count: 3,
          created_at: "2026-07-29T00:00:00Z",
        },
      },
    ],
    ["https://sink.example/checkpoints", { status: 204 }],
  ]);
  const http = transport(responses, calls);
  const https = transport(responses, calls);
  const files = memoryFS({
    "/data/checkpoint-sink-confirmed.json": JSON.stringify({
      confirmed_seq: 0,
      confirmed_root: null,
      last_confirmation_status: null,
    }),
    "/data/checkpoint-sink-records.json": "[]",
  });
  const globals = new Map();
  const run = new AsyncFunction(
    "msg",
    "env",
    "global",
    "node",
    "fs",
    "http",
    "https",
    "url",
    flowFunction("audit-checkpoint-anchor-flow.json", "audit-anchor-run"),
  );

  const result = await run(
    { req: {}, payload: { channel: "checkpoint_sink" } },
    environment({
      ORCE_AUDIT_EXECUTOR_TEST_CONTROLS_ENABLED: "true",
      CHECKPOINT_SINK_ENABLED: "true",
      CHECKPOINT_SINK_URL: "https://sink.example/checkpoints",
      CHECKPOINT_SINK_BEARER_TOKEN: "sink-token",
      CHECKPOINT_SINK_TIMEOUT_SECONDS: "2",
      CHECKPOINT_INTERNAL_TIMEOUT_SECONDS: "30",
      DCS_URL: "http://dcs",
      HYDRA_TOKEN_URL: "http://hydra/token",
      ORCE_NOTARY_CLIENT_ID: "notary",
      ORCE_NOTARY_CLIENT_SECRET: "secret",
    }),
    { get: (key) => globals.get(key), set: (key, value) => globals.set(key, value) },
    { error() {} },
    files,
    http,
    https,
    { URL },
  );

  assert.deepEqual(
    calls.map((call) => call.options.timeout),
    [30000, 30000, 30000, 2000],
  );
  assert.equal(calls[3].options.rejectUnauthorized, true);
  assert.equal(calls[3].options.headers.authorization, "Bearer sink-token");
  assert.equal(result.payload.sink_timeout_seconds, 2);
});

test("checkpoint reset uses the internal timeout budget", async () => {
  const calls = [];
  const responses = new Map([
    ["http://hydra/token", { status: 200, body: { access_token: "token" } }],
    ["http://dcs/pac/audit/checkpoint/head", { status: 404, body: {} }],
  ]);
  const http = transport(responses, calls);
  const files = memoryFS();
  const globals = new Map();
  const run = new AsyncFunction(
    "msg",
    "env",
    "global",
    "node",
    "fs",
    "http",
    "https",
    "url",
    flowFunction("audit-executor-flow.json", "pac-audit-reset"),
  );

  const result = await run(
    { payload: { channel: "checkpoint_sink" } },
    environment({
      ORCE_AUDIT_EXECUTOR_TEST_CONTROLS_ENABLED: "true",
      CHECKPOINT_SINK_TIMEOUT_SECONDS: "2",
      CHECKPOINT_INTERNAL_TIMEOUT_SECONDS: "30",
      DCS_URL: "http://dcs",
      HYDRA_TOKEN_URL: "http://hydra/token",
      ORCE_NOTARY_CLIENT_ID: "notary",
      ORCE_NOTARY_CLIENT_SECRET: "secret",
    }),
    { get: (key) => globals.get(key), set: (key, value) => globals.set(key, value) },
    { error() {} },
    files,
    http,
    http,
    { URL },
  );

  assert.deepEqual(
    calls.map((call) => call.options.timeout),
    [30000, 30000],
  );
  assert.equal(result.statusCode, 204);
});
