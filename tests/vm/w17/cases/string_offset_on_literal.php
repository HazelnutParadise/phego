<?php
echo "abc"[0], "\n";
echo "abc"[2], "\n";
echo "abc"[-1], "\n";
echo "abc"[3], "|\n";
echo "abc"[-4], "|\n";
echo "x"[0], "\n";
echo ""[0], "|\n";
echo "hello"[1] . "world"[0], "\n";
echo "abc"["1"], "\n";
echo "abc"[0][0], "\n";
$i = 1;
echo "abc"[$i], "\n";
echo ("a" . "bc")[2], "\n";
echo isset("abc"[1]) ? "y" : "n", "\n";
