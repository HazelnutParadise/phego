<?php
function undefined_in_init() {
    static $v = $missing . "suffix";
    return $v;
}
echo undefined_in_init(), "\n";
echo undefined_in_init(), "\n";

function array_to_string() {
    static $s = "list: " . [1, 2];
    return $s;
}
echo array_to_string(), "\n";
echo array_to_string(), "\n";

function non_numeric($x) {
    static $n = $x + 1;
    return $n;
}
echo non_numeric("5 apples"), "\n";
echo non_numeric("7 pears"), "\n";

function missing_key() {
    static $k = [1, 2][5] ?? "fallback", $m = ["a" => 1]["b"];
    return $k . ($m === null ? " null" : " set");
}
echo missing_key(), "\n";
echo missing_key(), "\n";

function silenced() {
    static $q = @$nothing["x"];
    return $q === null ? "null" : "set";
}
echo silenced(), silenced(), "\n";

function
multi_line()
{
    static $a = 1,
        $b = $undefined_b
            + 2,
        $c = 3;
    return $a + $b + $c;
}
echo multi_line(), "\n";
echo multi_line(), "\n";
