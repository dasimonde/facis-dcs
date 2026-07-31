import datetime
import os
import subprocess
import tempfile
import time
import urllib.error
import urllib.request

from cryptography import x509
from cryptography.hazmat.primitives import hashes, serialization
from cryptography.hazmat.primitives.asymmetric import ec
from cryptography.x509.oid import NameOID


ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))


def _make_c2pa_signing_material():
    """Generate the test P-256 key and its self-signed leaf certificate.

    pdf-core is keyless: it embeds this leaf as the COSE x5chain but never signs.
    The harness holds the private key and signs the Sig_structures pdf-core
    returns from /download and /update, posting the signatures to /c2pa/embed —
    exactly as the DCS backend signs with its dcs-c2pa HSM key in production. The
    leaf is self-signed; c2patool does not enforce a trust anchor by default.
    """
    key = ec.generate_private_key(ec.SECP256R1())
    subject = issuer = x509.Name([
        x509.NameAttribute(NameOID.COMMON_NAME, "DCS-PDF-CORE BDD c2pa signer"),
        x509.NameAttribute(NameOID.ORGANIZATION_NAME, "DCS-PDF-CORE"),
    ])
    now = datetime.datetime.now(datetime.timezone.utc)
    cert = (
        x509.CertificateBuilder()
        .subject_name(subject)
        .issuer_name(issuer)
        .public_key(key.public_key())
        .serial_number(x509.random_serial_number())
        .not_valid_before(now - datetime.timedelta(hours=1))
        .not_valid_after(now + datetime.timedelta(days=1))
        .add_extension(x509.KeyUsage(
            digital_signature=True, content_commitment=False, key_encipherment=False,
            data_encipherment=False, key_agreement=False, key_cert_sign=False,
            crl_sign=False, encipher_only=False, decipher_only=False), critical=True)
        .add_extension(x509.ExtendedKeyUsage([x509.ObjectIdentifier("1.3.6.1.5.5.7.3.4")]), critical=False)
        .sign(key, hashes.SHA256())
    )
    pem = cert.public_bytes(serialization.Encoding.PEM)
    fh = tempfile.NamedTemporaryFile(prefix="dcs-c2pa-bdd-x5chain-", suffix=".pem", delete=False)
    fh.write(pem)
    fh.close()
    return key, fh.name


def _load_dev_env(env):
    path = os.path.join(ROOT, ".dev.env")
    if not os.path.exists(path):
        return
    with open(path, "r", encoding="utf-8") as fh:
        for raw in fh:
            line = raw.strip()
            if not line or line.startswith("#") or "=" not in line:
                continue
            key, value = line.split("=", 1)
            env[key.strip()] = value.strip()

def before_all(context):
    env = os.environ.copy()
    _load_dev_env(env)
    env.setdefault("GO111MODULE", "on")

    # pdf-core no longer signs: generate the test key + self-signed leaf, point the
    # server's embedded x5chain at it, and drop the removed signing-endpoint var.
    # The harness signs Sig_structures with the matching key (see the steps'
    # sign_sig_structure) and posts them to /c2pa/embed.
    signing_key, x5chain_path = _make_c2pa_signing_material()
    context.c2pa_private_key = signing_key
    env["DCS_PDF_CORE_C2PA_X5CHAIN_PEM_FILE"] = x5chain_path
    env.pop("DCS_PDF_CORE_C2PA_X5CHAIN_PEM", None)
    env.pop("DCS_PDF_CORE_C2PA_SIGNING_ENDPOINT", None)

    # Keep BDD runtime stable and deterministic on the documented loopback port.
    subprocess.run(["fuser", "-k", "8080/tcp"], check=False, capture_output=True)
    server_addr = "127.0.0.1:8080"
    server_url = f"http://{server_addr}"
    env["DCS_PDF_CORE_ADDR"] = server_addr
    # Force ontology base URL to match the server address so dcsCoreIRI in the
    # compiler aligns with the 127.0.0.1 IRIs used in BDD test payloads.
    env["DCS_PDF_CORE_ONTOLOGY_BASE_URL"] = server_url
    context.server = subprocess.Popen(
        ["go", "run", "."],
        cwd=ROOT,
        stdout=subprocess.PIPE,
        stderr=subprocess.STDOUT,
        text=True,
        env=env,
    )
    deadline = time.time() + 20
    last_error = None
    while time.time() < deadline:
        if context.server.poll() is not None:
            break
        try:
            with urllib.request.urlopen(f"{server_url}/swagger.json", timeout=1) as response:
                if response.status == 200:
                    context.server_url = server_url
                    return
        except Exception as exc:
            last_error = exc
            time.sleep(0.25)
    output = ""
    if context.server.stdout is not None:
        try:
            output = context.server.stdout.read()
        except Exception:
            output = ""
    raise RuntimeError(f"server failed to start: {last_error}\n{output}")


def after_all(context):
    server = getattr(context, "server", None)
    if server is None:
        return
    server.terminate()
    try:
        server.wait(timeout=5)
    except subprocess.TimeoutExpired:
        server.kill()
