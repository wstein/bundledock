import importlib.util
from pathlib import Path
import unittest

ROOT=Path(__file__).resolve().parents[2]
def load(name):
    spec=importlib.util.spec_from_file_location(name,ROOT/'scripts'/f'{name}.py')
    module=importlib.util.module_from_spec(spec);spec.loader.exec_module(module);return module

class NodeChannels(unittest.TestCase):
    def test_source_tags_keep_source_identity_in_shared_repository(self):
        tags=load('bundle-git-tags')
        record=dict(bundle='node-current',image='quay.io/bundledock/node',tag='26.11.1-b1',
                    digest='sha256:'+'a'*64,commit='b'*40,source=tags.SOURCE,run_id='123',run_attempt='1')
        validated=tags.validate_record(record)
        self.assertEqual(tags.tag_name(validated),'node-current/26.11.1-b1')
        self.assertIn('quay.io/bundledock/node:26.11.1-b1',tags.annotation(validated))
        with self.assertRaises(ValueError): tags.validate_record(dict(record,image='quay.io/bundledock/node-current'))
    def test_current_oci_source_label_required(self):
        tags=load('bundle-git-tags')
        r=dict(bundle='node-current',tag='26.11.1-b1',source=tags.SOURCE,commit='b'*40,run_id='123',attempt=1,legacy=False)
        labels={'org.opencontainers.image.source':tags.SOURCE,'org.opencontainers.image.revision':'b'*40,
                'org.opencontainers.image.version':r['tag'],'io.bundledock.github.run-id':'123','io.bundledock.github.run-attempt':'1'}
        with self.assertRaises(ValueError): tags.verify_labels(r,labels)
        tags.verify_labels(r,dict(labels,**{'io.bundledock.bundle':'node-current'}))
        with self.assertRaises(ValueError): tags.verify_labels(r,dict(labels,**{'io.bundledock.bundle':'node'}))

    def test_shared_workflow_serializes_node_channels_and_selects_repository(self):
        workflow=(ROOT/'.github/workflows/bundle-publish.yml').read_text()
        self.assertIn("(inputs.bundle == 'node' || inputs.bundle == 'node-current') && 'node' || inputs.bundle",workflow)
        self.assertIn('cancel-in-progress: false',workflow)
        self.assertIn('scripts/bundle-config.py --repository "$BUNDLE"',workflow)
        self.assertNotIn('promote version, stable and latest',workflow)

    def test_semantic_lts_oci_identity_required_but_historical_date_allowed(self):
        tags=load('bundle-git-tags')
        r=dict(bundle='node',tag='26.11.1-b1',source=tags.SOURCE,commit='b'*40,run_id='123',attempt=1,legacy=False)
        labels={'org.opencontainers.image.source':tags.SOURCE,'org.opencontainers.image.revision':'b'*40,
                'org.opencontainers.image.version':r['tag'],'io.bundledock.github.run-id':'123','io.bundledock.github.run-attempt':'1'}
        with self.assertRaises(ValueError): tags.verify_labels(r,labels)
        tags.verify_labels(r,dict(labels,**{'io.bundledock.bundle':'node'}))
        r['tag']='2026.10.12-b1.1';labels['org.opencontainers.image.version']=r['tag']
        tags.verify_labels(r,labels)

    def test_semantic_publication_source_binding_for_both_channels(self):
        tags=load('bundle-git-tags')
        for bundle in ('node','node-current'):
            receipt={'tag':'26.11.1-b1','platforms':[{'architecture':a} for a in ('amd64','arm64')]}
            with self.assertRaises(ValueError): tags.bind_publication(receipt,bundle)
            receipt['bundle']=bundle
            for p in receipt['platforms']: p['bundle']=bundle
            self.assertEqual(tags.bind_publication(receipt,bundle)['bundle'],bundle)
            del receipt['platforms'][1]['bundle']
            with self.assertRaises(ValueError): tags.bind_publication(receipt,bundle)
        self.assertEqual(tags.bind_publication({'tag':'2026.10.12-b1.1'},'node')['bundle'],'node')
