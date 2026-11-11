import contextlib
import io
from pathlib import Path
import tempfile
import unittest

import php_diagnostics


class PHPDiagnosticsTests(unittest.TestCase):
    def test_identifier_operand_uses_its_rhs_slot(self):
        text = """  case 1: /* four operands */
          if (zend_lex_tstring(&zv, (yyvsp[-3].ident)) == FAILURE) { YYABORT; }
    break;
  case 2: /* three operands */
          if (zend_lex_tstring(&zv, (yyvsp[0].ident)) == FAILURE) { YYABORT; }
    break;
"""
        names, guards = php_diagnostics.diagnostic_guards(text, [0, 4, 3])
        self.assertEqual(guards, {1: (names.index("Identifier"), 0), 2: (names.index("Identifier"), 2)})

    def test_modifier_operand_and_target(self):
        text = """  case 1:
          uint32_t x = zend_modifier_token_to_flag(ZEND_MODIFIER_TARGET_METHOD, (yyvsp[-1].num));
    break;
  case 2:
          x = zend_modifier_list_to_flags(ZEND_MODIFIER_TARGET_PROPERTY_HOOK, (yyvsp[0].ast));
    break;
"""
        names, guards = php_diagnostics.diagnostic_guards(text, [0, 4, 1])
        self.assertEqual(guards, {1: (names.index("Method"), 2), 2: (names.index("PropertyHook"), 0)})

    def test_unknown_operand_fails_closed(self):
        with self.assertRaisesRegex(ValueError, "unrecognized diagnostic operand"):
            php_diagnostics.diagnostic_guards("  case 1: zend_lex_tstring(&zv, changed);", [0, 1])
        with self.assertRaisesRegex(ValueError, "outside rule"):
            php_diagnostics.diagnostic_guards("  case 1: zend_lex_tstring(&zv, (yyvsp[-2].ident));", [0, 1])

    def test_wrong_release_preserves_existing_outputs(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            source = root / "parser.c"
            source.write_text("untrusted source")
            output = root / "php8512_diagnostics.bin"
            output.write_bytes(b"keep")
            with contextlib.redirect_stderr(io.StringIO()):
                result = php_diagnostics.main(["--source", str(source), "--output-dir", tmp])
            self.assertEqual(result, 1)
            self.assertEqual(output.read_bytes(), b"keep")
            self.assertFalse((root / "diagnostics_generated.go").exists())

    def test_no_arguments_show_operations(self):
        text = io.StringIO()
        with contextlib.redirect_stdout(text):
            self.assertEqual(php_diagnostics.main([]), 0)
        for flag in ("--source", "--output-dir", "--check"):
            self.assertIn(flag, text.getvalue())


if __name__ == "__main__":
    unittest.main()
