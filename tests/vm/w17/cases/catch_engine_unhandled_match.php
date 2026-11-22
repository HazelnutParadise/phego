<?php
// catch_engine_unhandled_match: UnhandledMatchError messages for each subject type
function pick($v)
{
    return match ($v) {
        "known" => "matched",
    };
}

$cases = [
    "int" => 5,
    "string" => "other",
    "long_string" => "a string that is much longer than fifteen bytes",
    "float" => 1.5,
    "bool" => false,
    "null" => null,
    "array" => [1, 2],
    "object" => new stdClass(),
];

foreach ($cases as $label => $value) {
    try {
        $r = pick($value);
        echo $label, ": matched\n";
    } catch (UnhandledMatchError $e) {
        echo $label, ": ", $e::class, " | ", $e->getMessage(), " | line ", $e->getLine(), "\n";
    }
}

try {
    $r = match (7) {
        1 => "one",
        2 => "two",
    };
} catch (UnhandledMatchError $e) {
    echo "direct: ", $e::class, " | ", $e->getMessage(), " | line ", $e->getLine(), "\n";
}

$r = pick("known");
echo "known: ", $r, "\n";
echo "end: ok\n";
