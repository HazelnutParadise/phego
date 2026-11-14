<?php
echo match ($u) {
    0 => "z",
    $a
    + PHP_INT_SIZE => "a",
    default => "d",
}, "\n";
echo match ($v) {
    0 => "z",
    $a
    ? 1
    : PHP_INT_SIZE => "a",
    default => "d",
}, "\n";
echo match ($w) {
    0 => "z",
    true
    || $a => "a",
    default => "d",
}, "\n";
echo match ($x) {
    0 => "z",
    false
    && $a => "a",
    default => "d",
}, "\n";
echo match ($y) {
    0 => "z",
    [$a,
    PHP_INT_SIZE] => "a",
    default => "d",
}, "\n";
echo match ($z) {
    0 => "z",
    $a
    ?? PHP_INT_SIZE => "a",
    default => "d",
}, "\n";
echo match ($p) {
    0 => "z",
    $a
    || true => "a",
    default => "d",
}, "\n";
echo match ($q) {
    0 => "z",
    1
    && 0 => "a",
    default => "d",
}, "\n";
echo match ($r) {
    0 => "z",
    1
    || $a => "a",
    default => "d",
}, "\n";
echo match ($s) {
    0 => "z",
    !$a
    => "a",
    default => "d",
}, "\n";
echo match ($t) {
    [10, 20]
    [1] => "a",
    "abc"
    [1] => "b",
    PHP_EOL
    [0] => "c",
    1.5 => "f",
    default => "d",
}, "\n";
echo match (20) { [10, 20][1] => "x", "b" => "y", default => "d" }, "\n";
echo match ("b") { "abc"[1] => "x", 2 => "y", default => "d" }, "\n";
echo match ($m) {
    (int)
    1.9 => "a",
    (int) "12abc" => "b",
    (string)
    true => "c",
    (string) 1.5 => "e",
    default => "d",
}, "\n";
echo match ($n) {
    0 => "z",
    (float)
    "1.5x" => "a",
    (int) "abc" => "b",
    (array) 1
    => "c",
    default => "d",
}, "\n";
echo match ($o) {
    0 => "z",
    (bool)
    NAN => "a",
    default => "d",
}, "\n";
echo match (12) { (int) "12abc" => "twelve", default => "d" }, match (1.5) { (float) "1.5x" => "float", default => "d" }, "\n";
