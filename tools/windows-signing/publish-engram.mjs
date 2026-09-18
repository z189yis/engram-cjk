import { lstatSync, readFileSync } from 'node:fs';
import path from 'node:path';
import { spawnSync } from 'node:child_process';
import { sourceFor } from './provenance.mjs';

const [directory, tag] = process.argv.slice(2);
const source = sourceFor('engram');
if (!source.tag.test(tag ?? '')) throw new Error('Invalid verified release tag.');
const artifacts = JSON.parse(readFileSync(path.join(directory, 'artifacts.json'), 'utf8'));
const selected = artifacts.filter(file => ['Archive', 'Checksum', 'SBOM'].includes(file.type));
if (selected.filter(file => file.type === 'Archive').length !== 6 ||
    selected.filter(file => file.type === 'Checksum').length !== 1 ||
    selected.filter(file => file.type === 'SBOM').length !== 6) {
  throw new Error('Expected six platform archives, six SBOMs and one checksum file.');
}
const root = path.resolve(directory);
const names = new Set();
const files = selected.map(file => {
  // GoReleaser paths are relative to its source working directory, not dist.
  const filename = path.resolve(root, '..', file.path);
  const relative = path.relative(root, filename);
  const stat = lstatSync(filename);
  if (relative.startsWith('..') || path.isAbsolute(relative) ||
      !stat.isFile() || stat.isSymbolicLink() || names.has(path.basename(filename))) {
    throw new Error('Release artifact paths must be unique regular files inside dist.');
  }
  names.add(path.basename(filename));
  return filename;
});
for (const os of ['darwin', 'linux', 'windows']) {
  for (const arch of ['amd64', 'arm64']) {
    const extension = os === 'windows' ? 'zip' : 'tar.gz';
    if (!names.has(`engram_${tag}_${os}_${arch}.${extension}`)) {
      throw new Error('Archive version or platform does not match the verified release.');
    }
  }
}
const result = spawnSync('gh', ['release', 'create', tag, ...files, '--repo', source.repo,
  '--verify-tag', '--title', tag, '--generate-notes'], { stdio: 'inherit' });
if (result.error) throw result.error;
if (result.status !== 0) throw new Error('Original repository release publication failed.');
