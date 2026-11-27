<?php
// catch_engine_member_on_null: member calls and property writes on non-objects are Errors; property reads are only warnings
function report($label, $e)
{
    echo $label, ": ", $e::class, " | ", $e->getMessage(), " | line ", $e->getLine(), "\n";
}

$n = null;
$i = 5;
$s = "str";
$a = [1];
$b = false;

try {
    $n->f();
} catch (Error $e) {
    report("call_null", $e);
}

try {
    $i->f();
} catch (Error $e) {
    report("call_int", $e);
}

try {
    $s->f();
} catch (Error $e) {
    report("call_string", $e);
}

try {
    $a->f();
} catch (Error $e) {
    report("call_array", $e);
}

try {
    $b->f();
} catch (Error $e) {
    report("call_bool", $e);
}

try {
    $n->p = 1;
} catch (Error $e) {
    report("assign_null", $e);
}

try {
    $i->p = 1;
} catch (Error $e) {
    report("assign_int", $e);
}

echo "coalesce_read: ", $n->p ?? "none", "\n";

$r = $n?->f();
echo "nullsafe_call: ok\n";

$r = $n->p;
echo "after_read_warning: ", $r === null ? "null" : "other", "\n";
echo "end: ok\n";
