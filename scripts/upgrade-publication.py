#!/usr/bin/env python3
"""Read-only publication evidence for an unchanged updater lock."""
import importlib.util
import json
import os
from pathlib import Path
import re
import subprocess


def load(name):
    spec = importlib.util.spec_from_file_location(name, Path(__file__).with_name(name+'.py'))
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


provenance = load('bundle-git-tags')
config = load('bundle-config')


class PublicRegistry(provenance.Registry):
    def __init__(self, image):
        self.path = image.removeprefix('quay.io/')
        query = provenance.urllib.parse.urlencode({'service': 'quay.io', 'scope': 'repository:'+self.path+':pull'})
        self.token = json.loads(provenance.request('https://quay.io/v2/auth?'+query))['token']
        if not isinstance(self.token, str) or not self.token or len(self.token) > 65536:
            raise ValueError('invalid anonymous pull token')


def publication_status(bundle, prefix):
    """Only a complete public empty listing proves unpublished; errors are unknown."""
    config.select_bundle(bundle)
    if not re.fullmatch(r'quay\.io/[a-z0-9]+(?:[._-][a-z0-9]+)*/?', prefix):
        return 'unknown'
    image = prefix.rstrip('/')+'/'+bundle
    try:
        listing = json.loads(provenance.request('https://quay.io/api/v1/repository/'+image.removeprefix('quay.io/')+'/tag/?limit=100&page=1&onlyActiveTags=true'))
        tags = listing['tags']
        if not isinstance(tags, list) or listing.get('has_additional') is not False or len(tags) > 100:
            return 'unknown'
        if not tags:
            return 'unpublished'
        # A staging-only listing is ambiguous. Verify one immutable published
        # candidate, never infer a baseline from a moving alias or staging tag.
        candidate = next((tag for tag in tags if isinstance(tag, dict) and
                          isinstance(tag.get('name'), str) and
                          (provenance.VERSION.fullmatch(tag['name']) or provenance.LEGACY.fullmatch(tag['name']))), None)
        if candidate is None or not provenance.DIGEST.fullmatch(candidate.get('manifest_digest', '')):
            return 'unknown'
        registry = PublicRegistry(image)
        index = provenance.decode_digest(registry.get('manifests', candidate['name']), candidate['manifest_digest'])
        if len(index['manifests']) != 2:
            return 'unknown'
        manifest = provenance.decode_digest(registry.get('manifests', index['manifests'][0]['digest']), index['manifests'][0]['digest'])
        labels = provenance.decode_digest(registry.get('blobs', manifest['config']['digest']), manifest['config']['digest'])['config']['Labels']
        commit = labels.get('org.opencontainers.image.revision', '')
        run_id = labels.get('io.bundledock.github.run-id', '')
        legacy = provenance.LEGACY.fullmatch(candidate['name'])
        if legacy and not run_id:
            run_id = legacy[2]
        if not re.fullmatch('[0-9a-f]{40}', commit) or not re.fullmatch('[1-9][0-9]*', run_id):
            return 'unknown'
        record = dict(tag=candidate['name'], digest=candidate['manifest_digest'], source=provenance.SOURCE,
                      commit=commit, run_id=run_id, legacy=bool(legacy))
        provenance.verify_registry(record, registry)
        return 'published'
    except Exception:
        # Registry/network diagnostics may contain signed URLs or response data.
        # Unknown is informational and never authorizes bootstrap/publication.
        return 'unknown'


def summary(bundle, prefix, sha):
    selected = config.select_bundle(bundle)
    if not re.fullmatch('[0-9a-f]{40}', sha):
        raise ValueError('expected full source HEAD SHA')
    state = publication_status(bundle, prefix)
    if state == 'unpublished':
        url = provenance.SOURCE+'/actions/workflows/'+selected['workflow']
        return (f'Bundle unpublished—initial publication required ({bundle}).\n\n'
                f'Request explicit approval to build and publish source revision `{sha}`, '
                f'and verify repository visibility and robot access. After approval, use the '
                f'[manual publisher]({url}) on that exact main revision with `approved_sha` '
                f'set to `{sha}`. This updater does not build, publish, dispatch, or activate images.')
    if state == 'published':
        return f'Bundle published ({bundle}): verified an immutable version and both Linux platform images. The updater created no branch or pull request for this unchanged lock.'
    return (f'Bundle publication status unknown ({bundle}): public registry evidence could not verify '
            'an empty repository or a published baseline. Check registry visibility, access and '
            'publication history before requesting publication; no build or publication was started.')


if __name__ == '__main__':
    head = subprocess.run(['git', 'rev-parse', '--verify', 'HEAD^{commit}'], check=True, capture_output=True, text=True).stdout.strip()
    print(summary(os.environ['BUNDLE'], os.environ.get('REPOSITORY_URL', ''), head))
