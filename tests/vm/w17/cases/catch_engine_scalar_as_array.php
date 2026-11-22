<?php
// catch_engine_scalar_as_array: Errors for array syntax written into scalars and strings
function report($label, $e)
{
    echo $label, ": ", $e::class, " | ", $e->getMessage(), " | line ", $e->getLine(), "\n";
}

$i = 5;
$f = 1.5;
$t = true;
$s = "abc";

try {
    $i[0] = 1;
} catch (Error $e) {
    report("int_write", $e);
}

try {
    $f[0] = 1;
} catch (Error $e) {
    report("float_write", $e);
}

try {
    $t[0] = 1;
} catch (Error $e) {
    report("bool_write", $e);
}

try {
    $s[] = "x";
} catch (Error $e) {
    report("string_append", $e);
}

try {
    $s[0][0] = "x";
} catch (Error $e) {
    report("string_nested", $e);
}

try {
    unset($s[0]);
} catch (Error $e) {
    report("string_unset", $e);
}

try {
    $s[0] .= "x";
} catch (Error $e) {
    report("string_compound", $e);
}

try {
    $s["x"] = "y";
} catch (Error $e) {
    report("string_non_numeric_key", $e);
}

$n = null;
$n[0] = "auto";
echo "null_autovivify: ", $n[0], "\n";
echo "i_still: ", $i, "\n";
echo "s_still: ", $s, "\n";
echo "end: ok\n";
