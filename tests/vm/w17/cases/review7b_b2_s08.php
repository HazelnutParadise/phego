<?php
function d($v) { if ($v === null) return "NULL"; if ($v === true) return "true"; if ($v === false) return "false"; if ($v === (array)$v) return "array"; if ($v === (int)$v) return "int($v)"; if ($v === (float)$v) return "float($v)"; if ($v === (string)$v) return "string($v)"; return "?"; }
class A { public static int $s = 1; }
$arr = ["2"]; A::$s = &$arr[0]; echo d($arr[0]), "\n"; $arr[0] = "6"; echo d(A::$s), "\n";
