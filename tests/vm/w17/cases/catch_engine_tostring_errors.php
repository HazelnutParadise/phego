<?php
// catch_engine_tostring_errors: __toString returning a non-string, an object without __toString, and array conversion as a warning
function report($label, $e)
{
    echo $label, ": ", $e::class, " | ", $e->getMessage(), " | line ", $e->getLine(), "\n";
}

class BadStr
{
    public function __toString(): string
    {
        return [];
    }
}

class GoodStr
{
    public function __toString(): string
    {
        return "good";
    }
}

$bad = new BadStr();
try {
    $s = (string) $bad;
} catch (TypeError $e) {
    report("tostring_returns_array", $e);
}

$plain = new stdClass();
try {
    $s = (string) $plain;
} catch (Error $e) {
    report("no_tostring", $e);
}

try {
    echo $plain;
} catch (Error $e) {
    report("echo_no_tostring", $e);
}

$arr = [1];
$joined = "list: " . $arr;
echo "array_conversion_warning_only: ", $joined, "\n";

$g = new GoodStr();
echo "good_echo: ", $g, "\n";
echo "good_concat: " . $g . "!\n";
echo "end: ok\n";
