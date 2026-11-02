const assert = require("node:assert/strict");
const fs = require("node:fs");
const path = require("node:path");
const test = require("node:test");

function acceptTimeStampReq() {
  const nodes = JSON.parse(fs.readFileSync(path.join(__dirname, "tsa_orce_flow.json"), "utf8"));
  const code = nodes.find((node) => node.id === "accept_tsq").func;
  return new Function("msg", "fs", code);
}

function memoryFS() {
  const files = new Map();
  return { files, writeFileSync: (name, value) => files.set(name, Buffer.from(value)) };
}

const request = (type, body) => ({
  _msgid: "m1",
  payload: body,
  req: { headers: { "content-type": type } },
});

// The DER TimeStampReq DSS sends: SEQUENCE { version, messageImprint, ... }.
const timeStampReq = Buffer.concat([Buffer.from([0x30, 0x39, 0x02, 0x01, 0x01]), Buffer.alloc(52, 1)]);

test("an RFC 3161 POST writes the query and runs the local TSA reply", () => {
  const files = memoryFS();
  const [reply, error] = acceptTimeStampReq()(request("application/timestamp-query", timeStampReq), files);
  assert.equal(error, null);
  assert.deepEqual(files.files.get("/tmp/request-m1.tsq"), timeStampReq);
  assert.match(reply.payload, /openssl ts -reply -queryfile \/tmp\/request-m1\.tsq .* -out \/tmp\/request-m1\.tsr$/);
  assert.equal(reply.tsrfile, "/tmp/request-m1.tsr");
});

test("a POST with another content type is refused", () => {
  const files = memoryFS();
  const [reply, error] = acceptTimeStampReq()(request("application/json", timeStampReq), files);
  assert.equal(reply, null);
  assert.equal(error.statusCode, 415);
  assert.equal(files.files.size, 0);
});

test("a body that is not a DER sequence is refused", () => {
  const files = memoryFS();
  const [reply, error] = acceptTimeStampReq()(request("application/timestamp-query", Buffer.from("not der at all, really")), files);
  assert.equal(reply, null);
  assert.equal(error.statusCode, 400);
  assert.equal(files.files.size, 0);
});

test("a query that Node-RED decoded as UTF-8 text keeps its bytes", () => {
  const files = memoryFS();
  const asciiReq = Buffer.concat([Buffer.from([0x30, 0x20, 0x02, 0x01, 0x01]), Buffer.alloc(29, 0x41)]);
  const [reply, error] = acceptTimeStampReq()(request("application/timestamp-query", asciiReq.toString("utf8")), files);
  assert.equal(error, null);
  assert.ok(reply);
  assert.deepEqual(files.files.get("/tmp/request-m1.tsq"), asciiReq);
});
