import * as path from 'path';

/**
 * Compiled engine files land at dist/engine/.
 * Scripts (deploy.sh, uninstall.sh) are packaged under helm/ at the package root.
 */
export const SCRIPTS_DIR = path.join(__dirname, '..', '..', 'helm');
