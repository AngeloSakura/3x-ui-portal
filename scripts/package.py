"""Package already-built portal and frontend; never includes runtime data or secrets."""
import argparse, hashlib, pathlib, tarfile, json, subprocess
p = argparse.ArgumentParser()
p.add_argument('--arch', choices=['amd64', 'arm64'], required=True)
args = p.parse_args()
root = pathlib.Path(__file__).resolve().parents[1]
output = root / 'dist'
output.mkdir(exist_ok=True)
binary = output / f'portal-linux-{args.arch}'
assert binary.is_file() and (root / 'frontend/dist/index.html').is_file()
archive = output / f'3x-ui-portal-linux-{args.arch}.tar.gz'
with tarfile.open(archive, 'w:gz') as tar:
    tar.add(binary, arcname='portal')
    for src, dest in [('frontend/dist','web'), ('deploy','deploy'), ('scripts/install-linux.sh','scripts/install-linux.sh'), ('LICENSE','LICENSE'), ('NOTICE','NOTICE'), ('README.md','README.md'), ('docs','docs')]:
        tar.add(root / src, arcname=dest)
    # 分发依赖原始许可，不用本项目非商业许可替代第三方授权。
    modules = subprocess.check_output(['go', 'list', '-m', '-json', 'all'], cwd=root, text=True)
    decoder = json.JSONDecoder()
    while modules.strip():
        module, end = decoder.raw_decode(modules.lstrip())
        modules = modules.lstrip()[end:]
        if module.get('Main') or not module.get('Dir'): continue
        directory = pathlib.Path(module['Dir'])
        for file in directory.iterdir():
            if file.is_file() and file.name.upper().startswith(('LICENSE', 'COPYING', 'NOTICE')):
                tar.add(file, arcname='licenses/go/'+module['Path']+'/'+file.name)
    for file in (root/'frontend/node_modules').rglob('*'):
        if file.is_file() and file.name.upper().startswith(('LICENSE','COPYING','NOTICE')):
            tar.add(file, arcname='licenses/npm/'+str(file.relative_to(root/'frontend/node_modules')).replace('\\','/'))
sums = ''.join(f'{hashlib.sha256(f.read_bytes()).hexdigest()}  {f.name}\n' for f in sorted(output.glob('3x-ui-portal-linux-*.tar.gz')))
(output / 'SHA256SUMS').write_text(sums, encoding='ascii', newline='\n')
print(archive.name)
