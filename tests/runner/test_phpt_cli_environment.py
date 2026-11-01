"""CLI request environment behavior for the PHP 8.5.12 PHPT runner."""
import importlib.util
import os
from pathlib import Path
import re
import shutil
import subprocess
import sys
import unittest

ROOT = Path(__file__).resolve().parents[2]
TOOLS = ROOT / "tools"
if str(TOOLS) not in sys.path:
    sys.path.insert(0, str(TOOLS))

spec = importlib.util.spec_from_file_location("phego_phpt_cli_env_execute", TOOLS / "phpt_execute.py")
executor = importlib.util.module_from_spec(spec)
sys.modules[spec.name] = executor
spec.loader.exec_module(executor)

IMAGE = os.environ.get(
    "PHEGO_PHPT_SESSION_IMAGE",
    "i386/php@sha256:07e835693e041c2a79cd7a3a7a0fda4b2a8d65c6f93bd89bb8dad5c616f28049",
)
EXECUTABLE = "/usr/local/bin/php"
PLATFORM = os.environ.get("PHEGO_PHPT_SESSION_PLATFORM", "linux/386")
RELEASE = "8.5.12"
PHP_IMAGE_AVAILABLE = bool(
    re.fullmatch(r"[^\s]+@sha256:[0-9a-f]{64}", IMAGE)
    and shutil.which("docker")
    and subprocess.run(["docker", "image", "inspect", IMAGE], stdout=subprocess.DEVNULL,
                       stderr=subprocess.DEVNULL, check=False).returncode == 0
)


def sections(source, expected):
    return {"TEST": b"PHP 8.5.12 default CLI request environment",
            "FILE": source, "EXPECT": expected}


@unittest.skipUnless(PHP_IMAGE_AVAILABLE, "pinned PHP 8.5.12 linux/386 image and Docker are required")
class PHPTCLIRequestEnvironmentTests(unittest.TestCase):
    def run_sections(self, phpt_sections, **kwargs):
        args = dict(image=IMAGE, executable=EXECUTABLE, platform=PLATFORM,
                    release=RELEASE, timeout=15, output_limit=8192)
        args.update(kwargs)
        return executor.run(phpt_sections, **args)

    def test_basic_file_receives_official_default_cli_request_environment(self):
        source = (
            b"<?php\n"
            b"$names = ['QUERY_STRING','CONTENT_TYPE','CONTENT_LENGTH','HTTP_COOKIE'];\n"
            b"$values = []; foreach ($names as $name) { $v=getenv($name); "
            b"$values[]=$v===false?'<unset>':$v; }\n"
            b"$self=realpath(__FILE__);\n"
            b"echo PHP_SAPI, '|', getenv('REQUEST_METHOD'), '|', getenv('REDIRECT_STATUS'), '|', "
            b"(getenv('PATH_TRANSLATED')===$self?'same':'different'), '|', "
            b"(getenv('SCRIPT_FILENAME')===$self?'same':'different'), '|', implode('|',$values);\n"
            b"?>"
        )
        result = self.run_sections(
            sections(source, b"cli|GET|1|same|same|<unset>|<unset>|<unset>|<unset>")
        )
        self.assertEqual(result["status"], "pass", result)

    def test_fixture_session_uses_official_defaults_and_phase_specific_environment(self):
        request_names = (
            b"['REQUEST_METHOD','REDIRECT_STATUS','PATH_TRANSLATED','SCRIPT_FILENAME',"
            b"'QUERY_STRING','CONTENT_TYPE','CONTENT_LENGTH','HTTP_COOKIE']"
        )
        skipif = (
            b"<?php if (PHP_SAPI !== 'cli') exit(1); $names=" + request_names +
            b"; foreach ($names as $name) { if (getenv($name)!==false) exit(1); } ?>"
        )
        source = (
            b"<?php\n"
            b"$names=['QUERY_STRING','CONTENT_TYPE','CONTENT_LENGTH','HTTP_COOKIE']; $values=[]; "
            b"foreach($names as $name) {$v=getenv($name);$values[]=$v===false?'<unset>':$v;}\n"
            b"$self=realpath(__FILE__); echo PHP_SAPI,'|',getenv('REQUEST_METHOD'),'|',"
            b"getenv('REDIRECT_STATUS'),'|',(getenv('PATH_TRANSLATED')===$self?'same':'different'), '|',"
            b"(getenv('SCRIPT_FILENAME')===$self?'same':'different'),'|',implode('|',$values),'|',"
            b"file_get_contents(__DIR__.'/fixture.txt');\n?>"
        )
        clean = (
            b"<?php\n"
            b"$expected=['REQUEST_METHOD'=>false,'REDIRECT_STATUS'=>'1','PATH_TRANSLATED'=>false,"
            b"'SCRIPT_FILENAME'=>false,'QUERY_STRING'=>false,'CONTENT_TYPE'=>false,"
            b"'CONTENT_LENGTH'=>false,'HTTP_COOKIE'=>false];\n"
            b"if (PHP_SAPI !== 'cli') { echo 'CLEAN_SAPI_MISMATCH'; }\n"
            b"foreach($expected as $name=>$value) {$actual=getenv($name); "
            b"if (($value===false && $actual!==false) || ($value!==false && $actual!==$value)) { "
            b"echo 'CLEAN_ENV_MISMATCH'; break; }}\n?>"
        )
        phpt = sections(
            source,
            b"cli|GET|1|same|same|<unset>|<unset>|<unset>|<unset>|fixture-data",
        )
        phpt.update({"SKIPIF": skipif, "CLEAN": clean})
        result = self.run_sections(phpt, fixtures={"fixture.txt": b"fixture-data"})
        self.assertEqual(result["status"], "pass", result)

    def test_expectation_of_absent_request_method_is_a_mismatch(self):
        source = b"<?php $v=getenv('REQUEST_METHOD'); echo $v===false?'<unset>':$v; ?>"
        result = self.run_sections(sections(source, b"<unset>"))
        # Official run-tests.php sets REQUEST_METHOD=GET for FILE. An executor
        # that forgets the request environment must not turn this into a pass.
        self.assertEqual(result["status"], "fail", result)


if __name__ == "__main__":
    unittest.main()
