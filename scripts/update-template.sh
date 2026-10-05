#!/bin/sh
set -eu

task_root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
python3 - "$task_root" <<'PY'
import os
import sys
import zipfile
from pathlib import Path

root = Path(sys.argv[1])
source = root / 'template'
required = ['go.mod', 'go.sum', '.env.template', 'cmd/main.go', 'cmd/api.go',
            'docker-compose.yaml', 'sqlc.yaml', 'README.md',
            'internal/adapters/postgresql/migrations/.gitkeep',
            'internal/adapters/postgresql/sqlc/queries.sql']
excluded_dirs = {'.git', '.idea', 'bin', 'build', 'dist', 'coverage', '__pycache__'}

def excluded(relative):
    name = relative.name
    return (any(part in excluded_dirs for part in relative.parts)
            or name == '.env' or (name.startswith('.env.') and name != '.env.template')
            or name == 'template.zip' or name == '.DS_Store' or name.startswith('._')
            or name.startswith('__debug_bin') or name in {'go.work', 'go.work.sum'}
            or name.endswith(('.out', '.prof', '.test', '.exe', '.pyc')))

for name in required:
    if not (source / name).is_file():
        raise SystemExit(f'模板缺少文件：{name}')

output = root / 'template.zip'
temporary = root / '.template.zip.tmp'
try:
    with zipfile.ZipFile(temporary, 'w', compression=zipfile.ZIP_DEFLATED) as archive:
        for current, dirs, names in os.walk(source, followlinks=False):
            dirs[:] = sorted(d for d in dirs if not excluded(Path(current).relative_to(source) / d))
            for name in sorted(dirs + names):
                item = Path(current) / name
                relative = item.relative_to(source)
                if excluded(relative):
                    continue
                if item.is_symlink():
                    raise SystemExit(f'模板不支持符号链接：{relative}')
                if item.is_dir():
                    continue
                if not item.is_file():
                    raise SystemExit(f'模板不支持此文件类型：{relative}')
                info = zipfile.ZipInfo(relative.as_posix(), date_time=(1980, 1, 1, 0, 0, 0))
                info.create_system = 3
                mode = 0o755 if item.stat().st_mode & 0o111 else 0o644
                info.external_attr = (0o100000 | mode) << 16
                info.compress_type = zipfile.ZIP_DEFLATED
                archive.writestr(info, item.read_bytes())
    with zipfile.ZipFile(temporary) as archive:
        names = set(archive.namelist())
        if not set(required) <= names or archive.testzip() is not None:
            raise SystemExit('模板完整性检查失败')
    temporary.replace(output)
finally:
    temporary.unlink(missing_ok=True)
print(f'已更新 {output}（{len(names)} 个文件）')
PY
