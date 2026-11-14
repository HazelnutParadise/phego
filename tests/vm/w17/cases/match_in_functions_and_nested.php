<?php
function grade($score)
{
    return match (true) {
        $score >= 90 => "A",
        $score >= 80 => "B",
        $score >= 70 => "C",
        default => "F",
    };
}
foreach ([95, 85, 75, 10] as $s) {
    echo grade($s);
}
echo "\n";
function describe($v)
{
    return match ($v) {
        0 => "zero",
        1, 2 => match ($v) { 1 => "one", 2 => "two" },
        default => "many",
    };
}
echo describe(0), describe(1), describe(2), describe(7), "\n";
$list = [match (1) { 1 => "a" }, match (2) { 2 => "b" }];
echo $list[0], $list[1], "\n";
echo grade(match ("x") { "x" => 100 }), "\n";
$matrix = ["k" => match (true) { default => [1, 2] }];
echo $matrix["k"][1], "\n";
function early($v)
{
    if ($v) {
        return match ($v) { 1 => "early" };
    }
    return "late";
}
echo early(1), early(0), "\n";
