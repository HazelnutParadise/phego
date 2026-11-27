<?php
// try_catch_typed_function_boundaries: exceptions through typed returns and by-reference parameters; changes made before the throw are kept
class Stop extends Exception {}

function fill(array &$out, int $n): int
{
    $out[] = "a" . $n;
    if ($n > 1) {
        throw new Stop("stop at " . $n);
    }
    $out[] = "b" . $n;
    return $n * 2;
}

function outer(array &$out): string
{
    $r = fill($out, 1);
    $r2 = fill($out, 2);
    return "never " . $r . $r2;
}

$items = [];
try {
    $v = outer($items);
    echo "not reached: ", $v, "\n";
} catch (Stop $e) {
    echo "caught: ", $e->getMessage(), "\n";
}
foreach ($items as $i => $item) {
    echo "item ", $i, ": ", $item, "\n";
}

function setThenThrow(&$v): void
{
    $v = "changed";
    throw new Stop("after set");
}

$val = "original";
try {
    setThenThrow($val);
} catch (Stop $e) {
    echo "val after throw: ", $val, "\n";
}

function badType(): int
{
    throw new Stop("typed");
}

try {
    $n = badType();
} catch (Stop $e) {
    echo "typed function threw before return check: ", $e->getMessage(), "\n";
}

function mayFail(?string $s): ?string
{
    if ($s === null) {
        throw new Stop("null input");
    }
    return $s;
}

try {
    $t = mayFail(null);
} catch (Stop $e) {
    echo "nullable: ", $e->getMessage(), "\n";
}
$t = mayFail("kept");
echo "nullable ok: ", $t, "\n";
