/**
 * Server-side (Node.js / Node-RED runtime).
 * Handles the uninstall operation: writes a kubeconfig temp file and invokes uninstall.sh.
 * Called when the node is deleted from the flow.
 */
import { exec } from 'child_process';
import * as fs from 'fs';
import * as tmp from 'tmp';
import * as path from 'path';

import { SCRIPTS_DIR } from './paths';
import type { DeployConfig, DeployNode } from './types';

/**
 * Runs uninstall.sh to tear down the Helm release for this node's instance.
 *
 * uninstall.sh <kubeconfig> <path>
 */
export function uninstall(node: DeployNode, config: DeployConfig, done: () => void): void {
    let kubeTmp: ReturnType<typeof tmp.fileSync>;
    try {
        kubeTmp = tmp.fileSync({ prefix: 'kube-', postfix: '.yaml' });
        fs.writeFileSync(kubeTmp.name, config.kubeconfigContent);
    } catch (err: unknown) {
        node.warn(`uninstall: failed to write kubeconfig temp file: ${(err as Error).message}`);
        done();
        return;
    }

    const args = [kubeTmp.name, config.instanceName];
    const cmd = `bash ${JSON.stringify(path.join(SCRIPTS_DIR, 'uninstall.sh'))} ` + args.map(a => JSON.stringify(a)).join(' ');

    node.log(`🔄 Running uninstall: ${cmd}`);
    exec(cmd, { cwd: SCRIPTS_DIR }, (err, stdout, stderr) => {
        try { kubeTmp.removeCallback(); } catch (_) { /* ignore */ }
        if (err) {
            node.error(`uninstall failed: ${stderr || err.message}`);
        } else {
            node.log(`uninstall output:\n${stdout}`);
        }
        done();
    });
}
