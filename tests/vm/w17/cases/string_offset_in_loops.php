<?php
$s = "abcdef";
$i = 0;
while (isset($s[$i])) {
    echo $s[$i], "-";
    $i++;
}
echo $i, "\n";
for ($j = 1; isset($s[-$j]); $j++) {
    echo $s[-$j];
}
echo "\n";
$rev = "";
for ($k = 0; isset($s[$k]); $k++) {
    $rev = $s[$k] . $rev;
}
echo $rev, "\n";
$count = 0;
$word = "banana";
for ($k = 0; isset($word[$k]); $k++) {
    if ($word[$k] == "a") {
        $count++;
    }
}
echo $count, "\n";
$pal = "racecar";
$ok = true;
for ($a = 0, $b = -1; isset($pal[$a]); $a++, $b--) {
    if ($pal[$a] !== $pal[$b]) {
        $ok = false;
    }
}
echo $ok ? "palindrome" : "not", "\n";
for ($q = 0; $q < 5; $q++) {
    echo $word[$q % 3];
}
echo "\n";
$n = "9081";
$digits = 0;
for ($d = 0; isset($n[$d]); $d++) {
    $digits += $n[$d];
}
echo $digits, "\n";
