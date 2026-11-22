<?php
// CVE: CVE-2006-3017
// zend_hash_del_key_or_index compared only hash values, so unset() of a
// string key could remove an integer key whose value equals the string's
// hash. 5863110 and 177670 are the DJB hashes of "a" with and without the
// trailing NUL that PHP 4 and 5 hashed; "Ez" and "FY" collide with each other.
$a = [5863110 => "int", "a" => "str", 177670 => "int2", "Ez" => "ez", "FY" => "fy"];
unset($a["a"]);
echo isset($a[5863110]) ? "5863110 kept\n" : "5863110 lost\n";
echo isset($a["a"]) ? "a kept\n" : "a removed\n";
echo isset($a[177670]) ? "177670 kept\n" : "177670 lost\n";
unset($a["Ez"]);
echo isset($a["FY"]) ? "FY kept\n" : "FY lost\n";
unset($a[177670]);
foreach ($a as $k => $v) {
    echo $k, "=", $v, "\n";
}
$g = ["a" => 1];
$h = $g;
unset($h["a"], $h[177670], $h[5863110]);
echo count_keys($g), " ", count_keys($h), "\n";
function count_keys(array $x) {
    $n = 0;
    foreach ($x as $unused) {
        $n++;
    }
    return $n;
}
