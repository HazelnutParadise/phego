<?php
echo @
  $u
  . $v, "\n";
echo @$u1 ?? "d", "\n";
echo @$a["x"]["y"] ?? "d", "\n";
$r = @($a["x"] = 5);
echo $r, "\n";
echo @$u2++, "|", $u2, "\n";
echo @++$u3, "|", $u3, "\n";
echo $x = @$undef, "|", $x === null ? "null" : "nn", "\n";
$y = @$undef2 . "tail";
echo $y, "\n";
echo @$b[0] . $b[1], "\n";
echo (@$c["k"]) . $c["j"], "\n";
echo @"literal", @1, @1.5, "\n";
echo @isset($u9), @empty($u9), "\n";
echo @match ($u8) { null => "matched", default => "no" }, "\n";
