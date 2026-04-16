import { defineConfig, type Plugin } from 'vite';
import { resolve } from 'path';
import fs from 'fs';
import path from 'path';
import archiver from 'archiver';
import nunjucks from 'nunjucks';
import pkg from './package.json';

const ROOT = __dirname;                      // deployment/node-red/
const DEPLOY_ROOT = resolve(ROOT, '..');     // deployment/  — shell scripts, Helm chart
const DIST = resolve(ROOT, 'dist');
const SRC = resolve(ROOT, 'src');
const TGZ_NAME = `digital-contracting-service-${pkg.version}.tgz`;

/**
 * After the IIFE for editor.ts is emitted, this plugin:
 *  1. Renders src/Digital-Contracting-Service.njk (resolving {% include %} partials)
 *  2. Inlines the compiled editor JS into the EDITOR_SCRIPT_PLACEHOLDER
 *  3. Writes the final Node-RED HTML fragment to dist/Digital-Contracting-Service.html
 */
function assembleHtmlPlugin(): Plugin {
    return {
        name: 'assemble-node-red-html',
        closeBundle() {
            const editorJsPath = resolve(DIST, 'editor.iife.js');
            if (!fs.existsSync(editorJsPath)) {
                this.warn(`editor.iife.js not found at ${editorJsPath} — HTML assembly skipped`);
                return;
            }

            const editorJs = fs.readFileSync(editorJsPath, 'utf-8');

            // Render the Nunjucks template; includes are resolved relative to SRC
            const env = nunjucks.configure(SRC, { autoescape: false });
            const rendered = env.render('Digital-Contracting-Service.njk');

            const inlineScript = `<script type="text/javascript">\n${editorJs}\n</script>`;
            const assembled = rendered.replace('<!-- EDITOR_SCRIPT_PLACEHOLDER -->', inlineScript);

            const outPath = resolve(DIST, 'engine', 'Digital-Contracting-Service.html');
            fs.writeFileSync(outPath, assembled, 'utf-8');
            console.log(`[assemble-html] wrote ${outPath}`);

            // Clean up the intermediate IIFE file — it is now inlined in the HTML
            fs.rmSync(editorJsPath);
            const mapPath = editorJsPath + '.map';
            if (fs.existsSync(mapPath)) fs.rmSync(mapPath);
        },
    };
}

/**
 * After the bundle is assembled, packages everything into a zip:
 *   dist/Digital-Contracting-Service.zip
 *
 */
function zipPlugin(): Plugin {
    return {
        name: 'zip-node-red-package',
        closeBundle() {
            const zipPath = resolve(DIST, TGZ_NAME);
            const output = fs.createWriteStream(zipPath);
            const archive = archiver('tar', { gzip: true, gzipOptions: { level: 9 } });

            archive.on('error', (err: Error) => { throw err; });
            output.on('close', () => {
                console.log(`[zip] ${zipPath} (${archive.pointer()} bytes)`);
            });

            archive.pipe(output);

            // Compiled artifacts — placed at package/dist/ so that after npm
            // strips the package/ prefix they land at dist/, matching package.json "main".
            // Node backend: dist/engine/Digital-Contracting-Service.js
            const engineJs = resolve(DIST, 'engine', 'Digital-Contracting-Service.js');
            if (fs.existsSync(engineJs)) {
                archive.file(engineJs, { name: path.join('package', 'dist', 'engine', 'Digital-Contracting-Service.js') });
            } else {
                console.warn('[zip] warning: engine/Digital-Contracting-Service.js not found in dist/');
            }
            // Editor UI: dist/engine/Digital-Contracting-Service.html (assembled by assembleHtmlPlugin)
            const editorHtml = resolve(DIST, 'engine', 'Digital-Contracting-Service.html');
            if (fs.existsSync(editorHtml)) {
                archive.file(editorHtml, { name: path.join('package', 'dist', 'engine', 'Digital-Contracting-Service.html') });
            } else {
                console.warn('[zip] warning: Digital-Contracting-Service.html not found in dist/');
            }

            // package.json lives alongside the compiled artifacts in node-red/
            const p = resolve(ROOT, 'package.json');
            if (fs.existsSync(p)) {
                archive.file(p, { name: path.join('package', 'package.json') });
            }

            // Shell scripts and Helm chart live one level up in deployment/
            for (const file of ['deploy.sh', 'uninstall.sh']) {
                const fp = resolve(DEPLOY_ROOT, file);
                if (fs.existsSync(fp)) {
                    archive.file(fp, { name: path.join('package', file) });
                }
            }

            for (const dir of ['helm']) {
                const dp = resolve(DEPLOY_ROOT, dir);
                if (fs.existsSync(dp)) {
                    archive.directory(dp, path.join('package', dir));
                }
            }

            archive.finalize();
        },
    };
}

/**
 * Builds editor.ts as an IIFE so it can be inlined into the Node-RED HTML fragment.
 * The assembleHtmlPlugin and zipPlugin then finalize the artifacts.
 */
export default defineConfig({
    build: {
        lib: {
            entry: resolve(SRC, 'client/editor.ts'),
            formats: ['iife'],
            name: 'DCSEditor',
            fileName: (format) => `editor.${format}.js`,
        },
        outDir: DIST,
        emptyOutDir: false,   // node backend was already emitted here by vite.node.config.ts
        sourcemap: false,     // source maps are not useful for inlined browser scripts
        minify: true,
        rollupOptions: {
            // RED and jQuery are globals provided by the Node-RED runtime
            external: ['node-red'],
            output: {
                globals: {},
            },
        },
    },
    plugins: [
        assembleHtmlPlugin(),
        zipPlugin(),
    ],
});
