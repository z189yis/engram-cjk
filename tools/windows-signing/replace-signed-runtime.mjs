import { copyFileSync, readFileSync } from 'node:fs';
import path from 'node:path';
import { executableHash } from './provenance.mjs';

const [target, os, arch] = process.argv.slice(2);
if (os === 'windows') {
  if (!['amd64', 'arm64'].includes(arch)) throw new Error('Unsupported Windows architecture.');
  const directory = process.env.SIGNED_RUNTIME_DIRECTORY;
  if (!directory) {
    if (process.env.RUNTIME_UNSIGNED_BUILD !== '1') throw new Error('Centrally signed runtime bytes are required for publication.');
  } else {
    const name = `engram-windows-${arch}.exe`;
    const manifest = JSON.parse(readFileSync(path.join(directory, 'signed-runtime-manifest.json'), 'utf8'));
    const entry = manifest.files.find(file => file.name === name);
    if (!entry || executableHash(path.dirname(target), path.basename(target)) !== entry.unsignedSha256 ||
        executableHash(directory, name) !== entry.signedSha256) {
      throw new Error('Rebuilt executable does not match the source bytes authenticated by central signing.');
    }
    copyFileSync(path.join(directory, name), target);
  }
}
