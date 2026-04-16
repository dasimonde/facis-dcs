/**
 * Server-side entry point (Node.js / Node-RED runtime).
 *
 * This file is loaded by Node-RED when the module is installed.
 * It only registers the node type and admin endpoints — all business logic
 * lives in the other engine/ modules.
 */
import type { NodeAPI } from 'node-red';

import type { DeployConfig, DeployNode } from './types';
import { deploy }                from './deploy';
import { uninstall }             from './uninstall';
import { registerInfoEndpoint }  from './info-endpoint';

export default function (RED: NodeAPI): void {
    function DeployNode(this: DeployNode, config: DeployConfig) {
        RED.nodes.createNode(this, config);
        const node = this;

        node.on('input', (msg) => deploy(node, config, msg));
        node.on('close', (removed: boolean, done: () => void) => {
            if (removed) uninstall(node, config, done);
            else done();
        });
    }

    RED.nodes.registerType('Digital-Contracting-Service', DeployNode);
    registerInfoEndpoint(RED);
}
