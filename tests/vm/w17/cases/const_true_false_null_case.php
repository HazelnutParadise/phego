<?php
echo TRUE, True, tRuE, "|", FALSE, False, "|", NULL, Null, "|\n";
echo true === TRUE ? "same" : "diff", null === NULL ? "same" : "diff", false === FALSE ? "same" : "diff", "\n";
echo \TRUE, \null, \False, "|\n";
$a = [TRUE => "t", FALSE => "f"];
echo $a[1], $a[0], "\n";
echo TRUE ? "yes" : "no", NULL ?? "coalesced", "\n";
