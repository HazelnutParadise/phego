<?php
$s = "hello";
echo "$s[1]\n";
echo "$s[-1]\n";
echo "{$s[-1]}\n";
echo "{$s[0]}x\n";
echo "$s[0]$s[1]\n";
$i = 2;
echo "$s[$i]\n";
echo "{$s[$i]}\n";
echo "{$s[$i + 1]}\n";
echo "{$s[-$i]}\n";
echo "a{$s[0]}b{$s[4]}c\n";
echo "$s[1]st\n";
$a = ["abc", "de"];
echo "{$a[0][1]}{$a[1][-1]}\n";
echo "$a[0]\n";
echo "$s[9]|\n";
echo "{$s[9]}|\n";
$n = "5";
echo "val: $n[0] and {$n[0]}\n";
echo <<<TXT
heredoc $s[1] and {$s[-2]}
TXT;
echo "\n";
