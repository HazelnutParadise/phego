<?php
// exception_trace_as_string: getTraceAsString for function, method, static, closure, arrow and nested frames with every argument kind
class Box {
    public $v = 1;
    function inst($a) {
        return make_error("inst", $a);
    }
    static function stat($a) {
        return make_error("stat", $a);
    }
}

function make_error($tag, $a) {
    return new Exception("made $tag");
}

function f_scalars($a, $b, $c, $d, $e) {
    return new Exception("scalars");
}

function allkinds(...$xs) {
    return new Exception("kinds");
}

function nest1($a) { return nest2($a); }
function nest2($a) { return nest3($a); }
function nest3($a) { return nest4($a); }
function nest4($a) { return new Exception("nested"); }

echo "[function]\n";
$e1 = f_scalars(1, 2.5, true, false, null);
echo $e1->getTraceAsString(), "\n";

$box = new Box();
echo "[method]\n";
$e2 = $box->inst("abc");
echo $e2->getTraceAsString(), "\n";

echo "[static]\n";
$e3 = Box::stat("xyz");
echo $e3->getTraceAsString(), "\n";

$cl = function ($z) { return make_error("closure", $z); };
echo "[closure]\n";
$e4 = $cl("cl");
echo $e4->getTraceAsString(), "\n";

$af = fn($z) => make_error("arrow", $z);
echo "[arrow]\n";
$e5 = $af("ar");
echo $e5->getTraceAsString(), "\n";

echo "[nested 4 deep]\n";
$e6 = nest1("n");
echo $e6->getTraceAsString(), "\n";

echo "[argument kinds]\n";
$e7 = allkinds(1, 2.5, true, false, null, "short", "a string longer than fifteen", [1, 2], [[3]], new Box(), $cl);
echo $e7->getTraceAsString(), "\n";

echo "frames_end\n";
