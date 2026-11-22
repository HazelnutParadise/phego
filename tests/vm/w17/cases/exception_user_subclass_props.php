<?php
// exception_user_subclass_props: redeclared protected $message and $code defaults, read without calling a constructor
class Preset extends Exception {
    protected $message = 'preset';
    protected $code = 42;
}

class StringCode extends Exception {
    protected $message = 'string code';
    protected $code = 'E42';
}

class PresetError extends Error {
    protected $message = 'error preset';
    protected $code = 7;
}

class Partial extends Exception {
    protected $message = 'partial';
}

$a = new Preset();
echo "preset_message: ", $a->getMessage(), "\n";
echo "preset_code: ", $a->getCode(), "\n";
echo "preset_previous_null: ", ($a->getPrevious() === null ? 1 : 0), "\n";

$b = new StringCode();
echo "string_code_message: ", $b->getMessage(), "\n";
echo "string_code_value: ", $b->getCode(), "\n";
echo "string_code_is_E42: ", ($b->getCode() === 'E42' ? 1 : 0), "\n";

$c = new PresetError();
echo "error_preset_message: ", $c->getMessage(), "\n";
echo "error_preset_code: ", $c->getCode(), "\n";
echo "error_is_error: ", ($c instanceof Error ? 1 : 0), "\n";

$d = new Partial();
echo "partial_message: ", $d->getMessage(), "\n";
echo "partial_code: ", $d->getCode(), "\n";

$e = new Preset("given", 5);
echo "given_overrides_message: ", $e->getMessage(), "\n";
echo "given_overrides_code: ", $e->getCode(), "\n";

$f = new Preset("");
echo "explicit_empty_overrides: [", $f->getMessage(), "]\n";
echo "explicit_empty_keeps_code: ", $f->getCode(), "\n";

$g = new PresetError("error given");
echo "error_given_message: ", $g->getMessage(), "\n";
echo "error_given_code_kept: ", $g->getCode(), "\n";
