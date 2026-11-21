<?php
function d($v) { if ($v === null) return "NULL"; if ($v === true) return "true"; if ($v === false) return "false"; if ($v === (array)$v) return "array"; if ($v === (int)$v) return "int($v)"; if ($v === (float)$v) return "float($v)"; if ($v === (string)$v) return "string($v)"; return "?"; }
class A { public int $i = 1; }
$o = new A; $arr = [1]; $o->i = &$arr[0]; $arr[0] = "3"; echo d($o->i), "\n"; $arr[0] = "x";
