"""Reference PHP resets inherited environment before PHPT phases."""
import os
from pathlib import Path
import re
import shutil
import subprocess
import sys
import unittest
from unittest import mock

ROOT = Path(__file__).resolve().parents[2]
sys.path.insert(0, str(ROOT / 'tools'))
import phpt_execute as executor
import phpt_session as session

IMAGE = os.environ.get('PHEGO_PHPT_SESSION_IMAGE', 'i386/php@sha256:07e835693e041c2a79cd7a3a7a0fda4b2a8d65c6f93bd89bb8dad5c616f28049')
PLATFORM = os.environ.get('PHEGO_PHPT_SESSION_PLATFORM', 'linux/386')
AVAILABLE = bool(re.fullmatch(r'[^\s]+@sha256:[0-9a-f]{64}', IMAGE) and shutil.which('docker') and
                 subprocess.run(['docker', 'image', 'inspect', IMAGE], stdout=subprocess.DEVNULL,
                                stderr=subprocess.DEVNULL, check=False).returncode == 0)

@unittest.skipUnless(AVAILABLE, 'pinned reference image and Docker required')
class InheritedEnvironmentTests(unittest.TestCase):
    def invoke(self, tz):
        expected_tz = tz or '<unset>'
        skipif = b'''<?php $v=[]; foreach (['REQUEST_METHOD','REDIRECT_STATUS','TZ'] as $k) { $x=getenv($k); $v[]=$x===false?'<unset>':$x; } file_put_contents('skip-env',implode('|',$v)); ?>'''
        source = b'''<?php $x=getenv('TZ'); echo file_get_contents('skip-env'),'|',getenv('REQUEST_METHOD'),'|',getenv('REDIRECT_STATUS'),'|',($x===false?'<unset>':$x); ?>'''
        clean = ("<?php if (getenv('REQUEST_METHOD') !== false || getenv('REDIRECT_STATUS') !== '1' || "
                 "(getenv('TZ') === false ? '<unset>' : getenv('TZ')) !== '%s') echo 'bad clean environment'; ?>" % expected_tz).encode()
        sections = {'TEST': b'inherited environment', 'SKIPIF': skipif, 'FILE': source,
                    'EXPECT': ('<unset>|<unset>|%s|GET|1|%s' % (expected_tz, expected_tz)).encode(), 'CLEAN': clean}
        if tz:
            sections['ENV'] = ('TZ=' + tz).encode()
        original = session.subprocess.Popen
        def inject(args, *pos, **kwargs):
            if len(args) > 2 and args[:2] == ['docker', 'create']:
                args = list(args)
                index = args.index(IMAGE)
                args[index:index] = ['--env', 'REQUEST_METHOD=POST', '--env', 'REDIRECT_STATUS=777', '--env', 'TZ=Asia/Taipei']
            return original(args, *pos, **kwargs)
        with mock.patch.object(session.subprocess, 'Popen', side_effect=inject):
            return executor.run(sections, IMAGE, '/usr/local/bin/php', PLATFORM, '8.5.12', 15, 8192)

    def test_image_environment_is_reset_in_all_phases(self):
        result = self.invoke(None)
        self.assertEqual(result['status'], 'pass', result)

    def test_empty_env_is_unset_even_if_inherited(self):
        sections = {
            'TEST': b'empty ENV removes inherited value', 'ENV': b'PHEGO_EMPTY=\nTZ=\nPHEGO_VALUE=abc=',
            'SKIPIF': b"<?php if (getenv('PHEGO_EMPTY') !== false || getenv('TZ') !== false || getenv('PHEGO_VALUE') !== 'abc=') exit(1); ?>",
            'FILE': b"<?php echo getenv('PHEGO_EMPTY') === false && getenv('TZ') === false && getenv('PHEGO_VALUE') === 'abc=' ? 'unset' : 'set'; ?>",
            'CLEAN': b"<?php if (getenv('PHEGO_EMPTY') !== false || getenv('TZ') !== false || getenv('PHEGO_VALUE') !== 'abc=') echo 'bad clean'; ?>",
            'EXPECT': b'unset',
        }
        original = session.subprocess.Popen
        def inject(args, *pos, **kwargs):
            if len(args) > 2 and args[:2] == ['docker', 'create']:
                args = list(args)
                index = args.index(IMAGE)
                args[index:index] = ['--env', 'PHEGO_EMPTY=inherited', '--env', 'TZ=Asia/Taipei']
            return original(args, *pos, **kwargs)
        with mock.patch.object(session.subprocess, 'Popen', side_effect=inject):
            result = executor.run(sections, IMAGE, '/usr/local/bin/php', PLATFORM, '8.5.12', 15, 8192)
        self.assertEqual(result['status'], 'pass', result)

    def test_cgi_profile_ignores_inherited_request(self):
        original = session.subprocess.Popen
        def inject(args, *pos, **kwargs):
            if len(args) > 2 and args[:2] == ['docker', 'create']:
                args = list(args)
                index = args.index(IMAGE)
                args[index:index] = ['--env', 'REQUEST_METHOD=POST', '--env', 'REDIRECT_STATUS=777',
                                     '--env', 'SCRIPT_FILENAME=/does/not/exist', '--env', 'TZ=Asia/Taipei']
            return original(args, *pos, **kwargs)
        sections = {'TEST': b'CGI profile', 'CGI': b'', 'FILE': b"<?php echo 'body'; ?>", 'EXPECT': b'body'}
        with mock.patch.object(session.subprocess, 'Popen', side_effect=inject):
            result = executor.run(sections, IMAGE, '/usr/local/bin/php', PLATFORM, '8.5.12', 15, 8192,
                                  cgi_executable='/usr/local/bin/php-cgi')
        self.assertEqual(result['status'], 'pass', result)

    def test_cgi_profile_ignores_individual_gateway_markers(self):
        original = session.subprocess.Popen
        for name in ('SERVER_NAME', 'SERVER_SOFTWARE', 'GATEWAY_INTERFACE'):
            with self.subTest(name=name):
                def inject(args, *pos, **kwargs):
                    if len(args) > 2 and args[:2] == ['docker', 'create']:
                        args = list(args)
                        index = args.index(IMAGE)
                        args[index:index] = ['--env', name + '=ambient']
                    return original(args, *pos, **kwargs)
                with mock.patch.object(session.subprocess, 'Popen', side_effect=inject):
                    result = session.run_case(IMAGE, '/usr/local/bin/php', PLATFORM, '8.5.12',
                                              b"<?php echo 'body'; ?>", b'', {}, 15, 8192,
                                              cgi_executable='/usr/local/bin/php-cgi')
                self.assertIsNotNone(result.cgi_profile, result)
                self.assertEqual(result.cgi_profile.status, 'completed', result)
                self.assertTrue(result.cleanup_confirmed, result)

    def test_explicit_timezone_is_preserved_in_all_phases(self):
        result = self.invoke('Etc/GMT-5')
        self.assertEqual(result['status'], 'pass', result)

if __name__ == '__main__':
    unittest.main()
