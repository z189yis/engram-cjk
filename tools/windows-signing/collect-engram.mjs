import { copyFileSync, mkdirSync, readFileSync } from 'node:fs';
import path from 'node:path';
import { executableHash } from './provenance.mjs';

const artifacts = JSON.parse(readFileSync('dist/artifacts.json', 'utf8'));
mkdirSync('unsigned-runtime', { recursive: true });
for (const arch of ['amd64', 'arm64']) {
  const binaries = artifacts.filter(file => file.type === 'Binary' && file.goos === 'windows' && file.goarch === arch);
  if (binaries.length !== 1) throw new Error(`Expected one Windows ${arch} build artifact.`);
  const file = path.resolve(binaries[0].path);
  const relative = path.relative(path.resolve('dist'), file);
  if (relative.startsWith('..') || path.isAbsolute(relative)) throw new Error('Build artifact is outside dist.');
  executableHash(path.dirname(file), path.basename(file));
  copyFileSync(file, `unsigned-runtime/engram-windows-${arch}.exe`);
}
