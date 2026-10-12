"""Exercise Make bundle contracts with fake adapters; never invoke Docker or Go."""
import json
import os
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest

ROOT=Path(__file__).resolve().parents[1]

class BundleMakeTests(unittest.TestCase):
    def invoke(self, targets, overrides=(), environment=None):
        with tempfile.TemporaryDirectory() as directory:
            root=Path(directory); (root/'Makefile').write_bytes((ROOT/'Makefile').read_bytes())
            adapter=root/'adapters'; adapter.mkdir(); calls=root/'calls.jsonl'
            for name in ['docker','sh','go']:
                path=adapter/name
                path.write_text('#!'+sys.executable+'\nimport json,os,sys\nwith open(os.environ["CALLS"],"a") as stream: stream.write(json.dumps(sys.argv)+"\\n")\n')
                path.chmod(0o700)
            env={key:value for key,value in os.environ.items() if key not in {'IMAGE','CONSUMER','BUNDLE','MAKEFLAGS','MAKEOVERRIDES','MFLAGS'}}
            env.update(PATH=str(adapter)+os.pathsep+os.environ['PATH'],CALLS=str(calls))
            env.update(environment or {})
            result=subprocess.run(['make','--no-print-directory',*targets,*overrides,'GO='+str(adapter/'go')],cwd=root,env=env,text=True,capture_output=True)
            self.assertEqual(result.returncode,0,result.stdout+result.stderr)
            return [json.loads(line) for line in calls.read_text().splitlines()] if calls.exists() else []
    def test_all_bundle_targets_forward_overrides(self):
        for bundle in ['toolbox','code-server','node','bun','codex','agy','claude']:
            with self.subTest(bundle=bundle):
                calls=self.invoke(['bundle-build','bundle-smoke','bundle-volume','bundle-test'],
                    ['BUNDLE='+bundle,'IMAGE=example/image:fixture','CONSUMER=example/consumer:fixture'])
                self.assertEqual([Path(call[0]).name for call in calls],['docker','sh','sh','go'])
                self.assertEqual(calls[0][1:],['build','-f','bundles/'+bundle+'/Dockerfile','-t','example/image:fixture','.'])
                self.assertEqual(calls[1][1:],['bundles/'+bundle+'/smoke.sh','example/image:fixture','example/consumer:fixture'])
                self.assertEqual(calls[2][1:],['scripts/bundle-volume.sh','example/image:fixture',bundle])
                self.assertEqual(calls[3][1], 'test')
                self.assertIn('./internal/toolvolume',calls[3])
                self.assertIn('./internal/dockercli',calls[3])
    def test_bundle_defaults(self):
        for bundle in ['toolbox','code-server','node','bun','codex','agy','claude']:
            with self.subTest(bundle=bundle):
                calls=self.invoke(['bundle-build','bundle-smoke','bundle-volume'],[] if bundle=='toolbox' else ['BUNDLE='+bundle])
                self.assertEqual(calls[0][-2:],['bundledock-'+bundle+':poc','.'])
                self.assertEqual(calls[1][-2:],['bundledock-'+bundle+':poc','mcr.microsoft.com/devcontainers/go:2-1.27-trixie'])
                self.assertEqual(calls[2][-2:],['bundledock-'+bundle+':poc',bundle])
    def test_shared_volume_uses_exact_id_and_bundle_namespace(self):
        for bundle in ['toolbox','code-server','node','bun','codex','agy','claude']:
            with self.subTest(bundle=bundle), tempfile.TemporaryDirectory() as directory:
                root=Path(directory); log=root/'calls'; adapter=root/'docker'
                adapter.write_text('#!'+sys.executable+'\nimport json,os,sys\nwith open(os.environ["CALLS"],"a") as stream:stream.write(json.dumps(sys.argv[1:])+"\\n")\nif sys.argv[1]=="image":print("sha256:"+"a"*64)\n')
                adapter.chmod(0o700)
                result=subprocess.run(['sh',str(ROOT/'scripts/bundle-volume.sh'),'example/image:fixture',bundle],env=dict(os.environ,PATH=str(root)+os.pathsep+os.environ['PATH'],CALLS=str(log)),capture_output=True,text=True)
                self.assertEqual(result.returncode,0,result.stdout+result.stderr)
                calls=[json.loads(line) for line in log.read_text().splitlines()]
                volume='bundledock-'+bundle+'-'+'a'*64
                self.assertEqual(calls,[['image','inspect','--format','{{.Id}}','--','example/image:fixture'],['volume','create',volume],['run','--rm','--network','none','--mount','type=volume,src='+volume+',dst=/bundle,volume-nocopy','sha256:'+'a'*64]])
                self.assertIn('Volume: '+volume,result.stdout)

    def test_invalid_selection_fails_before_adapters(self):
        for bundle in ['unknown','toolbox node','../node','--help','%','tool%','toolbox ','']:
            with self.subTest(bundle=bundle), tempfile.TemporaryDirectory() as directory:
                root=Path(directory); log=root/'calls'; docker=root/'docker'
                docker.write_text('#!/bin/sh\nprintf called > "$CALLS"\n');docker.chmod(0o700)
                env=dict(os.environ,PATH=str(root)+os.pathsep+os.environ['PATH'],CALLS=str(log))
                result=subprocess.run(['make','-f',str(ROOT/'Makefile'),'-n','bundle-build','BUNDLE='+bundle],env=env,capture_output=True,text=True)
                self.assertNotEqual(result.returncode,0)
                if bundle:
                    result=subprocess.run(['sh',str(ROOT/'scripts/bundle-volume.sh'),'fixture',bundle],env=env,capture_output=True,text=True)
                    self.assertNotEqual(result.returncode,0)
                self.assertFalse(log.exists())

    def test_image_and_consumer_are_literal_shell_data(self):
        with tempfile.TemporaryDirectory() as directory:
            marker=Path(directory)/'must-not-exist'
            values=['fixture"; touch '+str(marker)+'; #', 'fixture`touch '+str(marker)+'`', 'fixture$$(touch '+str(marker)+')']
            for value in values:
                for mode in ['argument','environment']:
                    with self.subTest(value=value,mode=mode):
                        args=['IMAGE='+value,'CONSUMER='+value] if mode=='argument' else []
                        environment={'IMAGE':value.replace('$$','$'),'CONSUMER':value.replace('$$','$')} if mode=='environment' else {}
                        calls=self.invoke(['bundle-build','bundle-smoke','bundle-volume'],args,environment)
                        expected=value.replace('$$','$')
                        self.assertEqual(calls[0][-2],expected)
                        self.assertEqual(calls[1][-2:],[expected,expected])
                        self.assertEqual(calls[2][-2],expected)
                        self.assertFalse(marker.exists())
