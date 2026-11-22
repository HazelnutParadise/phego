<?php
// CVE: CVE-2010-2191
// ZEND_CONCAT and ZEND_ASSIGN_CONCAT read their operands before a userspace
// callback could replace them, leaking or reusing freed memory. Here the
// callback is __toString, which rewrites the target and the other operand.
// The parse_str, preg_match, pack and unpack paths of this CVE wait for
// their functions.
class Rewrite {
    public function __toString(): string {
        global $s, $t;
        $s = 12345;
        $t = [1];
        return "tail";
    }
}
$s = "head";
$s .= new Rewrite;
echo $s, "\n";
$t = "left";
$u = $t . new Rewrite;
echo $u, "\n";
foreach ($t as $k => $v) {
    echo "t is now an array: ", $k, "=", $v, "\n";
}
class Rewrite2 {
    public function __toString(): string {
        global $arr;
        $arr = null;
        return "!";
    }
}
$arr = ["k" => "v"];
$arr["k"] .= new Rewrite2;
echo $arr === null ? "arr is null" : $arr["k"], "\n";
class Rewrite3 {
    public $o;
    public function __toString(): string {
        $this->o->p = "gone";
        return "?";
    }
}
$obj = new stdClass;
$obj->p = "start";
$r = new Rewrite3;
$r->o = $obj;
$obj->p .= $r;
echo $obj->p, "\n";
