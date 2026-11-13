<?php
$s = "é";
echo $s[0] === "\xC3" ? "lead" : "other", $s[1] === "\xA9" ? "trail" : "other", "\n";
echo isset($s[1]) ? "y" : "n", isset($s[2]) ? "y" : "n", "\n";
$t = "日本";
$i = 0;
while (isset($t[$i])) {
    echo $i, ":", $t[$i] === "\xE6" ? "E6" : "..", " ";
    $i++;
}
echo $i, "\n";
echo $t[0] . $t[1] . $t[2], "\n";
echo $t[-3] . $t[-2] . $t[-1], "\n";
echo $s[2], "|\n";
$n = "a\0b";
echo $n[1] === "\0" ? "nul" : "other", isset($n[2]) ? "y" : "n", "\n";
