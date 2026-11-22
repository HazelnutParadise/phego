<?php
// catch_engine_type_error_property: TypeError from typed property assignments, plus readonly and uninitialized Errors
function report($label, $e)
{
    echo $label, ": ", $e::class, " | ", $e->getMessage(), " | line ", $e->getLine(), "\n";
}

class Typed
{
    public int $n = 0;
    public ?string $s = null;
    public float $f = 0.0;
    public readonly int $ro;
    public string $never_set;

    public function __construct()
    {
        $this->ro = 5;
    }
}

$t = new Typed();

try {
    $t->n = "abc";
} catch (TypeError $e) {
    report("int_word", $e);
}

$t->n = "42";
echo "int_numeric_string: ", $t->n, "\n";

try {
    $t->n = [1];
} catch (TypeError $e) {
    report("int_array", $e);
}

try {
    $t->n = null;
} catch (TypeError $e) {
    report("int_null", $e);
}

$t->s = 7;
echo "string_from_int: ", $t->s, "\n";

try {
    $t->s = [];
} catch (TypeError $e) {
    report("nullable_string_array", $e);
}

$t->f = 3;
echo "float_from_int: ", $t->f, "\n";

try {
    $t->f = "abc";
} catch (TypeError $e) {
    report("float_word", $e);
}

try {
    $t->ro = 9;
} catch (Error $e) {
    report("readonly_outside", $e);
}
echo "ro_still: ", $t->ro, "\n";

try {
    echo $t->never_set, "\n";
} catch (Error $e) {
    report("uninitialized", $e);
}

echo "n_still: ", $t->n, "\n";
echo "end: ok\n";
