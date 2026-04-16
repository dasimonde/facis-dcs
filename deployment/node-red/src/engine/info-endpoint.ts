/**
 * Server-side (Node.js / Node-RED runtime).
 * Registers the GET /digital-contracting-service/info/:id admin endpoint.
 * Node-RED calls this to let the editor panel read live deployment state from the node.
 */
import type { NodeAPI } from 'node-red';
import type { DeployNode } from './types';

export function registerInfoEndpoint(RED: NodeAPI): void {
    RED.httpAdmin.get('/digital-contracting-service/info/:id', (req, res) => {
        const node = RED.nodes.getNode(req.params.id) as DeployNode | null;
        if (!node) {
            res.status(404).send({ error: 'Node not found' });
            return;
        }
        res.json({
            ingressExternalIp: node.ingressExternalIp ?? '',
            dcsUrl:            node.dcsUrl ?? '',
        });
    });
}
