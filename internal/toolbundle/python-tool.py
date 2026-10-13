"""BundleDock-owned first-use installer; imports only the stock Python stdlib."""
import fcntl
import hashlib
import json
import os
from pathlib import Path
import platform
import shutil
import stat
import subprocess
import sys
import sysconfig


def fail(message):
    raise RuntimeError(message)


def outside(path, bundle, label):
    path = path.resolve()
    for root in (bundle, Path('/opt/bdock/toolbox'), Path('/opt/bundledock/toolbox')):
        if path == root or root in path.parents:
            fail(f'{label} must be outside the toolbox')
    return path


def writable(path, bundle, label):
    path = outside(path, bundle, label)
    path.mkdir(parents=True, exist_ok=True)
    if not path.is_dir() or not os.access(path, os.W_OK):
        fail(f'{label} must be a writable directory outside the toolbox')
    return path


def private_directory(path, label):
    info = os.lstat(path)
    if (not stat.S_ISDIR(info.st_mode) or info.st_uid != os.getuid()
            or info.st_mode & 0o022):
        fail(f'{label} must be owned by the current user and not group/other writable')


def regular(path):
    return not path.is_symlink() and path.is_file()


def valid_venv(venv, python):
    config = venv / 'pyvenv.cfg'
    if venv.is_symlink() or (venv / 'bin').is_symlink() or not regular(config):
        return False
    values = {}
    for line in config.read_text().splitlines():
        if '=' in line:
            key, value = (part.strip() for part in line.split('=', 1))
            if key in values:
                return False
            values[key] = value
    return (values.get('include-system-site-packages') == 'false'
            and Path(values.get('home', '')).resolve() == python.parent
            and (venv / 'bin/python').resolve() == python)


def completion(identity, venv, executable):
    return identity + '\n' + hashlib.sha256((venv / 'pyvenv.cfg').read_bytes()
                                           + executable.read_bytes()).hexdigest()


def main():
    if len(sys.argv) < 2 or sys.argv[1] not in ('specify', 'tldr'):
        fail('unsupported Python tool')
    tool = sys.argv.pop(1)
    minimum = {'specify': (3, 11), 'tldr': (3, 10)}[tool]
    bundle = Path(__file__).resolve().parents[1]
    python = Path(sys.executable).resolve()
    managed = os.environ.get('UV_PYTHON_INSTALL_DIR')
    if (sys.version_info < minimum or sys.prefix != sys.base_prefix
            or '/uv/python/' in str(python)
            or (managed and Path(managed).resolve() in (python, *python.parents))):
        fail(f'Python {minimum[0]}.{minimum[1]}+ from the language image is required; virtual or uv-managed Python is unsupported')
    home_value = os.environ.get('HOME')
    if not home_value:
        fail('HOME must be a writable directory outside the toolbox')
    home = outside(Path(home_value), bundle, 'HOME')
    if not home.is_dir() or not os.access(home, os.W_OK):
        fail('HOME must be a writable directory outside the toolbox')
    data = Path(os.environ.get('XDG_DATA_HOME') or home / '.local/share')
    cache_home = Path(os.environ.get('XDG_CACHE_HOME') or home / '.cache')
    if not data.is_absolute() or not cache_home.is_absolute():
        fail('XDG_DATA_HOME and XDG_CACHE_HOME must be absolute paths')
    state = writable(data / 'bundledock/tools' / tool, bundle, 'Python tool state')
    private_directory(state, 'Python tool state')
    cache = writable(cache_home / 'bundledock/pip' / tool, bundle, 'Python tool pip cache')
    requirements = bundle / 'share' / tool / f'{tool}.requirements.txt'
    identity = hashlib.sha256(requirements.read_bytes() + Path(__file__).read_bytes()
                              + json.dumps([str(python), sys.version, sysconfig.get_config_var('SOABI'),
                                            platform.machine()]).encode()).hexdigest()
    venv = state / identity
    owned, complete = venv / '.bundledock-owned', venv / '.bundledock-complete'
    executable = venv / 'bin' / tool
    env = {key: value for key, value in os.environ.items()
           if not key.startswith(('PIP_', 'PYTHON', 'UV_', 'CONDA'))
           and key not in ('VIRTUAL_ENV', '__PYVENV_LAUNCHER__', '_PYTHON_HOST_PLATFORM')}
    # All installers for this tool serialize; the lock is released before exec.
    # Refuse links at owned state leaves rather than following/deleting them.
    lockfd = os.open(state / '.install.lock', os.O_CREAT | os.O_RDWR | os.O_NOFOLLOW, 0o600)
    with os.fdopen(lockfd, 'w') as lock:
        fcntl.flock(lock, fcntl.LOCK_EX)
        if venv.is_symlink():
            fail('Python tool environment must not be a symbolic link')
        if venv.exists():
            private_directory(venv, 'Python tool environment')
        ready = (regular(owned) and owned.read_text() == identity
                 and regular(complete) and regular(executable)
                 and os.access(executable, os.X_OK) and valid_venv(venv, python)
                 and complete.read_text() == completion(identity, venv, executable))
        if not ready:
            if venv.exists():
                if not regular(owned) or owned.read_text() != identity:
                    fail('unrecognized Python tool environment; preserve it and use another XDG_DATA_HOME')
                shutil.rmtree(venv)
            venv.mkdir(mode=0o700)
            owned.write_text(identity)
            # Explicit options plus an empty pip configuration prevent caller
            # indexes, requirements, targets, user installs and source builds.
            install_env = dict(env)
            install_env.update(HOME=str(home), PIP_CONFIG_FILE=os.devnull,
                       PATH=str(venv / 'bin') + os.pathsep + os.environ.get('PATH', ''))
            if subprocess.run([str(python), '-I', '-B', '-m', 'venv', str(venv)], env=install_env).returncode:
                fail('venv creation failed; the stock language image must supply Python venv and pip')
            if not valid_venv(venv, python):
                fail('venv configuration does not isolate the stock Python interpreter')
            dependency_policy = ['--no-deps'] if tool == 'tldr' else []
            result = subprocess.run([str(venv / 'bin/python'), '-I', '-B', '-m', 'pip',
                                     'install', '--disable-pip-version-check', '--no-input',
                                     '--only-binary=:all:', '--index-url', 'https://pypi.org/simple',
                                     '--cache-dir', str(cache), '--requirement', str(requirements), *dependency_policy], env=install_env)
            if result.returncode:
                fail('wheel installation failed; first use needs PyPI access and compatible wheels for this Python/ABI/platform; no source build was attempted')
            if not regular(executable) or not os.access(executable, os.X_OK):
                fail('installed Python tool entry point is missing')
            pending = venv / '.bundledock-complete.tmp'
            pending.write_text(completion(identity, venv, executable))
            pending.replace(complete)
    # Isolate Python imports while retaining the caller's application environment,
    # working directory, arguments and exec exit/signal behavior.
    os.execve(str(venv / 'bin/python'), [str(venv / 'bin/python'), '-I', '-B', str(executable), *sys.argv[1:]], env)


if __name__ == '__main__':
    try:
        main()
    except (OSError, RuntimeError) as error:
        print(f'BundleDock Python launcher: {error}', file=sys.stderr)
        sys.exit(1)
