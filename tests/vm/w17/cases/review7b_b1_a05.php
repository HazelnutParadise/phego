<?php
function d($v) { if ($v === null) return "NULL"; if ($v === true) return "true"; if ($v === false) return "false"; if ($v === (array)$v) return "array"; if ($v === (int)$v) return "int($v)"; if ($v === (float)$v) return "float($v)"; if ($v === (string)$v) return "string($v)"; return "?"; }
class A { public ?int $i = null; }
$o = new A; $o->i[0] .= "x"; echo "obj\n";
