import fs from 'fs';
import path from 'path';
import { fileURLToPath } from 'url';

const __dirname = path.dirname(fileURLToPath(import.meta.url));

// Read package.json
const packageJson = JSON.parse(
  fs.readFileSync(path.join(__dirname, '../package.json'), 'utf8')
);

// Create version info
const versionInfo = {
  version: packageJson.version,
  buildDate: new Date().toISOString(),
};

// Write to src/version.json
fs.writeFileSync(
  path.join(__dirname, '../src/version.json'),
  JSON.stringify(versionInfo, null, 2) + '\n',
  'utf8'
);

console.log(`✅ Generated version.json: v${versionInfo.version} (${versionInfo.buildDate})`);
