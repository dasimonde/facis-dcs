// The "ensure root CA + issuer cert" function of the issuer flows, run with
// the real openssl. ORCE runs as a non-root user on a read-only root
// filesystem, so the function must not install anything at run time.
// Run: node --test deployment/helm/charts/orce/flows-issuer/flow-pki.test.js
const assert = require("node:assert/strict");
const childProcess = require("node:child_process");
const crypto = require("node:crypto");
const fs = require("node:fs");
const os = require("node:os");
const path = require("node:path");
const test = require("node:test");

const CHART = path.join(__dirname, "..");

function pkiFunction(file) {
  const nodes = JSON.parse(fs.readFileSync(path.join(CHART, file), "utf8"));
  return nodes.find((node) => node.name === "ensure root CA + issuer cert").func;
}

// Runs the function node with /data/pki and the fixture mount in temporary
// directories and returns the globals it installs.
function runPki(file, withFixture) {
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), "orce-pki-"));
  const pkiDir = path.join(dir, "pki");
  const fixtureDir = path.join(dir, withFixture ? "fixture" : "absent");
  if (withFixture) {
    fs.mkdirSync(fixtureDir);
    for (const name of ["root-ca.crt", "root-ca.key", "issuer.key"]) {
      fs.copyFileSync(path.join(CHART, "pki-dev", name), path.join(fixtureDir, name));
    }
  }
  const code = pkiFunction(file).replaceAll("/data/pki", pkiDir).replaceAll("/etc/orce/pki-root", fixtureDir);
  const cp = {
    execSync(cmd, opts) {
      assert.doesNotMatch(cmd, /\bapk\b|faketime/, "installs or needs a tool the image lacks: " + cmd);
      return childProcess.execSync(cmd, opts);
    },
  };
  const store = new Map();
  const global = { get: (k) => store.get(k), set: (k, v) => store.set(k, v) };
  const node = { log() {}, status() {}, warn() {}, error() {} };
  new Function("msg", "node", "global", "fs", "cp", "crypto", "env", code)({}, node, global, fs, cp, crypto, {
    get: () => "",
  });
  return { global, pkiDir };
}

for (const [file, withFixture] of [
  ["flows-issuer/flow-pki.json", true],
  ["flows-issuer/flow-pki.json", false],
  ["flows-pid-issuer/flow-pki.json", false],
]) {
  test(`${file} (${withFixture ? "mounted" : "generated"} root): backdated leaf without run-time installs`, () => {
    const { global, pkiDir } = runPki(file, withFixture);
    const baseUrl = "https://issuer.example.org/issuer";
    const pki = global.get("ensureIssuerCertFor")(baseUrl, true);

    const leaf = new crypto.X509Certificate(pki.issuerCertPem);
    const root = new crypto.X509Certificate(pki.rootCaCertPem);
    assert.equal(new Date(leaf.validFrom).toISOString(), "2020-01-01T00:00:00.000Z");
    assert.equal(new Date(leaf.validTo).toISOString(), "2040-01-01T00:00:00.000Z");
    assert.ok(leaf.verify(root.publicKey), "leaf not signed by the root");
    assert.ok(leaf.checkIssued(root), "leaf issuer is not the root subject");
    assert.match(leaf.subjectAltName, /URI:did:web:issuer\.example\.org:issuer/);
    assert.match(leaf.subjectAltName, /URI:https:\/\/issuer\.example\.org\/issuer/);
    assert.ok(root.ca, "root is not a CA");
    if (!withFixture) {
      assert.equal(new Date(root.validFrom).toISOString(), "2020-01-01T00:00:00.000Z");
      assert.ok(root.verify(root.publicKey), "generated root is not self-signed");
    }
    // A second leaf for another URL keeps the issuer key.
    const again = global.get("ensureIssuerCertFor")("https://other.example.org", true);
    assert.equal(again.issuerKeyPem, pki.issuerKeyPem);
    fs.rmSync(path.dirname(pkiDir), { recursive: true, force: true });
  });
}
