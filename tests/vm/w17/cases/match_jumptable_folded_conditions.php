<?php
echo match ($u) {
    "a" . "b" => "a",
    "c" => "b",
    default => "d",
}, "\n";
echo match ($v) {
    PHP_INT_SIZE * 2 => "a",
    3 => "b",
    default => "d",
}, "\n";
echo match ($w) {
    (int) "5" => "a",
    -3 => "b",
    ~1 => "c",
    "x" . 1 => "e",
    default => "d",
}, "\n";
echo match ($x) {
    true ? 1 : 2 => "a",
    PHP_EOL => "b",
    default => "d",
}, "\n";
echo match ($y) {
    (string) 5 => "a",
    (bool) 1 => "b",
    default => "d",
}, "\n";
echo match ($z) {
    1 ?: 2 => "a",
    null ?? 3 => "b",
    (float) 1 => "f",
    default => "d",
}, "\n";
echo match ($q) {
    1 . 2 => "a",
    PHP_INT_SIZE > 4 ? "big" : "small" => "b",
    default => "d",
}, "\n";
echo match ($r) {
    __LINE__ => "a",
    __FILE__ => "b",
    default => "d",
}, "\n";
echo match ($s) {
    1.5 => "c",
    (int) "6"
    => "a",
    -3
    => "b",
    ~1
    => "e",
    true ? 1 : 2
    => "f",
    "x" . 1
    => "g",
    PHP_EOL
    => "h",
    [1]
    => "i",
    __LINE__
    => "j",
    default => "d",
}, "\n";
echo match ($t) {
    __LINE__
    => "a",
    1.5 => "b",
    default => "d",
}, "\n";
echo match (
    $m
) {
    PHP_INT_SIZE * 2
        => "a",
    PHP_EOL . "x"
        => "b",
    default => "d",
}, "\n";
echo match (16) { PHP_INT_SIZE * 2 => "sixteen", 8 => "eight", default => "other" }, "\n";
