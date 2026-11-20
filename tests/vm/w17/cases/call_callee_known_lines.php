<?php
function same($a, $b) { echo "same\n"; } same(1,
$u1);
function same_named($a, $b) { echo "same named\n"; } same_named(a: 1,
b: $u2);
later_named(
  b:
   $u3,
  a: 1
);
later_concat(
  'a'
  . 'b'
  . 'c',
  $u4
);
later_spread(
  ...[
    1],
  b: $u5
);
function later_named($a, $b) { echo "later named\n"; }
function later_concat($a, $b) { echo "later concat\n"; }
function later_spread($a, $b) { echo "later spread\n"; }
