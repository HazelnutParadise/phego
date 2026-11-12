<?php
echo "s" . 1 . 2.5 . true . false . null . "|\n";
echo 1 . 2, "\n";
echo 1.0 . "", "\n";
echo -0.0 . "", "\n";
echo 1e30 . "", "\n";
$s = "a";
$s .= "b";
$s .= 3;
$s .= 4.5;
$s .= null;
$s .= false;
$s .= true;
echo $s, "\n";
echo "1" + "1" . "1", "\n";
echo "x" . 1 + 2, "\n";
