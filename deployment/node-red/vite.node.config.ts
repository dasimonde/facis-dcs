import { defineConfig } from 'vite';
import { resolve } from 'path';

/**
 * Builds the Node-RED backend node as a CommonJS library.
 * All Node.js built-ins and runtime dependencies are externalized —
 * they are resolved at runtime by Node-RED, not bundled.
 */
export default defineConfig({
    build: {
        lib: {
            entry: resolve(__dirname, 'src/engine/Digital-Contracting-Service.ts'),
            formats: ['cjs'],
            fileName: () => 'engine/Digital-Contracting-Service.js',
        },
        outDir: 'dist',
        emptyOutDir: true,
        sourcemap: true,
        rollupOptions: {
            external: [
                // Node.js built-ins
                'child_process',
                'fs',
                'path',
                'os',
                'util',
                'stream',
                'events',
                'net',
                'http',
                'https',
                'url',
                'crypto',
                // Runtime dependencies (present in the Node-RED environment)
                'tmp',
                'request',
            ],
            output: {
                exports: 'auto',
            },
        },
    },
});
