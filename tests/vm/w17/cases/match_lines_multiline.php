<?php
echo match ($a)
{
  $b => 1,
  $c => 2,
}, "\n";
echo match (
  $d
)
{
  1,
  2 => 3,
  $e
   => 4,
  default
  => $f,
}, "\n";
echo match (
  $g
) {
  1 => 1,
  2 => 2,
  default => "jt",
}, "\n";
echo match (true) {
  $h > 1
    => "x",
  default => match ($i) {
    null => "inner",
  },
}, "\n";
echo match ($j) {
  null => 1,
  default => 2,
}, "\n";
echo match ($k) {
  2 => 1,
  null => 2,
  default => 3,
}, "\n";
echo match ($l) {
  1 + 1
  => 1,
  default => 2,
}, "\n";
echo match ($m) {
  -1
  => 1,
  default => 2,
}, "\n";
echo match ($n) {
  [1]
  => 1,
  default => 2,
}, "\n";
echo match ($o) {
  PHP_EOL
  => 1,
  default => 2,
}, "\n";
echo match ($p) {
  "a" . "b"
  => 1,
  default => 2,
}, "\n";
echo match ($q) {
  1.5,
  null => 1,
  default => 2,
}, "\n";
echo match ($r) {
  1,
  2 => 1,
  default => 2,
}, "\n";
echo match (true) {
  $s
  > 1 => "x",
  default => 0,
}, "\n";
echo match ($t) {
  __LINE__ => "line",
  default => match ($t) {
    true,
    false => "bool",
    default => "d",
  },
}, "\n";
