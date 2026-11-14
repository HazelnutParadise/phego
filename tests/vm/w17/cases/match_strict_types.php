<?php
function kind($v)
{
    return match ($v) {
        1 => "int 1",
        "1" => "string 1",
        1.0 => "float 1",
        true => "true",
        false => "false",
        null => "null",
        [] => "empty array",
        [1, 2] => "list",
        ["a" => 1] => "map",
        "" => "empty string",
        0 => "int 0",
        default => "other",
    };
}
foreach ([1, "1", 1.0, true, false, null, [], [1, 2], [2, 1], ["a" => 1], ["a" => 1.0], "", 0, 0.0, "0", " 1", 2] as $v) {
    echo kind($v), "\n";
}
echo match ("abc") { "ABC" => "upper", "abc" => "lower" }, "\n";
echo match (-0.0) { 0.0 => "zero", default => "other" }, "\n";
echo match (PHP_INT_MAX) { PHP_INT_MAX => "max", default => "other" }, "\n";
