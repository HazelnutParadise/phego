<?php
// use_clone: cloning copies by-value captures and shares by-reference ones
$v = 1;
$r = 10;
$orig = function () use ($v, &$r) {
    return "v=" . $v . " r=" . $r;
};
$copy = clone $orig;
$v = 2;
$r = 20;
echo "orig: ", $orig(), "\n";
echo "copy: ", $copy(), "\n";

$writer = function ($n) use (&$r) {
    $r = $n;
    return $r;
};
$writer2 = clone $writer;
echo "writer2_sets: ", $writer2(55), "\n";
echo "r_outside: ", $r, "\n";
echo "writer_sets: ", $writer(66), "\n";
echo "r_after_writer: ", $r, "\n";

$solo = 5;
$w = function ($n) use (&$solo) {
    $solo = $n;
    return $solo;
};
unset($solo);
$wc = clone $w;
echo "orig_write: ", $w(7), "\n";
echo "clone_write: ", $wc(8), "\n";
echo "orig_write_again: ", $w(9), "\n";
echo "clone_after_orig: ", $wc(1), "\n";

$keep = "k";
$kept = function () use (&$keep) {
    return $keep;
};
$kc = clone $kept;
$keep = "k2";
echo "kept_clone_sees: ", $kc(), "\n";
echo "kept_orig_sees: ", $kept(), "\n";
