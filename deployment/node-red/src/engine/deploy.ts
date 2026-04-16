/**
 * Server-side (Node.js / Node-RED runtime).
 * Handles the deploy operation: writes credential temp files, invokes deploy.sh,
 * parses its output, and updates the node's status and properties.
 */
import { exec } from 'child_process';
import * as fs from 'fs';
import * as tmp from 'tmp';
import * as path from 'path';
import type { NodeMessage } from 'node-red';

import { SCRIPTS_DIR } from './paths';
import type { DeployConfig, DeployNode } from './types';

type TmpFile = ReturnType<typeof tmp.fileSync>;

function writeTempFiles(config: DeployConfig): { kubeTmp: TmpFile; keyTmp: TmpFile; crtTmp: TmpFile } {
    const kubeTmp = tmp.fileSync({ prefix: 'kube-', postfix: '.yaml' });
    fs.writeFileSync(kubeTmp.name, config.kubeconfigContent);
    const keyTmp = tmp.fileSync({ prefix: 'key-', postfix: '.key' });
    fs.writeFileSync(keyTmp.name, config.privateKeyContent);
    const crtTmp = tmp.fileSync({ prefix: 'crt-', postfix: '.crt' });
    fs.writeFileSync(crtTmp.name, config.certificateContent);
    return { kubeTmp, keyTmp, crtTmp };
}

function parseDeployOutput(stdout: string): Record<string, string> {
    const res: Record<string, string> = {};
    stdout.split(/\r?\n/).forEach(line => {
        let m: RegExpMatchArray | null;
        if      ((m = line.match(/^🔹 ingress External-IP: (.+)$/))) res.ingressExternalIp = m[1];
        else if ((m = line.match(/^🔹 DCS URL:\s+(.+)$/)))           res.dcsUrl = m[1];
    });
    return res;
}

/**
 * Runs deploy.sh with the node's configuration.
 * Called from the node's 'input' event handler.
 *
 * deploy.sh <kubeconfig> <key> <cert> <domain> <path> <oidc_issuer_url> <oidc_client_id>
 */
export function deploy(node: DeployNode, config: DeployConfig, msg: NodeMessage): void {
    let kubeTmp: TmpFile, keyTmp: TmpFile, crtTmp: TmpFile;
    try {
        ({ kubeTmp, keyTmp, crtTmp } = writeTempFiles(config));
    } catch (err: unknown) {
        node.error(`Failed to write temp files: ${(err as Error).message}`, msg);
        node.status({ fill: 'red', shape: 'ring', text: 'file error' });
        return;
    }

    const clientId = config.clientId || 'digital-contracting-service';
    const args = [
        kubeTmp.name,
        keyTmp.name,
        crtTmp.name,
        config.domainAddress,
        config.instanceName,
        config.oidcIssuerUrl,
        clientId,
    ];
    const cmd = `bash ${JSON.stringify(path.join(SCRIPTS_DIR, 'deploy.sh'))} ` + args.map(a => JSON.stringify(a)).join(' ');

    const env = {
        ...process.env,
        DEP_POSTGRESQL:          config.depPostgresql    ? 'true' : 'false',
        DEP_PG_USER:             config.depPgUser        || 'dcs',
        DEP_PG_PASSWORD:         config.depPgPassword    || 'dcs',
        DEP_PG_DATABASE:         config.depPgDatabase    || 'dcs',
        DEP_PG_PERSIST:          config.depPgPersist     ? 'true' : 'false',
        DEP_KEYCLOAK:            config.depKeycloak      ? 'true' : 'false',
        DEP_KC_ADMIN_USER:       config.depKcAdminUser       || 'admin',
        DEP_KC_ADMIN_PASSWORD:   config.depKcAdminPassword   || 'admin',
        DEP_KC_REALM_IMPORT:     config.depKcRealmImport ? 'true' : 'false',
        DEP_NATS:                config.depNats          ? 'true' : 'false',
        DEP_NEO4J:               config.depNeo4j         ? 'true' : 'false',
        DEP_NEO4J_PASSWORD:      config.depNeo4jPassword     || 'changeme',
        DEP_NEO4J_PERSIST:       config.depNeo4jPersist  ? 'true' : 'false',
    };

    node.log(`Executing: ${cmd}`);
    node.status({ fill: 'blue', shape: 'dot', text: 'deploying' });

    exec(cmd, { cwd: SCRIPTS_DIR, env }, (err, stdout, stderr) => {
        try { kubeTmp.removeCallback(); keyTmp.removeCallback(); crtTmp.removeCallback(); } catch (_) { /* ignore */ }

        if (err) {
            const parts = [err.message];
            if (stdout.trim()) parts.push(`stdout:\n${stdout.trim()}`);
            if (stderr.trim()) parts.push(`stderr:\n${stderr.trim()}`);
            const errorMsg = parts.join('\n');
            node.error(errorMsg, msg);
            node.status({ fill: 'red', shape: 'ring', text: 'deploy failed' });
            msg.payload = errorMsg;
            node.send(msg);
            return;
        }

        const res = parseDeployOutput(stdout);
        node.ingressExternalIp = res.ingressExternalIp;
        node.dcsUrl = res.dcsUrl;

        node.log(`Deployment result: ${JSON.stringify(res)}`);
        node.status({ fill: 'green', shape: 'dot', text: 'deployed' });

        msg.payload = res;
        node.send(msg);
    });
}
