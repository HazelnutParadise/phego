<?php
// catch_engine_type_error_return: TypeError from return type violations in functions and methods
function report($label, $e)
{
    echo $label, ": ", $e::class, " | ", $e->getMessage(), " | line ", $e->getLine(), "\n";
}
function ret_int_from_string(): int
{
    return "abc";
}
function ret_int_from_array(): int
{
    return [1];
}
function ret_int_numeric(): int
{
    return "5";
}
function ret_string_from_null(): string
{
    return null;
}
function ret_missing(): int
{
}
function ret_never(): never
{
}
class Plain {}
class Maker
{
    public function make(): static
    {
        return new Plain();
    }
    public function bad_self(): self
    {
        return new Plain();
    }
    public function good_self(): self
    {
        return $this;
    }
}

try {
    $r = ret_int_from_string();
} catch (TypeError $e) {
    report("int_from_string", $e);
}
try {
    $r = ret_int_from_array();
} catch (TypeError $e) {
    report("int_from_array", $e);
}
$r = ret_int_numeric();
echo "int_numeric_ok: ", $r, "\n";
try {
    $r = ret_string_from_null();
} catch (TypeError $e) {
    report("string_from_null", $e);
}
try {
    $r = ret_missing();
} catch (TypeError $e) {
    report("missing_return", $e);
}
try {
    $r = ret_never();
} catch (TypeError $e) {
    report("never_returns", $e);
}
$m = new Maker();
try {
    $r = $m->make();
} catch (TypeError $e) {
    report("static_wrong_class", $e);
}
try {
    $r = $m->bad_self();
} catch (TypeError $e) {
    report("self_wrong_class", $e);
}
$r = $m->good_self();
echo "self_ok: ", $r::class, "\n";
echo "end: ok\n";
