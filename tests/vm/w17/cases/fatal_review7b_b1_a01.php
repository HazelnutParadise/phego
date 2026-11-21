<?php
function d($v) { if ($v === null) return "NULL"; if ($v === true) return "true"; if ($v === false) return "false"; if ($v === (array)$v) return "array"; if ($v === (int)$v) return "int($v)"; if ($v === (float)$v) return "float($v)"; if ($v === (string)$v) return "string($v)"; return "?"; }
class A { public int|false $p = false; }
$o = new A; $o->p[] = 1; echo "obj\n";
