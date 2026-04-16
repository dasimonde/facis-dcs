import type { EditorRED } from 'node-red';

declare const RED: EditorRED;

interface InfoResponse {
    ingressExternalIp?: string;
    dcsUrl?: string;
}

// All boolean fields that Node-RED won't auto-persist (checkboxes)
const CHECKBOX_FIELDS = [
    'depPostgresql', 'depPgPersist',
    'depKeycloak',   'depKcRealmImport',
    'depNats',
    'depNeo4j',      'depNeo4jPersist',
] as const;

// Maps a toggle checkbox id to the config block it controls
const DEP_TOGGLES: Record<string, string> = {
    'node-input-depPostgresql': 'dep-config-postgresql',
    'node-input-depKeycloak':   'dep-config-keycloak',
    'node-input-depNeo4j':      'dep-config-neo4j',
};

(function () {
    function setupFile(fieldFile: string, fieldHidden: string): void {
        const inp = document.getElementById('node-input-' + fieldFile) as HTMLInputElement | null;
        if (!inp) return;
        inp.addEventListener('change', function () {
            const f = this.files?.[0];
            if (!f) return;
            const reader = new FileReader();
            reader.onload = function (e) {
                const hidden = document.getElementById('node-input-' + fieldHidden) as HTMLInputElement | null;
                if (hidden) hidden.value = (e.target?.result as string) ?? '';
            };
            reader.readAsText(f);
        });
    }

    RED.nodes.registerType('Digital-Contracting-Service', {
        category: 'FAPs',
        color: '#c0deed',
        icon: 'bridge.svg',
        defaults: {
            name:               { value: '' },
            instanceName:       { value: '' },
            domainAddress:      { value: '' },
            kubeconfigContent:  { value: '' },
            privateKeyContent:  { value: '' },
            certificateContent: { value: '' },
            oidcIssuerUrl:      { value: '' },
            clientId:           { value: 'digital-contracting-service' },
            depPostgresql:      { value: false },
            depPgUser:          { value: 'dcs' },
            depPgPassword:      { value: 'dcs' },
            depPgDatabase:      { value: 'dcs' },
            depPgPersist:       { value: false },
            depKeycloak:        { value: false },
            depKcAdminUser:     { value: 'admin' },
            depKcAdminPassword: { value: 'admin' },
            depKcRealmImport:   { value: false },
            depNats:            { value: false },
            depNeo4j:           { value: false },
            depNeo4jPassword:   { value: 'changeme' },
            depNeo4jPersist:    { value: false },
        },
        inputs: 1,
        outputs: 1,
        label() {
            return (this as unknown as { name: string }).name || 'Digital-Contracting-Service';
        },
        oneditprepare() {
            // Tab switching
            document.querySelectorAll<HTMLElement>('.tab-link').forEach(link => {
                link.addEventListener('click', e => {
                    e.preventDefault();
                    document.querySelectorAll('.tab-link').forEach(l => l.classList.remove('active'));
                    link.classList.add('active');
                    document.querySelectorAll<HTMLElement>('.tab-content').forEach(c => { c.style.display = 'none'; });
                    const tabId = link.dataset.tab;
                    if (tabId) {
                        const tab = document.getElementById(tabId);
                        if (tab) tab.style.display = '';
                    }
                });
            });

            const nodeId = (this as unknown as { id: string }).id;
            fetch(`digital-contracting-service/info/${encodeURIComponent(nodeId)}`)
                .then(r => r.json() as Promise<InfoResponse>)
                .then(data => {
                    const ip  = document.getElementById('node-input-ingressExternalIp') as HTMLInputElement | null;
                    const url = document.getElementById('node-input-dcsUrl') as HTMLInputElement | null;
                    if (ip)  ip.value  = data.ingressExternalIp ?? '';
                    if (url) url.value = data.dcsUrl ?? '';
                })
                .catch(() => { /* node not found or not yet deployed */ });

            setupFile('kubeconfig', 'kubeconfigContent');
            setupFile('privateKey', 'privateKeyContent');
            setupFile('certificate', 'certificateContent');

            // Restore checkbox state and wire up dep-config show/hide
            const node = this as unknown as Record<string, boolean>;
            CHECKBOX_FIELDS.forEach(field => {
                const el = document.getElementById('node-input-' + field) as HTMLInputElement | null;
                if (!el) return;
                el.checked = !!node[field];
                const configId = DEP_TOGGLES['node-input-' + field];
                if (configId) {
                    const block = document.getElementById(configId);
                    if (block) block.style.display = el.checked ? '' : 'none';
                    el.addEventListener('change', () => {
                        if (block) block.style.display = el.checked ? '' : 'none';
                    });
                }
            });
        },
        oneditsave() {
            // Persist checkbox state back to the node
            const node = this as unknown as Record<string, boolean>;
            CHECKBOX_FIELDS.forEach(field => {
                const el = document.getElementById('node-input-' + field) as HTMLInputElement | null;
                if (el) node[field] = el.checked;
            });
        },
    });
})();
